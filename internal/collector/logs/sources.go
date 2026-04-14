package logs

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ldesfontaine/bientot/internal"
)

// Source lit les entrées de log depuis un backend spécifique.
type Source interface {
	// Name return l'identifiant de la source (ssh, nftables, ufw, docker, crowdsec)
	Name() string
	// Available vérifie si cette source est accessible sur la machine courante
	Available() bool
	// Collect lit les nouvelles entrées depuis le dernier appel
	Collect(ctx context.Context, machine string) ([]internal.LogEntry, error)
}

// --- Journald source (SSH + nftables/UFW via journalctl) ---

// JournaldSource reads from journald via journalctl process invocation.
// Uses --since to read only entries since the last collect.
type JournaldSource struct {
	name     string
	unit     string   // systemd unit filter (e.g. "ssh", "sshd")
	syslogID string   // syslog identifier filter (e.g. "nftables", "UFW")
	parser   func(line, machine string) *internal.LogEntry
	lastRead time.Time
	mu       sync.Mutex
}

func NewJournaldSSHSource() *JournaldSource {
	return &JournaldSource{
		name:     "ssh",
		unit:     "ssh",
		parser:   ParseSSH,
		lastRead: time.Now(),
	}
}

func NewJournaldNftablesSource() *JournaldSource {
	return &JournaldSource{
		name:     "nftables",
		syslogID: "nftables",
		parser:   ParseNftables,
		lastRead: time.Now(),
	}
}

func NewJournaldUFWSource() *JournaldSource {
	return &JournaldSource{
		name:     "ufw",
		syslogID: "UFW",
		parser:   ParseUFW,
		lastRead: time.Now(),
	}
}

func (s *JournaldSource) Name() string { return s.name }

func (s *JournaldSource) Available() bool {
	_, err := exec.LookPath("journalctl")
	if err != nil {
		return false
	}

	// Sonde rapide : peut-on lire quelque chose ?
	args := s.buildArgs("1min ago")
	args = append(args, "-n", "0") // just check access, no output
	cmd := exec.Command("journalctl", args...)
	return cmd.Run() == nil
}

func (s *JournaldSource) Collect(ctx context.Context, machine string) ([]internal.LogEntry, error) {
	s.mu.Lock()
	since := s.lastRead
	s.lastRead = time.Now()
	s.mu.Unlock()

	sinceStr := since.Format("2006-01-02 15:04:05")
	args := s.buildArgs(sinceStr)
	args = append(args, "--no-pager", "-o", "short")

	cmd := exec.CommandContext(ctx, "journalctl", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("journalctl %s: %w", s.name, err)
	}

	var entries []internal.LogEntry
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		entry := s.parser(line, machine)
		if entry != nil {
			entries = append(entries, *entry)
		}
	}

	return entries, nil
}

func (s *JournaldSource) buildArgs(since string) []string {
	args := []string{"--since", since}
	if s.unit != "" {
		// Try both "ssh" and "sshd" unit names
		args = append(args, "-u", s.unit)
	}
	if s.syslogID != "" {
		args = append(args, "-t", s.syslogID)
	}
	return args
}

// --- File source (UFW fallback via /var/log/ufw.log) ---

// FileSource reads from a log file, keeping track of position.
type FileSource struct {
	name   string
	path   string
	parser func(line, machine string) *internal.LogEntry
	offset int64
	mu     sync.Mutex
}

func NewFileUFWSource() *FileSource {
	return &FileSource{
		name:   "ufw",
		path:   "/var/log/ufw.log",
		parser: ParseUFW,
	}
}

func (s *FileSource) Name() string { return s.name }

func (s *FileSource) Available() bool {
	_, err := os.Stat(s.path)
	return err == nil
}

func (s *FileSource) Collect(ctx context.Context, machine string) ([]internal.LogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.path)
	if err != nil {
		return nil, fmt.Errorf("ouverture de %s: %w", s.path, err)
	}
	defer f.Close()

	// Vérification si le fichier a été tronqué (rotation de logs)
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", s.path, err)
	}
	if info.Size() < s.offset {
		s.offset = 0
	}

	if s.offset > 0 {
		if _, err := f.Seek(s.offset, io.SeekStart); err != nil {
			return nil, fmt.Errorf("positionnement dans %s: %w", s.path, err)
		}
	}

	var entries []internal.LogEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		entry := s.parser(line, machine)
		if entry != nil {
			entries = append(entries, *entry)
		}
	}

	// Mise à jour de l'offset
	pos, _ := f.Seek(0, io.SeekCurrent)
	s.offset = pos

	return entries, nil
}

// --- Docker source (container logs via Docker API) ---

// DockerSource reads container logs via Docker API over unix socket or TCP.
type DockerSource struct {
	client     *http.Client
	socketPath string
	lastRead   time.Time
	mu         sync.Mutex
}

func NewDockerSource(socketPath string) *DockerSource {
	var client *http.Client

	if strings.HasPrefix(socketPath, "tcp://") {
		client = &http.Client{Timeout: 15 * time.Second}
	} else {
		transport := &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		}
		client = &http.Client{Transport: transport, Timeout: 15 * time.Second}
	}

	return &DockerSource{
		client:     client,
		socketPath: socketPath,
		lastRead:   time.Now(),
	}
}

func (s *DockerSource) Name() string { return "docker" }

func (s *DockerSource) Available() bool {
	url := s.baseURL() + "/containers/json?limit=1"
	resp, err := s.client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (s *DockerSource) Collect(ctx context.Context, machine string) ([]internal.LogEntry, error) {
	s.mu.Lock()
	since := s.lastRead
	s.lastRead = time.Now()
	s.mu.Unlock()

	// Liste des conteneurs actifs
	url := s.baseURL() + "/containers/json"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("création de la requête: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listage des conteneurs: %w", err)
	}
	defer resp.Body.Close()

	var containers []struct {
		ID    string   `json:"Id"`
		Names []string `json:"Names"`
		Image string   `json:"Image"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, fmt.Errorf("décodage des conteneurs: %w", err)
	}

	var entries []internal.LogEntry
	sinceUnix := fmt.Sprintf("%d", since.Unix())

	for _, c := range containers {
		name := c.Names[0]
		if len(name) > 0 && name[0] == '/' {
			name = name[1:]
		}

		logEntries, err := s.collectContainerLogs(ctx, c.ID, name, c.Image, sinceUnix, machine)
		if err != nil {
			continue // skip failed containers
		}
		entries = append(entries, logEntries...)
	}

	return entries, nil
}

func (s *DockerSource) collectContainerLogs(ctx context.Context, id, name, image, since, machine string) ([]internal.LogEntry, error) {
	url := fmt.Sprintf("%s/containers/%s/logs?stdout=1&stderr=1&since=%s&timestamps=1", s.baseURL(), id, since)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var entries []internal.LogEntry
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		raw := scanner.Bytes()
		// Flux multiplexé Docker : les 8 premiers octets sont l'en-tête
		// octet 0 : type de flux (1=stdout, 2=stderr)
		// octets 4-7 : taille du payload (big-endian)
		stream := "stdout"
		line := scanner.Text()

		if len(raw) > 8 {
			if raw[0] == 2 {
				stream = "stderr"
			}
			line = string(raw[8:])
		}

		entry := ParseDockerLog(line, name, image, stream, machine)
		if entry != nil {
			entries = append(entries, *entry)
		}
	}

	return entries, nil
}

func (s *DockerSource) baseURL() string {
	if strings.HasPrefix(s.socketPath, "tcp://") {
		return "http://" + strings.TrimPrefix(s.socketPath, "tcp://")
	}
	return "http://localhost"
}

// --- CrowdSec source (bans from LAPI) ---

// CrowdSecSource reads active decisions from CrowdSec LAPI.
type CrowdSecSource struct {
	url      string
	client   *http.Client
	knownIPs map[string]bool // suivi des bans déjà rapportés pour éviter les doublons
	mu       sync.Mutex
}

func NewCrowdSecSource(url string) *CrowdSecSource {
	return &CrowdSecSource{
		url:      strings.TrimRight(url, "/"),
		client:   &http.Client{Timeout: 10 * time.Second},
		knownIPs: make(map[string]bool),
	}
}

func (s *CrowdSecSource) Name() string { return "crowdsec" }

func (s *CrowdSecSource) Available() bool {
	resp, err := s.client.Get(s.url + "/v1/decisions")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (s *CrowdSecSource) Collect(ctx context.Context, machine string) ([]internal.LogEntry, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.url+"/v1/decisions", nil)
	if err != nil {
		return nil, fmt.Errorf("création de la requête: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("récupération des décisions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("statut inattendu: %d", resp.StatusCode)
	}

	var decisions []struct {
		Value    string `json:"value"`    // IP
		Scenario string `json:"scenario"`
		Duration string `json:"duration"`
		Scope    string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decisions); err != nil {
		// CrowdSec return null au lieu de [] quand c'est vide
		return nil, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var entries []internal.LogEntry
	currentIPs := make(map[string]bool)

	for _, d := range decisions {
		key := d.Value + "|" + d.Scenario
		currentIPs[key] = true

		// Ne rapporter que les nouveaux bans
		if !s.knownIPs[key] {
			entry := ParseCrowdSecDecision(d.Value, d.Scenario, d.Duration, d.Scope, machine)
			if entry != nil {
				entries = append(entries, *entry)
			}
		}
	}

	s.knownIPs = currentIPs
	return entries, nil
}
