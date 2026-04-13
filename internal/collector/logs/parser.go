package logs

import (
	"regexp"
	"strings"
	"time"

	"github.com/ldesfontaine/bientot/internal"
)

const maxMessageLen = 500

// ParseSSH parses an SSH auth log line into a LogEntry.
// Handles: accepted, failed password, invalid user, disconnected, connection closed.
func ParseSSH(line, machine string) *internal.LogEntry {
	if line == "" {
		return nil
	}

	parsed := map[string]any{}
	severity := "info"

	// Failed password: "Failed password for root from 1.2.3.4 port 22 ssh2"
	if m := reSSHFailed.FindStringSubmatch(line); m != nil {
		parsed["user"] = m[1]
		parsed["src_ip"] = m[2]
		parsed["port"] = m[3]
		parsed["action"] = "failed"
		severity = "warning"
	} else if m := reSSHInvalidUser.FindStringSubmatch(line); m != nil {
		// Invalid user: "Invalid user admin from 1.2.3.4 port 22"
		parsed["user"] = m[1]
		parsed["src_ip"] = m[2]
		parsed["port"] = m[3]
		parsed["action"] = "invalid_user"
		severity = "warning"
	} else if m := reSSHAccepted.FindStringSubmatch(line); m != nil {
		// Accepted: "Accepted publickey for lucas from 1.2.3.4 port 54321 ssh2"
		parsed["method"] = m[1]
		parsed["user"] = m[2]
		parsed["src_ip"] = m[3]
		parsed["port"] = m[4]
		parsed["action"] = "accepted"
		severity = "info"
	} else if m := reSSHDisconnected.FindStringSubmatch(line); m != nil {
		parsed["src_ip"] = m[1]
		parsed["port"] = m[2]
		parsed["action"] = "disconnected"
		severity = "info"
	} else {
		// Not an interesting SSH line
		return nil
	}

	return &internal.LogEntry{
		Timestamp: time.Now(),
		Source:    "ssh",
		Machine:  machine,
		Severity: severity,
		Message:  truncate(line),
		Parsed:   parsed,
	}
}

// ParseNftables parses an nftables drop log line.
// Format: "nftables drop: IN=eth0 ... SRC=1.2.3.4 DST=5.6.7.8 ... PROTO=TCP DPT=22"
func ParseNftables(line, machine string) *internal.LogEntry {
	if !strings.Contains(line, "nftables") && !strings.Contains(line, "NFT") {
		return nil
	}

	parsed := map[string]any{
		"action": "drop",
	}

	if m := reSrcIP.FindStringSubmatch(line); m != nil {
		parsed["src_ip"] = m[1]
	}
	if m := reDstPort.FindStringSubmatch(line); m != nil {
		parsed["dst_port"] = m[1]
	}
	if m := reProto.FindStringSubmatch(line); m != nil {
		parsed["protocol"] = m[1]
	}

	// Must have at least src_ip to be useful
	if _, ok := parsed["src_ip"]; !ok {
		return nil
	}

	return &internal.LogEntry{
		Timestamp: time.Now(),
		Source:    "nftables",
		Machine:  machine,
		Severity: "warning",
		Message:  truncate(line),
		Parsed:   parsed,
	}
}

// ParseUFW parses a UFW block log line.
// Format: "[UFW BLOCK] IN=eth0 ... SRC=1.2.3.4 DST=5.6.7.8 ... PROTO=TCP DPT=22"
func ParseUFW(line, machine string) *internal.LogEntry {
	if !strings.Contains(line, "UFW") {
		return nil
	}

	parsed := map[string]any{}

	// Extract action: BLOCK, ALLOW, AUDIT, etc.
	if m := reUFWAction.FindStringSubmatch(line); m != nil {
		parsed["action"] = strings.ToLower(m[1])
	} else {
		parsed["action"] = "block"
	}

	if m := reSrcIP.FindStringSubmatch(line); m != nil {
		parsed["src_ip"] = m[1]
	}
	if m := reDstPort.FindStringSubmatch(line); m != nil {
		parsed["dst_port"] = m[1]
	}
	if m := reProto.FindStringSubmatch(line); m != nil {
		parsed["protocol"] = m[1]
	}

	if _, ok := parsed["src_ip"]; !ok {
		return nil
	}

	severity := "warning"
	if parsed["action"] == "allow" {
		severity = "info"
	}

	return &internal.LogEntry{
		Timestamp: time.Now(),
		Source:    "ufw",
		Machine:  machine,
		Severity: severity,
		Message:  truncate(line),
		Parsed:   parsed,
	}
}

// ParseDockerLog parses a Docker container log line.
// Only keeps stderr lines or lines containing error/warn/fatal/panic keywords.
func ParseDockerLog(line, container, image, stream, machine string) *internal.LogEntry {
	isError := stream == "stderr" || reDockerError.MatchString(strings.ToLower(line))
	if !isError {
		return nil
	}

	severity := "warning"
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "fatal") || strings.Contains(lower, "panic"):
		severity = "critical"
	case strings.Contains(lower, "error"):
		severity = "error"
	}

	return &internal.LogEntry{
		Timestamp: time.Now(),
		Source:    "docker",
		Machine:  machine,
		Severity: severity,
		Message:  truncate(line),
		Parsed: map[string]any{
			"container": container,
			"image":     image,
			"stream":    stream,
		},
	}
}

// ParseCrowdSecDecision parses a CrowdSec decision (ban) from LAPI JSON.
func ParseCrowdSecDecision(ip, scenario, duration, scope, machine string) *internal.LogEntry {
	return &internal.LogEntry{
		Timestamp: time.Now(),
		Source:    "crowdsec",
		Machine:  machine,
		Severity: "warning",
		Message:  truncate("CrowdSec ban: " + ip + " (" + scenario + ") for " + duration),
		Parsed: map[string]any{
			"ip":       ip,
			"scenario": scenario,
			"duration": duration,
			"scope":    scope,
		},
	}
}

func truncate(s string) string {
	if len(s) > maxMessageLen {
		return s[:maxMessageLen]
	}
	return s
}

// Compiled regexes for parsing
var (
	// SSH patterns
	reSSHFailed      = regexp.MustCompile(`Failed password for (\S+) from (\S+) port (\d+)`)
	reSSHInvalidUser = regexp.MustCompile(`Invalid user (\S+) from (\S+) port (\d+)`)
	reSSHAccepted    = regexp.MustCompile(`Accepted (\S+) for (\S+) from (\S+) port (\d+)`)
	reSSHDisconnected = regexp.MustCompile(`Disconnected from (\S+) port (\d+)`)

	// Firewall patterns (shared between nftables and UFW)
	reSrcIP  = regexp.MustCompile(`SRC=(\S+)`)
	reDstPort = regexp.MustCompile(`DPT=(\d+)`)
	reProto  = regexp.MustCompile(`PROTO=(\S+)`)

	// UFW specific
	reUFWAction = regexp.MustCompile(`\[UFW (\w+)\]`)

	// Docker error keywords
	reDockerError = regexp.MustCompile(`\b(error|warn|warning|fatal|panic)\b`)
)
