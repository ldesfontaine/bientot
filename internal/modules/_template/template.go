// Package template est un squelette pour les nouveaux modules agent.
// Copier ce répertoire, renommer le package, et implémenter les 3 méthodes.
package template

import (
	"context"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

// Module est un module template. Renommer et implémenter.
type Module struct {
	// Ajouter les champs de configuration ici.
}

func New() *Module { return &Module{} }

// Name return l'identifiant du module (doit être unique parmi tous les modules).
func (m *Module) Name() string { return "template" }

// Detect return true si les prérequis de ce module sont disponibles.
func (m *Module) Detect() bool { return false }

// Collect collecte les métriques. Appelé uniquement si Detect() a retourné true.
func (m *Module) Collect(_ context.Context) (transport.ModuleData, error) {
	return transport.ModuleData{
		Module:    m.Name(),
		Metrics:   []transport.MetricPoint{},
		Timestamp: time.Now(),
	}, nil
}
