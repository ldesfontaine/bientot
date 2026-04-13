package enrichment

import "testing"

func TestBudgetTracker(t *testing.T) {
	bt := NewBudgetTracker(map[string]int{
		"abuseipdb": 3,
		"greynoise": 1,
	})

	// Can spend initially
	if !bt.CanSpend("abuseipdb") {
		t.Fatal("should be able to spend abuseipdb")
	}

	// Spend 3 times
	for i := 0; i < 3; i++ {
		if err := bt.Spend("abuseipdb"); err != nil {
			t.Fatalf("spend %d failed: %v", i, err)
		}
	}

	// Budget exhausted
	if bt.CanSpend("abuseipdb") {
		t.Fatal("should NOT be able to spend abuseipdb after 3 uses")
	}
	if err := bt.Spend("abuseipdb"); err == nil {
		t.Fatal("should fail when budget exhausted")
	}

	// Unknown provider
	if bt.CanSpend("unknown") {
		t.Fatal("unknown provider should return false")
	}

	// Status check
	status := bt.Status()
	if status["abuseipdb"]["remaining"] != 0 {
		t.Errorf("abuseipdb remaining = %d, want 0", status["abuseipdb"]["remaining"])
	}
	if status["greynoise"]["remaining"] != 1 {
		t.Errorf("greynoise remaining = %d, want 1", status["greynoise"]["remaining"])
	}
}
