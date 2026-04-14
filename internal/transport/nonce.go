package transport

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// MaxTimestampSkew est l'âge maximum autorisé d'un timestamp de payload.
	MaxTimestampSkew = 60 * time.Second
	// NonceTTL est la durée de conservation des nonces vus pour empêcher le rejeu.
	NonceTTL = 2 * time.Minute
)

// NewNonce génère un nouveau nonce UUID.
func NewNonce() string {
	return uuid.NewString()
}

// NonceCache suit les nonces récemment vus pour rejeter les rejeux.
type NonceCache struct {
	mu    sync.Mutex
	seen  map[string]time.Time
	ttl   time.Duration
}

// NewNonceCache crée un cache qui expire les entrées après le ttl.
func NewNonceCache() *NonceCache {
	nc := &NonceCache{
		seen: make(map[string]time.Time),
		ttl:  NonceTTL,
	}
	go nc.evictLoop()
	return nc
}

// Check valide la fraîcheur du timestamp et l'unicité du nonce.
// return une erreur si le payload doit être rejeté.
func (nc *NonceCache) Check(ts time.Time, nonce string) error {
	age := time.Since(ts)
	if age < 0 {
		age = -age
	}
	if age > MaxTimestampSkew {
		return fmt.Errorf("timestamp trop ancien: %s", age)
	}

	nc.mu.Lock()
	defer nc.mu.Unlock()

	if _, exists := nc.seen[nonce]; exists {
		return fmt.Errorf("nonce dupliqué: %s", nonce)
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
