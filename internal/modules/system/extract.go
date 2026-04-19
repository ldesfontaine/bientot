package system

import (
	"time"

	"github.com/ldesfontaine/bientot/internal/modules"
	"github.com/ldesfontaine/bientot/internal/shared/promparse"
)

// Extract pulls the 14 Bientot system metrics from raw node_exporter samples.
// Metrics that are not found in the input are silently omitted — the module
// caller can decide whether a partial set is acceptable.
//
// `now` is the reference time for computing uptime; pass time.Now() in
// production, a fixed value in tests.
func Extract(samples []promparse.Sample, now time.Time) []modules.Metric {
	idx := indexSamples(samples)

	var out []modules.Metric

	simpleMap := map[string]string{
		"node_memory_MemTotal_bytes":     "memory_total_bytes",
		"node_memory_MemAvailable_bytes": "memory_available_bytes",
		"node_memory_MemFree_bytes":      "memory_free_bytes",
		"node_memory_SwapTotal_bytes":    "swap_total_bytes",
		"node_memory_SwapFree_bytes":     "swap_free_bytes",
		"node_load1":                     "load_average_1m",
		"node_load5":                     "load_average_5m",
		"node_load15":                    "load_average_15m",
	}
	for src, dst := range simpleMap {
		if vs, ok := idx[src]; ok && len(vs) > 0 {
			out = append(out, modules.Metric{Name: dst, Value: vs[0].Value})
		}
	}

	cpuSamples := idx["node_cpu_seconds_total"]
	for _, mode := range []string{"user", "system", "idle", "iowait"} {
		var sum float64
		found := false
		for _, s := range cpuSamples {
			if s.Labels["mode"] == mode {
				sum += s.Value
				found = true
			}
		}
		if found {
			out = append(out, modules.Metric{
				Name:  "cpu_" + mode + "_seconds_total",
				Value: sum,
			})
		}
	}

	if len(cpuSamples) > 0 {
		cpus := make(map[string]struct{})
		for _, s := range cpuSamples {
			if s.Labels["mode"] == "idle" {
				cpus[s.Labels["cpu"]] = struct{}{}
			}
		}
		if len(cpus) > 0 {
			out = append(out, modules.Metric{
				Name:  "cpu_cores",
				Value: float64(len(cpus)),
			})
		}
	}

	for _, s := range idx["node_filesystem_size_bytes"] {
		if s.Labels["mountpoint"] == "/" {
			out = append(out, modules.Metric{Name: "filesystem_size_bytes", Value: s.Value})
			break
		}
	}
	for _, s := range idx["node_filesystem_avail_bytes"] {
		if s.Labels["mountpoint"] == "/" {
			out = append(out, modules.Metric{Name: "filesystem_avail_bytes", Value: s.Value})
			break
		}
	}

	if vs, ok := idx["node_boot_time_seconds"]; ok && len(vs) > 0 {
		uptime := now.Unix() - int64(vs[0].Value)
		if uptime > 0 {
			out = append(out, modules.Metric{
				Name:  "uptime_seconds",
				Value: float64(uptime),
			})
		}
	}

	return out
}

// indexSamples groups samples by metric name for O(1) lookup per name.
func indexSamples(samples []promparse.Sample) map[string][]promparse.Sample {
	idx := make(map[string][]promparse.Sample)
	for _, s := range samples {
		idx[s.Name] = append(idx[s.Name], s)
	}
	return idx
}
