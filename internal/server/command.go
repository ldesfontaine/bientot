package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Command représente une commande serveur vers agent.
type Command struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"`   // "collect", "restart_module", "update_config", "ping"
	Target    string      `json:"target"` // module name or empty
	Payload   interface{} `json:"payload,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

// CommandResult est renvoyé par l'agent.
type CommandResult struct {
	CommandID string      `json:"command_id"`
	MachineID string      `json:"machine_id"`
	Status    string      `json:"status"` // "ok", "error"
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// CommandChannel gère les connexions WebSocket des agents.
type CommandChannel struct {
	mu      sync.RWMutex
	conns   map[string]*agentConn // machine_id -> connexion
	logger  *slog.Logger
}

type agentConn struct {
	conn      *websocket.Conn
	machineID string
	mu        sync.Mutex
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// NewCommandChannel crée un gestionnaire de canal de commandes.
func NewCommandChannel(logger *slog.Logger) *CommandChannel {
	return &CommandChannel{
		conns:  make(map[string]*agentConn),
		logger: logger,
	}
}

// SendCommand envoie une commande à un agent spécifique.
func (cc *CommandChannel) SendCommand(machineID string, cmd Command) error {
	cc.mu.RLock()
	ac, ok := cc.conns[machineID]
	cc.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s non connecté", machineID)
	}

	ac.mu.Lock()
	defer ac.mu.Unlock()

	cmd.Timestamp = time.Now()
	return ac.conn.WriteJSON(cmd)
}

// BroadcastCommand envoie une commande à tous les agents connectés.
func (cc *CommandChannel) BroadcastCommand(cmd Command) map[string]error {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	cmd.Timestamp = time.Now()
	errors := make(map[string]error)

	for id, ac := range cc.conns {
		ac.mu.Lock()
		err := ac.conn.WriteJSON(cmd)
		ac.mu.Unlock()
		if err != nil {
			errors[id] = err
		}
	}

	return errors
}

// ConnectedAgents return la liste des machine_id avec des connexions WS actives.
func (cc *CommandChannel) ConnectedAgents() []string {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	agents := make([]string, 0, len(cc.conns))
	for id := range cc.conns {
		agents = append(agents, id)
	}
	return agents
}

// handleAgentWS gère les connexions WebSocket des agents.
func (s *Server) handleAgentWS(w http.ResponseWriter, r *http.Request) {
	if s.cmdChannel == nil {
		http.Error(w, "canal de commandes désactivé", http.StatusServiceUnavailable)
		return
	}

	// Authentification via paramètre de requête (token envoyé en ?token=xxx&machine_id=yyy)
	machineID := r.URL.Query().Get("machine_id")
	token := r.URL.Query().Get("token")

	expectedToken, ok := s.tokens[machineID]
	if !ok || token != expectedToken {
		s.logger.Warn("échec auth WS", "machine_id", machineID)
		http.Error(w, "non autorisé", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("échec de l'upgrade WS", "error", err)
		return
	}

	ac := &agentConn{conn: conn, machineID: machineID}

	s.cmdChannel.mu.Lock()
	// Fermeture de la connexion existante si présente
	if existing, ok := s.cmdChannel.conns[machineID]; ok {
		existing.conn.Close()
	}
	s.cmdChannel.conns[machineID] = ac
	s.cmdChannel.mu.Unlock()

	s.logger.Info("agent connecté au canal de commandes", "machine_id", machineID)

	// Boucle de lecture : réception des résultats de commandes depuis l'agent
	defer func() {
		s.cmdChannel.mu.Lock()
		delete(s.cmdChannel.conns, machineID)
		s.cmdChannel.mu.Unlock()
		conn.Close()
		s.logger.Info("agent déconnecté du canal de commandes", "machine_id", machineID)
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				s.logger.Warn("erreur de lecture WS", "machine_id", machineID, "error", err)
			}
			return
		}

		var result CommandResult
		if err := json.Unmarshal(msg, &result); err != nil {
			s.logger.Warn("résultat de commande invalide", "machine_id", machineID, "error", err)
			continue
		}

		result.MachineID = machineID
		s.logger.Debug("résultat de commande reçu", "machine_id", machineID, "command_id", result.CommandID, "status", result.Status)

		// Publication via SSE
		s.sse.Publish(SSEEvent{
			Type: "command_result",
			Data: result,
		})
	}
}

// handleSendCommand permet au dashboard d'envoyer des commandes aux agents.
func (s *Server) handleSendCommand(w http.ResponseWriter, r *http.Request) {
	if s.cmdChannel == nil {
		http.Error(w, "canal de commandes désactivé", http.StatusServiceUnavailable)
		return
	}

	var cmd Command
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		http.Error(w, "JSON invalide", http.StatusBadRequest)
		return
	}

	machineID := r.URL.Query().Get("machine_id")
	if machineID == "" {
		// Diffusion à tous
		errors := s.cmdChannel.BroadcastCommand(cmd)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "broadcast",
			"errors": errors,
		})
		return
	}

	if err := s.cmdChannel.SendCommand(machineID, cmd); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
}

// handleConnectedAgents liste les agents avec des connexions WS actives.
func (s *Server) handleConnectedAgents(w http.ResponseWriter, _ *http.Request) {
	if s.cmdChannel == nil {
		http.Error(w, "canal de commandes désactivé", http.StatusServiceUnavailable)
		return
	}

	agents := s.cmdChannel.ConnectedAgents()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"connected": agents,
		"count":     len(agents),
	})
}
