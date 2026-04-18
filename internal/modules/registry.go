package modules

import (
	"fmt"
	"log/slog"
)

// FactoryFunc builds a Module from a free-form config map.
type FactoryFunc func(config map[string]interface{}) (Module, error)

// registry is the set of known module types indexed by their declared "type" name.
var registry = make(map[string]FactoryFunc)

// Register adds a factory to the registry for a given module type.
// Typically called from an init() function of each module package.
// Panics if the type name is already registered — prevents silent override.
func Register(typeName string, factory FactoryFunc) {
	if _, exists := registry[typeName]; exists {
		panic(fmt.Sprintf("modules.Register: type %q already registered", typeName))
	}
	registry[typeName] = factory
}

// ModuleConfig mirrors config.ModuleConfig to avoid an import cycle.
// The caller converts from their own config type.
type ModuleConfig struct {
	Type    string
	Enabled bool
	Config  map[string]interface{}
}

// Build instantiates all enabled modules from the given configs.
// Returns an error on the first module that fails to construct.
// Unknown module types also fail — no silent skip.
func Build(configs []ModuleConfig, log *slog.Logger) ([]Module, error) {
	var result []Module

	for i, mc := range configs {
		if !mc.Enabled {
			log.Info("module skipped (disabled)", "type", mc.Type)
			continue
		}

		factory, ok := registry[mc.Type]
		if !ok {
			return nil, fmt.Errorf("modules[%d]: unknown type %q (known: %v)", i, mc.Type, knownTypes())
		}

		mod, err := factory(mc.Config)
		if err != nil {
			return nil, fmt.Errorf("modules[%d] (type=%s): %w", i, mc.Type, err)
		}

		log.Info("module enabled", "type", mc.Type, "name", mod.Name())
		result = append(result, mod)
	}

	return result, nil
}

func knownTypes() []string {
	types := make([]string, 0, len(registry))
	for t := range registry {
		types = append(types, t)
	}
	return types
}
