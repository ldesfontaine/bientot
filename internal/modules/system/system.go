package system

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

// Module collects basic system metrics from /proc (Linux).
type Module struct{}

func New() *Module { return &Module{} }

func (m *Module) Name() string { return "system" }

func (m *Module) Detect() bool { return runtime.GOOS == "linux" }

func (m *Module) Collect(_ context.Context) (transport.ModuleData, error) {
	now := time.Now()
	var metrics []transport.MetricPoint

	if cpu, err := readCPU(); err == nil {
		metrics = append(metrics, cpu...)
	}
	if mem, err := readMemory(); err == nil {
		metrics = append(metrics, mem...)
	}
	if load, err := readLoadAvg(); err == nil {
		metrics = append(metrics, load...)
	}
	if disk, err := readDiskUsage(); err == nil {
		metrics = append(metrics, disk...)
	}
	if uptime, err := readUptime(); err == nil {
		metrics = append(metrics, uptime)
	}

	hostname, _ := os.Hostname()

	return transport.ModuleData{
		Module:    "system",
		Metrics:   metrics,
		Metadata:  map[string]string{"hostname": hostname},
		Timestamp: now,
	}, nil
}

// readCPU parses /proc/stat for aggregate CPU usage.
func readCPU() ([]transport.MetricPoint, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			return nil, fmt.Errorf("unexpected /proc/stat format")
		}
		// user, nice, system, idle, iowait, irq, softirq
		user, _ := strconv.ParseFloat(fields[1], 64)
		nice, _ := strconv.ParseFloat(fields[2], 64)
		system, _ := strconv.ParseFloat(fields[3], 64)
		idle, _ := strconv.ParseFloat(fields[4], 64)
		iowait, _ := strconv.ParseFloat(fields[5], 64)

		total := user + nice + system + idle + iowait
		usedPct := 0.0
		if total > 0 {
			usedPct = (total - idle - iowait) / total * 100
		}

		return []transport.MetricPoint{
			{Name: "system_cpu_usage_percent", Value: usedPct},
			{Name: "system_cpu_user", Value: user},
			{Name: "system_cpu_system", Value: system},
			{Name: "system_cpu_idle", Value: idle},
			{Name: "system_cpu_iowait", Value: iowait},
		}, nil
	}
	return nil, fmt.Errorf("cpu line not found in /proc/stat")
}

// readMemory parses /proc/meminfo.
func readMemory() ([]transport.MetricPoint, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	values := make(map[string]float64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, " kB")
		valStr = strings.TrimSpace(valStr)
		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		values[key] = val * 1024 // kB -> bytes
	}

	total := values["MemTotal"]
	available := values["MemAvailable"]
	usedPct := 0.0
	if total > 0 {
		usedPct = (total - available) / total * 100
	}

	return []transport.MetricPoint{
		{Name: "system_memory_total_bytes", Value: total},
		{Name: "system_memory_available_bytes", Value: available},
		{Name: "system_memory_used_percent", Value: usedPct},
		{Name: "system_swap_total_bytes", Value: values["SwapTotal"]},
		{Name: "system_swap_free_bytes", Value: values["SwapFree"]},
	}, nil
}

// readLoadAvg parses /proc/loadavg.
func readLoadAvg() ([]transport.MetricPoint, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return nil, fmt.Errorf("unexpected /proc/loadavg format")
	}

	load1, _ := strconv.ParseFloat(fields[0], 64)
	load5, _ := strconv.ParseFloat(fields[1], 64)
	load15, _ := strconv.ParseFloat(fields[2], 64)

	return []transport.MetricPoint{
		{Name: "system_load_1", Value: load1},
		{Name: "system_load_5", Value: load5},
		{Name: "system_load_15", Value: load15},
	}, nil
}

// readDiskUsage reads root filesystem usage via /proc/mounts + syscall.
func readDiskUsage() ([]transport.MetricPoint, error) {
	return readDiskUsageSyscall()
}

// readUptime parses /proc/uptime.
func readUptime() (transport.MetricPoint, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return transport.MetricPoint{}, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return transport.MetricPoint{}, fmt.Errorf("unexpected /proc/uptime format")
	}
	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return transport.MetricPoint{}, err
	}
	return transport.MetricPoint{Name: "system_uptime_seconds", Value: uptime}, nil
}
