package enrichment

// sensitivePaths are paths that indicate targeted reconnaissance.
var sensitivePaths = map[string]bool{
	"/.env":       true,
	"/.git":       true,
	"/admin":      true,
	"/wp-login":   true,
	"/wp-admin":   true,
	"/.git/HEAD":  true,
	"/.aws":       true,
	"/phpmyadmin": true,
	"/xmlrpc.php": true,
	"/config":     true,
}

// ScoreIP computes a priority score for an IP.
// Higher score = more interesting = worth spending API budget.
func ScoreIP(reqCount int, paths []string, inBlocklist bool, inCrowdSec bool) int {
	score := 0

	// Not in CrowdSec = passes through defenses
	if !inCrowdSec {
		score += 50
	}

	// Request volume: +10 per 10 requests/hour
	score += (reqCount / 10) * 10

	// Sensitive paths targeted
	for _, p := range paths {
		if sensitivePaths[p] {
			score += 20
			break // count once
		}
	}

	// Already in blocklist = less interesting, we already know
	if inBlocklist {
		score -= 30
	}

	return score
}
