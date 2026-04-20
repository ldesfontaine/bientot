// Package storage manages persistence of agent pushes in SQLite.
// It exposes a single Storage handle with the lifecycle methods Open/Close,
// and domain-specific methods for writes (5.0.2) and reads (5.0.3).
package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Storage is the dashboard's persistence layer. It wraps a *sql.DB
// and serializes access implicitly via SQLite's built-in locking.
type Storage struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at the given path and
// applies the schema. It is safe to call Open on an existing database:
// CREATE TABLE IF NOT EXISTS makes schema application idempotent.
//
// Recommended path: "/data/dashboard.db" inside the container,
// exposed through the BIENTOT_DB_PATH env var in the caller.
func Open(path string) (*Storage, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite at %s: %w", path, err)
	}

	// Connection pool tuning for SQLite:
	// SQLite handles one writer at a time. Multiple connections serialize
	// writes and can cause "database is locked" errors under concurrency.
	// A single connection (MaxOpenConns=1) ensures predictable behavior.
	db.SetMaxOpenConns(1)

	// Enable foreign keys (off by default in SQLite, must be set per-connection).
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign_keys: %w", err)
	}

	// WAL mode: enables concurrent readers during a write, safer on crash.
	// Standard setting for any SQLite app with a non-trivial workload.
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set journal_mode WAL: %w", err)
	}

	// Apply schema (idempotent thanks to IF NOT EXISTS).
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return &Storage{db: db}, nil
}

// Close releases the underlying database handle. Safe to call multiple times.
func (s *Storage) Close() error {
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

// Ping verifies the database is reachable. Useful for startup healthcheck.
func (s *Storage) Ping(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("storage is closed")
	}
	return s.db.PingContext(ctx)
}
