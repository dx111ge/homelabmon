package plugin

import "time"

// Event is emitted by Probes during collection.
type Event struct {
	Probe     string                 `json:"probe"`
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Severity  string                 `json:"severity"` // info, warning, critical
	Data      map[string]interface{} `json:"data"`
}

// DiscoveredDevice is returned by Scanners and Integrations.
type DiscoveredDevice struct {
	MAC        string `json:"mac"`
	IP         string `json:"ip"`
	Hostname   string `json:"hostname"`
	Vendor     string `json:"vendor"`
	DeviceType string `json:"device_type"` // phone, tablet, tv, iot, printer, etc.
	Source     string `json:"source"`      // arp, mdns, snmp, unifi, homeassistant
}
