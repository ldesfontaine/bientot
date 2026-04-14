package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ldesfontaine/bientot/internal/enrichment"
)

// GreyNoise interroge l'API communautaire GreyNoise.
type GreyNoise struct {
	apiKey     string
	dailyLimit int
	client     *http.Client
}

type greyNoiseResponse struct {
	IP             string `json:"ip"`
	Noise          bool   `json:"noise"`
	Riot           bool   `json:"riot"`
	Classification string `json:"classification"` // bénin, malveillant, inconnu
	Name           string `json:"name"`
	Link           string `json:"link"`
}

// NewGreyNoise crée un fournisseur GreyNoise.
func NewGreyNoise(apiKey string, dailyLimit int) *GreyNoise {
	return &GreyNoise{
		apiKey:     apiKey,
		dailyLimit: dailyLimit,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (g *GreyNoise) Name() string    { return "greynoise" }
func (g *GreyNoise) DailyLimit() int { return g.dailyLimit }

func (g *GreyNoise) Enrich(ip string) (*enrichment.ProviderResult, error) {
	url := fmt.Sprintf("https://api.greynoise.io/v3/community/%s", ip)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("création de la requête: %w", err)
	}

	req.Header.Set("key", g.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requête GreyNoise: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GreyNoise: statut %d", resp.StatusCode)
	}

	var result greyNoiseResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("décodage de la réponse GreyNoise: %w", err)
	}

	score := 0
	switch result.Classification {
	case "malicious":
		score = 80
	case "unknown":
		score = 40
	case "benign":
		score = 0
	}

	return &enrichment.ProviderResult{
		Source: "greynoise",
		Score:  score,
		Data: map[string]string{
			"classification": result.Classification,
			"name":           result.Name,
			"noise":          fmt.Sprintf("%t", result.Noise),
			"riot":           fmt.Sprintf("%t", result.Riot),
		},
	}, nil
}
