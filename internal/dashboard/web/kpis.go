package web

import (
	"fmt"
	"time"

	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
)

// kpiCard is the data for a single KPI card in the overview grid.
// Value is pre-formatted; the template renders it as-is.
// A card with Missing=true is shown as "—" in muted style.
type kpiCard struct {
	Label   string
	Value   string
	Meta    string
	Missing bool
	Variant string // "" | "warn" | "crit"
}

// overviewKPIs holds all 6 KPI cards in display order.
type overviewKPIs struct {
	Cards []kpiCard
}

// buildKPIs constructs the 6-card grid from the latest metrics map.
// metrics may be empty or partial; missing inputs produce Missing=true cards.
func buildKPIs(metrics map[string]storage.Metric, now time.Time, lastPushAt time.Time) overviewKPIs {
	return overviewKPIs{
		Cards: []kpiCard{
			buildUptimeCard(metrics),
			buildLoadCard(metrics),
			buildMemoryCard(metrics),
			buildDiskCard(metrics),
			buildContainersCard(),
			buildLastPushCard(now, lastPushAt),
		},
	}
}

func buildUptimeCard(metrics map[string]storage.Metric) kpiCard {
	m, ok := metrics["uptime_seconds"]
	if !ok {
		return kpiCard{Label: "Uptime", Missing: true}
	}
	d := time.Duration(m.Value) * time.Second
	bootTime := time.Unix(0, m.TimestampNs).Add(-d).UTC()
	return kpiCard{
		Label: "Uptime",
		Value: fmtDuration(d),
		Meta:  "since " + bootTime.Format("2006-01-02 15:04"),
	}
}

func buildLoadCard(metrics map[string]storage.Metric) kpiCard {
	m, ok := metrics["load_average_1m"]
	if !ok {
		return kpiCard{Label: "Load 1m", Missing: true}
	}

	meta := ""
	if avg5, ok := metrics["load_average_5m"]; ok {
		meta = fmt.Sprintf("5m avg %.2f", avg5.Value)
	}

	return kpiCard{
		Label: "Load 1m",
		Value: fmt.Sprintf("%.2f", m.Value),
		Meta:  meta,
	}
}

func buildMemoryCard(metrics map[string]storage.Metric) kpiCard {
	total, okT := metrics["memory_total_bytes"]
	avail, okA := metrics["memory_available_bytes"]
	if !okT || !okA {
		return kpiCard{Label: "Memory", Missing: true}
	}

	used := total.Value - avail.Value
	pct := 0.0
	if total.Value > 0 {
		pct = (used / total.Value) * 100
	}

	return kpiCard{
		Label:   "Memory",
		Value:   fmtPercent(pct),
		Meta:    fmt.Sprintf("%s / %s", fmtBytes(int64(used)), fmtBytes(int64(total.Value))),
		Variant: variantFromPercent(pct, 85, 95),
	}
}

func buildDiskCard(metrics map[string]storage.Metric) kpiCard {
	size, okS := metrics["filesystem_size_bytes"]
	avail, okA := metrics["filesystem_avail_bytes"]
	if !okS || !okA {
		return kpiCard{Label: "Disk /", Missing: true}
	}

	used := size.Value - avail.Value
	pct := 0.0
	if size.Value > 0 {
		pct = (used / size.Value) * 100
	}

	return kpiCard{
		Label:   "Disk /",
		Value:   fmtPercent(pct),
		Meta:    fmt.Sprintf("%s / %s", fmtBytes(int64(used)), fmtBytes(int64(size.Value))),
		Variant: variantFromPercent(pct, 85, 95),
	}
}

// buildContainersCard always returns a placeholder until 5.4 ships docker.
func buildContainersCard() kpiCard {
	return kpiCard{
		Label:   "Containers",
		Value:   "—",
		Meta:    "docker module in 5.4",
		Missing: true,
	}
}

func buildLastPushCard(now, lastPushAt time.Time) kpiCard {
	if lastPushAt.IsZero() {
		return kpiCard{Label: "Last push", Missing: true}
	}
	age := now.Sub(lastPushAt)
	return kpiCard{
		Label: "Last push",
		Value: fmtRelativeDuration(age),
		Meta:  "every 30s",
	}
}

// variantFromPercent: "warn" if pct >= warnAt, "crit" if pct >= critAt, else "".
func variantFromPercent(pct, warnAt, critAt float64) string {
	switch {
	case pct >= critAt:
		return "crit"
	case pct >= warnAt:
		return "warn"
	default:
		return ""
	}
}
