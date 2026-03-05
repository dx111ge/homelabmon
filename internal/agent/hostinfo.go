package agent

import (
	"context"
	"fmt"
	"net"
	"runtime"

	"github.com/google/uuid"
	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/shirou/gopsutil/v4/cpu"
	gopshost "github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

// CollectHostInfo gathers static host details. Called once at startup.
func CollectHostInfo(ctx context.Context, existingID string) (*models.Host, error) {
	hostInfo, err := gopshost.InfoWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("host info: %w", err)
	}

	cpuInfo, err := cpu.InfoWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("cpu info: %w", err)
	}
	cpuModel := ""
	if len(cpuInfo) > 0 {
		cpuModel = cpuInfo[0].ModelName
	}

	cores, _ := cpu.CountsWithContext(ctx, true)

	nodeID := existingID
	if nodeID == "" {
		nodeID = uuid.New().String()
	}

	ips := collectIPAddresses()

	var memTotal uint64
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		memTotal = vm.Total
	}

	return &models.Host{
		ID:            nodeID,
		Hostname:      hostInfo.Hostname,
		MonitorType:   "agent",
		DeviceType:    "server",
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		Platform:      fmt.Sprintf("%s %s", hostInfo.Platform, hostInfo.PlatformVersion),
		Kernel:        hostInfo.KernelVersion,
		CPUModel:      cpuModel,
		CPUCores:      cores,
		MemoryTotal:   memTotal,
		IPAddresses:   ips,
		DiscoveredVia: "agent",
		Status:        "online",
	}, nil
}

func collectIPAddresses() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					ips = append(ips, ipnet.IP.String())
				}
			}
		}
	}
	return ips
}
