package plugin

import (
	"context"
	"encoding/json"
	"time"

	"github.com/dx111ge/homelabmon/internal/models"
)

// Plugin is the base interface for all plugin types.
type Plugin interface {
	Name() string
	Type() models.PluginType
	Detect() bool
	Interval() time.Duration
	Start(ctx context.Context) error
	Stop() error
}

// Observer collects local system metrics (CPU, memory, disk, network).
type Observer interface {
	Plugin
	Collect(ctx context.Context) (*ObserverResult, error)
}

// ObserverResult holds partial metrics from a single observer.
// Only non-nil fields are merged into the final MetricSnapshot.
type ObserverResult struct {
	CPUPercent   *float64
	Load1        *float64
	Load5        *float64
	Load15       *float64
	MemTotal     *uint64
	MemUsed      *uint64
	MemPercent   *float64
	SwapTotal    *uint64
	SwapUsed     *uint64
	Disks        []models.DiskUsage
	NetBytesSent *uint64
	NetBytesRecv *uint64
	Extra        map[string]interface{}
}

// Probe monitors a specific service (Docker, PostgreSQL, etc.).
type Probe interface {
	Plugin
	Collect(ctx context.Context) ([]Event, error)
	Query(method string, params json.RawMessage) (any, error)
}

// Scanner discovers passive network devices (ARP, mDNS, SNMP, ping).
type Scanner interface {
	Plugin
	Scan(ctx context.Context) ([]DiscoveredDevice, error)
}

// Integration pulls data from external APIs (Unifi, Home Assistant, etc.).
type Integration interface {
	Plugin
	Sync(ctx context.Context) ([]DiscoveredDevice, error)
	Configure(config map[string]string) error
}
