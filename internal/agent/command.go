package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Command représente une commande serveur vers agent.
type Command struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"`
	Target    string      `json:"target"`
	Payload   interface{} `json:"payload,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

// CommandResult est renvoyé au serveur.
type CommandResult struct {
	CommandID string      `json:"command_id"`
	MachineID string      `json:"machine_id"`
	Status    string      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// CommandHandler traite une commande et return un résultat.
type CommandHandler func(cmd Command) CommandResult

// RunCommandChannel se connecte à l'endpoint WS du serveur et écoute les commandes.
// Se reconnecte automatiquement en cas d'échec. Bloque jusqu'à l'annulation du ctx.
func (a *Agent) RunCommandChannel(ctx context.Context, handler CommandHandler) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			a.connectAndListen(ctx, handler)
			// Attente avant reconnexion
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func (a *Agent) connectAndListen(ctx context.Context, handler CommandHandler) {
	wsURL := a.buildWSURL()
	a.logger.Info("connexion au canal de commandes", "url", wsURL)

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		a.logger.Warn("échec de connexion au canal de commandes", "error", err)
		return
	}
	defer conn.Close()

	a.logger.Info("canal de commandes connecté")

	// Fermeture à l'annulation du contexte
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				a.logger.Warn("erreur de lecture du canal de commandes", "error", err)
			}
			return
		}

		var cmd Command
		if err := json.Unmarshal(msg, &cmd); err != nil {
			a.logger.Warn("commande invalide", "error", err)
			continue
		}

		a.logger.Info("commande reçue", "id", cmd.ID, "type", cmd.Type, "target", cmd.Target)

		result := handler(cmd)
		result.MachineID = a.cfg.MachineID

		if err := conn.WriteJSON(result); err != nil {
			a.logger.Error("échec de l'envoi du résultat de commande", "error", err)
			return
		}
	}
}

func (a *Agent) buildWSURL() string {
	serverURL := a.cfg.ServerURL

	// Conversion http(s) en ws(s)
	wsURL := strings.Replace(serverURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)

	u, err := url.Parse(wsURL + "/ws")
	if err != nil {
		return wsURL + "/ws"
	}

	q := u.Query()
	q.Set("machine_id", a.cfg.MachineID)
	q.Set("token", a.cfg.Token)
	u.RawQuery = q.Encode()

	return u.String()
}

// DefaultCommandHandler return un handler qui traite les commandes intégrées.
func (a *Agent) DefaultCommandHandler() CommandHandler {
	return func(cmd Command) CommandResult {
		switch cmd.Type {
		case "ping":
			return CommandResult{
				CommandID: cmd.ID,
				Status:    "ok",
				Data:      map[string]string{"pong": time.Now().Format(time.RFC3339)},
			}

		case "collect":
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if cmd.Target != "" {
				// Collecte depuis un module spécifique
				for _, m := range a.modules {
					if m.Name() == cmd.Target {
						data, err := m.Collect(ctx)
						if err != nil {
							return CommandResult{
								CommandID: cmd.ID,
								Status:    "error",
								Error:     err.Error(),
							}
						}
						return CommandResult{
							CommandID: cmd.ID,
							Status:    "ok",
							Data:      data,
						}
					}
				}
				return CommandResult{
					CommandID: cmd.ID,
					Status:    "error",
					Error:     fmt.Sprintf("module %s introuvable", cmd.Target),
				}
			}

			// Collecte de tous les modules
			data := a.collectAll(ctx)
			return CommandResult{
				CommandID: cmd.ID,
				Status:    "ok",
				Data:      data,
			}

		case "status":
			moduleNames := make([]string, len(a.modules))
			for i, m := range a.modules {
				moduleNames[i] = m.Name()
			}
			return CommandResult{
				CommandID: cmd.ID,
				Status:    "ok",
				Data: map[string]interface{}{
					"machine_id": a.cfg.MachineID,
					"modules":    moduleNames,
				},
			}

		default:
			return CommandResult{
				CommandID: cmd.ID,
				Status:    "error",
				Error:     fmt.Sprintf("type de commande inconnu : %s", cmd.Type),
			}
		}
	}
}

