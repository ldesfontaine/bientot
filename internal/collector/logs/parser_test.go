package logs

import (
	"testing"
)

func TestParseSSH(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantNil  bool
		severity string
		action   string
		user     string
		srcIP    string
	}{
		{
			name:     "failed password",
			line:     "Jun 15 12:34:56 vps sshd[1234]: Failed password for root from 192.168.1.100 port 54321 ssh2",
			severity: "warning",
			action:   "failed",
			user:     "root",
			srcIP:    "192.168.1.100",
		},
		{
			name:     "invalid user",
			line:     "Jun 15 12:34:56 vps sshd[1234]: Invalid user admin from 10.0.0.1 port 22",
			severity: "warning",
			action:   "invalid_user",
			user:     "admin",
			srcIP:    "10.0.0.1",
		},
		{
			name:     "accepted publickey",
			line:     "Jun 15 12:34:56 vps sshd[1234]: Accepted publickey for lucas from 100.64.0.1 port 54321 ssh2: RSA SHA256:xxx",
			severity: "info",
			action:   "accepted",
			user:     "lucas",
			srcIP:    "100.64.0.1",
		},
		{
			name:     "accepted password",
			line:     "Jun 15 12:34:56 vps sshd[1234]: Accepted password for deploy from 10.0.0.5 port 12345 ssh2",
			severity: "info",
			action:   "accepted",
			user:     "deploy",
			srcIP:    "10.0.0.5",
		},
		{
			name:     "disconnected",
			line:     "Jun 15 12:34:56 vps sshd[1234]: Disconnected from 192.168.1.100 port 54321",
			severity: "info",
			action:   "disconnected",
			srcIP:    "192.168.1.100",
		},
		{
			name:    "irrelevant line",
			line:    "Jun 15 12:34:56 vps sshd[1234]: pam_unix(sshd:session): session opened",
			wantNil: true,
		},
		{
			name:    "empty line",
			line:    "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := ParseSSH(tt.line, "test-machine")

			if tt.wantNil {
				if entry != nil {
					t.Fatalf("expected nil, got entry with action=%v", entry.Parsed["action"])
				}
				return
			}

			if entry == nil {
				t.Fatal("expected entry, got nil")
			}
			if entry.Source != "ssh" {
				t.Errorf("source = %q, want ssh", entry.Source)
			}
			if entry.Severity != tt.severity {
				t.Errorf("severity = %q, want %q", entry.Severity, tt.severity)
			}
			if entry.Machine != "test-machine" {
				t.Errorf("machine = %q, want test-machine", entry.Machine)
			}
			if entry.Parsed["action"] != tt.action {
				t.Errorf("action = %v, want %q", entry.Parsed["action"], tt.action)
			}
			if tt.user != "" && entry.Parsed["user"] != tt.user {
				t.Errorf("user = %v, want %q", entry.Parsed["user"], tt.user)
			}
			if tt.srcIP != "" && entry.Parsed["src_ip"] != tt.srcIP {
				t.Errorf("src_ip = %v, want %q", entry.Parsed["src_ip"], tt.srcIP)
			}
		})
	}
}

func TestParseNftables(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantNil bool
		srcIP   string
		dstPort string
		proto   string
	}{
		{
			name:    "standard drop",
			line:    "Jun 15 12:34:56 vps kernel: nftables drop: IN=eth0 OUT= MAC=aa:bb:cc SRC=45.33.32.156 DST=10.0.0.1 LEN=40 PROTO=TCP SPT=12345 DPT=22 WINDOW=1024",
			srcIP:   "45.33.32.156",
			dstPort: "22",
			proto:   "TCP",
		},
		{
			name:    "UDP drop",
			line:    "Jun 15 12:34:56 vps kernel: nftables drop: IN=eth0 SRC=1.2.3.4 DST=5.6.7.8 PROTO=UDP DPT=53",
			srcIP:   "1.2.3.4",
			dstPort: "53",
			proto:   "UDP",
		},
		{
			name:    "no nftables keyword",
			line:    "Jun 15 12:34:56 vps kernel: some other message SRC=1.2.3.4",
			wantNil: true,
		},
		{
			name:    "nftables but no SRC",
			line:    "Jun 15 12:34:56 vps kernel: nftables rule loaded",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := ParseNftables(tt.line, "vps")

			if tt.wantNil {
				if entry != nil {
					t.Fatal("expected nil")
				}
				return
			}
			if entry == nil {
				t.Fatal("expected entry, got nil")
			}
			if entry.Source != "nftables" {
				t.Errorf("source = %q, want nftables", entry.Source)
			}
			if entry.Severity != "warning" {
				t.Errorf("severity = %q, want warning", entry.Severity)
			}
			if entry.Parsed["src_ip"] != tt.srcIP {
				t.Errorf("src_ip = %v, want %q", entry.Parsed["src_ip"], tt.srcIP)
			}
			if tt.dstPort != "" && entry.Parsed["dst_port"] != tt.dstPort {
				t.Errorf("dst_port = %v, want %q", entry.Parsed["dst_port"], tt.dstPort)
			}
			if tt.proto != "" && entry.Parsed["protocol"] != tt.proto {
				t.Errorf("protocol = %v, want %q", entry.Parsed["protocol"], tt.proto)
			}
			if entry.Parsed["action"] != "drop" {
				t.Errorf("action = %v, want drop", entry.Parsed["action"])
			}
		})
	}
}

func TestParseUFW(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantNil bool
		action  string
		srcIP   string
		dstPort string
		proto   string
		sev     string
	}{
		{
			name:    "block",
			line:    "Jun 15 12:34:56 brain kernel: [UFW BLOCK] IN=eth0 OUT= MAC=aa:bb:cc SRC=203.0.113.5 DST=10.0.0.2 LEN=60 PROTO=TCP SPT=45678 DPT=443 WINDOW=65535",
			action:  "block",
			srcIP:   "203.0.113.5",
			dstPort: "443",
			proto:   "TCP",
			sev:     "warning",
		},
		{
			name:    "allow",
			line:    "Jun 15 12:34:56 brain kernel: [UFW ALLOW] IN=eth0 SRC=100.64.0.1 DST=10.0.0.2 PROTO=TCP DPT=80",
			action:  "allow",
			srcIP:   "100.64.0.1",
			dstPort: "80",
			proto:   "TCP",
			sev:     "info",
		},
		{
			name:    "audit",
			line:    "Jun 15 12:34:56 brain kernel: [UFW AUDIT] IN=eth0 SRC=10.0.0.5 DST=10.0.0.2 PROTO=UDP DPT=53",
			action:  "audit",
			srcIP:   "10.0.0.5",
			dstPort: "53",
			proto:   "UDP",
			sev:     "warning",
		},
		{
			name:    "no UFW keyword",
			line:    "Jun 15 12:34:56 brain kernel: something else",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := ParseUFW(tt.line, "brain")

			if tt.wantNil {
				if entry != nil {
					t.Fatal("expected nil")
				}
				return
			}
			if entry == nil {
				t.Fatal("expected entry, got nil")
			}
			if entry.Source != "ufw" {
				t.Errorf("source = %q, want ufw", entry.Source)
			}
			if entry.Severity != tt.sev {
				t.Errorf("severity = %q, want %q", entry.Severity, tt.sev)
			}
			if entry.Parsed["action"] != tt.action {
				t.Errorf("action = %v, want %q", entry.Parsed["action"], tt.action)
			}
			if entry.Parsed["src_ip"] != tt.srcIP {
				t.Errorf("src_ip = %v, want %q", entry.Parsed["src_ip"], tt.srcIP)
			}
			if tt.dstPort != "" && entry.Parsed["dst_port"] != tt.dstPort {
				t.Errorf("dst_port = %v, want %q", entry.Parsed["dst_port"], tt.dstPort)
			}
		})
	}
}

func TestParseDockerLog(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		container string
		image     string
		stream    string
		wantNil   bool
		severity  string
	}{
		{
			name:      "stderr error",
			line:      "2024-06-15T12:34:56Z ERROR: connection refused to database",
			container: "api",
			image:     "myapp:latest",
			stream:    "stderr",
			severity:  "error",
		},
		{
			name:      "stdout error keyword",
			line:      "time=2024-06-15T12:34:56Z level=error msg=\"failed to connect\"",
			container: "nginx",
			image:     "nginx:alpine",
			stream:    "stdout",
			severity:  "error",
		},
		{
			name:      "fatal message",
			line:      "FATAL: could not start server",
			container: "postgres",
			image:     "postgres:16",
			stream:    "stderr",
			severity:  "critical",
		},
		{
			name:      "panic message",
			line:      "panic: runtime error: index out of range",
			container: "api",
			image:     "myapp:latest",
			stream:    "stderr",
			severity:  "critical",
		},
		{
			name:      "warning message",
			line:      "WARN: deprecated config option used",
			container: "traefik",
			image:     "traefik:v3",
			stream:    "stdout",
			severity:  "warning",
		},
		{
			name:      "normal stdout",
			line:      "Server started on port 8080",
			container: "api",
			image:     "myapp:latest",
			stream:    "stdout",
			wantNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := ParseDockerLog(tt.line, tt.container, tt.image, tt.stream, "test")

			if tt.wantNil {
				if entry != nil {
					t.Fatal("expected nil")
				}
				return
			}
			if entry == nil {
				t.Fatal("expected entry, got nil")
			}
			if entry.Source != "docker" {
				t.Errorf("source = %q, want docker", entry.Source)
			}
			if entry.Severity != tt.severity {
				t.Errorf("severity = %q, want %q", entry.Severity, tt.severity)
			}
			if entry.Parsed["container"] != tt.container {
				t.Errorf("container = %v, want %q", entry.Parsed["container"], tt.container)
			}
			if entry.Parsed["image"] != tt.image {
				t.Errorf("image = %v, want %q", entry.Parsed["image"], tt.image)
			}
			if entry.Parsed["stream"] != tt.stream {
				t.Errorf("stream = %v, want %q", entry.Parsed["stream"], tt.stream)
			}
		})
	}
}

func TestParseCrowdSecDecision(t *testing.T) {
	entry := ParseCrowdSecDecision("1.2.3.4", "ssh-bf", "4h", "ip", "vps")

	if entry == nil {
		t.Fatal("expected entry, got nil")
	}
	if entry.Source != "crowdsec" {
		t.Errorf("source = %q, want crowdsec", entry.Source)
	}
	if entry.Severity != "warning" {
		t.Errorf("severity = %q, want warning", entry.Severity)
	}
	if entry.Parsed["ip"] != "1.2.3.4" {
		t.Errorf("ip = %v, want 1.2.3.4", entry.Parsed["ip"])
	}
	if entry.Parsed["scenario"] != "ssh-bf" {
		t.Errorf("scenario = %v, want ssh-bf", entry.Parsed["scenario"])
	}
	if entry.Parsed["duration"] != "4h" {
		t.Errorf("duration = %v, want 4h", entry.Parsed["duration"])
	}
	if entry.Parsed["scope"] != "ip" {
		t.Errorf("scope = %v, want ip", entry.Parsed["scope"])
	}
}

func TestTruncate(t *testing.T) {
	short := "hello"
	if got := truncate(short); got != short {
		t.Errorf("truncate(%q) = %q, want %q", short, got, short)
	}

	long := ""
	for i := 0; i < 600; i++ {
		long += "x"
	}
	got := truncate(long)
	if len(got) != maxMessageLen {
		t.Errorf("truncate(600 chars) len = %d, want %d", len(got), maxMessageLen)
	}
}
