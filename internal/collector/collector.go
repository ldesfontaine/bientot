package collector

import (
	"context"
	"time"

	"github.com/ldesfontaine/bientot/internal"
)

// Collector is the interface all metric collectors must implement
type Collector interface {
	// Name returns the collector instance name
	Name() string

	// Type returns the collector type (prometheus, docker, zfs, etc.)
	Type() string

	// Collect gathers metrics from the source
	Collect(ctx context.Context) ([]internal.Metric, error)

	// Interval returns how often this collector should run
	Interval() time.Duration
}

// Registry holds all registered collectors
type Registry struct {
	collectors []Collector
}

// NewRegistry creates a new collector registry
func NewRegistry() *Registry {
	return &Registry{
		collectors: make([]Collector, 0),
	}
}

// Register adds a collector to the registry
func (r *Registry) Register(c Collector) {
	r.collectors = append(r.collectors, c)
}

// All returns all registered collectors
func (r *Registry) All() []Collector {
	return r.collectors
}

// ByType returns collectors of a specific type
func (r *Registry) ByType(t string) []Collector {
	var result []Collector
	for _, c := range r.collectors {
		if c.Type() == t {
			result = append(result, c)
		}
	}
	return result
}
