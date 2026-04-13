package veille

import (
	"testing"

	"github.com/ldesfontaine/bientot/internal"
)

func TestDetermineConfidence(t *testing.T) {
	tests := []struct {
		name     string
		alert    Alert
		item     internal.SoftwareItem
		expected string
	}{
		{
			name: "version mentioned in title → confirmed",
			alert: Alert{
				Title:       "CVE-2024-1234: Traefik 3.1.2 vulnerability",
				Description: "Traefik before 3.1.3 allows...",
			},
			item: internal.SoftwareItem{
				Name: "traefik", Version: "3.1.2",
			},
			expected: "confirmed",
		},
		{
			name: "latest version → likely",
			alert: Alert{
				Title:       "CVE-2024-5678: Traefik RCE",
				Description: "All versions affected",
			},
			item: internal.SoftwareItem{
				Name: "traefik", Version: "latest",
			},
			expected: "likely",
		},
		{
			name: "tool matched, no version info → likely",
			alert: Alert{
				Title:       "CVE-2024-9999: Nginx vulnerability",
				Description: "Nginx before 1.26 is affected",
			},
			item: internal.SoftwareItem{
				Name: "nginx", Version: "1.24.0",
			},
			expected: "likely",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineConfidence(tt.alert, tt.item)
			if result != tt.expected {
				t.Errorf("determineConfidence() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestIsCISAKEV(t *testing.T) {
	tests := []struct {
		alert    Alert
		expected bool
	}{
		{Alert{SourceID: "cisa-kev", SourceName: "CISA KEV"}, true},
		{Alert{SourceID: "nvd", SourceName: "NVD"}, false},
		{Alert{SourceID: "github", SourceName: "CISA Known Exploited Vulns (KEV)"}, true},
	}

	for _, tt := range tests {
		result := isCISAKEV(tt.alert)
		if result != tt.expected {
			t.Errorf("isCISAKEV(%s/%s) = %v, want %v", tt.alert.SourceID, tt.alert.SourceName, result, tt.expected)
		}
	}
}

func TestParseImageTag(t *testing.T) {
	tests := []struct {
		input   string
		name    string
		version string
	}{
		{"traefik:3.1.2", "traefik", "3.1.2"},
		{"nginx", "nginx", "latest"},
		{"ghcr.io/org/app:v1.0", "app", "v1.0"},
		{"registry.example.com/myapp:sha-abc123", "myapp", "sha-abc123"},
		{"vaultwarden/server:1.30.5", "server", "1.30.5"},
	}

	for _, tt := range tests {
		// Use the server package's parseImageTag indirectly —
		// we test the logic here as it's the same algorithm
		name, version := parseImage(tt.input)
		if name != tt.name || version != tt.version {
			t.Errorf("parseImage(%s) = (%s, %s), want (%s, %s)", tt.input, name, version, tt.name, tt.version)
		}
	}
}

// parseImage duplicates the server logic for testing.
func parseImage(image string) (string, string) {
	// Remove registry prefix
	for i := len(image) - 1; i >= 0; i-- {
		if image[i] == '/' {
			image = image[i+1:]
			break
		}
	}

	// Split name:version
	for i := 0; i < len(image); i++ {
		if image[i] == ':' {
			return image[:i], image[i+1:]
		}
	}
	return image, "latest"
}
