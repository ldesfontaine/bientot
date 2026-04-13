package modules

import (
	"context"

	"github.com/ldesfontaine/bientot/internal/transport"
)

// Module is the interface every agent module implements.
// Detect returns true if the module's prerequisites are available on this machine.
// Collect gathers metrics and returns them as ModuleData.
type Module interface {
	Name() string
	Detect() bool
	Collect(ctx context.Context) (transport.ModuleData, error)
}
