package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ldesfontaine/bientot/internal"
)

// UpsertSoftware inserts or updates a software inventory entry.
func (s *SQLiteStorage) UpsertSoftware(ctx context.Context, item *internal.SoftwareItem) error {
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO software_inventory (machine, name, version, source, container, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(machine, name, source, container) DO UPDATE SET
			version = excluded.version,
			last_seen = excluded.last_seen
	`, item.Machine, item.Name, item.Version, item.Source, item.Container, now, now)
	if err != nil {
		return fmt.Errorf("upserting software: %w", err)
	}
	return nil
}

// QuerySoftware returns software inventory for a machine (or all if machine is empty).
func (s *SQLiteStorage) QuerySoftware(ctx context.Context, machine string) ([]internal.SoftwareItem, error) {
	query := `SELECT id, machine, name, version, source, container, first_seen, last_seen FROM software_inventory`
	var args []interface{}

	if machine != "" {
		query += ` WHERE machine = ?`
		args = append(args, machine)
	}
	query += ` ORDER BY name`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying software: %w", err)
	}
	defer rows.Close()

	var items []internal.SoftwareItem
	for rows.Next() {
		var item internal.SoftwareItem
		var container sql.NullString
		if err := rows.Scan(&item.ID, &item.Machine, &item.Name, &item.Version,
			&item.Source, &container, &item.FirstSeen, &item.LastSeen); err != nil {
			return nil, fmt.Errorf("scanning software: %w", err)
		}
		if container.Valid {
			item.Container = container.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// FindSoftwareByName finds all machines running a given software name.
func (s *SQLiteStorage) FindSoftwareByName(ctx context.Context, name string) ([]internal.SoftwareItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, machine, name, version, source, container, first_seen, last_seen
		FROM software_inventory WHERE name = ? ORDER BY machine
	`, name)
	if err != nil {
		return nil, fmt.Errorf("finding software: %w", err)
	}
	defer rows.Close()

	var items []internal.SoftwareItem
	for rows.Next() {
		var item internal.SoftwareItem
		var container sql.NullString
		if err := rows.Scan(&item.ID, &item.Machine, &item.Name, &item.Version,
			&item.Source, &container, &item.FirstSeen, &item.LastSeen); err != nil {
			return nil, fmt.Errorf("scanning software: %w", err)
		}
		if container.Valid {
			item.Container = container.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// UpsertVulnMatch inserts or updates a vulnerability match.
func (s *SQLiteStorage) UpsertVulnMatch(ctx context.Context, v *internal.VulnMatch) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO vuln_matches (
			cve_id, severity, cvss_score, title, link,
			matched_software, machine, installed_version,
			confidence, veille_alert_id, cisa_kev, first_seen
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cve_id, machine, matched_software) DO UPDATE SET
			severity = excluded.severity,
			cvss_score = excluded.cvss_score,
			title = excluded.title,
			installed_version = excluded.installed_version,
			confidence = excluded.confidence,
			cisa_kev = excluded.cisa_kev
	`, v.CVEID, v.Severity, v.CVSSScore, v.Title, v.Link,
		v.MatchedSoftware, v.Machine, v.InstalledVersion,
		v.Confidence, v.VeilleAlertID, boolToInt(v.CISAKEV), v.FirstSeen)
	if err != nil {
		return fmt.Errorf("upserting vuln_match: %w", err)
	}
	return nil
}

// QueryVulnMatches returns vulnerability matches with optional filters.
func (s *SQLiteStorage) QueryVulnMatches(ctx context.Context, machine string, activeOnly bool, limit int) ([]internal.VulnMatch, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `SELECT id, cve_id, severity, cvss_score, title, link,
		matched_software, machine, installed_version,
		confidence, veille_alert_id, cisa_kev, first_seen, resolved_at, dismissed
		FROM vuln_matches WHERE 1=1`
	var args []interface{}

	if machine != "" {
		query += ` AND machine = ?`
		args = append(args, machine)
	}
	if activeOnly {
		query += ` AND resolved_at IS NULL AND dismissed = 0`
	}
	query += ` ORDER BY cvss_score DESC, first_seen DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying vuln_matches: %w", err)
	}
	defer rows.Close()

	var matches []internal.VulnMatch
	for rows.Next() {
		var v internal.VulnMatch
		var resolvedAt sql.NullTime
		var dismissed, cisakev int

		if err := rows.Scan(&v.ID, &v.CVEID, &v.Severity, &v.CVSSScore, &v.Title, &v.Link,
			&v.MatchedSoftware, &v.Machine, &v.InstalledVersion,
			&v.Confidence, &v.VeilleAlertID, &cisakev, &v.FirstSeen, &resolvedAt, &dismissed); err != nil {
			return nil, fmt.Errorf("scanning vuln_match: %w", err)
		}

		v.CISAKEV = cisakev != 0
		v.Dismissed = dismissed != 0
		if resolvedAt.Valid {
			v.ResolvedAt = &resolvedAt.Time
		}
		matches = append(matches, v)
	}
	return matches, rows.Err()
}

// DismissVuln marks a vulnerability match as dismissed.
func (s *SQLiteStorage) DismissVuln(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE vuln_matches SET dismissed = 1 WHERE id = ?`, id)
	return err
}

// ResolveVuln marks a vulnerability match as resolved.
func (s *SQLiteStorage) ResolveVuln(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE vuln_matches SET resolved_at = ? WHERE id = ?`, time.Now(), id)
	return err
}

// InsertSyncLog records a veille-secu sync operation.
func (s *SQLiteStorage) InsertSyncLog(ctx context.Context, alertsReceived, matchesFound int, status string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO veille_sync_log (timestamp, alerts_received, matches_found, status)
		VALUES (?, ?, ?, ?)
	`, time.Now(), alertsReceived, matchesFound, status)
	return err
}

// QuerySyncLogs returns recent sync log entries.
func (s *SQLiteStorage) QuerySyncLogs(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, timestamp, alerts_received, matches_found, status
		FROM veille_sync_log ORDER BY timestamp DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying sync logs: %w", err)
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var id int64
		var ts time.Time
		var received, found int
		var status string
		if err := rows.Scan(&id, &ts, &received, &found, &status); err != nil {
			return nil, fmt.Errorf("scanning sync log: %w", err)
		}
		logs = append(logs, map[string]interface{}{
			"id":              id,
			"timestamp":       ts,
			"alerts_received": received,
			"matches_found":   found,
			"status":          status,
		})
	}
	return logs, rows.Err()
}
