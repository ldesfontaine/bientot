package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

// Module reads backup status JSON files produced by backup-zfs.
// Expected format: { "type": "...", "timestamp": "...", "success": true/false,
//   "size_bytes": N, "checksum": "...", ... }
type Module struct {
	statusDir string // directory containing *-latest.json files
}

func New(statusDir string) *Module {
	return &Module{statusDir: statusDir}
}

func (m *Module) Name() string { return "backup" }

func (m *Module) Detect() bool {
	if m.statusDir == "" {
		return false
	}
	info, err := os.Stat(m.statusDir)
	return err == nil && info.IsDir()
}

func (m *Module) Collect(_ context.Context) (transport.ModuleData, error) {
	entries, err := os.ReadDir(m.statusDir)
	if err != nil {
		return transport.ModuleData{}, fmt.Errorf("reading status dir: %w", err)
	}

	now := time.Now()
	var metrics []transport.MetricPoint
	metadata := make(map[string]string)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}

		label := name[:len(name)-len(".json")] // e.g. "vps-latest"

		data, err := os.ReadFile(filepath.Join(m.statusDir, name))
		if err != nil {
			continue
		}

		var status map[string]interface{}
		if err := json.Unmarshal(data, &status); err != nil {
			continue
		}

		labels := map[string]string{"backup": label}

		// success
		if success, ok := status["success"].(bool); ok {
			v := 0.0
			if success {
				v = 1.0
			}
			metrics = append(metrics, transport.MetricPoint{
				Name: "backup_success", Value: v, Labels: labels,
			})
		}

		// size_bytes
		if size, ok := status["size_bytes"].(float64); ok {
			metrics = append(metrics, transport.MetricPoint{
				Name: "backup_size_bytes", Value: size, Labels: labels,
			})
		}

		// age from timestamp
		if tsStr, ok := status["timestamp"].(string); ok {
			if ts, err := time.Parse(time.RFC3339, tsStr); err == nil {
				age := now.Sub(ts)
				metrics = append(metrics, transport.MetricPoint{
					Name: "backup_age_hours", Value: age.Hours(), Labels: labels,
				})
			}
		}

		// days_since_last (USB backups)
		if days, ok := status["days_since_last"].(float64); ok {
			metrics = append(metrics, transport.MetricPoint{
				Name: "backup_days_since_last", Value: days, Labels: labels,
			})
		}

		// vps_disk_remaining_gb
		if remain, ok := status["vps_disk_remaining_gb"].(float64); ok {
			metrics = append(metrics, transport.MetricPoint{
				Name: "backup_disk_remaining_gb", Value: remain, Labels: labels,
			})
		}

		// type as metadata
		if t, ok := status["type"].(string); ok {
			metadata["type_"+label] = t
		}
	}

	return transport.ModuleData{
		Module:    "backup",
		Metrics:   metrics,
		Metadata:  metadata,
		Timestamp: now,
	}, nil
}
