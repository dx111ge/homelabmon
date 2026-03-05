package discovery

import (
	"context"
	"time"

	"github.com/dx111ge/homelabmon/internal/agent/observers"
	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/store"
	"github.com/rs/zerolog/log"
)

// Engine runs service discovery by matching ports+processes against fingerprints.
type Engine struct {
	hostID      string
	store       *store.Store
	portsObs    *observers.PortsObserver
	procObs     *observers.ProcessObserver
	fingerprints []Fingerprint
}

func NewEngine(hostID string, st *store.Store, portsObs *observers.PortsObserver, procObs *observers.ProcessObserver, fps []Fingerprint) *Engine {
	return &Engine{
		hostID:       hostID,
		store:        st,
		portsObs:     portsObs,
		procObs:      procObs,
		fingerprints: fps,
	}
}

// Discover runs one discovery cycle: collect ports, resolve process names, match fingerprints, store services.
func (e *Engine) Discover(ctx context.Context) {
	ports := e.portsObs.Latest()
	if len(ports) == 0 {
		return
	}

	// Build PortProcess list with resolved process names
	var listening []PortProcess
	for _, p := range ports {
		procName := e.procObs.NameByPID(p.PID)
		listening = append(listening, PortProcess{
			Port:    int(p.Port),
			Proto:   p.Proto,
			Process: procName,
		})
	}

	// Match against fingerprints
	matches := MatchServices(e.fingerprints, listening)

	now := time.Now().UTC()
	for _, m := range matches {
		svc := &models.DiscoveredService{
			HostID:   e.hostID,
			Name:     m.Fingerprint.Name,
			Port:     m.MatchedPort,
			Protocol: "tcp",
			Process:  m.Process,
			Category: m.Fingerprint.Category,
			Source:   "fingerprint",
			Status:   "active",
			LastSeen: now,
		}
		if err := e.store.UpsertService(ctx, svc); err != nil {
			log.Warn().Err(err).Str("service", svc.Name).Msg("failed to store service")
		}
	}

	// Also store unmatched listening ports as "unknown" services (only well-known ports < 49152)
	matched := make(map[int]bool)
	for _, m := range matches {
		matched[m.MatchedPort] = true
	}
	for _, lp := range listening {
		if matched[int(lp.Port)] || lp.Port >= 49152 {
			continue
		}
		svc := &models.DiscoveredService{
			HostID:   e.hostID,
			Name:     lp.Process,
			Port:     lp.Port,
			Protocol: lp.Proto,
			Process:  lp.Process,
			Category: "unknown",
			Source:   "portscan",
			Status:   "active",
			LastSeen: now,
		}
		if svc.Name == "" {
			svc.Name = "unknown"
		}
		if err := e.store.UpsertService(ctx, svc); err != nil {
			log.Warn().Err(err).Int("port", svc.Port).Msg("failed to store unknown service")
		}
	}

	if len(matches) > 0 {
		log.Info().Int("matched", len(matches)).Int("ports", len(ports)).Msg("service discovery complete")
	}
}
