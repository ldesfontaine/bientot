package transport

import (
	"testing"
	"time"
)

func TestNonceCacheRejectsReplay(t *testing.T) {
	nc := NewNonceCache()
	now := time.Now()
	nonce := "test-nonce-1"

	if err := nc.Check(now, nonce); err != nil {
		t.Fatalf("first check should pass: %v", err)
	}

	if err := nc.Check(now, nonce); err == nil {
		t.Fatal("duplicate nonce should be rejected")
	}
}

func TestNonceCacheRejectsOldTimestamp(t *testing.T) {
	nc := NewNonceCache()
	old := time.Now().Add(-5 * time.Minute)

	if err := nc.Check(old, "nonce-old"); err == nil {
		t.Fatal("old timestamp should be rejected")
	}
}

func TestNonceCacheAcceptsFreshTimestamp(t *testing.T) {
	nc := NewNonceCache()
	fresh := time.Now().Add(-30 * time.Second)

	if err := nc.Check(fresh, "nonce-fresh"); err != nil {
		t.Fatalf("fresh timestamp should pass: %v", err)
	}
}
