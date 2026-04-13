//go:build linux

package system

import (
	"syscall"

	"github.com/ldesfontaine/bientot/internal/transport"
)

func readDiskUsageSyscall() ([]transport.MetricPoint, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return nil, err
	}

	total := float64(stat.Blocks) * float64(stat.Bsize)
	free := float64(stat.Bfree) * float64(stat.Bsize)
	avail := float64(stat.Bavail) * float64(stat.Bsize)
	usedPct := 0.0
	if total > 0 {
		usedPct = (total - free) / total * 100
	}

	return []transport.MetricPoint{
		{Name: "system_disk_total_bytes", Value: total, Labels: map[string]string{"mount": "/"}},
		{Name: "system_disk_free_bytes", Value: free, Labels: map[string]string{"mount": "/"}},
		{Name: "system_disk_avail_bytes", Value: avail, Labels: map[string]string{"mount": "/"}},
		{Name: "system_disk_used_percent", Value: usedPct, Labels: map[string]string{"mount": "/"}},
	}, nil
}
