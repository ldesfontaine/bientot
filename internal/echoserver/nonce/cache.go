// Package nonce provides a thread-safe, TTL-bounded cache for tracking
// recently-seen message nonces to prevent replay attacks.
package nonce

import (
	"context"
	"sync"
	"time"
)

// Cache tracks recently-seen nonces for a bounded window.
type Cache struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
}

// NewCache creates a Cache with the given TTL for entries.
// The caller is expected to run Cache.Evict in a goroutine to prune expired entries.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		seen: make(map[string]time.Time),
		ttl:  ttl,
	}
}

// CheckAndAdd returns true if nonce is fresh (not seen in the window).
// It records the nonce in the cache. Subsequent calls with the same nonce
// within the TTL window return false.
func (c *Cache) CheckAndAdd(nonce string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if prev, exists := c.seen[nonce]; exists {
		if now.Sub(prev) < c.ttl {
			return false
		}
	}
	c.seen[nonce] = now
	return true
}

// Evict runs a periodic cleanup of expired entries. Blocks until ctx is cancelled.
func (c *Cache) Evict(ctx context.Context, period time.Duration) {
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.evictOnce(time.Now())
		}
	}
}

func (c *Cache) evictOnce(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for nonce, t := range c.seen {
		if now.Sub(t) >= c.ttl {
			delete(c.seen, nonce)
		}
	}
}

// Size returns the current number of entries (for metrics/tests).
func (c *Cache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.seen)
}
