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

// --- Trivy JSON structures ---

type trivyIngestRequest struct {
	Machine string          `json:"machine"`
	Image   string          `json:"image"`
	Results json.RawMessage `json:"results"`
}

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

type trivyScanTracker struct {
	mu       sync.Mutex
	lastScan map[string]time.Time
}

func newTrivyScanTracker() *trivyScanTracker {
	return &trivyScanTracker{lastScan: make(map[string]time.Time)}
}

func (t *trivyScanTracker) touch(machine string) {
	t.mu.Lock()
	t.lastScan[machine] = time.Now()
	t.mu.Unlock()
}

// staleMachines retourne les machines sans scan depuis maxAge.
// Ne retourne rien si aucun scan n'a jamais ete recu (feature non utilisee).
func (t *trivyScanTracker) staleMachines(maxAge time.Duration) []string {
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

// handleTrivyIngest recoit les resultats de scan Trivy du script cron.
// Auth : Bearer token correspondant a un token agent configure.
func (s *Server) handleTrivyIngest(w http.ResponseWriter, r *http.Request) {
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

	var req trivyIngestRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "JSON invalide", http.StatusBadRequest)
		return
	}

	if req.Machine != tokenMachine {
		s.logger.Warn("trivy ingest: machine_id mismatch",
			"token_machine", tokenMachine, "request_machine", req.Machine)
		http.Error(w, "machine_id ne correspond pas au token", http.StatusForbidden)
		return
	}

	if req.Image == "" {
		http.Error(w, "champ image requis", http.StatusBadRequest)
		return
	}

	var trivyOut trivyOutput
	if err := json.Unmarshal(req.Results, &trivyOut); err != nil {
		http.Error(w, "JSON Trivy invalide", http.StatusBadRequest)
		return
	}

	imageName, _ := parseImageTag(req.Image)

	var matchCount int
	for _, result := range trivyOut.Results {
		if result.Vulnerabilities == nil {
			continue
		}
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
				Machine:          req.Machine,
				InstalledVersion: vuln.InstalledVersion,
				Confidence:       "confirmed",
				FirstSeen:        time.Now(),
			}

			if err := s.enrichStore.UpsertVulnMatch(r.Context(), match); err != nil {
				s.logger.Warn("trivy: echec upsert vuln_match",
					"cve", vuln.VulnerabilityID, "error", err)
				continue
			}
			matchCount++

			s.sse.Publish(SSEEvent{
				Type: "vuln_match",
				Data: match,
			})
		}
	}

	s.trivyTracker.touch(req.Machine)
	s.resetTrivyStaleAlert(req.Machine)

	status := "trivy-ok"
	if matchCount > 0 {
		status = fmt.Sprintf("trivy-%d-vulns", matchCount)
	}
	if err := s.enrichStore.InsertSyncLog(r.Context(), countAllVulns(trivyOut), matchCount, status); err != nil {
		s.logger.Warn("trivy: echec insertion sync_log", "error", err)
	}

	s.logger.Info("trivy ingest",
		"machine", req.Machine,
		"image", req.Image,
		"vulns_received", countAllVulns(trivyOut),
		"matches_created", matchCount,
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"matches": matchCount,
	})
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

func bestCVSSScore(cvss map[string]cvssData) float64 {
	var best float64
	for _, data := range cvss {
		if data.V3Score > best {
			best = data.V3Score
		}
	}
	return best
}

func countAllVulns(out trivyOutput) int {
	var count int
	for _, r := range out.Results {
		count += len(r.Vulnerabilities)
	}
	return count
}

// checkTrivyStaleness verifie si un scan Trivy n'a pas ete recu depuis trop longtemps.
// Appele periodiquement par runAlertLoop.
func (s *Server) checkTrivyStaleness() {
	const maxAge = 8 * 24 * time.Hour // 8 jours (le cron tourne chaque dimanche)

	stale := s.trivyTracker.staleMachines(maxAge)
	if len(stale) == 0 {
		return
	}

	for _, machine := range stale {
		alertName := fmt.Sprintf("trivy_stale_%s", machine)

		s.trivyStaleMu.Lock()
		if s.trivyStaleAlerted[alertName] {
			s.trivyStaleMu.Unlock()
			continue
		}
		s.trivyStaleAlerted[alertName] = true
		s.trivyStaleMu.Unlock()

		alert := internal.Alert{
			ID:       alertName,
			Name:     "Trivy scan manquant",
			Severity: internal.SeverityWarning,
			Message:  fmt.Sprintf("Aucun scan Trivy recu depuis 8 jours sur %s", machine),
			Labels:   map[string]string{"machine_id": machine},
			FiredAt:  time.Now(),
		}

		s.sse.Publish(SSEEvent{Type: "alert_fired", Data: alert})

		if s.alerter != nil {
			s.alerter.FireManual(alert)
		}

		s.logger.Warn("trivy scan stale", "machine", machine)
	}
}

// resetTrivyStaleAlert reinitialise l'alerte de staleness quand un scan est recu.
func (s *Server) resetTrivyStaleAlert(machine string) {
	alertName := fmt.Sprintf("trivy_stale_%s", machine)
	s.trivyStaleMu.Lock()
	delete(s.trivyStaleAlerted, alertName)
	s.trivyStaleMu.Unlock()
}
