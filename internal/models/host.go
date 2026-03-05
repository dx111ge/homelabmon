package models

import (
	"encoding/json"
	"time"
)

type Host struct {
	ID           string    `json:"id"`
	Hostname     string    `json:"hostname"`
	DisplayName  string    `json:"display_name,omitempty"`
	MonitorType  string    `json:"monitor_type"`  // agent, passive, integration
	DeviceType   string    `json:"device_type"`   // server, desktop, phone, tablet, tv, iot, router, etc.
	OS           string    `json:"os"`
	Arch         string    `json:"arch"`
	Platform     string    `json:"platform"`
	Kernel       string    `json:"kernel"`
	CPUModel     string    `json:"cpu_model"`
	CPUCores     int       `json:"cpu_cores"`
	MemoryTotal  uint64    `json:"memory_total"`
	IPAddresses  []string  `json:"ip_addresses"`
	MACAddress   string    `json:"mac_address"`
	Vendor       string    `json:"vendor"`
	DiscoveredVia string   `json:"discovered_via"` // agent, arp, mdns, snmp, unifi, homeassistant
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	Status       string    `json:"status"` // online, offline, unknown
}

// Label returns the display name if set, otherwise hostname.
func (h *Host) Label() string {
	if h.DisplayName != "" {
		return h.DisplayName
	}
	return h.Hostname
}

func (h *Host) IPAddressesJSON() string {
	b, _ := json.Marshal(h.IPAddresses)
	return string(b)
}

func ParseIPAddresses(s string) []string {
	var ips []string
	json.Unmarshal([]byte(s), &ips)
	return ips
}

// ParseDBTime parses a datetime string from SQLite, handling multiple formats.
func ParseDBTime(s string) time.Time {
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
