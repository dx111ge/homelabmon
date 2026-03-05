package mesh

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/rs/zerolog/log"
)

func (t *Transport) setupRoutes() {
	t.mux.HandleFunc("POST /api/v1/register", t.handleRegister)
	t.mux.HandleFunc("POST /api/v1/heartbeat", t.handleHeartbeat)
	t.mux.HandleFunc("GET /api/v1/status", t.handleStatus)
	t.mux.HandleFunc("GET /api/v1/peers", t.handleListPeers)
	t.mux.HandleFunc("GET /api/v1/hosts", t.handleListHosts)
	t.mux.HandleFunc("GET /api/v1/metrics/latest", t.handleLatestMetric)
	t.mux.HandleFunc("GET /api/v1/metrics/history", t.handleMetricHistory)
	t.mux.HandleFunc("GET /api/v1/services", t.handleListServices)
	t.mux.HandleFunc("GET /api/v1/services/{host_id}", t.handleHostServices)
}

type registerRequest struct {
	NodeID   string `json:"node_id"`
	Hostname string `json:"hostname"`
	Address  string `json:"address"`
	Version  string `json:"version"`
}

func (t *Transport) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	now := time.Now().UTC()
	peer := &models.PeerInfo{
		ID:            req.NodeID,
		Hostname:      req.Hostname,
		Address:       req.Address,
		Version:       req.Version,
		Status:        "online",
		LastHeartbeat: &now,
	}

	if err := t.store.UpsertPeer(r.Context(), peer); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store failed"})
		return
	}

	log.Info().Str("peer", req.Hostname).Str("addr", req.Address).Msg("peer registered")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"node_id": t.identity.ID,
	})
}

func (t *Transport) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var hb models.Heartbeat
	if err := json.NewDecoder(r.Body).Decode(&hb); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid heartbeat"})
		return
	}

	// Store peer's host info
	if hb.Host != nil {
		hb.Host.Status = "online"
		t.store.UpsertHost(r.Context(), hb.Host)
	}

	// Store peer's metrics
	if hb.Metric != nil {
		t.store.InsertMetric(r.Context(), hb.Metric)
	}

	// Store peer's services
	for i := range hb.Services {
		t.store.UpsertService(r.Context(), &hb.Services[i])
	}

	// Update peer record
	now := time.Now().UTC()
	t.store.UpsertPeer(r.Context(), &models.PeerInfo{
		ID:            hb.NodeID,
		Hostname:      hb.Hostname,
		Address:       r.RemoteAddr,
		LastHeartbeat: &now,
		Status:        "online",
		Version:       hb.Version,
	})

	// Respond with our own heartbeat (including services)
	host, _ := t.store.GetHost(r.Context(), t.identity.ID)
	allSvcs, _ := t.store.ListServicesByHost(r.Context(), t.identity.ID)
	var mySvcs []models.DiscoveredService
	for _, s := range allSvcs {
		if s.Status == "active" && s.Category != "unknown" {
			mySvcs = append(mySvcs, s)
		}
	}
	myHB := models.Heartbeat{
		NodeID:    t.identity.ID,
		Hostname:  t.identity.Hostname,
		Version:   t.identity.Version,
		Timestamp: time.Now().UTC(),
		Host:      host,
		Metric:    t.collector.Latest(),
		Services:  mySvcs,
	}

	writeJSON(w, http.StatusOK, myHB)
}

func (t *Transport) handleStatus(w http.ResponseWriter, r *http.Request) {
	host, _ := t.store.GetHost(r.Context(), t.identity.ID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"node_id":  t.identity.ID,
		"hostname": t.identity.Hostname,
		"version":  t.identity.Version,
		"host":     host,
		"metric":   t.collector.Latest(),
	})
}

func (t *Transport) handleListPeers(w http.ResponseWriter, r *http.Request) {
	peers, err := t.store.ListPeers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, peers)
}

func (t *Transport) handleListHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := t.store.ListHosts(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, hosts)
}

func (t *Transport) handleLatestMetric(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("host_id")
	if hostID == "" {
		hostID = t.identity.ID
	}
	metric, err := t.store.GetLatestMetric(r.Context(), hostID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no metrics"})
		return
	}
	writeJSON(w, http.StatusOK, metric)
}

func (t *Transport) handleMetricHistory(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("host_id")
	if hostID == "" {
		hostID = t.identity.ID
	}
	hours := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		if v, err := strconv.Atoi(h); err == nil {
			hours = v
		}
	}
	metrics, err := t.store.GetMetricHistory(r.Context(), hostID, hours)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (t *Transport) handleListServices(w http.ResponseWriter, r *http.Request) {
	services, err := t.store.ListAllServices(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, services)
}

func (t *Transport) handleHostServices(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("host_id")
	services, err := t.store.ListServicesByHost(r.Context(), hostID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, services)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
