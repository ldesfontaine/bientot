package modules

import (
	"context"

	"github.com/ldesfontaine/bientot/internal/transport"
)

// Module est l'interface que chaque module agent implémente.
// Detect return true si les prérequis du module sont disponibles sur cette machine.
// Collect collecte les métriques et les return en tant que ModuleData.
type Module interface {
	Name() string
	Detect() bool
	Collect(ctx context.Context) (transport.ModuleData, error)
}
