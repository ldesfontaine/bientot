package zfs

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

// Module collects ZFS pool metrics via zpool CLI.
type Module struct {
	pools []string // if empty, auto-detect all pools
}

func New(pools []string) *Module {
	return &Module{pools: pools}
}

func (m *Module) Name() string { return "zfs" }

func (m *Module) Detect() bool {
	_, err := exec.LookPath("zpool")
	return err == nil
}

func (m *Module) Collect(ctx context.Context) (transport.ModuleData, error) {
	pools := m.pools
	if len(pools) == 0 {
		var err error
		pools, err = listPools(ctx)
		if err != nil {
			return transport.ModuleData{}, fmt.Errorf("listing pools: %w", err)
		}
	}

	now := time.Now()
	var metrics []transport.MetricPoint

	for _, pool := range pools {
		labels := map[string]string{"pool": pool}

		health, err := poolHealth(ctx, pool)
		if err != nil {
			return transport.ModuleData{}, fmt.Errorf("pool %s health: %w", pool, err)
		}
		metrics = append(metrics, transport.MetricPoint{
			Name: "zfs_pool_health", Value: health, Labels: labels,
		})

		used, avail, err := poolSpace(ctx, pool)
		if err != nil {
			return transport.ModuleData{}, fmt.Errorf("pool %s space: %w", pool, err)
		}
		metrics = append(metrics,
			transport.MetricPoint{Name: "zfs_pool_used_bytes", Value: used, Labels: labels},
			transport.MetricPoint{Name: "zfs_pool_available_bytes", Value: avail, Labels: labels},
		)
		total := used + avail
		if total > 0 {
			metrics = append(metrics, transport.MetricPoint{
				Name: "zfs_pool_used_percent", Value: (used / total) * 100, Labels: labels,
			})
		}

		snapCount, err := snapshotCount(ctx, pool)
		if err == nil {
			metrics = append(metrics, transport.MetricPoint{
				Name: "zfs_snapshots_count", Value: float64(snapCount), Labels: labels,
			})
		}
	}

	return transport.ModuleData{
		Module:    "zfs",
		Metrics:   metrics,
		Timestamp: now,
	}, nil
}

func listPools(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "zpool", "list", "-Ho", "name")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	var pools []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			pools = append(pools, line)
		}
	}
	return pools, nil
}

func poolHealth(ctx context.Context, pool string) (float64, error) {
	cmd := exec.CommandContext(ctx, "zpool", "status", "-x", pool)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0, err
	}
	output := out.String()
	switch {
	case strings.Contains(output, "is healthy"):
		return 2, nil
	case strings.Contains(output, "DEGRADED"):
		return 1, nil
	default:
		return 0, nil
	}
}

func poolSpace(ctx context.Context, pool string) (used, avail float64, err error) {
	cmd := exec.CommandContext(ctx, "zpool", "list", "-Hp", "-o", "alloc,free", pool)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(strings.TrimSpace(out.String()))
	if len(fields) < 2 {
		return 0, 0, fmt.Errorf("unexpected zpool output")
	}
	used, err = strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, 0, err
	}
	avail, err = strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 0, err
	}
	return used, avail, nil
}

func snapshotCount(ctx context.Context, pool string) (int, error) {
	cmd := exec.CommandContext(ctx, "zfs", "list", "-t", "snapshot", "-Ho", "name", "-r", pool)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0, err
	}
	output := strings.TrimSpace(out.String())
	if output == "" {
		return 0, nil
	}
	return len(strings.Split(output, "\n")), nil
}
