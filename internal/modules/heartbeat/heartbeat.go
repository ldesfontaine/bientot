package heartbeat

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ldesfontaine/bientot/internal/modules"
)

// Module is the heartbeat collector: always available, reports hostname and up=1.
type Module struct{}

// New returns a new heartbeat module.
func New() *Module {
	return &Module{}
}

// Name implements modules.Module.
func (m *Module) Name() string {
	return "heartbeat"
}

// Detect implements modules.Module. Heartbeat is always available.
func (m *Module) Detect(_ context.Context) error {
	return nil
}

// Collect implements modules.Module.
func (m *Module) Collect(_ context.Context) (*modules.Data, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("heartbeat: get hostname: %w", err)
	}

	return &modules.Data{
		Module:    m.Name(),
		Metrics:   []modules.Metric{{Name: "up", Value: 1}},
		Metadata:  map[string]string{"hostname": hostname},
		Timestamp: time.Now(),
	}, nil
}

// Interval implements modules.Module.
func (m *Module) Interval() time.Duration {
	return 30 * time.Second
}

// Compile-time check that *Module implements modules.Module.
var _ modules.Module = (*Module)(nil)
