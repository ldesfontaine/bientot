package storage

import (
	"context"
	"time"

	"github.com/ldesfontaine/bientot/internal"
)

// Storage est l'interface pour les backends de stockage de métriques
type Storage interface {
	// Write stocke les métriques
	Write(ctx context.Context, metrics []internal.Metric) error

	// Query récupère les données de séries temporelles
	Query(ctx context.Context, name string, from, to time.Time, resolution internal.Resolution) ([]internal.Point, error)

	// QueryLatest récupère la valeur la plus récente d'une métrique
	QueryLatest(ctx context.Context, name string, labels map[string]string) (*internal.Metric, error)

	// List return tous les noms de métriques connus
	List(ctx context.Context) ([]string, error)

	// Downsample agrège les anciennes données vers des résolutions inférieures
	Downsample(ctx context.Context) error

	// Cleanup supprime les données plus anciennes que la période de rétention
	Cleanup(ctx context.Context) error

	// Close ferme la connexion au stockage
	Close() error

	// InsertLogs stocke les entrées de log par lot
	InsertLogs(ctx context.Context, entries []internal.LogEntry) error

	// QueryLogs récupère les entrées de log correspondant aux filtres
	QueryLogs(ctx context.Context, machine, source, severity string, since time.Time, limit int) ([]internal.LogEntry, error)

	// QueryLogStats return les compteurs par source et sévérité des dernières 24h
	QueryLogStats(ctx context.Context) (*internal.LogStats, error)

	// PurgeLogs supprime les entrées de log plus anciennes que la durée donnée
	PurgeLogs(ctx context.Context, olderThan time.Duration) error
}

// Config contient la configuration du stockage
type Config struct {
	DBPath        string
	RetentionDays int
}

// DefaultConfig return la configuration de stockage par défaut
func DefaultConfig() Config {
	return Config{
		DBPath:        "/data/metrics.db",
		RetentionDays: 90,
	}
}
