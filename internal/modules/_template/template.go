// Package template is a skeleton for new agent modules.
// Copy this directory, rename the package, and implement the 3 methods.
package template

import (
	"context"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

// Module is a template module. Rename and implement.
type Module struct {
	// Add configuration fields here.
}

func New() *Module { return &Module{} }

// Name returns the module identifier (must be unique across all modules).
func (m *Module) Name() string { return "template" }

// Detect returns true if this module's prerequisites are available.
func (m *Module) Detect() bool { return false }

// Collect gathers metrics. Only called if Detect() returned true.
func (m *Module) Collect(_ context.Context) (transport.ModuleData, error) {
	return transport.ModuleData{
		Module:    m.Name(),
		Metrics:   []transport.MetricPoint{},
		Timestamp: time.Now(),
	}, nil
}
