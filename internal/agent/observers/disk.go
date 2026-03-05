package observers

import (
	"context"
	"runtime"
	"strings"
	"time"

	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/plugin"
	"github.com/shirou/gopsutil/v4/disk"
)

type DiskObserver struct{}

var _ plugin.Observer = (*DiskObserver)(nil)

func (o *DiskObserver) Name() string                       { return "disk" }
func (o *DiskObserver) Type() models.PluginType            { return models.PluginTypeObserver }
func (o *DiskObserver) Detect() bool                       { return true }
func (o *DiskObserver) Interval() time.Duration            { return 30 * time.Second }
func (o *DiskObserver) Start(_ context.Context) error      { return nil }
func (o *DiskObserver) Stop() error                        { return nil }

func (o *DiskObserver) Collect(ctx context.Context) (*plugin.ObserverResult, error) {
	partitions, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return nil, err
	}

	var disks []models.DiskUsage
	seen := make(map[string]bool)

	for _, p := range partitions {
		if shouldSkipPartition(p) || seen[p.Mountpoint] {
			continue
		}
		seen[p.Mountpoint] = true

		usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			continue
		}

		disks = append(disks, models.DiskUsage{
			Path:        p.Mountpoint,
			Fstype:      p.Fstype,
			Total:       usage.Total,
			Used:        usage.Used,
			Free:        usage.Free,
			UsedPercent: usage.UsedPercent,
		})
	}

	return &plugin.ObserverResult{Disks: disks}, nil
}

func shouldSkipPartition(p disk.PartitionStat) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	skipTypes := []string{"tmpfs", "devtmpfs", "devfs", "squashfs", "overlay", "aufs", "proc", "sysfs", "cgroup"}
	for _, st := range skipTypes {
		if strings.EqualFold(p.Fstype, st) {
			return true
		}
	}
	if strings.HasPrefix(p.Mountpoint, "/snap/") {
		return true
	}
	return false
}
