package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ldesfontaine/bientot/internal"
)

// --- Scan JSON structures (Grype + Trivy rétrocompatibilité) ---

type scanIngestRequest struct {
	Machine string          `json:"machine"`
	Image   string          `json:"image"`
	Results json.RawMessage `json:"results"`
}

// Grype format
type grypeOutput struct {
	Matches []grypeMatch `json:"matches"`
}

type grypeMatch struct {
	Vulnerability grypeVuln    `json:"vulnerability"`
	Artifact      grypeArtifact `json:"artifact"`
}

type grypeVuln struct {
	ID        string        `json:"id"`
	Severity  string        `json:"severity"`
	Fix       grypeVulnFix  `json:"fix"`
	URLs      []string      `json:"urls"`
	Cvss      []grypeVulnCvss `json:"cvss"`
}

type grypeVulnFix struct {
	Versions []string `json:"versions"`
	State    string   `json:"state"`
}

type grypeVulnCvss struct {
	Metrics grypeVulnCvssMetrics `json:"metrics"`
}

type grypeVulnCvssMetrics struct {
	BaseScore float64 `json:"baseScore"`
}

type grypeArtifact struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Trivy format (rétrocompatibilité)
type trivyOutput struct {
	Results []trivyResult `json:"Results"`
}

type trivyResult struct {
	Target          string      `json:"Target"`
	Vulnerabilities []trivyVuln `json:"Vulnerabilities"`
}

type trivyVuln struct {
	VulnerabilityID  string              `json:"VulnerabilityID"`
	PkgName          string              `json:"PkgName"`
	InstalledVersion string              `json:"InstalledVersion"`
	FixedVersion     string              `json:"FixedVersion"`
	Severity         string              `json:"Severity"`
	Title            string              `json:"Title"`
	PrimaryURL       string              `json:"PrimaryURL"`
	CVSS             map[string]cvssData `json:"CVSS"`
}

type cvssData struct {
	V3Score float64 `json:"V3Score"`
}

// --- Staleness tracking ---

type scanTracker struct {
	mu       sync.Mutex
	lastScan map[string]time.Time
}

func newScanTracker() *scanTracker {
	return &scanTracker{lastScan: make(map[string]time.Time)}
}

func (t *scanTracker) touch(machine string) {
	t.mu.Lock()
	t.lastScan[machine] = time.Now()
	t.mu.Unlock()
}

// staleMachines retourne les machines sans scan depuis maxAge.
// Ne retourne rien si aucun scan n'a jamais ete recu (feature non utilisee).
func (t *scanTracker) staleMachines(maxAge time.Duration) []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	var stale []string
	for machine, last := range t.lastScan {
		if time.Since(last) > maxAge {
			stale = append(stale, machine)
		}
	}
	return stale
}

// --- Handler ---

// handleScanIngest recoit les resultats de scan CVE (Grype ou Trivy) du script cron.
// Auth : Bearer token correspondant a un token agent configure.
// Detecte automatiquement le format par la structure JSON :
//   - Grype : { "matches": [...] }
//   - Trivy : { "Results": [...] }
func (s *Server) handleScanIngest(w http.ResponseWriter, r *http.Request) {
	tokenMachine, ok := s.validateBearerToken(r)
	if !ok {
		http.Error(w, "non autorise", http.StatusUnauthorized)
		return
	}

	if s.enrichStore == nil {
		http.Error(w, "enrichissement non configure", http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10 MB max
	if err != nil {
		http.Error(w, "erreur de lecture", http.StatusBadRequest)
		return
	}

	var req scanIngestRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "JSON invalide", http.StatusBadRequest)
		return
	}

	if req.Machine != tokenMachine {
		s.logger.Warn("scan ingest: machine_id mismatch",
			"token_machine", tokenMachine, "request_machine", req.Machine)
		http.Error(w, "machine_id ne correspond pas au token", http.StatusForbidden)
		return
	}

	if req.Image == "" {
		http.Error(w, "champ image requis", http.StatusBadRequest)
		return
	}

	// Detection du format par la structure JSON
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(req.Results, &probe); err != nil {
		http.Error(w, "JSON scan invalide", http.StatusBadRequest)
		return
	}

	var matches []*internal.VulnMatch
	var totalVulns int
	var scanner string
	imageName, _ := parseImageTag(req.Image)

	if _, hasMatches := probe["matches"]; hasMatches {
		// Format Grype
		scanner = "grype"
		matches, totalVulns = s.parseGrypeResults(req.Results, imageName, req.Machine)
	} else if _, hasResults := probe["Results"]; hasResults {
		// Format Trivy (rétrocompatibilité)
		scanner = "trivy"
		matches, totalVulns = s.parseTrivyResults(req.Results, imageName, req.Machine)
	} else {
		http.Error(w, "format scan non reconnu (attendu: Grype ou Trivy)", http.StatusBadRequest)
		return
	}

	var matchCount int
	for _, match := range matches {
		if err := s.enrichStore.UpsertVulnMatch(r.Context(), match); err != nil {
			s.logger.Warn("scan: echec upsert vuln_match",
				"cve", match.CVEID, "error", err)
			continue
		}
		matchCount++

		s.sse.Publish(SSEEvent{
			Type: "vuln_match",
			Data: match,
		})
	}

	s.scanTracker.touch(req.Machine)
	s.resetScanStaleAlert(req.Machine)

	status := fmt.Sprintf("%s-ok", scanner)
	if matchCount > 0 {
		status = fmt.Sprintf("%s-%d-vulns", scanner, matchCount)
	}
	if err := s.enrichStore.InsertSyncLog(r.Context(), totalVulns, matchCount, status); err != nil {
		s.logger.Warn("scan: echec insertion sync_log", "error", err)
	}

	s.logger.Info("scan ingest",
		"scanner", scanner,
		"machine", req.Machine,
		"image", req.Image,
		"vulns_received", totalVulns,
		"matches_created", matchCount,
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"scanner": scanner,
		"matches": matchCount,
	})
}

// parseGrypeResults extrait les VulnMatch depuis un JSON Grype.
func (s *Server) parseGrypeResults(raw json.RawMessage, imageName, machine string) ([]*internal.VulnMatch, int) {
	var grypeOut grypeOutput
	if err := json.Unmarshal(raw, &grypeOut); err != nil {
		s.logger.Warn("scan: echec parsing Grype", "error", err)
		return nil, 0
	}

	var matches []*internal.VulnMatch
	for _, m := range grypeOut.Matches {
		if m.Vulnerability.ID == "" {
			continue
		}

		fixedVersion := ""
		if len(m.Vulnerability.Fix.Versions) > 0 {
			fixedVersion = m.Vulnerability.Fix.Versions[0]
		}

		link := ""
		if len(m.Vulnerability.URLs) > 0 {
			link = m.Vulnerability.URLs[0]
		}

		title := m.Vulnerability.ID
		if fixedVersion != "" {
			title = fmt.Sprintf("%s (fix: %s)", m.Vulnerability.ID, fixedVersion)
		}

		match := &internal.VulnMatch{
			CVEID:            m.Vulnerability.ID,
			Severity:         strings.ToLower(m.Vulnerability.Severity),
			CVSSScore:        bestGrypeCVSS(m.Vulnerability.Cvss),
			Title:            title,
			Link:             link,
			MatchedSoftware:  imageName + "/" + m.Artifact.Name,
			Machine:          machine,
			InstalledVersion: m.Artifact.Version,
			Confidence:       "confirmed",
			FirstSeen:        time.Now(),
		}
		matches = append(matches, match)
	}

	return matches, len(grypeOut.Matches)
}

// parseTrivyResults extrait les VulnMatch depuis un JSON Trivy (rétrocompatibilité).
func (s *Server) parseTrivyResults(raw json.RawMessage, imageName, machine string) ([]*internal.VulnMatch, int) {
	var trivyOut trivyOutput
	if err := json.Unmarshal(raw, &trivyOut); err != nil {
		s.logger.Warn("scan: echec parsing Trivy", "error", err)
		return nil, 0
	}

	var matches []*internal.VulnMatch
	var totalVulns int
	for _, result := range trivyOut.Results {
		if result.Vulnerabilities == nil {
			continue
		}
		totalVulns += len(result.Vulnerabilities)
		for _, vuln := range result.Vulnerabilities {
			if vuln.VulnerabilityID == "" {
				continue
			}

			match := &internal.VulnMatch{
				CVEID:            vuln.VulnerabilityID,
				Severity:         strings.ToLower(vuln.Severity),
				CVSSScore:        bestCVSSScore(vuln.CVSS),
				Title:            vuln.Title,
				Link:             vuln.PrimaryURL,
				MatchedSoftware:  imageName + "/" + vuln.PkgName,
				Machine:          machine,
				InstalledVersion: vuln.InstalledVersion,
				Confidence:       "confirmed",
				FirstSeen:        time.Now(),
			}
			matches = append(matches, match)
		}
	}

	return matches, totalVulns
}

func bestCVSSScore(cvss map[string]cvssData) float64 {
	var best float64
	for _, data := range cvss {
		if data.V3Score > best {
			best = data.V3Score
		}
	}
	return best
}

func bestGrypeCVSS(cvss []grypeVulnCvss) float64 {
	var best float64
	for _, entry := range cvss {
		if entry.Metrics.BaseScore > best {
			best = entry.Metrics.BaseScore
		}
	}
	return best
}

// checkScanStaleness verifie si un scan CVE n'a pas ete recu depuis trop longtemps.
// Appele periodiquement par runAlertLoop.
func (s *Server) checkScanStaleness() {
	const maxAge = 8 * 24 * time.Hour // 8 jours (le cron tourne chaque dimanche)

	stale := s.scanTracker.staleMachines(maxAge)
	if len(stale) == 0 {
		return
	}

	for _, machine := range stale {
		alertName := fmt.Sprintf("scan_stale_%s", machine)

		s.scanStaleMu.Lock()
		if s.scanStaleAlerted[alertName] {
			s.scanStaleMu.Unlock()
			continue
		}
		s.scanStaleAlerted[alertName] = true
		s.scanStaleMu.Unlock()

		alert := internal.Alert{
			ID:       alertName,
			Name:     "Scan CVE manquant",
			Severity: internal.SeverityWarning,
			Message:  fmt.Sprintf("Aucun scan CVE recu depuis 8 jours sur %s", machine),
			Labels:   map[string]string{"machine_id": machine},
			FiredAt:  time.Now(),
		}

		s.sse.Publish(SSEEvent{Type: "alert_fired", Data: alert})

		if s.alerter != nil {
			s.alerter.FireManual(alert)
		}

		s.logger.Warn("scan CVE stale", "machine", machine)
	}
}

// validateBearerToken extrait et valide un Bearer token.
// Retourne le machine_id associe au token.
func (s *Server) validateBearerToken(r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", false
	}
	token := strings.TrimPrefix(auth, "Bearer ")

	for machineID, t := range s.tokens {
		if t == token {
			return machineID, true
		}
	}
	return "", false
}

// resetScanStaleAlert reinitialise l'alerte de staleness quand un scan est recu.
func (s *Server) resetScanStaleAlert(machine string) {
	alertName := fmt.Sprintf("scan_stale_%s", machine)
	s.scanStaleMu.Lock()
	delete(s.scanStaleAlerted, alertName)
	s.scanStaleMu.Unlock()
}
