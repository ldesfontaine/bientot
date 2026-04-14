package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ldesfontaine/bientot/internal/enrichment"
)

// CrowdSecCTI interroge l'API smoke CrowdSec CTI.
type CrowdSecCTI struct {
	apiKey     string
	dailyLimit int
	client     *http.Client
}

type crowdsecCTIResponse struct {
	IP              string `json:"ip"`
	Reputation      string `json:"reputation"` // malveillant, suspect, connu, sûr
	BackgroundNoise bool   `json:"background_noise_score"`
	Confidence      string `json:"confidence"`
	Behaviors       []struct {
		Name  string `json:"name"`
		Label string `json:"label"`
	} `json:"behaviors"`
	AttackDetails []struct {
		Name  string `json:"name"`
		Label string `json:"label"`
	} `json:"attack_details"`
	Scores struct {
		Overall struct {
			Aggressiveness int `json:"aggressiveness"`
			Threat         int `json:"threat"`
			Trust          int `json:"trust"`
			Anomaly        int `json:"anomaly"`
			Total          int `json:"total"`
		} `json:"overall"`
	} `json:"scores"`
}

// NewCrowdSecCTI crée un fournisseur CrowdSec CTI.
func NewCrowdSecCTI(apiKey string, dailyLimit int) *CrowdSecCTI {
	return &CrowdSecCTI{
		apiKey:     apiKey,
		dailyLimit: dailyLimit,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *CrowdSecCTI) Name() string    { return "crowdsec_cti" }
func (c *CrowdSecCTI) DailyLimit() int { return c.dailyLimit }

func (c *CrowdSecCTI) Enrich(ip string) (*enrichment.ProviderResult, error) {
	url := fmt.Sprintf("https://cti.api.crowdsec.net/v2/smoke/%s", ip)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("création de la requête: %w", err)
	}

	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requête CrowdSec CTI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CrowdSec CTI: statut %d", resp.StatusCode)
	}

	var result crowdsecCTIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("décodage de la réponse CrowdSec CTI: %w", err)
	}

	// Conversion de la réputation en score
	score := 0
	switch result.Reputation {
	case "malicious":
		score = 90
	case "suspicious":
		score = 60
	case "known":
		score = 30
	case "safe":
		score = 0
	}

	// Collecte des noms de comportements
	var behaviors []string
	for _, b := range result.Behaviors {
		behaviors = append(behaviors, b.Name)
	}

	return &enrichment.ProviderResult{
		Source: "crowdsec_cti",
		Score:  score,
		Data: map[string]string{
			"reputation":     result.Reputation,
			"confidence":     result.Confidence,
			"behaviors":      strings.Join(behaviors, ","),
			"score_total":    strconv.Itoa(result.Scores.Overall.Total),
			"aggressiveness": strconv.Itoa(result.Scores.Overall.Aggressiveness),
			"threat":         strconv.Itoa(result.Scores.Overall.Threat),
		},
	}, nil
}
