package enrichment

import "testing"

func TestScoreIP(t *testing.T) {
	tests := []struct {
		name       string
		reqCount   int
		paths      []string
		inBlock    bool
		inCrowdSec bool
		wantMin    int
		wantMax    int
	}{
		{
			name:       "new IP not in CrowdSec, low volume",
			reqCount:   5,
			paths:      []string{"/"},
			inBlock:    false,
			inCrowdSec: false,
			wantMin:    50,
			wantMax:    60,
		},
		{
			name:       "blocked by CrowdSec, in blocklist",
			reqCount:   5,
			paths:      []string{"/"},
			inBlock:    true,
			inCrowdSec: true,
			wantMin:    -30,
			wantMax:    0,
		},
		{
			name:       "sensitive path targeted, not in CrowdSec",
			reqCount:   25,
			paths:      []string{"/.env", "/admin"},
			inBlock:    false,
			inCrowdSec: false,
			wantMin:    80,
			wantMax:    100,
		},
		{
			name:       "high volume, in CrowdSec",
			reqCount:   100,
			paths:      []string{"/api/data"},
			inBlock:    false,
			inCrowdSec: true,
			wantMin:    90,
			wantMax:    110,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ScoreIP(tt.reqCount, tt.paths, tt.inBlock, tt.inCrowdSec)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("ScoreIP() = %d, want [%d, %d]", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}
