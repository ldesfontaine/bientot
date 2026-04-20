package web

import "net/http"

// handleHome is a temporary placeholder until real rendering arrives in 5.2.2.
// It proves the web router is mounted and reachable.
func (r *Router) handleHome(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("hello from web — templates come in 5.2.2\n"))
}
