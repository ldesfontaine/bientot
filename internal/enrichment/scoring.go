package enrichment

// sensitivePaths sont les chemins indiquant une reconnaissance ciblée.
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

// ScoreIP calcule un score de priorité pour une IP.
// Score élevé = plus intéressant = mérite de dépenser du budget API.
func ScoreIP(reqCount int, paths []string, inBlocklist bool, inCrowdSec bool) int {
	score := 0

	// Pas dans CrowdSec = passe à travers les défenses
	if !inCrowdSec {
		score += 50
	}

	// Volume de requêtes : +10 par tranche de 10 requêtes/heure
	score += (reqCount / 10) * 10

	// Chemins sensibles ciblés
	for _, p := range paths {
		if sensitivePaths[p] {
			score += 20
			break // compter une seule fois
		}
	}

	// Déjà en blocklist = moins intéressant, on sait déjà
	if inBlocklist {
		score -= 30
	}

	return score
}
