package web

import (
	"net/http"
	"time"
)

// homePageData is the struct passed to the home template.
type homePageData struct {
	Title      string
	Subtitle   string
	UptimeFake time.Duration
	RenderedAt string
}

// handleHome renders the placeholder home page.
// Will be rewritten in 5.3 to redirect to /agents/{firstMachine}.
func (r *Router) handleHome(w http.ResponseWriter, _ *http.Request) {
	data := homePageData{
		Title:      "Home",
		Subtitle:   "Placeholder page — layout scaffolding only",
		UptimeFake: 2*24*time.Hour + 14*time.Hour + 22*time.Minute,
		RenderedAt: time.Now().UTC().Format("15:04:05"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.renderer.Render(w, "home", data); err != nil {
		r.log.Error("render home failed", "error", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
}
