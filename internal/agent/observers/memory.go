package observers

import (
	"context"
	"time"

	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/plugin"
	"github.com/shirou/gopsutil/v4/mem"
)

type MemoryObserver struct{}

var _ plugin.Observer = (*MemoryObserver)(nil)

func (o *MemoryObserver) Name() string                       { return "memory" }
func (o *MemoryObserver) Type() models.PluginType            { return models.PluginTypeObserver }
func (o *MemoryObserver) Detect() bool                       { return true }
func (o *MemoryObserver) Interval() time.Duration            { return 30 * time.Second }
func (o *MemoryObserver) Start(_ context.Context) error      { return nil }
func (o *MemoryObserver) Stop() error                        { return nil }

func (o *MemoryObserver) Collect(ctx context.Context) (*plugin.ObserverResult, error) {
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, err
	}

	sw, _ := mem.SwapMemoryWithContext(ctx)
	if sw == nil {
		sw = &mem.SwapMemoryStat{}
	}

	return &plugin.ObserverResult{
		MemTotal:  &vm.Total,
		MemUsed:   &vm.Used,
		MemPercent: &vm.UsedPercent,
		SwapTotal: &sw.Total,
		SwapUsed:  &sw.Used,
	}, nil
}
