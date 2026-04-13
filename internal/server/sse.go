package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SSEEvent is a typed event sent to SSE clients.
type SSEEvent struct {
	Type string      `json:"type"` // "alert", "metric", "push", "pattern"
	Data interface{} `json:"data"`
	Time time.Time   `json:"time"`
}

// SSEBroker manages SSE client connections and event fanout.
type SSEBroker struct {
	mu      sync.RWMutex
	clients map[chan SSEEvent]struct{}
}

// NewSSEBroker creates a new broker.
func NewSSEBroker() *SSEBroker {
	return &SSEBroker{
		clients: make(map[chan SSEEvent]struct{}),
	}
}

// Subscribe adds a client. Returns channel and unsubscribe func.
func (b *SSEBroker) Subscribe() (chan SSEEvent, func()) {
	ch := make(chan SSEEvent, 64)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		delete(b.clients, ch)
		b.mu.Unlock()
		// Drain remaining events
		for range ch {
		}
	}
}

// Publish sends an event to all connected clients.
func (b *SSEBroker) Publish(event SSEEvent) {
	event.Time = time.Now()
	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.clients {
		select {
		case ch <- event:
		default:
			// Client too slow, drop event
		}
	}
}

// ClientCount returns the number of connected SSE clients.
func (b *SSEBroker) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}

// handleSSE streams server-sent events to the client.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch, unsub := s.sse.Subscribe()
	defer unsub()

	s.logger.Debug("sse client connected", "clients", s.sse.ClientCount())

	// Send initial keepalive
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	// Keepalive ticker
	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			s.logger.Debug("sse client disconnected", "clients", s.sse.ClientCount()-1)
			return

		case event := <-ch:
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
			flusher.Flush()

		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
