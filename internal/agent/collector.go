package agent

import (
	"context"
	"sync"
	"time"

	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/plugin"
	"github.com/dx111ge/homelabmon/internal/store"
	"github.com/rs/zerolog/log"
)

// Collector orchestrates all observers and stores metric snapshots.
type Collector struct {
	nodeID   string
	store    *store.Store
	registry *plugin.Registry
	interval time.Duration
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	mu     sync.RWMutex
	latest *models.MetricSnapshot
}

func NewCollector(nodeID string, s *store.Store, reg *plugin.Registry, interval time.Duration) *Collector {
	return &Collector{
		nodeID:   nodeID,
		store:    s,
		registry: reg,
		interval: interval,
	}
}

func (c *Collector) Start(ctx context.Context) {
	ctx, c.cancel = context.WithCancel(ctx)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.collect(ctx)

		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.collect(ctx)
			}
		}
	}()
	log.Info().Dur("interval", c.interval).Msg("collector started")
}

func (c *Collector) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	log.Info().Msg("collector stopped")
}

func (c *Collector) Latest() *models.MetricSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latest
}

func (c *Collector) collect(ctx context.Context) {
	snap := &models.MetricSnapshot{
		HostID:      c.nodeID,
		CollectedAt: time.Now().UTC(),
		DiskJSON:    "[]",
	}

	for _, obs := range c.registry.RunningObservers() {
		result, err := obs.Collect(ctx)
		if err != nil {
			log.Warn().Err(err).Str("observer", obs.Name()).Msg("collect failed")
			continue
		}
		mergeResult(snap, result)
	}

	if err := c.store.InsertMetric(ctx, snap); err != nil {
		log.Error().Err(err).Msg("failed to store metric")
	}

	c.mu.Lock()
	c.latest = snap
	c.mu.Unlock()

	log.Debug().
		Float64("cpu", snap.CPUPercent).
		Float64("mem", snap.MemPercent).
		Msg("collected metrics")
}

func mergeResult(snap *models.MetricSnapshot, r *plugin.ObserverResult) {
	if r == nil {
		return
	}
	if r.CPUPercent != nil {
		snap.CPUPercent = *r.CPUPercent
	}
	if r.Load1 != nil {
		snap.Load1 = *r.Load1
	}
	if r.Load5 != nil {
		snap.Load5 = *r.Load5
	}
	if r.Load15 != nil {
		snap.Load15 = *r.Load15
	}
	if r.MemTotal != nil {
		snap.MemTotal = *r.MemTotal
	}
	if r.MemUsed != nil {
		snap.MemUsed = *r.MemUsed
	}
	if r.MemPercent != nil {
		snap.MemPercent = *r.MemPercent
	}
	if r.SwapTotal != nil {
		snap.SwapTotal = *r.SwapTotal
	}
	if r.SwapUsed != nil {
		snap.SwapUsed = *r.SwapUsed
	}
	if r.Disks != nil {
		snap.DiskJSON = models.EncodeDiskUsages(r.Disks)
	}
	if r.NetBytesSent != nil {
		snap.NetBytesSent = *r.NetBytesSent
	}
	if r.NetBytesRecv != nil {
		snap.NetBytesRecv = *r.NetBytesRecv
	}
}
