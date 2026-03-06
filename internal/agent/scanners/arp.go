package scanners

import (
	"bufio"
	"context"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/plugin"
)

// ARPScanner discovers devices on the LAN by reading the ARP table.
type ARPScanner struct{}

var _ plugin.Scanner = (*ARPScanner)(nil)

func (s *ARPScanner) Name() string                  { return "arp" }
func (s *ARPScanner) Type() models.PluginType       { return models.PluginTypeScanner }
func (s *ARPScanner) Detect() bool                  { return true }
func (s *ARPScanner) Interval() time.Duration       { return 2 * time.Minute }
func (s *ARPScanner) Start(_ context.Context) error { return nil }
func (s *ARPScanner) Stop() error                   { return nil }

func (s *ARPScanner) Scan(ctx context.Context) ([]plugin.DiscoveredDevice, error) {
	if runtime.GOOS == "linux" {
		return scanARPLinux(ctx)
	}
	return scanARPCommand(ctx)
}

// scanARPLinux reads /proc/net/arp directly (no subprocess).
func scanARPLinux(_ context.Context) ([]plugin.DiscoveredDevice, error) {
	f, err := openProcNetARP()
	if err != nil {
		return scanARPCommand(context.Background())
	}
	defer f.Close()

	var devices []plugin.DiscoveredDevice
	scanner := bufio.NewScanner(f)
	scanner.Scan() // skip header line

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 6 {
			continue
		}
		ip := fields[0]
		mac := fields[3]
		if mac == "00:00:00:00:00:00" || mac == "" {
			continue
		}
		// Resolve hostname (throttled to avoid DNS flood)
		hostname := ""
		names, err := net.LookupAddr(ip)
		if err == nil && len(names) > 0 {
			hostname = strings.TrimSuffix(names[0], ".")
		}
		devices = append(devices, plugin.DiscoveredDevice{
			MAC:      normalizeMAC(mac),
			IP:       ip,
			Hostname: hostname,
			Source:   "arp",
		})
		time.Sleep(50 * time.Millisecond) // throttle DNS lookups
	}
	return devices, nil
}

// arp -a regex: matches "IP (x.x.x.x) at aa:bb:cc:dd:ee:ff" or "x.x.x.x  aa-bb-cc-dd-ee-ff"
var arpLineRe = regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+)\s+.*?([0-9a-fA-F]{2}[:\-][0-9a-fA-F]{2}[:\-][0-9a-fA-F]{2}[:\-][0-9a-fA-F]{2}[:\-][0-9a-fA-F]{2}[:\-][0-9a-fA-F]{2})`)

// scanARPCommand runs `arp -a` and parses the output (Windows/macOS/fallback).
func scanARPCommand(ctx context.Context) ([]plugin.DiscoveredDevice, error) {
	cmd := exec.CommandContext(ctx, "arp", "-a")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var devices []plugin.DiscoveredDevice
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		matches := arpLineRe.FindStringSubmatch(scanner.Text())
		if len(matches) < 3 {
			continue
		}
		ip := matches[1]
		mac := normalizeMAC(matches[2])
		if mac == "ff:ff:ff:ff:ff:ff" || mac == "00:00:00:00:00:00" {
			continue
		}
		hostname := ""
		names, err := net.LookupAddr(ip)
		if err == nil && len(names) > 0 {
			hostname = strings.TrimSuffix(names[0], ".")
		}
		devices = append(devices, plugin.DiscoveredDevice{
			MAC:      mac,
			IP:       ip,
			Hostname: hostname,
			Source:   "arp",
		})
		time.Sleep(50 * time.Millisecond) // throttle DNS lookups
	}
	return devices, nil
}

// normalizeMAC converts MAC address to lowercase colon-separated format.
func normalizeMAC(mac string) string {
	mac = strings.ToLower(strings.ReplaceAll(mac, "-", ":"))
	// Ensure zero-padded octets (e.g., "a:b:c" -> "0a:0b:0c")
	parts := strings.Split(mac, ":")
	for i, p := range parts {
		if len(p) == 1 {
			parts[i] = "0" + p
		}
	}
	return strings.Join(parts, ":")
}
