package storage

import (
	"context"
	"time"

	"github.com/ldesfontaine/bientot/internal"
)

// Storage is the interface for metric storage backends
type Storage interface {
	// Write stores metrics
	Write(ctx context.Context, metrics []internal.Metric) error

	// Query retrieves time-series data
	Query(ctx context.Context, name string, from, to time.Time, resolution internal.Resolution) ([]internal.Point, error)

	// QueryLatest retrieves the most recent value for a metric
	QueryLatest(ctx context.Context, name string, labels map[string]string) (*internal.Metric, error)

	// List returns all known metric names
	List(ctx context.Context) ([]string, error)

	// Downsample aggregates old data to lower resolutions
	Downsample(ctx context.Context) error

	// Cleanup removes data older than retention period
	Cleanup(ctx context.Context) error

	// Close closes the storage connection
	Close() error
}

// Config holds storage configuration
type Config struct {
	DBPath        string
	RetentionDays int
}

// DefaultConfig returns default storage configuration
func DefaultConfig() Config {
	return Config{
		DBPath:        "/data/metrics.db",
		RetentionDays: 90,
	}
}
