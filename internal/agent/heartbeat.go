package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/store"
	"github.com/rs/zerolog/log"
)

const DefaultHeartbeatInterval = 60 * time.Second

// HeartbeatService sends periodic heartbeats to all known peers.
type HeartbeatService struct {
	identity  *models.NodeIdentity
	collector *Collector
	store     *store.Store
	client    *http.Client
	interval  time.Duration
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

func NewHeartbeatService(identity *models.NodeIdentity, collector *Collector, s *store.Store) *HeartbeatService {
	return &HeartbeatService{
		identity:  identity,
		collector: collector,
		store:     s,
		client:    &http.Client{Timeout: 10 * time.Second},
		interval:  DefaultHeartbeatInterval,
	}
}

func (h *HeartbeatService) Start(ctx context.Context) {
	ctx, h.cancel = context.WithCancel(ctx)
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		// Short delay to let collector get first reading
		time.Sleep(5 * time.Second)
		h.sendHeartbeats(ctx)

		ticker := time.NewTicker(h.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.sendHeartbeats(ctx)
			}
		}
	}()
	log.Info().Dur("interval", h.interval).Msg("heartbeat service started")
}

func (h *HeartbeatService) Stop() {
	if h.cancel != nil {
		h.cancel()
	}
	h.wg.Wait()
}

func (h *HeartbeatService) sendHeartbeats(ctx context.Context) {
	peers, err := h.store.ListPeers(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to list peers for heartbeat")
		return
	}
	if len(peers) == 0 {
		return
	}

	host, _ := h.store.GetHost(ctx, h.identity.ID)

	// Include active known services (exclude unknown port scans)
	allSvcs, _ := h.store.ListServicesByHost(ctx, h.identity.ID)
	var svcs []models.DiscoveredService
	for _, s := range allSvcs {
		if s.Status == "active" && s.Category != "unknown" {
			svcs = append(svcs, s)
		}
	}

	hb := models.Heartbeat{
		NodeID:    h.identity.ID,
		Hostname:  h.identity.Hostname,
		Version:   h.identity.Version,
		Timestamp: time.Now().UTC(),
		Host:      host,
		Metric:    h.collector.Latest(),
		Services:  svcs,
	}

	body, _ := json.Marshal(hb)

	for _, peer := range peers {
		go h.sendToPeer(ctx, peer, body)
	}
}

func (h *HeartbeatService) sendToPeer(ctx context.Context, peer models.PeerInfo, body []byte) {
	url := fmt.Sprintf("http://%s/api/v1/heartbeat", peer.Address)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		log.Debug().Err(err).Str("peer", peer.Hostname).Msg("heartbeat send failed")
		h.store.UpdateHostStatus(ctx, peer.ID, "offline")
		return
	}
	defer resp.Body.Close()

	var peerHB models.Heartbeat
	if err := json.NewDecoder(resp.Body).Decode(&peerHB); err != nil {
		return
	}

	// Store peer's host info, metrics, and services
	if peerHB.Host != nil {
		peerHB.Host.Status = "online"
		h.store.UpsertHost(ctx, peerHB.Host)
	}
	if peerHB.Metric != nil {
		h.store.InsertMetric(ctx, peerHB.Metric)
	}
	for _, svc := range peerHB.Services {
		h.store.UpsertService(ctx, &svc)
	}

	// Update peer status
	now := time.Now().UTC()
	h.store.UpsertPeer(ctx, &models.PeerInfo{
		ID:            peerHB.NodeID,
		Hostname:      peerHB.Hostname,
		Address:       peer.Address,
		LastHeartbeat: &now,
		Status:        "online",
		Version:       peerHB.Version,
	})

	log.Debug().Str("peer", peer.Hostname).Msg("heartbeat exchanged")
}
