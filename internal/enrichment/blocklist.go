package enrichment

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// BlocklistSource defines a blocklist feed.
type BlocklistSource struct {
	Name   string `yaml:"name"`
	URL    string `yaml:"url"`
	Format string `yaml:"format"` // "ip-per-line" or "cidr-per-line"
}

// BlocklistChecker checks IPs against downloaded blocklists.
type BlocklistChecker struct {
	mu      sync.RWMutex
	ips     map[string][]string // ip -> list names
	nets    []*netEntry         // CIDR ranges
	sources []BlocklistSource
	client  *http.Client
	logger  *slog.Logger
}

type netEntry struct {
	net  *net.IPNet
	name string
}

// NewBlocklistChecker creates a checker and downloads all lists.
func NewBlocklistChecker(sources []BlocklistSource, logger *slog.Logger) *BlocklistChecker {
	bc := &BlocklistChecker{
		ips:     make(map[string][]string),
		sources: sources,
		client:  &http.Client{Timeout: 60 * time.Second},
		logger:  logger,
	}
	return bc
}

// Load downloads and parses all configured blocklists.
func (bc *BlocklistChecker) Load() error {
	newIPs := make(map[string][]string)
	var newNets []*netEntry

	for _, src := range bc.sources {
		ips, nets, err := bc.download(src)
		if err != nil {
			bc.logger.Warn("blocklist download failed", "name", src.Name, "error", err)
			continue
		}

		for _, ip := range ips {
			newIPs[ip] = append(newIPs[ip], src.Name)
		}
		for _, n := range nets {
			newNets = append(newNets, &netEntry{net: n, name: src.Name})
		}

		bc.logger.Info("blocklist loaded", "name", src.Name, "ips", len(ips), "cidrs", len(nets))
	}

	bc.mu.Lock()
	bc.ips = newIPs
	bc.nets = newNets
	bc.mu.Unlock()

	return nil
}

// Check returns the list of blocklists that contain this IP.
func (bc *BlocklistChecker) Check(ipStr string) []string {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	var matched []string

	// Exact match
	if lists, ok := bc.ips[ipStr]; ok {
		matched = append(matched, lists...)
	}

	// CIDR match
	ip := net.ParseIP(ipStr)
	if ip != nil {
		for _, entry := range bc.nets {
			if entry.net.Contains(ip) {
				matched = append(matched, entry.name)
			}
		}
	}

	return matched
}

// Count returns total unique IPs + CIDR entries loaded.
func (bc *BlocklistChecker) Count() int {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return len(bc.ips) + len(bc.nets)
}

// StartAutoRefresh refreshes blocklists on the given interval.
func (bc *BlocklistChecker) StartAutoRefresh(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := bc.Load(); err != nil {
				bc.logger.Error("blocklist refresh failed", "error", err)
			}
		}
	}
}

func (bc *BlocklistChecker) download(src BlocklistSource) ([]string, []*net.IPNet, error) {
	resp, err := bc.client.Get(src.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching %s: %w", src.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("fetching %s: status %d", src.Name, resp.StatusCode)
	}

	return bc.parse(resp.Body, src.Format)
}

func (bc *BlocklistChecker) parse(r io.Reader, format string) ([]string, []*net.IPNet, error) {
	var ips []string
	var nets []*net.IPNet

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		switch format {
		case "cidr-per-line":
			if strings.Contains(line, "/") {
				_, ipNet, err := net.ParseCIDR(line)
				if err == nil {
					nets = append(nets, ipNet)
				}
			} else if ip := net.ParseIP(line); ip != nil {
				ips = append(ips, ip.String())
			}
		default: // ip-per-line
			if ip := net.ParseIP(line); ip != nil {
				ips = append(ips, ip.String())
			}
		}
	}

	return ips, nets, scanner.Err()
}
