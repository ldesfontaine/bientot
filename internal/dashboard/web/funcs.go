package web

import (
	"fmt"
	"html/template"
	"time"
)

// funcMap returns the set of helper functions available in all templates.
func funcMap() template.FuncMap {
	return template.FuncMap{
		"fmtDuration":         fmtDuration,
		"fmtBytes":            fmtBytes,
		"fmtPercent":          fmtPercent,
		"fmtRelativeDuration": fmtRelativeDuration,
		"currentMachine":      currentMachine,
	}
}

// fmtBytes formats a byte count with SI decimal units, calibrated for dashboard
// display: < 1 MB → "NNN KB", < 1 GB → "NNN.N MB", < 1 TB → "NNN.N GB",
// else "NN.NN TB". Decimal units (1 GB = 10^9 bytes), matching `df --si`.
// Negative input returns "—".
func fmtBytes(b int64) string {
	if b < 0 {
		return "—"
	}
	const (
		kb = 1_000
		mb = 1_000_000
		gb = 1_000_000_000
		tb = 1_000_000_000_000
	)
	switch {
	case b < mb:
		return fmt.Sprintf("%.0f KB", float64(b)/kb)
	case b < gb:
		return fmt.Sprintf("%.1f MB", float64(b)/mb)
	case b < tb:
		return fmt.Sprintf("%.1f GB", float64(b)/gb)
	default:
		return fmt.Sprintf("%.2f TB", float64(b)/tb)
	}
}

// fmtPercent formats a float as "NN%" with 0 decimals. Negative returns "—".
func fmtPercent(pct float64) string {
	if pct < 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", pct)
}

// fmtRelativeDuration formats a positive duration as "Xs ago" / "Xm ago" /
// "Xh Xm ago". Negative duration returns "—" (timestamp in the future is
// a bug we don't mask).
func fmtRelativeDuration(d time.Duration) string {
	if d < 0 {
		return "—"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return fmtDuration(d) + " ago"
}

// currentMachine returns the sidebarMachine matching sidebar.CurrentMachineID.
// Falls back to the first machine if not found, then to a zero value.
// Defensive — handlers should always pass a valid CurrentMachineID.
func currentMachine(sidebar *sidebarData) sidebarMachine {
	if sidebar == nil {
		return sidebarMachine{}
	}
	for _, m := range sidebar.Machines {
		if m.ID == sidebar.CurrentMachineID {
			return m
		}
	}
	if len(sidebar.Machines) > 0 {
		return sidebar.Machines[0]
	}
	return sidebarMachine{}
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
