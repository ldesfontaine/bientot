package enrichment

import (
	"fmt"
	"net"
	"sync"

	"github.com/oschwald/maxminddb-golang"
)

// GeoIP effectue des recherches dans une base de données MaxMind MMDB.
type GeoIP struct {
	mu sync.RWMutex
	db *maxminddb.Reader
}

// maxmindRecord correspond au schéma MMDB GeoLite2-City.
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

// NewGeoIP ouvre un fichier MaxMind MMDB.
func NewGeoIP(dbPath string) (*GeoIP, error) {
	db, err := maxminddb.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("ouverture de la base GeoIP: %w", err)
	}
	return &GeoIP{db: db}, nil
}

// Lookup résout les données géo pour une IP.
func (g *GeoIP) Lookup(ipStr string) (*GeoResult, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("IP invalide: %s", ipStr)
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	var record maxmindRecord
	if err := g.db.Lookup(ip, &record); err != nil {
		return nil, fmt.Errorf("recherche GeoIP: %w", err)
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

// Close libère le lecteur MMDB.
func (g *GeoIP) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.db.Close()
}
