package collector

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ldesfontaine/bientot/internal"
)

// ZFSCollector collects ZFS pool metrics
type ZFSCollector struct {
	name     string
	pools    []string
	interval time.Duration
}

// NewZFSCollector creates a new ZFS collector
func NewZFSCollector(name string, pools []string, interval time.Duration) *ZFSCollector {
	return &ZFSCollector{
		name:     name,
		pools:    pools,
		interval: interval,
	}
}

func (c *ZFSCollector) Name() string           { return c.name }
func (c *ZFSCollector) Type() string           { return "zfs" }
func (c *ZFSCollector) Interval() time.Duration { return c.interval }

func (c *ZFSCollector) Collect(ctx context.Context) ([]internal.Metric, error) {
	var metrics []internal.Metric
	now := time.Now()

	for _, pool := range c.pools {
		poolMetrics, err := c.collectPool(ctx, pool, now)
		if err != nil {
			return nil, fmt.Errorf("collecting pool %s: %w", pool, err)
		}
		metrics = append(metrics, poolMetrics...)
	}

	return metrics, nil
}

func (c *ZFSCollector) collectPool(ctx context.Context, pool string, now time.Time) ([]internal.Metric, error) {
	var metrics []internal.Metric
	labels := map[string]string{"pool": pool}

	// Get pool status
	health, err := c.getPoolHealth(ctx, pool)
	if err != nil {
		return nil, err
	}
	metrics = append(metrics, internal.Metric{
		Name:      "zfs_pool_health",
		Value:     health,
		Labels:    labels,
		Timestamp: now,
		Source:    c.name,
	})

	// Get pool space
	used, avail, err := c.getPoolSpace(ctx, pool)
	if err != nil {
		return nil, err
	}
	metrics = append(metrics, internal.Metric{
		Name:      "zfs_pool_used_bytes",
		Value:     used,
		Labels:    labels,
		Timestamp: now,
		Source:    c.name,
	})
	metrics = append(metrics, internal.Metric{
		Name:      "zfs_pool_available_bytes",
		Value:     avail,
		Labels:    labels,
		Timestamp: now,
		Source:    c.name,
	})

	total := used + avail
	if total > 0 {
		metrics = append(metrics, internal.Metric{
			Name:      "zfs_pool_used_percent",
			Value:     (used / total) * 100,
			Labels:    labels,
			Timestamp: now,
			Source:    c.name,
		})
	}

	return metrics, nil
}

func (c *ZFSCollector) getPoolHealth(ctx context.Context, pool string) (float64, error) {
	cmd := exec.CommandContext(ctx, "zpool", "status", "-x", pool)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("running zpool status: %w", err)
	}

	output := strings.TrimSpace(out.String())
	// "pool 'X' is healthy" = 2, degraded = 1, other = 0
	switch {
	case strings.Contains(output, "is healthy"):
		return 2, nil
	case strings.Contains(output, "DEGRADED"):
		return 1, nil
	default:
		return 0, nil
	}
}

func (c *ZFSCollector) getPoolSpace(ctx context.Context, pool string) (used, avail float64, err error) {
	cmd := exec.CommandContext(ctx, "zpool", "list", "-Hp", "-o", "size,alloc,free", pool)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return 0, 0, fmt.Errorf("running zpool list: %w", err)
	}

	fields := strings.Fields(strings.TrimSpace(out.String()))
	if len(fields) < 3 {
		return 0, 0, fmt.Errorf("unexpected output format")
	}

	used, err = strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parsing used: %w", err)
	}

	avail, err = strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parsing available: %w", err)
	}

	return used, avail, nil
}
