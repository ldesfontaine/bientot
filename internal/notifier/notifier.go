package notifier

import (
	"github.com/ldesfontaine/bientot/internal"
)

// Notifier est l'interface pour les backends de notification d'alertes
type Notifier interface {
	// Name return le nom du notifier
	Name() string

	// Type return le type du notifier (ntfy, webhook, slack, etc.)
	Type() string

	// Send envoie une notification d'alerte
	Send(alert internal.Alert) error

	// SupportsSeverity vérifie si ce notifier gère la sévérité donnée
	SupportsSeverity(severity internal.Severity) bool
}

// Registry contient tous les notifiers enregistrés
type Registry struct {
	notifiers []Notifier
}

// NewRegistry crée un nouveau registre de notifiers
func NewRegistry() *Registry {
	return &Registry{
		notifiers: make([]Notifier, 0),
	}
}

// Register ajoute un notifier au registre
func (r *Registry) Register(n Notifier) {
	r.notifiers = append(r.notifiers, n)
}

// All return tous les notifiers enregistrés
func (r *Registry) All() []Notifier {
	return r.notifiers
}

// ForSeverity return les notifiers qui gèrent la sévérité donnée
func (r *Registry) ForSeverity(severity internal.Severity) []Notifier {
	var result []Notifier
	for _, n := range r.notifiers {
		if n.SupportsSeverity(severity) {
			result = append(result, n)
		}
	}
	return result
}

// Notify envoie une alerte à tous les notifiers correspondants
func (r *Registry) Notify(alert internal.Alert) []error {
	var errors []error
	for _, n := range r.ForSeverity(alert.Severity) {
		if err := n.Send(alert); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}
