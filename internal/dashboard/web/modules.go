package web

import (
	"fmt"
	"strings"
	"time"

	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
)

// moduleCard is the data for one module card in the overview grid.
type moduleCard struct {
	Name   string // display name, e.g. "System", "Docker"
	Slug   string // identifier, e.g. "system", "docker"
	Icon   string // partial template name: "icon-server", "icon-container", ...
	Status string // "active" | "coming"
	Meta   string // subtitle: "15 metrics · 30s ago" | "coming in 5.4"
}

// comingSoonModules lists modules planned but not yet implemented.
// When a module ships, REMOVE it here — it appears automatically as active
// once it starts pushing metrics.
var comingSoonModules = []struct {
	Name    string
	Slug    string
	Icon    string
	Version string
}{
	{Name: "Docker", Slug: "docker", Icon: "icon-container", Version: "5.4"},
	{Name: "Certs", Slug: "certs", Icon: "icon-shield-check", Version: "5.5"},
}

// iconForActiveModule maps detected module slugs to their icon partial.
// Unknown modules fall back to "icon-server".
var iconForActiveModule = map[string]string{
	"heartbeat": "icon-activity",
	"system":    "icon-server",
	"docker":    "icon-container",
	"certs":     "icon-shield-check",
}

// buildModuleCards merges detected active modules with the hardcoded
// coming-soon list. Active first (alphabetical from storage), then coming.
// Coming-soon entries that already appear as active are skipped (defensive
// against forgetting to remove a module from the coming list when it ships).
func buildModuleCards(active []storage.ModuleInfo, now time.Time) []moduleCard {
	activeSlugs := make(map[string]bool, len(active))
	cards := make([]moduleCard, 0, len(active)+len(comingSoonModules))

	for _, m := range active {
		activeSlugs[m.Module] = true
		icon := iconForActiveModule[m.Module]
		if icon == "" {
			icon = "icon-server"
		}
		cards = append(cards, moduleCard{
			Name:   displayName(m.Module),
			Slug:   m.Module,
			Icon:   icon,
			Status: "active",
			Meta:   fmt.Sprintf("%d metrics · %s", m.MetricCount, fmtRelativeDuration(now.Sub(m.LastUpdateAt))),
		})
	}

	for _, c := range comingSoonModules {
		if activeSlugs[c.Slug] {
			continue
		}
		cards = append(cards, moduleCard{
			Name:   c.Name,
			Slug:   c.Slug,
			Icon:   c.Icon,
			Status: "coming",
			Meta:   "coming in " + c.Version,
		})
	}

	return cards
}

// displayName converts a slug like "heartbeat" to "Heartbeat" — capitalize
// the first letter only. Suffices for current single-word module names.
func displayName(slug string) string {
	if slug == "" {
		return ""
	}
	return strings.ToUpper(slug[:1]) + slug[1:]
}
