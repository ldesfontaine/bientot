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

// SQLiteStorage implements Storage using SQLite
type SQLiteStorage struct {
	db     *sql.DB
	config Config
}

// NewSQLiteStorage creates a new SQLite storage
func NewSQLiteStorage(config Config) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite3", config.DBPath+"?_journal=WAL&_sync=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	s := &SQLiteStorage{db: db, config: config}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating: %w", err)
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
	`

	_, err := s.db.Exec(schema)
	return err
}

func (s *SQLiteStorage) Write(ctx context.Context, metrics []internal.Metric) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO metrics_raw (name, value, labels, source, timestamp)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	for _, m := range metrics {
		labels, _ := json.Marshal(m.Labels)
		_, err := stmt.ExecContext(ctx, m.Name, m.Value, string(labels), m.Source, m.Timestamp)
		if err != nil {
			return fmt.Errorf("inserting metric: %w", err)
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
		return nil, fmt.Errorf("unknown resolution: %s", resolution)
	}

	rows, err := s.db.QueryContext(ctx, query, name, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying: %w", err)
	}
	defer rows.Close()

	var points []internal.Point
	for rows.Next() {
		var p internal.Point
		if err := rows.Scan(&p.Timestamp, &p.Value, &p.Min, &p.Max); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
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
		return nil, fmt.Errorf("querying: %w", err)
	}

	json.Unmarshal([]byte(labelsStr), &m.Labels)
	return &m, nil
}

func (s *SQLiteStorage) List(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT name FROM metrics_raw ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("querying: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scanning: %w", err)
		}
		names = append(names, name)
	}

	return names, rows.Err()
}

func (s *SQLiteStorage) Downsample(ctx context.Context) error {
	// Aggregate raw to 5min (data older than 24h)
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
		return fmt.Errorf("downsampling to 5min: %w", err)
	}

	// Delete raw data older than 24h
	_, err = s.db.ExecContext(ctx, `DELETE FROM metrics_raw WHERE timestamp < datetime('now', '-24 hours')`)
	if err != nil {
		return fmt.Errorf("cleaning raw data: %w", err)
	}

	// Aggregate 5min to hourly (data older than 7 days)
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
		return fmt.Errorf("downsampling to hourly: %w", err)
	}

	// Delete 5min data older than 7 days
	_, err = s.db.ExecContext(ctx, `DELETE FROM metrics_5min WHERE timestamp < datetime('now', '-7 days')`)
	if err != nil {
		return fmt.Errorf("cleaning 5min data: %w", err)
	}

	return nil
}

func (s *SQLiteStorage) Cleanup(ctx context.Context) error {
	retention := fmt.Sprintf("-%d days", s.config.RetentionDays)
	_, err := s.db.ExecContext(ctx, `DELETE FROM metrics_hourly WHERE timestamp < datetime('now', ?)`, retention)
	return err
}

func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}
