package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ldesfontaine/bientot/internal/enrichment"
)

// AbuseIPDB queries the AbuseIPDB v2 API.
type AbuseIPDB struct {
	apiKey     string
	dailyLimit int
	client     *http.Client
}

type abuseIPDBResponse struct {
	Data struct {
		AbuseConfidenceScore int    `json:"abuseConfidenceScore"`
		CountryCode          string `json:"countryCode"`
		ISP                  string `json:"isp"`
		TotalReports         int    `json:"totalReports"`
		UsageType            string `json:"usageType"`
	} `json:"data"`
}

// NewAbuseIPDB creates an AbuseIPDB provider.
func NewAbuseIPDB(apiKey string, dailyLimit int) *AbuseIPDB {
	return &AbuseIPDB{
		apiKey:     apiKey,
		dailyLimit: dailyLimit,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (a *AbuseIPDB) Name() string    { return "abuseipdb" }
func (a *AbuseIPDB) DailyLimit() int { return a.dailyLimit }

func (a *AbuseIPDB) Enrich(ip string) (*enrichment.ProviderResult, error) {
	req, err := http.NewRequest("GET", "https://api.abuseipdb.com/api/v2/check", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	q := req.URL.Query()
	q.Set("ipAddress", ip)
	q.Set("maxAgeInDays", "90")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Key", a.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("abuseipdb request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("abuseipdb: status %d", resp.StatusCode)
	}

	var result abuseIPDBResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding abuseipdb response: %w", err)
	}

	return &enrichment.ProviderResult{
		Source: "abuseipdb",
		Score:  result.Data.AbuseConfidenceScore,
		Data: map[string]string{
			"abuse_score":   strconv.Itoa(result.Data.AbuseConfidenceScore),
			"country":       result.Data.CountryCode,
			"isp":           result.Data.ISP,
			"total_reports": strconv.Itoa(result.Data.TotalReports),
			"usage_type":    result.Data.UsageType,
		},
	}, nil
}
