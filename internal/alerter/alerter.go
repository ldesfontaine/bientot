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

// Alerter evaluates alert rules and triggers notifications
type Alerter struct {
	rules     []Rule
	storage   storage.Storage
	notifiers *notifier.Registry
	active    map[string]*internal.Alert
	mu        sync.RWMutex
	logger    *slog.Logger
}

// New creates a new alerter
func New(rules []Rule, store storage.Storage, notifiers *notifier.Registry, logger *slog.Logger) *Alerter {
	return &Alerter{
		rules:     rules,
		storage:   store,
		notifiers: notifiers,
		active:    make(map[string]*internal.Alert),
		logger:    logger,
	}
}

// Evaluate checks all rules against current metrics
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
		return fmt.Errorf("querying metric: %w", err)
	}
	if metric == nil {
		return nil // No data yet
	}

	firing := rule.Evaluate(metric.Value)
	alertID := a.alertID(rule, metric.Labels)

	a.mu.Lock()
	defer a.mu.Unlock()

	if firing {
		if _, exists := a.active[alertID]; !exists {
			// New alert
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

			// Notify
			go func(alert internal.Alert) {
				if errs := a.notifiers.Notify(alert); len(errs) > 0 {
					for _, err := range errs {
						a.logger.Error("notification failed", "alert", alert.Name, "error", err)
					}
				}
			}(*alert)

			a.logger.Info("alert fired", "name", rule.Name, "value", metric.Value)
		}
	} else {
		if alert, exists := a.active[alertID]; exists {
			// Alert resolved
			now := time.Now()
			alert.ResolvedAt = &now
			delete(a.active, alertID)
			a.logger.Info("alert resolved", "name", rule.Name)
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

// ActiveAlerts returns all currently firing alerts
func (a *Alerter) ActiveAlerts() []internal.Alert {
	a.mu.RLock()
	defer a.mu.RUnlock()

	alerts := make([]internal.Alert, 0, len(a.active))
	for _, alert := range a.active {
		alerts = append(alerts, *alert)
	}
	return alerts
}

// Acknowledge marks an alert as acknowledged
func (a *Alerter) Acknowledge(alertID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if alert, exists := a.active[alertID]; exists {
		alert.Acknowledged = true
		return true
	}
	return false
}
