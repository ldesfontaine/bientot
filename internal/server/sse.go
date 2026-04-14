package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SSEEvent est un événement typé envoyé aux clients SSE.
type SSEEvent struct {
	Type string      `json:"type"` // "alert", "metric", "push", "pattern"
	Data interface{} `json:"data"`
	Time time.Time   `json:"time"`
}

// SSEBroker gère les connexions clients SSE et la diffusion des événements.
type SSEBroker struct {
	mu      sync.RWMutex
	clients map[chan SSEEvent]struct{}
}

// NewSSEBroker crée un nouveau broker.
func NewSSEBroker() *SSEBroker {
	return &SSEBroker{
		clients: make(map[chan SSEEvent]struct{}),
	}
}

// Subscribe ajoute un client. return le canal et la fonction de désinscription.
func (b *SSEBroker) Subscribe() (chan SSEEvent, func()) {
	ch := make(chan SSEEvent, 64)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		delete(b.clients, ch)
		b.mu.Unlock()
		// Vidage des événements restants
		for range ch {
		}
	}
}

// Publish envoie un événement à tous les clients connectés.
func (b *SSEBroker) Publish(event SSEEvent) {
	event.Time = time.Now()
	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.clients {
		select {
		case ch <- event:
		default:
			// Client trop lent, événement ignoré
		}
	}
}

// ClientCount return le nombre de clients SSE connectés.
func (b *SSEBroker) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}

// handleSSE envoie les événements SSE au client en streaming.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming non supporté", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch, unsub := s.sse.Subscribe()
	defer unsub()

	s.logger.Debug("client SSE connecté", "clients", s.sse.ClientCount())

	// Envoi du keepalive initial
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	// Timer de keepalive
	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			s.logger.Debug("client SSE déconnecté", "clients", s.sse.ClientCount()-1)
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
