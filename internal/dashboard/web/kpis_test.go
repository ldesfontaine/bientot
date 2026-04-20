package web

import (
	"testing"
	"time"

	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
)

func mk(name string, value float64, tsNs int64) storage.Metric {
	return storage.Metric{
		Name:        name,
		Value:       value,
		Module:      "system",
		TimestampNs: tsNs,
	}
}

func TestBuildKPIs_Complete(t *testing.T) {
	now := time.Date(2026, 4, 20, 14, 0, 0, 0, time.UTC)
	ts := now.UnixNano()

	metrics := map[string]storage.Metric{
		"uptime_seconds":         mk("uptime_seconds", 3600*24*2+3600*14+60*22, ts),
		"load_average_1m":        mk("load_average_1m", 0.42, ts),
		"load_average_5m":        mk("load_average_5m", 0.60, ts),
		"memory_total_bytes":     mk("memory_total_bytes", 4_000_000_000, ts),
		"memory_available_bytes": mk("memory_available_bytes", 2_200_000_000, ts),
		"filesystem_size_bytes":  mk("filesystem_size_bytes", 30_000_000_000, ts),
		"filesystem_avail_bytes": mk("filesystem_avail_bytes", 11_400_000_000, ts),
	}

	lastPush := now.Add(-30 * time.Second)
	kpis := buildKPIs(metrics, now, lastPush)

	if len(kpis.Cards) != 6 {
		t.Fatalf("expected 6 cards, got %d", len(kpis.Cards))
	}

	wantLabels := []string{"Uptime", "Load 1m", "Memory", "Disk /", "Containers", "Last push"}
	for i, want := range wantLabels {
		if kpis.Cards[i].Label != want {
			t.Errorf("cards[%d].Label = %q, want %q", i, kpis.Cards[i].Label, want)
		}
	}

	if kpis.Cards[0].Value != "2d 14h 22m" {
		t.Errorf("uptime value = %q, want 2d 14h 22m", kpis.Cards[0].Value)
	}
	if kpis.Cards[1].Value != "0.42" {
		t.Errorf("load value = %q", kpis.Cards[1].Value)
	}
	if kpis.Cards[2].Value != "45%" {
		t.Errorf("memory value = %q, want 45%%", kpis.Cards[2].Value)
	}
	if kpis.Cards[3].Value != "62%" {
		t.Errorf("disk value = %q, want 62%%", kpis.Cards[3].Value)
	}
	if !kpis.Cards[4].Missing {
		t.Errorf("containers should be Missing=true")
	}
	if kpis.Cards[5].Value != "30s ago" {
		t.Errorf("last push value = %q, want 30s ago", kpis.Cards[5].Value)
	}
}

func TestBuildKPIs_AllMissing(t *testing.T) {
	now := time.Now()
	kpis := buildKPIs(map[string]storage.Metric{}, now, time.Time{})

	if len(kpis.Cards) != 6 {
		t.Fatalf("expected 6 cards, got %d", len(kpis.Cards))
	}

	for i, c := range kpis.Cards {
		if !c.Missing {
			t.Errorf("card[%d] (%s) should be Missing", i, c.Label)
		}
	}
}

func TestBuildKPIs_PartialMetrics(t *testing.T) {
	now := time.Now()
	ts := now.UnixNano()

	metrics := map[string]storage.Metric{
		"uptime_seconds": mk("uptime_seconds", 3600, ts),
	}

	kpis := buildKPIs(metrics, now, now)

	if kpis.Cards[0].Missing {
		t.Error("uptime should not be missing")
	}
	for _, idx := range []int{1, 2, 3} {
		if !kpis.Cards[idx].Missing {
			t.Errorf("card[%d] (%s) should be missing", idx, kpis.Cards[idx].Label)
		}
	}
}

func TestBuildMemoryCard_Warn(t *testing.T) {
	now := time.Now()
	ts := now.UnixNano()

	// 90% used → warn (warn=85, crit=95)
	metrics := map[string]storage.Metric{
		"memory_total_bytes":     mk("memory_total_bytes", 1_000_000_000, ts),
		"memory_available_bytes": mk("memory_available_bytes", 100_000_000, ts),
	}

	kpis := buildKPIs(metrics, now, now)
	if kpis.Cards[2].Variant != "warn" {
		t.Errorf("memory at 90%% should be warn, got %q", kpis.Cards[2].Variant)
	}
}

func TestBuildMemoryCard_Crit(t *testing.T) {
	now := time.Now()
	ts := now.UnixNano()

	// 97% used → crit
	metrics := map[string]storage.Metric{
		"memory_total_bytes":     mk("memory_total_bytes", 1_000_000_000, ts),
		"memory_available_bytes": mk("memory_available_bytes", 30_000_000, ts),
	}

	kpis := buildKPIs(metrics, now, now)
	if kpis.Cards[2].Variant != "crit" {
		t.Errorf("memory at 97%% should be crit, got %q", kpis.Cards[2].Variant)
	}
}

func TestBuildMemoryCard_ZeroTotalDoesntPanic(t *testing.T) {
	now := time.Now()
	ts := now.UnixNano()

	metrics := map[string]storage.Metric{
		"memory_total_bytes":     mk("memory_total_bytes", 0, ts),
		"memory_available_bytes": mk("memory_available_bytes", 0, ts),
	}

	kpis := buildKPIs(metrics, now, now)
	if kpis.Cards[2].Value != "0%" {
		t.Errorf("memory with zero total should be 0%%, got %q", kpis.Cards[2].Value)
	}
}

// ─── fmtBytes ─────────────────────────────────────────────

func TestFmtBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 KB"},
		{500, "0 KB"},
		{1_500, "2 KB"},
		{500_000, "500 KB"},
		{1_500_000, "1.5 MB"},
		{350_400_000, "350.4 MB"},
		{1_500_000_000, "1.5 GB"},
		{29_200_000_000, "29.2 GB"},
		{1_850_000_000_000, "1.85 TB"},
		{-1, "—"},
	}
	for _, tc := range cases {
		got := fmtBytes(tc.in)
		if got != tc.want {
			t.Errorf("fmtBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFmtPercent(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0%"},
		{45.2, "45%"},
		{99.9, "100%"},
		{-1, "—"},
	}
	for _, tc := range cases {
		got := fmtPercent(tc.in)
		if got != tc.want {
			t.Errorf("fmtPercent(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFmtRelativeDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{5 * time.Second, "5s ago"},
		{45 * time.Second, "45s ago"},
		{90 * time.Second, "1m ago"},
		{59 * time.Minute, "59m ago"},
		{90 * time.Minute, "1h 30m ago"},
		{-1 * time.Second, "—"},
	}
	for _, tc := range cases {
		got := fmtRelativeDuration(tc.in)
		if got != tc.want {
			t.Errorf("fmtRelativeDuration(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestVariantFromPercent(t *testing.T) {
	cases := []struct {
		pct  float64
		want string
	}{
		{10, ""},
		{84.9, ""},
		{85, "warn"},
		{94.9, "warn"},
		{95, "crit"},
		{100, "crit"},
	}
	for _, tc := range cases {
		got := variantFromPercent(tc.pct, 85, 95)
		if got != tc.want {
			t.Errorf("variantFromPercent(%v) = %q, want %q", tc.pct, got, tc.want)
		}
	}
}
