package web

import (
	"testing"
	"time"
)

func TestFmtDuration(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"seconds", 45 * time.Second, "45s"},
		{"minutes_seconds", 2*time.Minute + 30*time.Second, "2m 30s"},
		{"hours_minutes", 3*time.Hour + 8*time.Minute, "3h 08m"},
		{"days", 2*24*time.Hour + 14*time.Hour + 22*time.Minute, "2d 14h 22m"},
		{"zero", 0, "0s"},
		{"negative", -1 * time.Second, "—"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fmtDuration(tc.d)
			if got != tc.want {
				t.Errorf("fmtDuration(%v) = %q, want %q", tc.d, got, tc.want)
			}
		})
	}
}
