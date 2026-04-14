package alerter

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ldesfontaine/bientot/internal"
	"github.com/ldesfontaine/bientot/internal/notifier"
	"github.com/ldesfontaine/bientot/internal/storage"
)

// AlertCallback est appelé quand une alerte se déclenche ou se résout.
type AlertCallback func(alert internal.Alert, resolved bool)

// Alerter évalue les règles d'alerte et déclenche les notifications
type Alerter struct {
	rules     []Rule
	storage   storage.Storage
	notifiers *notifier.Registry
	active    map[string]*internal.Alert
	mu        sync.RWMutex
	logger    *slog.Logger
	onAlert   AlertCallback
}

// New crée un nouvel alerter
func New(rules []Rule, store storage.Storage, notifiers *notifier.Registry, logger *slog.Logger) *Alerter {
	return &Alerter{
		rules:     rules,
		storage:   store,
		notifiers: notifiers,
		active:    make(map[string]*internal.Alert),
		logger:    logger,
	}
}

// Evaluate vérifie toutes les règles par rapport aux métriques courantes
func (a *Alerter) Evaluate(ctx context.Context) error {
	for _, rule := range a.rules {
		if err := a.evaluateRule(ctx, rule); err != nil {
			a.logger.Error("rule evaluation failed", "rule", rule.Name, "error", err)
		}
	}
	return nil
}

func (a *Alerter) evaluateRule(ctx context.Context, rule Rule) error {
	metric, err := a.storage.QueryLatest(ctx, rule.MetricName, rule.Labels)
	if err != nil {
		return fmt.Errorf("requête de la métrique: %w", err)
	}
	if metric == nil {
		return nil // Pas encore de données
	}

	firing := rule.Evaluate(metric.Value)
	alertID := a.alertID(rule, metric.Labels)

	a.mu.Lock()
	defer a.mu.Unlock()

	if firing {
		if _, exists := a.active[alertID]; !exists {
			// Nouvelle alerte
			alert := &internal.Alert{
				ID:       alertID,
				Name:     rule.Name,
				Severity: rule.Severity,
				Message:  rule.FormatMessage(metric.Value, metric.Labels),
				Labels:   metric.Labels,
				Value:    metric.Value,
				FiredAt:  time.Now(),
			}
			a.active[alertID] = alert

			// Notification
			go func(alert internal.Alert) {
				if errs := a.notifiers.Notify(alert); len(errs) > 0 {
					for _, err := range errs {
						a.logger.Error("notification failed", "alert", alert.Name, "error", err)
					}
				}
			}(*alert)

			a.logger.Info("alert fired", "name", rule.Name, "value", metric.Value)

			if a.onAlert != nil {
				go a.onAlert(*alert, false)
			}
		}
	} else {
		if alert, exists := a.active[alertID]; exists {
			// Alerte résolue
			now := time.Now()
			alert.ResolvedAt = &now
			delete(a.active, alertID)
			a.logger.Info("alert resolved", "name", rule.Name)

			if a.onAlert != nil {
				go a.onAlert(*alert, true)
			}
		}
	}

	return nil
}

func (a *Alerter) alertID(rule Rule, labels map[string]string) string {
	h := sha256.New()
	h.Write([]byte(rule.Name))
	for k, v := range labels {
		h.Write([]byte(k))
		h.Write([]byte(v))
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// ActiveAlerts return toutes les alertes actuellement déclenchées
func (a *Alerter) ActiveAlerts() []internal.Alert {
	a.mu.RLock()
	defer a.mu.RUnlock()

	alerts := make([]internal.Alert, 0, len(a.active))
	for _, alert := range a.active {
		alerts = append(alerts, *alert)
	}
	return alerts
}

// OnAlert définit un callback pour les changements d'état d'alerte.
func (a *Alerter) OnAlert(cb AlertCallback) {
	a.onAlert = cb
}

// FireManual declenche une alerte manuellement (hors evaluation de regles).
// Utilise pour les alertes systeme comme le staleness scan CVE.
func (a *Alerter) FireManual(alert internal.Alert) {
	a.mu.Lock()
	if _, exists := a.active[alert.ID]; exists {
		a.mu.Unlock()
		return
	}
	a.active[alert.ID] = &alert
	a.mu.Unlock()

	go func() {
		if errs := a.notifiers.Notify(alert); len(errs) > 0 {
			for _, err := range errs {
				a.logger.Error("notification failed", "alert", alert.Name, "error", err)
			}
		}
	}()

	a.logger.Info("manual alert fired", "name", alert.Name)

	if a.onAlert != nil {
		go a.onAlert(alert, false)
	}
}

// Acknowledge marque une alerte comme acquittée
func (a *Alerter) Acknowledge(alertID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if alert, exists := a.active[alertID]; exists {
		alert.Acknowledged = true
		return true
	}
	return false
}
