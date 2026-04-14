package veille

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ldesfontaine/bientot/internal"
	"github.com/ldesfontaine/bientot/internal/storage"
)

// SyncerConfig configure la boucle de synchronisation veille-secu.
type SyncerConfig struct {
	PollInterval   time.Duration
	SyncTools      bool     // si true, push l'inventaire vers veille-secu
	SeverityFilter []string // ne traiter que ces sévérités
}

// Syncer récupère périodiquement les alertes veille-secu et les corrèle avec l'inventaire.
type Syncer struct {
	client   *Client
	store    *storage.SQLiteStorage
	cfg      SyncerConfig
	logger   *slog.Logger
	onMatch  func(internal.VulnMatch) // callback pour les nouvelles correspondances (alerting)
}

// NewSyncer crée un syncer veille-secu.
func NewSyncer(client *Client, store *storage.SQLiteStorage, cfg SyncerConfig, logger *slog.Logger) *Syncer {
	return &Syncer{
		client: client,
		store:  store,
		cfg:    cfg,
		logger: logger,
	}
}

// OnMatch définit un callback déclenché quand une nouvelle correspondance CVE est trouvée.
func (s *Syncer) OnMatch(fn func(internal.VulnMatch)) {
	s.onMatch = fn
}

// Run démarre la boucle de synchronisation. Bloque jusqu'à l'annulation du ctx.
func (s *Syncer) Run(ctx context.Context) {
	interval := s.cfg.PollInterval
	if interval == 0 {
		interval = 15 * time.Minute
	}

	s.logger.Info("syncer veille démarré", "interval", interval, "sync_tools", s.cfg.SyncTools)

	// Synchronisation initiale
	s.sync(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("arrêt du syncer veille")
			return
		case <-ticker.C:
			s.sync(ctx)
		}
	}
}

func (s *Syncer) sync(ctx context.Context) {
	// Vérification de santé d'abord
	if err := s.client.Health(); err != nil {
		s.logger.Warn("veille-secu injoignable, synchronisation ignorée", "error", err)
		s.store.InsertSyncLog(ctx, 0, 0, "unreachable")
		return
	}

	// Récupération des nouvelles alertes
	alerts, err := s.client.FetchAlerts("new", s.cfg.SeverityFilter, 200)
	if err != nil {
		s.logger.Error("échec de la récupération des alertes veille", "error", err)
		s.store.InsertSyncLog(ctx, 0, 0, fmt.Sprintf("error: %v", err))
		return
	}

	s.logger.Debug("alertes veille récupérées", "count", len(alerts))

	// Corrélation avec l'inventaire logiciel
	matchCount := 0
	for _, alert := range alerts {
		matches := s.correlate(ctx, alert)
		matchCount += len(matches)
	}

	s.store.InsertSyncLog(ctx, len(alerts), matchCount, "ok")
	s.logger.Info("synchronisation veille terminée", "alerts", len(alerts), "matches", matchCount)

	// Synchronisation optionnelle des outils vers veille-secu
	if s.cfg.SyncTools {
		s.syncToolsToVeille(ctx)
	}
}

// correlate vérifie si les matched_tools d'une alerte apparaissent dans software_inventory.
func (s *Syncer) correlate(ctx context.Context, alert Alert) []internal.VulnMatch {
	var matches []internal.VulnMatch

	for _, toolName := range alert.MatchedTools {
		// Recherche de toutes les machines avec ce logiciel
		items, err := s.store.FindSoftwareByName(ctx, toolName)
		if err != nil {
			s.logger.Warn("échec de la recherche logicielle", "tool", toolName, "error", err)
			continue
		}

		for _, item := range items {
			confidence := determineConfidence(alert, item)

			match := internal.VulnMatch{
				CVEID:            alert.CVEID,
				Severity:         alert.Severity,
				CVSSScore:        alert.CVSSScore,
				Title:            alert.Title,
				Link:             alert.Link,
				MatchedSoftware:  toolName,
				Machine:          item.Machine,
				InstalledVersion: item.Version,
				Confidence:       confidence,
				VeilleAlertID:    alert.ID,
				CISAKEV:          isCISAKEV(alert),
				FirstSeen:        time.Now(),
			}

			if err := s.store.UpsertVulnMatch(ctx, &match); err != nil {
				s.logger.Warn("échec de l'upsert de correspondance vuln", "cve", alert.CVEID, "machine", item.Machine, "error", err)
				continue
			}

			matches = append(matches, match)

			if s.onMatch != nil {
				s.onMatch(match)
			}
		}
	}

	return matches
}

// determineConfidence classifie la qualité de la correspondance.
func determineConfidence(alert Alert, item internal.SoftwareItem) string {
	// Vérification si la description CVE mentionne une plage de versions spécifique
	desc := strings.ToLower(alert.Title + " " + alert.Description)
	version := strings.ToLower(item.Version)

	// Si la version est explicitement mentionnée dans le texte de l'alerte → confirmed
	if version != "" && version != "latest" && strings.Contains(desc, version) {
		return "confirmed"
	}

	// Si la version est très ancienne (heuristique : "latest" ou vide) → likely
	if version == "latest" || version == "" {
		return "likely"
	}

	// Vérification des patterns "before X.Y.Z" — si notre version est mentionnée,
	// l'alerte s'applique probablement aux versions antérieures à la version corrigée
	if strings.Contains(desc, "before") || strings.Contains(desc, "prior to") {
		return "likely"
	}

	// Par défaut : outil correspondant mais sans confirmation de version
	return "likely"
}

// isCISAKEV vérifie si l'alerte provient de la source CISA KEV.
func isCISAKEV(alert Alert) bool {
	return alert.SourceID == "cisa-kev" || strings.Contains(strings.ToLower(alert.SourceName), "kev")
}

// syncToolsToVeille push l'inventaire logiciel bientot vers veille-secu en tant qu'outils.
func (s *Syncer) syncToolsToVeille(ctx context.Context) {
	items, err := s.store.QuerySoftware(ctx, "")
	if err != nil {
		s.logger.Warn("échec de la requête inventaire logiciel pour synchronisation", "error", err)
		return
	}

	// Déduplication par nom (ne push que les noms de logiciels uniques)
	seen := make(map[string]bool)
	synced := 0

	for _, item := range items {
		if seen[item.Name] {
			continue
		}
		seen[item.Name] = true

		tool := Tool{
			Name:     item.Name,
			Keywords: []string{item.Name},
			Version:  item.Version,
			Source:   "bientot-auto",
		}

		if err := s.client.AddTool(tool); err != nil {
			s.logger.Debug("échec de la synchronisation de l'outil", "name", item.Name, "error", err)
			continue
		}
		synced++
	}

	if synced > 0 {
		s.logger.Info("outils synchronisés vers veille-secu", "count", synced)
	}
}
