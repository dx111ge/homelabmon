package observers

import (
	"context"
	"time"

	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/plugin"
	psnet "github.com/shirou/gopsutil/v4/net"
)

type NetworkObserver struct {
	prevBytesSent uint64
	prevBytesRecv uint64
	initialized   bool
}

var _ plugin.Observer = (*NetworkObserver)(nil)

func (o *NetworkObserver) Name() string                       { return "network" }
func (o *NetworkObserver) Type() models.PluginType            { return models.PluginTypeObserver }
func (o *NetworkObserver) Detect() bool                       { return true }
func (o *NetworkObserver) Interval() time.Duration            { return 30 * time.Second }
func (o *NetworkObserver) Start(_ context.Context) error      { return nil }
func (o *NetworkObserver) Stop() error                        { return nil }

func (o *NetworkObserver) Collect(ctx context.Context) (*plugin.ObserverResult, error) {
	counters, err := psnet.IOCountersWithContext(ctx, false)
	if err != nil {
		return nil, err
	}
	if len(counters) == 0 {
		return &plugin.ObserverResult{}, nil
	}

	all := counters[0]

	if !o.initialized {
		o.prevBytesSent = all.BytesSent
		o.prevBytesRecv = all.BytesRecv
		o.initialized = true
		zero := uint64(0)
		return &plugin.ObserverResult{
			NetBytesSent: &zero,
			NetBytesRecv: &zero,
		}, nil
	}

	sent := all.BytesSent - o.prevBytesSent
	recv := all.BytesRecv - o.prevBytesRecv
	o.prevBytesSent = all.BytesSent
	o.prevBytesRecv = all.BytesRecv

	return &plugin.ObserverResult{
		NetBytesSent: &sent,
		NetBytesRecv: &recv,
	}, nil
}
