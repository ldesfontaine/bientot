package docker

import "testing"

func TestContainerName(t *testing.T) {
	tests := []struct {
		c    container
		want string
	}{
		{container{ID: "abc123def456", Names: []string{"/traefik"}}, "traefik"},
		{container{ID: "abc123def456", Names: []string{"myapp"}}, "myapp"},
		{container{ID: "abc123def456", Names: nil}, "abc123def456"},
	}

	for _, tt := range tests {
		got := containerName(tt.c)
		if got != tt.want {
			t.Errorf("containerName(%v) = %q, want %q", tt.c.Names, got, tt.want)
		}
	}
}

func TestParseHealth(t *testing.T) {
	tests := []struct {
		status string
		want   float64
	}{
		{"Up 2 hours (healthy)", 2},
		{"Up 5 min (unhealthy)", 1},
		{"Exited (0) 3 hours ago", 0},
	}

	for _, tt := range tests {
		got := parseHealth(tt.status)
		if got != tt.want {
			t.Errorf("parseHealth(%q) = %f, want %f", tt.status, got, tt.want)
		}
	}
}

func TestDetect_InvalidSocket(t *testing.T) {
	m := New("unix:///nonexistent/docker.sock")
	if m.Detect() {
		t.Fatal("should not detect with nonexistent socket")
	}
}
