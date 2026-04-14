package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/ldesfontaine/bientot/internal"
)

// SQLiteStorage implémente Storage avec SQLite
type SQLiteStorage struct {
	db     *sql.DB
	config Config
}

// NewSQLiteStorage crée un nouveau stockage SQLite
func NewSQLiteStorage(config Config) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite3", config.DBPath+"?_journal=WAL&_sync=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("ouverture de la base de données: %w", err)
	}

	s := &SQLiteStorage{db: db, config: config}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migration: %w", err)
	}

	return s, nil
}

func (s *SQLiteStorage) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS metrics_raw (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		value REAL NOT NULL,
		labels TEXT,
		source TEXT,
		timestamp DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS metrics_5min (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		min_value REAL NOT NULL,
		max_value REAL NOT NULL,
		avg_value REAL NOT NULL,
		count INTEGER NOT NULL,
		labels TEXT,
		source TEXT,
		timestamp DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS metrics_hourly (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		min_value REAL NOT NULL,
		max_value REAL NOT NULL,
		avg_value REAL NOT NULL,
		count INTEGER NOT NULL,
		labels TEXT,
		source TEXT,
		timestamp DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_raw_name_ts ON metrics_raw(name, timestamp);
	CREATE INDEX IF NOT EXISTS idx_5min_name_ts ON metrics_5min(name, timestamp);
	CREATE INDEX IF NOT EXISTS idx_hourly_name_ts ON metrics_hourly(name, timestamp);

	CREATE TABLE IF NOT EXISTS log_entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL,
		received_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		machine TEXT NOT NULL,
		source TEXT NOT NULL,
		severity TEXT NOT NULL,
		message TEXT NOT NULL,
		parsed_json TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_logs_machine_ts ON log_entries(machine, timestamp);
	CREATE INDEX IF NOT EXISTS idx_logs_source ON log_entries(source, timestamp);
	CREATE INDEX IF NOT EXISTS idx_logs_severity ON log_entries(severity, timestamp);

	-- Tables d'enrichissement (Sprint 3)
	CREATE TABLE IF NOT EXISTS attack_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL,
		machine TEXT NOT NULL,
		ip TEXT NOT NULL,
		path TEXT,
		status_code INTEGER,
		count INTEGER DEFAULT 1
	);
	CREATE INDEX IF NOT EXISTS idx_attack_ip_ts ON attack_log(ip, timestamp);
	CREATE INDEX IF NOT EXISTS idx_attack_machine ON attack_log(machine, timestamp);

	CREATE TABLE IF NOT EXISTS ip_intel (
		ip TEXT PRIMARY KEY,
		first_seen DATETIME NOT NULL,
		last_seen DATETIME NOT NULL,
		total_requests INTEGER DEFAULT 0,
		country TEXT,
		city TEXT,
		lat REAL,
		lon REAL,
		asn INTEGER,
		isp TEXT,
		blocklists_matched TEXT,
		crowdsec_banned INTEGER DEFAULT 0,
		crowdsec_reason TEXT,
		abuse_score INTEGER DEFAULT 0,
		greynoise_class TEXT,
		greynoise_name TEXT,
		enrichment_sources TEXT,
		enriched_at DATETIME,
		priority_score INTEGER DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS blocklist_ips (
		ip TEXT NOT NULL,
		list_name TEXT NOT NULL,
		PRIMARY KEY (ip, list_name)
	);

	CREATE TABLE IF NOT EXISTS enrichment_budget (
		provider TEXT PRIMARY KEY,
		daily_limit INTEGER NOT NULL,
		used_today INTEGER DEFAULT 0,
		last_reset DATETIME
	);

	CREATE TABLE IF NOT EXISTS patterns (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL,
		type TEXT NOT NULL,
		severity TEXT NOT NULL,
		detail TEXT,
		resolved INTEGER DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_patterns_type ON patterns(type, timestamp);

	-- Inventaire logiciel (Sprint 4)
	CREATE TABLE IF NOT EXISTS software_inventory (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		machine TEXT NOT NULL,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		source TEXT NOT NULL,
		container TEXT,
		first_seen DATETIME NOT NULL,
		last_seen DATETIME NOT NULL,
		UNIQUE(machine, name, source, container)
	);
	CREATE INDEX IF NOT EXISTS idx_software_machine ON software_inventory(machine);
	CREATE INDEX IF NOT EXISTS idx_software_name ON software_inventory(name);

	-- Corrélation CVE (Sprint 4)
	CREATE TABLE IF NOT EXISTS vuln_matches (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		cve_id TEXT NOT NULL,
		severity TEXT NOT NULL,
		cvss_score REAL,
		title TEXT,
		link TEXT,
		matched_software TEXT NOT NULL,
		machine TEXT NOT NULL,
		installed_version TEXT,
		confidence TEXT NOT NULL,
		veille_alert_id INTEGER,
		cisa_kev INTEGER DEFAULT 0,
		first_seen DATETIME NOT NULL,
		resolved_at DATETIME,
		dismissed INTEGER DEFAULT 0,
		UNIQUE(cve_id, machine, matched_software)
	);
	CREATE INDEX IF NOT EXISTS idx_vuln_cve ON vuln_matches(cve_id);
	CREATE INDEX IF NOT EXISTS idx_vuln_machine ON vuln_matches(machine);

	CREATE TABLE IF NOT EXISTS veille_sync_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL,
		alerts_received INTEGER DEFAULT 0,
		matches_found INTEGER DEFAULT 0,
		status TEXT NOT NULL
	);
	`

	_, err := s.db.Exec(schema)
	return err
}

func (s *SQLiteStorage) Write(ctx context.Context, metrics []internal.Metric) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("début de transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO metrics_raw (name, value, labels, source, timestamp)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("préparation de la requête: %w", err)
	}
	defer stmt.Close()

	for _, m := range metrics {
		labels, _ := json.Marshal(m.Labels)
		_, err := stmt.ExecContext(ctx, m.Name, m.Value, string(labels), m.Source, m.Timestamp)
		if err != nil {
			return fmt.Errorf("insertion de la métrique: %w", err)
		}
	}

	return tx.Commit()
}

func (s *SQLiteStorage) Query(ctx context.Context, name string, from, to time.Time, resolution internal.Resolution) ([]internal.Point, error) {
	var query string
	switch resolution {
	case internal.ResolutionRaw:
		query = `SELECT timestamp, value, value, value FROM metrics_raw WHERE name = ? AND timestamp BETWEEN ? AND ? ORDER BY timestamp`
	case internal.Resolution5Min:
		query = `SELECT timestamp, avg_value, min_value, max_value FROM metrics_5min WHERE name = ? AND timestamp BETWEEN ? AND ? ORDER BY timestamp`
	case internal.ResolutionHourly:
		query = `SELECT timestamp, avg_value, min_value, max_value FROM metrics_hourly WHERE name = ? AND timestamp BETWEEN ? AND ? ORDER BY timestamp`
	default:
		return nil, fmt.Errorf("résolution inconnue: %s", resolution)
	}

	rows, err := s.db.QueryContext(ctx, query, name, from, to)
	if err != nil {
		return nil, fmt.Errorf("requête: %w", err)
	}
	defer rows.Close()

	var points []internal.Point
	for rows.Next() {
		var p internal.Point
		if err := rows.Scan(&p.Timestamp, &p.Value, &p.Min, &p.Max); err != nil {
			return nil, fmt.Errorf("lecture de la ligne: %w", err)
		}
		p.Avg = p.Value
		points = append(points, p)
	}

	return points, rows.Err()
}

func (s *SQLiteStorage) QueryLatest(ctx context.Context, name string, labels map[string]string) (*internal.Metric, error) {
	var query string
	var args []interface{}

	if len(labels) > 0 {
		labelsJSON, _ := json.Marshal(labels)
		query = `SELECT name, value, labels, source, timestamp FROM metrics_raw WHERE name = ? AND labels = ? ORDER BY timestamp DESC LIMIT 1`
		args = []interface{}{name, string(labelsJSON)}
	} else {
		query = `SELECT name, value, labels, source, timestamp FROM metrics_raw WHERE name = ? ORDER BY timestamp DESC LIMIT 1`
		args = []interface{}{name}
	}

	var m internal.Metric
	var labelsStr string
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&m.Name, &m.Value, &labelsStr, &m.Source, &m.Timestamp)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("requête: %w", err)
	}

	json.Unmarshal([]byte(labelsStr), &m.Labels)
	return &m, nil
}

func (s *SQLiteStorage) List(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT name FROM metrics_raw ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("requête: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("lecture: %w", err)
		}
		names = append(names, name)
	}

	return names, rows.Err()
}

func (s *SQLiteStorage) Downsample(ctx context.Context) error {
	// Agrégation raw vers 5min (données de plus de 24h)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO metrics_5min (name, min_value, max_value, avg_value, count, labels, source, timestamp)
		SELECT
			name,
			MIN(value),
			MAX(value),
			AVG(value),
			COUNT(*),
			labels,
			source,
			datetime((strftime('%s', timestamp) / 300) * 300, 'unixepoch') as bucket
		FROM metrics_raw
		WHERE timestamp < datetime('now', '-24 hours')
		GROUP BY name, labels, source, bucket
	`)
	if err != nil {
		return fmt.Errorf("sous-échantillonnage 5min: %w", err)
	}

	// Suppression des données brutes de plus de 24h
	_, err = s.db.ExecContext(ctx, `DELETE FROM metrics_raw WHERE timestamp < datetime('now', '-24 hours')`)
	if err != nil {
		return fmt.Errorf("nettoyage des données brutes: %w", err)
	}

	// Agrégation 5min vers horaire (données de plus de 7 jours)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO metrics_hourly (name, min_value, max_value, avg_value, count, labels, source, timestamp)
		SELECT
			name,
			MIN(min_value),
			MAX(max_value),
			SUM(avg_value * count) / SUM(count),
			SUM(count),
			labels,
			source,
			datetime((strftime('%s', timestamp) / 3600) * 3600, 'unixepoch') as bucket
		FROM metrics_5min
		WHERE timestamp < datetime('now', '-7 days')
		GROUP BY name, labels, source, bucket
	`)
	if err != nil {
		return fmt.Errorf("sous-échantillonnage horaire: %w", err)
	}

	// Suppression des données 5min de plus de 7 jours
	_, err = s.db.ExecContext(ctx, `DELETE FROM metrics_5min WHERE timestamp < datetime('now', '-7 days')`)
	if err != nil {
		return fmt.Errorf("nettoyage des données 5min: %w", err)
	}

	return nil
}

func (s *SQLiteStorage) Cleanup(ctx context.Context) error {
	retention := fmt.Sprintf("-%d days", s.config.RetentionDays)
	_, err := s.db.ExecContext(ctx, `DELETE FROM metrics_hourly WHERE timestamp < datetime('now', ?)`, retention)
	return err
}

func (s *SQLiteStorage) InsertLogs(ctx context.Context, entries []internal.LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("début de transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO log_entries (timestamp, machine, source, severity, message, parsed_json)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("préparation de la requête: %w", err)
	}
	defer stmt.Close()

	for _, e := range entries {
		parsed, _ := json.Marshal(e.Parsed)
		_, err := stmt.ExecContext(ctx, e.Timestamp, e.Machine, e.Source, e.Severity, e.Message, string(parsed))
		if err != nil {
			return fmt.Errorf("insertion de l'entrée de log: %w", err)
		}
	}

	return tx.Commit()
}

func (s *SQLiteStorage) QueryLogs(ctx context.Context, machine, source, severity string, since time.Time, limit int) ([]internal.LogEntry, error) {
	query := `SELECT timestamp, machine, source, severity, message, parsed_json FROM log_entries WHERE timestamp >= ?`
	args := []interface{}{since}

	if machine != "" {
		query += ` AND machine = ?`
		args = append(args, machine)
	}
	if source != "" {
		query += ` AND source = ?`
		args = append(args, source)
	}
	if severity != "" {
		query += ` AND severity = ?`
		args = append(args, severity)
	}

	query += ` ORDER BY timestamp DESC LIMIT ?`
	if limit <= 0 {
		limit = 100
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("requête des logs: %w", err)
	}
	defer rows.Close()

	var entries []internal.LogEntry
	for rows.Next() {
		var e internal.LogEntry
		var parsedStr sql.NullString
		if err := rows.Scan(&e.Timestamp, &e.Machine, &e.Source, &e.Severity, &e.Message, &parsedStr); err != nil {
			return nil, fmt.Errorf("lecture de l'entrée de log: %w", err)
		}
		if parsedStr.Valid && parsedStr.String != "" {
			json.Unmarshal([]byte(parsedStr.String), &e.Parsed)
		}
		entries = append(entries, e)
	}

	return entries, rows.Err()
}

func (s *SQLiteStorage) QueryLogStats(ctx context.Context) (*internal.LogStats, error) {
	stats := &internal.LogStats{
		BySource:   make(map[string]int),
		BySeverity: make(map[string]int),
	}

	// Par source (dernières 24h)
	rows, err := s.db.QueryContext(ctx, `
		SELECT source, COUNT(*) FROM log_entries
		WHERE timestamp >= datetime('now', '-24 hours')
		GROUP BY source
	`)
	if err != nil {
		return nil, fmt.Errorf("requête des stats de log par source: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err != nil {
			return nil, fmt.Errorf("lecture de la stat source: %w", err)
		}
		stats.BySource[source] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Par sévérité (dernières 24h)
	rows2, err := s.db.QueryContext(ctx, `
		SELECT severity, COUNT(*) FROM log_entries
		WHERE timestamp >= datetime('now', '-24 hours')
		GROUP BY severity
	`)
	if err != nil {
		return nil, fmt.Errorf("requête des stats de log par sévérité: %w", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var severity string
		var count int
		if err := rows2.Scan(&severity, &count); err != nil {
			return nil, fmt.Errorf("lecture de la stat sévérité: %w", err)
		}
		stats.BySeverity[severity] = count
	}

	return stats, rows2.Err()
}

func (s *SQLiteStorage) PurgeLogs(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	_, err := s.db.ExecContext(ctx, `DELETE FROM log_entries WHERE timestamp < ?`, cutoff)
	if err != nil {
		return fmt.Errorf("purge des logs: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}
