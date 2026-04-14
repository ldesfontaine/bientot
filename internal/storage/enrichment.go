package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ldesfontaine/bientot/internal/enrichment"
)

// UpsertIPIntel insère ou met à jour les données d'intelligence IP.
func (s *SQLiteStorage) UpsertIPIntel(ctx context.Context, intel *enrichment.IPIntel) error {
	blocklists, _ := json.Marshal(intel.BlocklistsMatched)
	sources, _ := json.Marshal(intel.EnrichmentSources)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ip_intel (
			ip, first_seen, last_seen, total_requests,
			country, city, lat, lon, asn, isp,
			blocklists_matched, crowdsec_banned, crowdsec_reason,
			abuse_score, greynoise_class, greynoise_name,
			enrichment_sources, enriched_at, priority_score
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(ip) DO UPDATE SET
			last_seen = excluded.last_seen,
			total_requests = excluded.total_requests,
			country = COALESCE(excluded.country, ip_intel.country),
			city = COALESCE(excluded.city, ip_intel.city),
			lat = COALESCE(excluded.lat, ip_intel.lat),
			lon = COALESCE(excluded.lon, ip_intel.lon),
			asn = COALESCE(excluded.asn, ip_intel.asn),
			isp = COALESCE(excluded.isp, ip_intel.isp),
			blocklists_matched = excluded.blocklists_matched,
			crowdsec_banned = excluded.crowdsec_banned,
			crowdsec_reason = COALESCE(excluded.crowdsec_reason, ip_intel.crowdsec_reason),
			abuse_score = CASE WHEN excluded.abuse_score > 0 THEN excluded.abuse_score ELSE ip_intel.abuse_score END,
			greynoise_class = COALESCE(excluded.greynoise_class, ip_intel.greynoise_class),
			greynoise_name = COALESCE(excluded.greynoise_name, ip_intel.greynoise_name),
			enrichment_sources = excluded.enrichment_sources,
			enriched_at = excluded.enriched_at,
			priority_score = excluded.priority_score
	`,
		intel.IP, intel.FirstSeen, intel.LastSeen, intel.TotalRequests,
		intel.Country, intel.City, intel.Lat, intel.Lon, intel.ASN, intel.ISP,
		string(blocklists), boolToInt(intel.CrowdSecBanned), intel.CrowdSecReason,
		intel.AbuseScore, intel.GreyNoiseClass, intel.GreyNoiseName,
		string(sources), intel.EnrichedAt, intel.PriorityScore,
	)
	if err != nil {
		return fmt.Errorf("upsert ip_intel: %w", err)
	}
	return nil
}

// GetIPIntel récupère les données d'enrichissement pour une IP.
func (s *SQLiteStorage) GetIPIntel(ctx context.Context, ip string) (*enrichment.IPIntel, error) {
	var intel enrichment.IPIntel
	var blocklistsStr, sourcesStr sql.NullString
	var crowdsecBanned int

	err := s.db.QueryRowContext(ctx, `
		SELECT ip, first_seen, last_seen, total_requests,
			country, city, lat, lon, asn, isp,
			blocklists_matched, crowdsec_banned, crowdsec_reason,
			abuse_score, greynoise_class, greynoise_name,
			enrichment_sources, enriched_at, priority_score
		FROM ip_intel WHERE ip = ?
	`, ip).Scan(
		&intel.IP, &intel.FirstSeen, &intel.LastSeen, &intel.TotalRequests,
		&intel.Country, &intel.City, &intel.Lat, &intel.Lon, &intel.ASN, &intel.ISP,
		&blocklistsStr, &crowdsecBanned, &intel.CrowdSecReason,
		&intel.AbuseScore, &intel.GreyNoiseClass, &intel.GreyNoiseName,
		&sourcesStr, &intel.EnrichedAt, &intel.PriorityScore,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("requête ip_intel: %w", err)
	}

	intel.CrowdSecBanned = crowdsecBanned != 0
	if blocklistsStr.Valid {
		json.Unmarshal([]byte(blocklistsStr.String), &intel.BlocklistsMatched)
	}
	if sourcesStr.Valid {
		json.Unmarshal([]byte(sourcesStr.String), &intel.EnrichmentSources)
	}

	return &intel, nil
}

// InsertAttackLog stocke un événement d'attaque agrégé.
func (s *SQLiteStorage) InsertAttackLog(ctx context.Context, log *enrichment.AttackLog) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO attack_log (timestamp, machine, ip, path, status_code, count)
		VALUES (?, ?, ?, ?, ?, ?)
	`, log.Timestamp, log.Machine, log.IP, log.Path, log.Status, log.Count)
	if err != nil {
		return fmt.Errorf("insertion attack_log: %w", err)
	}
	return nil
}

// InsertPattern stocke un pattern de menace détecté.
func (s *SQLiteStorage) InsertPattern(ctx context.Context, pattern *enrichment.Pattern) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO patterns (timestamp, type, severity, detail, resolved)
		VALUES (?, ?, ?, ?, ?)
	`, pattern.Timestamp, pattern.Type, pattern.Severity, pattern.Detail, boolToInt(pattern.Resolved))
	if err != nil {
		return fmt.Errorf("insertion pattern: %w", err)
	}
	return nil
}

// QueryIPIntel return les IPs les plus récemment enrichies.
func (s *SQLiteStorage) QueryIPIntel(ctx context.Context, limit int) ([]enrichment.IPIntel, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT ip, first_seen, last_seen, total_requests,
			country, city, lat, lon, asn, isp,
			blocklists_matched, crowdsec_banned, crowdsec_reason,
			abuse_score, greynoise_class, greynoise_name,
			enrichment_sources, enriched_at, priority_score
		FROM ip_intel ORDER BY last_seen DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("requête ip_intel: %w", err)
	}
	defer rows.Close()

	var results []enrichment.IPIntel
	for rows.Next() {
		var intel enrichment.IPIntel
		var blocklistsStr, sourcesStr sql.NullString
		var crowdsecBanned int

		if err := rows.Scan(
			&intel.IP, &intel.FirstSeen, &intel.LastSeen, &intel.TotalRequests,
			&intel.Country, &intel.City, &intel.Lat, &intel.Lon, &intel.ASN, &intel.ISP,
			&blocklistsStr, &crowdsecBanned, &intel.CrowdSecReason,
			&intel.AbuseScore, &intel.GreyNoiseClass, &intel.GreyNoiseName,
			&sourcesStr, &intel.EnrichedAt, &intel.PriorityScore,
		); err != nil {
			return nil, fmt.Errorf("lecture ip_intel: %w", err)
		}

		intel.CrowdSecBanned = crowdsecBanned != 0
		if blocklistsStr.Valid {
			json.Unmarshal([]byte(blocklistsStr.String), &intel.BlocklistsMatched)
		}
		if sourcesStr.Valid {
			json.Unmarshal([]byte(sourcesStr.String), &intel.EnrichmentSources)
		}

		results = append(results, intel)
	}

	return results, rows.Err()
}

// QueryAttackLogs return les logs d'attaques récents.
func (s *SQLiteStorage) QueryAttackLogs(ctx context.Context, since time.Time, limit int) ([]enrichment.AttackLog, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, timestamp, machine, ip, path, status_code, count
		FROM attack_log WHERE timestamp >= ? ORDER BY timestamp DESC LIMIT ?
	`, since, limit)
	if err != nil {
		return nil, fmt.Errorf("requête attack_log: %w", err)
	}
	defer rows.Close()

	var logs []enrichment.AttackLog
	for rows.Next() {
		var l enrichment.AttackLog
		if err := rows.Scan(&l.ID, &l.Timestamp, &l.Machine, &l.IP, &l.Path, &l.Status, &l.Count); err != nil {
			return nil, fmt.Errorf("lecture attack_log: %w", err)
		}
		logs = append(logs, l)
	}

	return logs, rows.Err()
}

// QueryPatterns return les patterns détectés.
func (s *SQLiteStorage) QueryPatterns(ctx context.Context, unresolvedOnly bool, limit int) ([]enrichment.Pattern, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, timestamp, type, severity, detail, resolved FROM patterns`
	var args []interface{}

	if unresolvedOnly {
		query += ` WHERE resolved = 0`
	}
	query += ` ORDER BY timestamp DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("requête patterns: %w", err)
	}
	defer rows.Close()

	var patterns []enrichment.Pattern
	for rows.Next() {
		var p enrichment.Pattern
		var resolved int
		if err := rows.Scan(&p.ID, &p.Timestamp, &p.Type, &p.Severity, &p.Detail, &resolved); err != nil {
			return nil, fmt.Errorf("lecture pattern: %w", err)
		}
		p.Resolved = resolved != 0
		patterns = append(patterns, p)
	}

	return patterns, rows.Err()
}

// BudgetStatus return le budget d'enrichissement depuis le tracker (pas la BDD).
// La BDD est utilisée pour la persistance entre les redémarrages.
func (s *SQLiteStorage) BudgetStatus(ctx context.Context) (map[string]map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT provider, daily_limit, used_today FROM enrichment_budget`)
	if err != nil {
		return nil, fmt.Errorf("requête budget: %w", err)
	}
	defer rows.Close()

	status := make(map[string]map[string]int)
	for rows.Next() {
		var provider string
		var limit, used int
		if err := rows.Scan(&provider, &limit, &used); err != nil {
			return nil, fmt.Errorf("lecture budget: %w", err)
		}
		status[provider] = map[string]int{
			"daily_limit": limit,
			"used_today":  used,
			"remaining":   limit - used,
		}
	}

	return status, rows.Err()
}

// UnblockedHighRisk return les IPs avec un score d'abus élevé mais pas dans CrowdSec.
func (s *SQLiteStorage) UnblockedHighRisk(ctx context.Context, minScore int, limit int) ([]enrichment.IPIntel, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT ip, first_seen, last_seen, total_requests,
			country, city, lat, lon, asn, isp,
			blocklists_matched, crowdsec_banned, crowdsec_reason,
			abuse_score, greynoise_class, greynoise_name,
			enrichment_sources, enriched_at, priority_score
		FROM ip_intel
		WHERE abuse_score >= ? AND crowdsec_banned = 0
		ORDER BY abuse_score DESC LIMIT ?
	`, minScore, limit)
	if err != nil {
		return nil, fmt.Errorf("requête IPs à haut risque non bloquées: %w", err)
	}
	defer rows.Close()

	var results []enrichment.IPIntel
	for rows.Next() {
		var intel enrichment.IPIntel
		var blocklistsStr, sourcesStr sql.NullString
		var crowdsecBanned int

		if err := rows.Scan(
			&intel.IP, &intel.FirstSeen, &intel.LastSeen, &intel.TotalRequests,
			&intel.Country, &intel.City, &intel.Lat, &intel.Lon, &intel.ASN, &intel.ISP,
			&blocklistsStr, &crowdsecBanned, &intel.CrowdSecReason,
			&intel.AbuseScore, &intel.GreyNoiseClass, &intel.GreyNoiseName,
			&sourcesStr, &intel.EnrichedAt, &intel.PriorityScore,
		); err != nil {
			return nil, fmt.Errorf("lecture ip_intel: %w", err)
		}

		intel.CrowdSecBanned = crowdsecBanned != 0
		if blocklistsStr.Valid {
			json.Unmarshal([]byte(blocklistsStr.String), &intel.BlocklistsMatched)
		}
		if sourcesStr.Valid {
			json.Unmarshal([]byte(sourcesStr.String), &intel.EnrichmentSources)
		}

		results = append(results, intel)
	}

	return results, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

