package web

import (
	"testing"
	"time"

	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
)

func TestBuildModuleCards_Empty(t *testing.T) {
	now := time.Now()
	cards := buildModuleCards(nil, now)

	if len(cards) != len(comingSoonModules) {
		t.Errorf("with no active modules, expected %d coming-soon cards, got %d",
			len(comingSoonModules), len(cards))
	}

	for _, c := range cards {
		if c.Status != "coming" {
			t.Errorf("card %q should be coming, got %q", c.Name, c.Status)
		}
	}
}

func TestBuildModuleCards_ActiveAndComing(t *testing.T) {
	now := time.Now()
	active := []storage.ModuleInfo{
		{Module: "heartbeat", MetricCount: 1, LastUpdateAt: now.Add(-10 * time.Second)},
		{Module: "system", MetricCount: 15, LastUpdateAt: now.Add(-15 * time.Second)},
	}

	cards := buildModuleCards(active, now)

	if len(cards) != 4 {
		t.Fatalf("expected 4 cards (2 active + 2 coming), got %d", len(cards))
	}

	wantNames := []string{"Heartbeat", "System", "Docker", "Certs"}
	for i, want := range wantNames {
		if cards[i].Name != want {
			t.Errorf("cards[%d].Name = %q, want %q", i, cards[i].Name, want)
		}
	}

	if cards[0].Status != "active" || cards[1].Status != "active" {
		t.Error("first 2 cards should be active")
	}
	if cards[2].Status != "coming" || cards[3].Status != "coming" {
		t.Error("last 2 cards should be coming")
	}
}

func TestBuildModuleCards_DedupActiveOverComing(t *testing.T) {
	now := time.Now()
	active := []storage.ModuleInfo{
		{Module: "docker", MetricCount: 5, LastUpdateAt: now},
	}

	cards := buildModuleCards(active, now)

	if len(cards) != 2 {
		t.Fatalf("expected 2 cards (docker active + certs coming), got %d", len(cards))
	}

	if cards[0].Name != "Docker" || cards[0].Status != "active" {
		t.Errorf("cards[0] should be Docker (active), got %+v", cards[0])
	}
	if cards[1].Name != "Certs" || cards[1].Status != "coming" {
		t.Errorf("cards[1] should be Certs (coming), got %+v", cards[1])
	}
}

func TestBuildModuleCards_MetaFormatsCorrectly(t *testing.T) {
	now := time.Now()
	active := []storage.ModuleInfo{
		{Module: "system", MetricCount: 17, LastUpdateAt: now.Add(-30 * time.Second)},
	}

	cards := buildModuleCards(active, now)

	want := "17 metrics · 30s ago"
	if cards[0].Meta != want {
		t.Errorf("meta = %q, want %q", cards[0].Meta, want)
	}
}

func TestBuildModuleCards_DisplayNameCapitalization(t *testing.T) {
	tests := []struct {
		slug string
		want string
	}{
		{"heartbeat", "Heartbeat"},
		{"system", "System"},
		{"", ""},
	}
	for _, tc := range tests {
		got := displayName(tc.slug)
		if got != tc.want {
			t.Errorf("displayName(%q) = %q, want %q", tc.slug, got, tc.want)
		}
	}
}
