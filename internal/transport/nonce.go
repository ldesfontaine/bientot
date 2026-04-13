package transport

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// MaxTimestampSkew is the maximum allowed age of a payload timestamp.
	MaxTimestampSkew = 60 * time.Second
	// NonceTTL is how long seen nonces are kept to prevent replay.
	NonceTTL = 2 * time.Minute
)

// NewNonce generates a fresh UUID nonce.
func NewNonce() string {
	return uuid.NewString()
}

// NonceCache tracks recently seen nonces to reject replays.
type NonceCache struct {
	mu    sync.Mutex
	seen  map[string]time.Time
	ttl   time.Duration
}

// NewNonceCache creates a cache that evicts entries after ttl.
func NewNonceCache() *NonceCache {
	nc := &NonceCache{
		seen: make(map[string]time.Time),
		ttl:  NonceTTL,
	}
	go nc.evictLoop()
	return nc
}

// Check validates timestamp freshness and nonce uniqueness.
// Returns an error if the payload should be rejected.
func (nc *NonceCache) Check(ts time.Time, nonce string) error {
	age := time.Since(ts)
	if age < 0 {
		age = -age
	}
	if age > MaxTimestampSkew {
		return fmt.Errorf("timestamp too old: %s", age)
	}

	nc.mu.Lock()
	defer nc.mu.Unlock()

	if _, exists := nc.seen[nonce]; exists {
		return fmt.Errorf("duplicate nonce: %s", nonce)
	}
	nc.seen[nonce] = time.Now()
	return nil
}

func (nc *NonceCache) evictLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		nc.mu.Lock()
		cutoff := time.Now().Add(-nc.ttl)
		for k, v := range nc.seen {
			if v.Before(cutoff) {
				delete(nc.seen, k)
			}
		}
		nc.mu.Unlock()
	}
}
