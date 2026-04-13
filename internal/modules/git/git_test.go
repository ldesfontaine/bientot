package git

import "testing"

func TestDetect_NoRepos(t *testing.T) {
	m := New(nil)
	if m.Detect() {
		t.Fatal("should not detect with no repos")
	}
}

func TestRepoName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/projects/bientot", "bientot"},
		{"/opt/app/", "app"},
		{"myrepo", "myrepo"},
	}

	for _, tt := range tests {
		got := repoName(tt.path)
		if got != tt.want {
			t.Errorf("repoName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
