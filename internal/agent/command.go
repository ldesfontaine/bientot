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

// Command represents a server-to-agent command.
type Command struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"`
	Target    string      `json:"target"`
	Payload   interface{} `json:"payload,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

// CommandResult is sent back to the server.
type CommandResult struct {
	CommandID string      `json:"command_id"`
	MachineID string      `json:"machine_id"`
	Status    string      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// CommandHandler processes a command and returns a result.
type CommandHandler func(cmd Command) CommandResult

// RunCommandChannel connects to the server's WS endpoint and listens for commands.
// Reconnects automatically on failure. Blocks until ctx is cancelled.
func (a *Agent) RunCommandChannel(ctx context.Context, handler CommandHandler) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			a.connectAndListen(ctx, handler)
			// Backoff before reconnect
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
	a.logger.Info("connecting to command channel", "url", wsURL)

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		a.logger.Warn("command channel connection failed", "error", err)
		return
	}
	defer conn.Close()

	a.logger.Info("command channel connected")

	// Close on context cancellation
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				a.logger.Warn("command channel read error", "error", err)
			}
			return
		}

		var cmd Command
		if err := json.Unmarshal(msg, &cmd); err != nil {
			a.logger.Warn("invalid command", "error", err)
			continue
		}

		a.logger.Info("command received", "id", cmd.ID, "type", cmd.Type, "target", cmd.Target)

		result := handler(cmd)
		result.MachineID = a.cfg.MachineID

		if err := conn.WriteJSON(result); err != nil {
			a.logger.Error("failed to send command result", "error", err)
			return
		}
	}
}

func (a *Agent) buildWSURL() string {
	serverURL := a.cfg.ServerURL

	// Convert http(s) to ws(s)
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

// DefaultCommandHandler returns a handler that processes built-in commands.
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
				// Collect from specific module
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
					Error:     fmt.Sprintf("module %s not found", cmd.Target),
				}
			}

			// Collect all
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
				Error:     fmt.Sprintf("unknown command type: %s", cmd.Type),
			}
		}
	}
}

