package observers

import (
	"context"
	"time"

	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/plugin"
	"github.com/shirou/gopsutil/v4/process"
)

// ProcInfo holds a running process's key attributes.
type ProcInfo struct {
	PID  int32
	Name string
}

// ProcessObserver collects running process names with PIDs.
type ProcessObserver struct {
	latest map[int32]string // PID -> name
}

var _ plugin.Observer = (*ProcessObserver)(nil)

func (o *ProcessObserver) Name() string                  { return "processes" }
func (o *ProcessObserver) Type() models.PluginType       { return models.PluginTypeObserver }
func (o *ProcessObserver) Detect() bool                  { return true }
func (o *ProcessObserver) Interval() time.Duration       { return 60 * time.Second }
func (o *ProcessObserver) Start(_ context.Context) error { return nil }
func (o *ProcessObserver) Stop() error                   { return nil }

// NameByPID returns the process name for a PID from the latest collection.
func (o *ProcessObserver) NameByPID(pid int32) string {
	if o.latest == nil {
		return ""
	}
	return o.latest[pid]
}

func (o *ProcessObserver) Collect(ctx context.Context) (*plugin.ObserverResult, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}

	pidMap := make(map[int32]string, len(procs))
	for _, p := range procs {
		name, err := p.NameWithContext(ctx)
		if err != nil {
			continue
		}
		pidMap[p.Pid] = name
	}
	o.latest = pidMap

	return &plugin.ObserverResult{
		Extra: map[string]interface{}{"process_count": len(pidMap)},
	}, nil
}
