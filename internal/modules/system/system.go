package system

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

// Module collecte les métriques système depuis l'endpoint Prometheus de node-exporter.
// Remplace l'accès direct à /proc pour que l'agent puisse tourner dans un conteneur.
type Module struct {
	url    string // e.g. "http://node-exporter:9100"
	client *http.Client
}

func New(nodeExporterURL string) *Module {
	return &Module{
		url: nodeExporterURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (m *Module) Name() string { return "system" }

func (m *Module) Detect() bool {
	if m.url == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", m.url+"/metrics", nil)
	if err != nil {
		return false
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (m *Module) Collect(ctx context.Context) (transport.ModuleData, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", m.url+"/metrics", nil)
	if err != nil {
		return transport.ModuleData{}, fmt.Errorf("création de la requête : %w", err)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		slog.Warn("node-exporter unreachable", "url", m.url, "error", err)
		return transport.ModuleData{
			Module:    "system",
			Metrics:   []transport.MetricPoint{},
			Timestamp: time.Now(),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("node-exporter returned error", "status", resp.StatusCode)
		return transport.ModuleData{
			Module:    "system",
			Metrics:   []transport.MetricPoint{},
			Timestamp: time.Now(),
		}, nil
	}

	all := parsePrometheusMetrics(resp.Body)
	now := time.Now()

	var metrics []transport.MetricPoint
	metadata := make(map[string]string)

	metrics = append(metrics, extractCPU(all)...)
	metrics = append(metrics, extractMemory(all)...)
	metrics = append(metrics, extractLoad(all)...)
	metrics = append(metrics, extractDisk(all)...)
	metrics = append(metrics, extractTemperature(all)...)
	metrics = append(metrics, extractZFS(all)...)

	extractUname(all, metadata)

	hostname, _ := os.Hostname()
	metadata["hostname"] = hostname

	return transport.ModuleData{
		Module:    "system",
		Metrics:   metrics,
		Metadata:  metadata,
		Timestamp: now,
	}, nil
}

// extractCPU calcule l'utilisation CPU depuis node_cpu_seconds_total.
func extractCPU(all []promMetric) []transport.MetricPoint {
	var user, system, idle, iowait, total float64
	for _, m := range all {
		if m.Name != "node_cpu_seconds_total" {
			continue
		}
		mode := m.Labels["mode"]
		total += m.Value
		switch mode {
		case "user":
			user += m.Value
		case "system":
			system += m.Value
		case "idle":
			idle += m.Value
		case "iowait":
			iowait += m.Value
		}
	}
	if total == 0 {
		return nil
	}
	usedPct := (total - idle - iowait) / total * 100

	return []transport.MetricPoint{
		{Name: "system_cpu_usage_percent", Value: usedPct},
		{Name: "system_cpu_user", Value: user},
		{Name: "system_cpu_system", Value: system},
		{Name: "system_cpu_idle", Value: idle},
		{Name: "system_cpu_iowait", Value: iowait},
	}
}

// extractMemory lit les métriques node_memory_*.
func extractMemory(all []promMetric) []transport.MetricPoint {
	vals := make(map[string]float64)
	for _, m := range all {
		switch m.Name {
		case "node_memory_MemTotal_bytes":
			vals["total"] = m.Value
		case "node_memory_MemAvailable_bytes":
			vals["available"] = m.Value
		case "node_memory_SwapTotal_bytes":
			vals["swap_total"] = m.Value
		case "node_memory_SwapFree_bytes":
			vals["swap_free"] = m.Value
		}
	}

	total := vals["total"]
	available := vals["available"]
	usedPct := 0.0
	if total > 0 {
		usedPct = (total - available) / total * 100
	}

	return []transport.MetricPoint{
		{Name: "system_memory_total_bytes", Value: total},
		{Name: "system_memory_available_bytes", Value: available},
		{Name: "system_memory_used_percent", Value: usedPct},
		{Name: "system_swap_total_bytes", Value: vals["swap_total"]},
		{Name: "system_swap_free_bytes", Value: vals["swap_free"]},
	}
}

// extractLoad lit node_load1, node_load5, node_load15.
func extractLoad(all []promMetric) []transport.MetricPoint {
	var load1, load5, load15 float64
	var found bool
	for _, m := range all {
		switch m.Name {
		case "node_load1":
			load1 = m.Value
			found = true
		case "node_load5":
			load5 = m.Value
		case "node_load15":
			load15 = m.Value
		}
	}
	if !found {
		return nil
	}
	return []transport.MetricPoint{
		{Name: "system_load_1", Value: load1},
		{Name: "system_load_5", Value: load5},
		{Name: "system_load_15", Value: load15},
	}
}

// extractDisk lit les métriques node_filesystem_* pour les points de montage réels.
func extractDisk(all []promMetric) []transport.MetricPoint {
	type diskInfo struct {
		total, avail float64
	}
	disks := make(map[string]*diskInfo)

	for _, m := range all {
		mount := m.Labels["mountpoint"]
		if mount == "" {
			continue
		}
		// Ignorer les systèmes de fichiers virtuels
		fstype := m.Labels["fstype"]
		if fstype == "tmpfs" || fstype == "devtmpfs" || fstype == "overlay" {
			continue
		}

		if _, ok := disks[mount]; !ok {
			disks[mount] = &diskInfo{}
		}
		switch m.Name {
		case "node_filesystem_size_bytes":
			disks[mount].total = m.Value
		case "node_filesystem_avail_bytes":
			disks[mount].avail = m.Value
		}
	}

	var metrics []transport.MetricPoint
	for mount, d := range disks {
		if d.total == 0 {
			continue
		}
		labels := map[string]string{"mount": mount}
		free := d.avail
		usedPct := (d.total - free) / d.total * 100

		metrics = append(metrics,
			transport.MetricPoint{Name: "system_disk_total_bytes", Value: d.total, Labels: labels},
			transport.MetricPoint{Name: "system_disk_free_bytes", Value: free, Labels: labels},
			transport.MetricPoint{Name: "system_disk_avail_bytes", Value: d.avail, Labels: labels},
			transport.MetricPoint{Name: "system_disk_used_percent", Value: usedPct, Labels: labels},
		)
	}
	return metrics
}

// extractTemperature lit node_hwmon_temp_celsius si disponible.
func extractTemperature(all []promMetric) []transport.MetricPoint {
	var metrics []transport.MetricPoint
	for _, m := range all {
		if m.Name != "node_hwmon_temp_celsius" {
			continue
		}
		labels := make(map[string]string)
		if chip := m.Labels["chip"]; chip != "" {
			labels["chip"] = chip
		}
		if sensor := m.Labels["sensor"]; sensor != "" {
			labels["sensor"] = sensor
		}
		metrics = append(metrics, transport.MetricPoint{
			Name: "system_temperature_celsius", Value: m.Value, Labels: labels,
		})
	}
	return metrics
}

// extractZFS lit les métriques node_zfs_zpool_* si le collecteur ZFS est activé.
func extractZFS(all []promMetric) []transport.MetricPoint {
	var metrics []transport.MetricPoint
	for _, m := range all {
		switch m.Name {
		case "node_zfs_zpool_state":
			pool := m.Labels["zpool"]
			state := m.Labels["state"]
			if pool == "" || state == "" {
				continue
			}
			// le label state est "online", "degraded", etc. Value=1 signifie l'état actif.
			if m.Value != 1 {
				continue
			}
			health := 0.0
			switch strings.ToLower(state) {
			case "online":
				health = 2
			case "degraded":
				health = 1
			}
			metrics = append(metrics, transport.MetricPoint{
				Name: "zfs_pool_health", Value: health,
				Labels: map[string]string{"pool": pool},
			})

		case "node_zfs_zpool_allocated_bytes":
			pool := m.Labels["zpool"]
			if pool == "" {
				continue
			}
			metrics = append(metrics, transport.MetricPoint{
				Name: "zfs_pool_used_bytes", Value: m.Value,
				Labels: map[string]string{"pool": pool},
			})

		case "node_zfs_zpool_size_bytes":
			pool := m.Labels["zpool"]
			if pool == "" {
				continue
			}
			metrics = append(metrics, transport.MetricPoint{
				Name: "zfs_pool_size_bytes", Value: m.Value,
				Labels: map[string]string{"pool": pool},
			})
		}
	}

	// Calcul de la disponibilité et du pourcentage utilisé depuis la taille et l'allocation
	poolSizes := make(map[string]float64)
	poolUsed := make(map[string]float64)
	for _, m := range metrics {
		pool := m.Labels["pool"]
		switch m.Name {
		case "zfs_pool_size_bytes":
			poolSizes[pool] = m.Value
		case "zfs_pool_used_bytes":
			poolUsed[pool] = m.Value
		}
	}
	for pool, size := range poolSizes {
		used := poolUsed[pool]
		avail := size - used
		metrics = append(metrics, transport.MetricPoint{
			Name: "zfs_pool_available_bytes", Value: avail,
			Labels: map[string]string{"pool": pool},
		})
		if size > 0 {
			metrics = append(metrics, transport.MetricPoint{
				Name: "zfs_pool_used_percent", Value: (used / size) * 100,
				Labels: map[string]string{"pool": pool},
			})
		}
	}

	return metrics
}

// extractUname lit les labels node_uname_info dans les métadonnées.
func extractUname(all []promMetric, metadata map[string]string) {
	for _, m := range all {
		if m.Name != "node_uname_info" {
			continue
		}
		if v := m.Labels["sysname"]; v != "" {
			metadata["os"] = v
		}
		if v := m.Labels["release"]; v != "" {
			metadata["kernel"] = v
		}
		if v := m.Labels["machine"]; v != "" {
			metadata["arch"] = v
		}
		if v := m.Labels["nodename"]; v != "" {
			metadata["nodename"] = v
		}
		return
	}
}
