package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// writeJSON serializes v as JSON and writes it to w with the given status code.
// Sets Content-Type to application/json; charset=utf-8.
func writeJSON(w http.ResponseWriter, log *slog.Logger, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Headers are already sent at this point; we can only log.
		log.Error("writeJSON encode failed", "error", err)
	}
}

// errorBody is the canonical shape of every error response — same format
// for 4XX and 5XX so the frontend has one error-handling code path.
type errorBody struct {
	Error string `json:"error"`
}

// writeError writes an error response with the given status code.
// The message is public-safe (no sensitive internal detail).
func writeError(w http.ResponseWriter, log *slog.Logger, status int, message string) {
	writeJSON(w, log, status, errorBody{Error: message})
}
