package agent

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// ScanCoordinator prevents duplicate network scans across mesh peers.
// Each node reports its last scan time via heartbeat. Before scanning,
// a node checks whether any peer already scanned within the scan interval.
type ScanCoordinator struct {
	mu            sync.RWMutex
	localScanTime time.Time            // when this node last scanned
	peerScanTimes map[string]time.Time // nodeID -> last scan time
	scanInterval  time.Duration        // how often scans should happen
}

func NewScanCoordinator(scanInterval time.Duration) *ScanCoordinator {
	return &ScanCoordinator{
		peerScanTimes: make(map[string]time.Time),
		scanInterval:  scanInterval,
	}
}

// RecordLocalScan marks that this node just completed a scan.
func (c *ScanCoordinator) RecordLocalScan() {
	c.mu.Lock()
	c.localScanTime = time.Now().UTC()
	c.mu.Unlock()
}

// LocalScanTime returns when this node last scanned.
func (c *ScanCoordinator) LocalScanTime() *time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.localScanTime.IsZero() {
		return nil
	}
	t := c.localScanTime
	return &t
}

// RecordPeerScan records a peer's last scan time (from heartbeat).
func (c *ScanCoordinator) RecordPeerScan(nodeID string, scanTime time.Time) {
	c.mu.Lock()
	c.peerScanTimes[nodeID] = scanTime
	c.mu.Unlock()
}

// ShouldScan returns true if no node (self or peer) scanned recently
// enough to cover the current interval. This spreads scans across nodes
// so only one node scans per interval.
func (c *ScanCoordinator) ShouldScan() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	threshold := time.Now().UTC().Add(-c.scanInterval)

	// Check if any peer scanned recently
	for nodeID, t := range c.peerScanTimes {
		if t.After(threshold) {
			log.Debug().Str("peer", nodeID[:8]).Time("scan_time", t).Msg("skipping scan, peer scanned recently")
			return false
		}
	}

	return true
}
