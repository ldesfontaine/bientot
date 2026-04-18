package nonce

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCache_FreshNonce(t *testing.T) {
	c := NewCache(1 * time.Second)
	now := time.Now()

	if !c.CheckAndAdd("n1", now) {
		t.Error("first CheckAndAdd should return true")
	}
}

func TestCache_ReplayRejected(t *testing.T) {
	c := NewCache(1 * time.Second)
	now := time.Now()

	c.CheckAndAdd("n1", now)

	if c.CheckAndAdd("n1", now.Add(500*time.Millisecond)) {
		t.Error("replay within TTL should return false")
	}
}

func TestCache_AcceptedAfterExpiration(t *testing.T) {
	c := NewCache(1 * time.Second)
	now := time.Now()

	c.CheckAndAdd("n1", now)

	if !c.CheckAndAdd("n1", now.Add(2*time.Second)) {
		t.Error("nonce after TTL should be accepted again")
	}
}

func TestCache_Concurrent(t *testing.T) {
	c := NewCache(10 * time.Second)
	const N = 1000

	var wg sync.WaitGroup
	wg.Add(N)

	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			c.CheckAndAdd(fmt.Sprintf("nonce-%d", i), time.Now())
		}(i)
	}

	wg.Wait()

	if c.Size() != N {
		t.Errorf("expected %d entries after concurrent inserts, got %d", N, c.Size())
	}
}

func TestCache_EvictOnce(t *testing.T) {
	c := NewCache(1 * time.Second)
	now := time.Now()

	c.CheckAndAdd("old", now)
	c.CheckAndAdd("fresh", now.Add(900*time.Millisecond))

	c.evictOnce(now.Add(1500 * time.Millisecond))

	if c.Size() != 1 {
		t.Errorf("expected 1 entry after eviction, got %d", c.Size())
	}
}
