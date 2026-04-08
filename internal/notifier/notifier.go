package notifier

import (
	"github.com/ldesfontaine/bientot/internal"
)

// Notifier is the interface for alert notification backends
type Notifier interface {
	// Name returns the notifier name
	Name() string

	// Type returns the notifier type (ntfy, webhook, slack, etc.)
	Type() string

	// Send sends an alert notification
	Send(alert internal.Alert) error

	// SupportsSeverity checks if this notifier handles the given severity
	SupportsSeverity(severity internal.Severity) bool
}

// Registry holds all registered notifiers
type Registry struct {
	notifiers []Notifier
}

// NewRegistry creates a new notifier registry
func NewRegistry() *Registry {
	return &Registry{
		notifiers: make([]Notifier, 0),
	}
}

// Register adds a notifier to the registry
func (r *Registry) Register(n Notifier) {
	r.notifiers = append(r.notifiers, n)
}

// All returns all registered notifiers
func (r *Registry) All() []Notifier {
	return r.notifiers
}

// ForSeverity returns notifiers that handle the given severity
func (r *Registry) ForSeverity(severity internal.Severity) []Notifier {
	var result []Notifier
	for _, n := range r.notifiers {
		if n.SupportsSeverity(severity) {
			result = append(result, n)
		}
	}
	return result
}

// Notify sends an alert to all matching notifiers
func (r *Registry) Notify(alert internal.Alert) []error {
	var errors []error
	for _, n := range r.ForSeverity(alert.Severity) {
		if err := n.Send(alert); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}
