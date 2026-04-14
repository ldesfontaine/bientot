package collector

import (
	"context"
	"time"

	"github.com/ldesfontaine/bientot/internal"
)

// Collector est l'interface que tous les collecteurs de métriques doivent implémenter
type Collector interface {
	// Name return le nom de l'instance du collecteur
	Name() string

	// Type return le type du collecteur (prometheus, docker, zfs, etc.)
	Type() string

	// Collect récupère les métriques depuis la source
	Collect(ctx context.Context) ([]internal.Metric, error)

	// Interval return la fréquence d'exécution de ce collecteur
	Interval() time.Duration
}

// Registry contient tous les collecteurs enregistrés
type Registry struct {
	collectors []Collector
}

// NewRegistry crée un nouveau registre de collecteurs
func NewRegistry() *Registry {
	return &Registry{
		collectors: make([]Collector, 0),
	}
}

// Register ajoute un collecteur au registre
func (r *Registry) Register(c Collector) {
	r.collectors = append(r.collectors, c)
}

// All return tous les collecteurs enregistrés
func (r *Registry) All() []Collector {
	return r.collectors
}

// ByType return les collecteurs d'un type spécifique
func (r *Registry) ByType(t string) []Collector {
	var result []Collector
	for _, c := range r.collectors {
		if c.Type() == t {
			result = append(result, c)
		}
	}
	return result
}
