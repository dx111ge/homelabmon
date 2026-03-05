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

	// Also store unmatched listening ports as services (only ports < 49152).
	// Use well-known port names as fallback when process name is empty.
	matched := make(map[int]bool)
	for _, m := range matches {
		matched[m.MatchedPort] = true
	}
	for _, lp := range listening {
		if matched[int(lp.Port)] || lp.Port >= 49152 {
			continue
		}
		name := lp.Process
		category := "unknown"
		if name == "" {
			if wk, ok := wellKnownPorts[lp.Port]; ok {
				name = wk.Name
				category = wk.Category
			} else {
				name = "unknown"
			}
		}
		svc := &models.DiscoveredService{
			HostID:   e.hostID,
			Name:     name,
			Port:     lp.Port,
			Protocol: lp.Proto,
			Process:  lp.Process,
			Category: category,
			Source:   "portscan",
			Status:   "active",
			LastSeen: now,
		}
		if err := e.store.UpsertService(ctx, svc); err != nil {
			log.Warn().Err(err).Int("port", svc.Port).Msg("failed to store unknown service")
		}
	}

	log.Info().Int("matched", len(matches)).Int("ports", len(ports)).Msg("service discovery complete")
}

// wellKnownPort maps a port number to a human-readable name and category.
type wellKnownPort struct {
	Name     string
	Category string
}

// wellKnownPorts provides fallback names for common ports when process name is unavailable.
var wellKnownPorts = map[int]wellKnownPort{
	22:    {"ssh", "remote-access"},
	25:    {"smtp", "mail"},
	53:    {"dns", "dns"},
	80:    {"http", "web"},
	110:   {"pop3", "mail"},
	111:   {"rpcbind", "system"},
	143:   {"imap", "mail"},
	443:   {"https", "web"},
	445:   {"smb", "file-sharing"},
	465:   {"smtps", "mail"},
	548:   {"afp", "file-sharing"},
	587:   {"smtp-submission", "mail"},
	631:   {"ipp", "printing"},
	993:   {"imaps", "mail"},
	995:   {"pop3s", "mail"},
	1194:  {"openvpn", "remote-access"},
	1883:  {"mqtt", "mqtt"},
	2049:  {"nfs", "file-sharing"},
	3306:  {"mysql", "database"},
	3389:  {"rdp", "remote-access"},
	5353:  {"mdns", "dns"},
	5432:  {"postgresql", "database"},
	5900:  {"vnc", "remote-access"},
	6379:  {"redis", "cache"},
	8080:  {"http-alt", "web"},
	8443:  {"https-alt", "web"},
	9090:  {"prometheus", "monitoring"},
	27017: {"mongodb", "database"},
}
