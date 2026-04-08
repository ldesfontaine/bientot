package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ldesfontaine/bientot/internal"
	"github.com/ldesfontaine/bientot/internal/alerter"
	"github.com/ldesfontaine/bientot/internal/storage"
)

// API handles HTTP endpoints
type API struct {
	storage  storage.Storage
	alerter  *alerter.Alerter
	startTime time.Time
}

// New creates a new API handler
func New(store storage.Storage, alert *alerter.Alerter) *API {
	return &API{
		storage:   store,
		alerter:   alert,
		startTime: time.Now(),
	}
}

// Router returns the HTTP router
func (a *API) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	// Health check
	r.Get("/health", a.handleHealth)

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Get("/status", a.handleStatus)
		r.Get("/metrics", a.handleListMetrics)
		r.Get("/metrics/{name}", a.handleQueryMetric)
		r.Get("/metrics/{name}/latest", a.handleLatestMetric)
		r.Get("/alerts", a.handleListAlerts)
		r.Post("/alerts/{id}/ack", a.handleAckAlert)
		r.Get("/overview", a.handleOverview)
	})

	return r
}

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	activeAlerts := a.alerter.ActiveAlerts()
	health := internal.HealthOK
	for _, alert := range activeAlerts {
		if alert.Severity == internal.SeverityCritical {
			health = internal.HealthCritical
			break
		}
		if alert.Severity == internal.SeverityWarning {
			health = internal.HealthWarning
		}
	}

	status := internal.Status{
		Health:       health,
		AlertsActive: len(activeAlerts),
		Uptime:       time.Since(a.startTime),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (a *API) handleListMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	names, err := a.storage.List(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]string{"metrics": names})
}

func (a *API) handleQueryMetric(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := chi.URLParam(r, "name")

	// Parse query params
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()
	resolution := internal.ResolutionRaw

	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = t
		}
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = t
		}
	}
	if resStr := r.URL.Query().Get("resolution"); resStr != "" {
		resolution = internal.Resolution(resStr)
	}

	points, err := a.storage.Query(ctx, name, from, to, resolution)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"metric": name,
		"from":   from,
		"to":     to,
		"points": points,
	})
}

func (a *API) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	alerts := a.alerter.ActiveAlerts()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]internal.Alert{"alerts": alerts})
}

func (a *API) handleAckAlert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if a.alerter.Acknowledge(id) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "acknowledged"})
	} else {
		http.Error(w, "alert not found", http.StatusNotFound)
	}
}

func (a *API) handleLatestMetric(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := chi.URLParam(r, "name")

	metric, err := a.storage.QueryLatest(ctx, name, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if metric == nil {
		http.Error(w, "metric not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metric)
}

func (a *API) handleOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Key metrics to display in overview
	keyMetrics := []string{
		"node_cpu_seconds_total",
		"node_memory_MemAvailable_bytes",
		"node_memory_MemTotal_bytes",
		"node_filesystem_avail_bytes",
		"node_filesystem_size_bytes",
		"container_running",
		"containers_total",
		"zfs_pool_health",
		"zfs_pool_used_percent",
		"crowdsec_bans_active",
	}

	overview := make(map[string]interface{})

	for _, name := range keyMetrics {
		metric, err := a.storage.QueryLatest(ctx, name, nil)
		if err == nil && metric != nil {
			overview[name] = map[string]interface{}{
				"value":     metric.Value,
				"timestamp": metric.Timestamp,
				"source":    metric.Source,
			}
		}
	}

	// Add alerts
	overview["alerts"] = a.alerter.ActiveAlerts()

	// Add status
	activeAlerts := a.alerter.ActiveAlerts()
	health := internal.HealthOK
	for _, alert := range activeAlerts {
		if alert.Severity == internal.SeverityCritical {
			health = internal.HealthCritical
			break
		}
		if alert.Severity == internal.SeverityWarning {
			health = internal.HealthWarning
		}
	}
	overview["health"] = health
	overview["uptime"] = time.Since(a.startTime).Seconds()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(overview)
}
