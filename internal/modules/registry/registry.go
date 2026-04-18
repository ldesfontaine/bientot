// Package registry imports all built-in module packages for their side effects.
// Importing this package registers all modules in the central modules registry
// via each module's init() function.
//
// Usage:
//
//	import _ "github.com/ldesfontaine/bientot/internal/modules/registry"
package registry

import (
	_ "github.com/ldesfontaine/bientot/internal/modules/heartbeat"
	_ "github.com/ldesfontaine/bientot/internal/modules/system"
)
