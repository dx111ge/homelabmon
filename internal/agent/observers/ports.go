package observers

import (
	"context"
	"time"

	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/plugin"
	"github.com/shirou/gopsutil/v4/net"
)

// ListenPort represents a listening TCP/UDP port.
type ListenPort struct {
	Port    uint32
	Proto   string // tcp, udp
	PID     int32
	Process string
}

// PortsObserver collects listening ports on the local machine.
type PortsObserver struct {
	latest []ListenPort
}

var _ plugin.Observer = (*PortsObserver)(nil)

func (o *PortsObserver) Name() string                  { return "ports" }
func (o *PortsObserver) Type() models.PluginType       { return models.PluginTypeObserver }
func (o *PortsObserver) Detect() bool                  { return true }
func (o *PortsObserver) Interval() time.Duration       { return 60 * time.Second }
func (o *PortsObserver) Start(_ context.Context) error { return nil }
func (o *PortsObserver) Stop() error                   { return nil }

// Latest returns the most recently collected listening ports.
func (o *PortsObserver) Latest() []ListenPort { return o.latest }

func (o *PortsObserver) Collect(ctx context.Context) (*plugin.ObserverResult, error) {
	conns, err := net.ConnectionsWithContext(ctx, "all")
	if err != nil {
		return nil, err
	}

	// Deduplicate by port+proto, keep only LISTEN state
	seen := make(map[uint64]bool)
	var ports []ListenPort
	for _, c := range conns {
		if c.Status != "LISTEN" {
			continue
		}
		key := uint64(c.Laddr.Port)<<1 | protoFlag(c.Type)
		if seen[key] {
			continue
		}
		seen[key] = true
		ports = append(ports, ListenPort{
			Port:  c.Laddr.Port,
			Proto: connType(c.Type),
			PID:   c.Pid,
		})
	}

	o.latest = ports

	// Ports observer returns no metric data -- it stores results for the discovery engine
	return &plugin.ObserverResult{
		Extra: map[string]interface{}{"listen_ports": ports},
	}, nil
}

func connType(t uint32) string {
	// gopsutil: 1=tcp, 2=udp
	if t == 2 {
		return "udp"
	}
	return "tcp"
}

func protoFlag(t uint32) uint64 {
	if t == 2 {
		return 1
	}
	return 0
}
