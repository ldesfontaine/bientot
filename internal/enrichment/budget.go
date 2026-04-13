package enrichment

import (
	"fmt"
	"sync"
	"time"
)

// BudgetTracker tracks daily API usage per provider.
type BudgetTracker struct {
	mu      sync.Mutex
	budgets map[string]*providerBudget
}

type providerBudget struct {
	DailyLimit int
	UsedToday  int
	LastReset  time.Time
}

// NewBudgetTracker creates a tracker with configured limits.
func NewBudgetTracker(limits map[string]int) *BudgetTracker {
	budgets := make(map[string]*providerBudget, len(limits))
	now := time.Now().UTC()
	for name, limit := range limits {
		budgets[name] = &providerBudget{
			DailyLimit: limit,
			UsedToday:  0,
			LastReset:  now,
		}
	}
	return &BudgetTracker{budgets: budgets}
}

// CanSpend checks if a provider has budget remaining today.
func (bt *BudgetTracker) CanSpend(provider string) bool {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	b, ok := bt.budgets[provider]
	if !ok {
		return false
	}

	bt.resetIfNewDay(b)
	return b.UsedToday < b.DailyLimit
}

// Spend consumes one API call for a provider.
func (bt *BudgetTracker) Spend(provider string) error {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	b, ok := bt.budgets[provider]
	if !ok {
		return fmt.Errorf("unknown provider: %s", provider)
	}

	bt.resetIfNewDay(b)
	if b.UsedToday >= b.DailyLimit {
		return fmt.Errorf("budget exhausted for %s (%d/%d)", provider, b.UsedToday, b.DailyLimit)
	}

	b.UsedToday++
	return nil
}

// Status returns current budget state for all providers.
func (bt *BudgetTracker) Status() map[string]map[string]int {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	status := make(map[string]map[string]int, len(bt.budgets))
	for name, b := range bt.budgets {
		bt.resetIfNewDay(b)
		status[name] = map[string]int{
			"daily_limit": b.DailyLimit,
			"used_today":  b.UsedToday,
			"remaining":   b.DailyLimit - b.UsedToday,
		}
	}
	return status
}

func (bt *BudgetTracker) resetIfNewDay(b *providerBudget) {
	now := time.Now().UTC()
	if now.YearDay() != b.LastReset.YearDay() || now.Year() != b.LastReset.Year() {
		b.UsedToday = 0
		b.LastReset = now
	}
}
