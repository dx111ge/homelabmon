package observers

import (
	"bufio"
	"context"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/plugin"
	"github.com/shirou/gopsutil/v4/disk"
)

const hostfsRoot = "/hostfs"

type DiskObserver struct{}

var _ plugin.Observer = (*DiskObserver)(nil)

func (o *DiskObserver) Name() string                       { return "disk" }
func (o *DiskObserver) Type() models.PluginType            { return models.PluginTypeObserver }
func (o *DiskObserver) Detect() bool                       { return true }
func (o *DiskObserver) Interval() time.Duration            { return 30 * time.Second }
func (o *DiskObserver) Start(_ context.Context) error      { return nil }
func (o *DiskObserver) Stop() error                        { return nil }

func (o *DiskObserver) Collect(ctx context.Context) (*plugin.ObserverResult, error) {
	// If /hostfs is mounted (Docker container with host root bind-mount),
	// read the host's partitions and report host disk usage.
	if runtime.GOOS == "linux" {
		if _, err := os.Stat(hostfsRoot + "/proc/mounts"); err == nil {
			return o.collectFromHostFS(ctx)
		}
	}

	return o.collectNative(ctx)
}

// collectNative reads disk usage from the local filesystem (bare-metal or Windows).
func (o *DiskObserver) collectNative(ctx context.Context) (*plugin.ObserverResult, error) {
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

// collectFromHostFS reads disk usage from /hostfs (host root mounted read-only).
// It parses /hostfs/proc/mounts to find real host partitions and uses
// /hostfs-prefixed paths for stat calls, but reports the original mount paths.
func (o *DiskObserver) collectFromHostFS(_ context.Context) (*plugin.ObserverResult, error) {
	f, err := os.Open(hostfsRoot + "/proc/mounts")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var disks []models.DiskUsage
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		device, mountpoint, fstype := fields[0], fields[1], fields[2]

		if shouldSkipHostFSPartition(device, mountpoint, fstype) || seen[mountpoint] {
			continue
		}
		seen[mountpoint] = true

		// Stat the partition via /hostfs prefix
		hostPath := hostfsRoot + mountpoint
		usage, err := disk.Usage(hostPath)
		if err != nil {
			continue
		}
		if usage.Total == 0 {
			continue
		}

		disks = append(disks, models.DiskUsage{
			Path:        mountpoint, // report the real host path
			Fstype:      fstype,
			Total:       usage.Total,
			Used:        usage.Used,
			Free:        usage.Free,
			UsedPercent: usage.UsedPercent,
		})
	}

	return &plugin.ObserverResult{Disks: disks}, nil
}

func shouldSkipHostFSPartition(device, mountpoint, fstype string) bool {
	skipTypes := map[string]bool{
		"tmpfs": true, "devtmpfs": true, "devfs": true, "squashfs": true,
		"overlay": true, "aufs": true, "proc": true, "sysfs": true,
		"cgroup": true, "cgroup2": true, "securityfs": true, "debugfs": true,
		"tracefs": true, "configfs": true, "fusectl": true, "hugetlbfs": true,
		"mqueue": true, "pstore": true, "binfmt_misc": true, "autofs": true,
		"devpts": true, "nsfs": true, "rpc_pipefs": true, "nfsd": true,
	}
	if skipTypes[strings.ToLower(fstype)] {
		return true
	}
	if !strings.HasPrefix(device, "/") {
		return true
	}
	skipPrefixes := []string{"/proc", "/sys", "/dev", "/run", "/snap/"}
	for _, prefix := range skipPrefixes {
		if strings.HasPrefix(mountpoint, prefix) {
			return true
		}
	}
	return false
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
