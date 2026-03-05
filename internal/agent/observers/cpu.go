package observers

import (
	"context"
	"time"

	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/plugin"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"
)

type CPUObserver struct{}

var _ plugin.Observer = (*CPUObserver)(nil)

func (o *CPUObserver) Name() string                       { return "cpu" }
func (o *CPUObserver) Type() models.PluginType            { return models.PluginTypeObserver }
func (o *CPUObserver) Detect() bool                       { return true }
func (o *CPUObserver) Interval() time.Duration            { return 30 * time.Second }
func (o *CPUObserver) Start(_ context.Context) error      { return nil }
func (o *CPUObserver) Stop() error                        { return nil }

func (o *CPUObserver) Collect(ctx context.Context) (*plugin.ObserverResult, error) {
	percents, err := cpu.PercentWithContext(ctx, time.Second, false)
	if err != nil {
		return nil, err
	}
	cpuPct := 0.0
	if len(percents) > 0 {
		cpuPct = percents[0]
	}

	loadAvg, err := load.AvgWithContext(ctx)
	if err != nil {
		// load.Avg() returns error on Windows; use zeros
		loadAvg = &load.AvgStat{}
	}

	return &plugin.ObserverResult{
		CPUPercent: &cpuPct,
		Load1:      &loadAvg.Load1,
		Load5:      &loadAvg.Load5,
		Load15:     &loadAvg.Load15,
	}, nil
}
