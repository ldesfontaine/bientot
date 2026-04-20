package web

import (
	"fmt"
	"html/template"
	"time"
)

// funcMap returns the set of helper functions available in all templates.
func funcMap() template.FuncMap {
	return template.FuncMap{
		"fmtDuration": fmtDuration,
	}
}

// fmtDuration formats a time.Duration as "2d 14h 22m", "3h 08m", "45s".
// Compact, human-readable, matches the design mockups.
func fmtDuration(d time.Duration) string {
	if d < 0 {
		return "—"
	}

	days := int(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour

	hours := int(d / time.Hour)
	d -= time.Duration(hours) * time.Hour

	minutes := int(d / time.Minute)
	d -= time.Duration(minutes) * time.Minute

	seconds := int(d / time.Second)

	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh %02dm", hours, minutes)
	case minutes > 0:
		return fmt.Sprintf("%dm %02ds", minutes, seconds)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}
