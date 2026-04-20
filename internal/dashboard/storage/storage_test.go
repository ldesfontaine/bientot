package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpen_CreatesDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestOpen_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer s2.Close()

	if err := s2.Ping(context.Background()); err != nil {
		t.Fatalf("Ping after reopen: %v", err)
	}
}

func TestOpen_SchemaApplied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	expectedTables := []string{"pushes", "metrics", "module_state", "agents"}

	for _, table := range expectedTables {
		var name string
		err := s.db.QueryRowContext(
			context.Background(),
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`,
			table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestOpen_ForeignKeysEnabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	var fkEnabled int
	err = s.db.QueryRowContext(
		context.Background(),
		`PRAGMA foreign_keys`,
	).Scan(&fkEnabled)
	if err != nil {
		t.Fatalf("query foreign_keys pragma: %v", err)
	}

	if fkEnabled != 1 {
		t.Errorf("foreign_keys = %d, want 1", fkEnabled)
	}
}

func TestOpen_WALMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	var journalMode string
	err = s.db.QueryRowContext(
		context.Background(),
		`PRAGMA journal_mode`,
	).Scan(&journalMode)
	if err != nil {
		t.Fatalf("query journal_mode pragma: %v", err)
	}

	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want wal", journalMode)
	}
}

func TestClose_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestPing_AfterClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.Close()

	if err := s.Ping(context.Background()); err == nil {
		t.Error("Ping on closed storage should return error, got nil")
	}
}

func TestOpen_InvalidPath(t *testing.T) {
	path := "/nonexistent/path/that/cannot/be/created/test.db"

	_, err := Open(path)
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}
