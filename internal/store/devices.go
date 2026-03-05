package store

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/plugin"
)

// UpsertPassiveDevice stores or updates a passively discovered device in the hosts table.
// The device ID is derived from its MAC address (or IP if no MAC).
// Common DNS suffixes to strip from discovered hostnames.
var dnsSuffixes = []string{
	".fritz.box",
	".local",
	".lan",
	".home",
	".localdomain",
	".internal",
}

func cleanHostname(name string) string {
	lower := strings.ToLower(name)
	for _, suffix := range dnsSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return name[:len(name)-len(suffix)]
		}
	}
	return name
}

func (s *Store) UpsertPassiveDevice(ctx context.Context, dev plugin.DiscoveredDevice) error {
	id := passiveDeviceID(dev)
	hostname := dev.Hostname
	if hostname == "" {
		hostname = dev.IP
	} else {
		hostname = cleanHostname(hostname)
	}

	ips := fmt.Sprintf(`["%s"]`, dev.IP)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO hosts (id, hostname, monitor_type, device_type, os, arch, platform, kernel,
			cpu_model, cpu_cores, memory_total, ip_addresses, mac_address, vendor,
			discovered_via, last_seen, status)
		VALUES (?, ?, 'passive', ?, '', '', '', '', '', 0, 0, ?, ?, ?, ?, datetime('now'), 'online')
		ON CONFLICT(id) DO UPDATE SET
			hostname = CASE WHEN ? != '' AND ? != ? THEN ? ELSE hosts.hostname END,
			device_type = CASE WHEN excluded.device_type != '' AND excluded.device_type != 'other' THEN excluded.device_type ELSE hosts.device_type END,
			ip_addresses = excluded.ip_addresses,
			mac_address = CASE WHEN excluded.mac_address != '' THEN excluded.mac_address ELSE hosts.mac_address END,
			vendor = CASE WHEN excluded.vendor != '' THEN excluded.vendor ELSE hosts.vendor END,
			last_seen = datetime('now'),
			status = 'online'
	`, id, hostname, deviceTypeOrDefault(dev.DeviceType),
		ips, dev.MAC, dev.Vendor, dev.Source,
		hostname, hostname, dev.IP, hostname)
	return err
}

// ListPassiveDevices returns all hosts with monitor_type = "passive".
func (s *Store) ListPassiveDevices(ctx context.Context) ([]models.Host, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, hostname, display_name, monitor_type, device_type, os, arch, platform, kernel,
			cpu_model, cpu_cores, memory_total, ip_addresses, mac_address, vendor,
			discovered_via, first_seen, last_seen, status
		FROM hosts WHERE monitor_type = 'passive' ORDER BY last_seen DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []models.Host
	for rows.Next() {
		var h models.Host
		var ips, firstSeen, lastSeen string
		if err := rows.Scan(&h.ID, &h.Hostname, &h.DisplayName, &h.MonitorType, &h.DeviceType, &h.OS, &h.Arch,
			&h.Platform, &h.Kernel, &h.CPUModel, &h.CPUCores, &h.MemoryTotal, &ips,
			&h.MACAddress, &h.Vendor, &h.DiscoveredVia, &firstSeen, &lastSeen, &h.Status); err != nil {
			return nil, err
		}
		h.IPAddresses = models.ParseIPAddresses(ips)
		h.FirstSeen = models.ParseDBTime(firstSeen)
		h.LastSeen = models.ParseDBTime(lastSeen)
		devices = append(devices, h)
	}
	return devices, nil
}

func passiveDeviceID(dev plugin.DiscoveredDevice) string {
	key := dev.MAC
	if key == "" {
		key = dev.IP
	}
	hash := sha256.Sum256([]byte("passive:" + key))
	return fmt.Sprintf("passive-%x", hash[:8])
}

func deviceTypeOrDefault(dt string) string {
	if dt == "" {
		return "other"
	}
	return dt
}
