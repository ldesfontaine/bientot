package enrichment

import (
	"fmt"
	"net"
	"sync"

	"github.com/oschwald/maxminddb-golang"
)

// GeoIP performs lookups against a MaxMind MMDB database.
type GeoIP struct {
	mu sync.RWMutex
	db *maxminddb.Reader
}

// maxmindRecord matches the GeoLite2-City MMDB schema.
type maxmindRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	Location struct {
		Latitude  float64 `maxminddb:"latitude"`
		Longitude float64 `maxminddb:"longitude"`
	} `maxminddb:"location"`
	Traits struct {
		ASN              int    `maxminddb:"autonomous_system_number"`
		ISP              string `maxminddb:"autonomous_system_organization"`
	} `maxminddb:"traits"`
}

// NewGeoIP opens a MaxMind MMDB file.
func NewGeoIP(dbPath string) (*GeoIP, error) {
	db, err := maxminddb.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening geoip db: %w", err)
	}
	return &GeoIP{db: db}, nil
}

// Lookup resolves geo data for an IP string.
func (g *GeoIP) Lookup(ipStr string) (*GeoResult, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP: %s", ipStr)
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	var record maxmindRecord
	if err := g.db.Lookup(ip, &record); err != nil {
		return nil, fmt.Errorf("geoip lookup: %w", err)
	}

	city := ""
	if name, ok := record.City.Names["en"]; ok {
		city = name
	}

	return &GeoResult{
		Country: record.Country.ISOCode,
		City:    city,
		Lat:     record.Location.Latitude,
		Lon:     record.Location.Longitude,
		ASN:     record.Traits.ASN,
		ISP:     record.Traits.ISP,
	}, nil
}

// Close releases the MMDB reader.
func (g *GeoIP) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.db.Close()
}
