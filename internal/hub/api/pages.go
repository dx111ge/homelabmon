package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dx111ge/homelabmon/internal/agent/integrations"
	"github.com/dx111ge/homelabmon/internal/agent/scanners"
	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/notify"
	"github.com/dx111ge/homelabmon/internal/plugin"
	"github.com/dx111ge/homelabmon/internal/store"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type hostWithMetric struct {
	Host     models.Host
	Metric   *models.MetricSnapshot
	TopDisk  []models.DiskUsage
	Services []models.DiscoveredService
}

type dashboardData struct {
	Title          string
	Version        string
	HostCount      int
	OnlineCount    int
	Hosts          []hostWithMetric
	PassiveDevices []models.Host
}

func (u *UIServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data := u.buildDashboardData(r)
	if err := u.dashTmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Error().Err(err).Msg("render dashboard")
		http.Error(w, "render error", 500)
	}
}

func (u *UIServer) handleDashboardContent(w http.ResponseWriter, r *http.Request) {
	data := u.buildDashboardData(r)
	if err := u.dashTmpl.ExecuteTemplate(w, "dashboard-cards", data); err != nil {
		log.Error().Err(err).Msg("render dashboard content")
		http.Error(w, "render error", 500)
	}
}

func (u *UIServer) buildDashboardData(r *http.Request) dashboardData {
	hosts, _ := u.store.ListHosts(r.Context())

	var hostCards []hostWithMetric
	onlineCount := 0

	var passiveDevices []models.Host
	for _, h := range hosts {
		if h.Status == "online" {
			onlineCount++
		}
		if h.MonitorType == "passive" {
			passiveDevices = append(passiveDevices, h)
			continue
		}
		hm := hostWithMetric{Host: h}
		metric, err := u.store.GetLatestMetric(r.Context(), h.ID)
		if err == nil && metric != nil {
			hm.Metric = metric
			disks := metric.Disks()
			if len(disks) > 0 {
				hm.TopDisk = disks[:1] // show primary disk only on card
			}
		}
		svcs, _ := u.store.ListServicesByHost(r.Context(), h.ID)
		// Only show active fingerprint/docker services on the card, deduplicated by name
		seen := make(map[string]bool)
		var activeSvcs []models.DiscoveredService
		for _, s := range svcs {
			if s.Status == "active" && s.Category != "unknown" && !seen[s.Name] {
				seen[s.Name] = true
				activeSvcs = append(activeSvcs, s)
			}
		}
		hm.Services = activeSvcs
		hostCards = append(hostCards, hm)
	}

	return dashboardData{
		Title:          "Dashboard",
		Version:        u.identity.Version,
		HostCount:      len(hosts),
		OnlineCount:    onlineCount,
		Hosts:          hostCards,
		PassiveDevices: passiveDevices,
	}
}

type hostDetailData struct {
	Title       string
	Version     string
	HostCount   int
	OnlineCount int
	Host        models.Host
	Metric      *models.MetricSnapshot
	Disks       []models.DiskUsage
	Services    []models.DiscoveredService
}

func (u *UIServer) handleHostDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	host, err := u.store.GetHost(r.Context(), id)
	if err != nil || host == nil {
		http.NotFound(w, r)
		return
	}

	allHosts, _ := u.store.ListHosts(r.Context())
	onlineCount := 0
	for _, h := range allHosts {
		if h.Status == "online" {
			onlineCount++
		}
	}

	data := hostDetailData{
		Title:       host.Hostname,
		Version:     u.identity.Version,
		HostCount:   len(allHosts),
		OnlineCount: onlineCount,
		Host:        *host,
	}

	metric, err := u.store.GetLatestMetric(r.Context(), id)
	if err == nil && metric != nil {
		data.Metric = metric
		data.Disks = metric.Disks()
	}

	svcs, _ := u.store.ListServicesByHost(r.Context(), id)
	data.Services = svcs

	if err := u.hostTmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Error().Err(err).Msg("render host detail")
		http.Error(w, "render error", 500)
	}
}

type servicesPageData struct {
	Title       string
	Version     string
	HostCount   int
	OnlineCount int
	Services    []models.DiscoveredService
	Hosts       map[string]string // id -> hostname
}

func (u *UIServer) handleServicesPage(w http.ResponseWriter, r *http.Request) {
	allHosts, _ := u.store.ListHosts(r.Context())
	onlineCount := 0
	hostNames := make(map[string]string)
	for _, h := range allHosts {
		hostNames[h.ID] = h.Hostname
		if h.Status == "online" {
			onlineCount++
		}
	}

	allSvcs, _ := u.store.ListAllServices(r.Context())
	// Filter: only show known (fingerprint/docker) services on the main page
	var services []models.DiscoveredService
	for _, s := range allSvcs {
		if s.Category != "unknown" {
			services = append(services, s)
		}
	}

	data := servicesPageData{
		Title:       "Services",
		Version:     u.identity.Version,
		HostCount:   len(allHosts),
		OnlineCount: onlineCount,
		Services:    services,
		Hosts:       hostNames,
	}

	if err := u.svcsTmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Error().Err(err).Msg("render services page")
		http.Error(w, "render error", 500)
	}
}

type devicesPageData struct {
	Title       string
	Version     string
	HostCount   int
	OnlineCount int
	Devices     []models.Host
	ScanEnabled bool
}

func (u *UIServer) buildDevicesData(r *http.Request) devicesPageData {
	allHosts, _ := u.store.ListHosts(r.Context())
	onlineCount := 0
	for _, h := range allHosts {
		if h.Status == "online" {
			onlineCount++
		}
	}
	devices, _ := u.store.ListPassiveDevices(r.Context())

	return devicesPageData{
		Title:       "Devices",
		Version:     u.identity.Version,
		HostCount:   len(allHosts),
		OnlineCount: onlineCount,
		Devices:     devices,
		ScanEnabled: u.scanEnabled,
	}
}

func (u *UIServer) handleDevicesPage(w http.ResponseWriter, r *http.Request) {
	data := u.buildDevicesData(r)
	if err := u.devsTmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Error().Err(err).Msg("render devices page")
		http.Error(w, "render error", 500)
	}
}

func (u *UIServer) handleDevicesContent(w http.ResponseWriter, r *http.Request) {
	data := u.buildDevicesData(r)
	if err := u.devsTmpl.ExecuteTemplate(w, "devices-grid", data); err != nil {
		log.Error().Err(err).Msg("render devices content")
		http.Error(w, "render error", 500)
	}
}

func (u *UIServer) handleHostMetrics(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	metric, err := u.store.GetLatestMetric(r.Context(), id)
	if err != nil || metric == nil {
		w.WriteHeader(204)
		return
	}

	data := struct {
		Metric *models.MetricSnapshot
	}{Metric: metric}

	if err := u.hostTmpl.ExecuteTemplate(w, "metric-cards", data); err != nil {
		log.Error().Err(err).Msg("render host metrics")
	}
}

type settingsPageData struct {
	Title         string
	Version       string
	HostCount     int
	OnlineCount   int
	Senders       []string
	ScanEnabled   bool
	NodeID        string
	CPUThreshold  float64
	MemThreshold  float64
	DiskThreshold float64
	Integrations  []store.Integration
}

func (u *UIServer) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	allHosts, _ := u.store.ListHosts(r.Context())
	onlineCount := 0
	for _, h := range allHosts {
		if h.Status == "online" {
			onlineCount++
		}
	}

	integrations, _ := u.store.ListIntegrations(r.Context())

	data := settingsPageData{
		Title:         "Settings",
		Version:       u.identity.Version,
		HostCount:     len(allHosts),
		OnlineCount:   onlineCount,
		Senders:       u.dispatcher.SenderNames(),
		ScanEnabled:   u.scanEnabled,
		NodeID:        u.identity.ID,
		CPUThreshold:  viper.GetFloat64("notify-cpu-threshold"),
		MemThreshold:  viper.GetFloat64("notify-mem-threshold"),
		DiskThreshold: viper.GetFloat64("notify-disk-threshold"),
		Integrations:  integrations,
	}

	if err := u.settingsTmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Error().Err(err).Msg("render settings page")
		http.Error(w, "render error", 500)
	}
}

func (u *UIServer) handleTestNotification(w http.ResponseWriter, r *http.Request) {
	u.dispatcher.Send(notify.Notification{
		Title:    "HomeMonitor Test",
		Message:  "This is a test notification from HomeMonitor.",
		Severity: notify.SeverityInfo,
		Category: "test",
	})
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<span class="text-sm text-green-400"><i class="fa-solid fa-check mr-1"></i>Test notification sent!</span>`))
}

// Chat page

type chatPageData struct {
	Title       string
	Version     string
	HostCount   int
	OnlineCount int
	LLMEnabled  bool
	LLMModel    string
}

func (u *UIServer) handleChatPage(w http.ResponseWriter, r *http.Request) {
	allHosts, _ := u.store.ListHosts(r.Context())
	onlineCount := 0
	for _, h := range allHosts {
		if h.Status == "online" {
			onlineCount++
		}
	}

	model := ""
	if u.llmClient != nil {
		model = u.llmClient.Model()
	}

	data := chatPageData{
		Title:       "AI Chat",
		Version:     u.identity.Version,
		HostCount:   len(allHosts),
		OnlineCount: onlineCount,
		LLMEnabled:  u.chatHandler != nil,
		LLMModel:    model,
	}

	if err := u.chatTmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Error().Err(err).Msg("render chat page")
		http.Error(w, "render error", 500)
	}
}

// LLM API handlers

func (u *UIServer) handleLLMStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	connected := false
	model := ""
	if u.llmClient != nil {
		model = u.llmClient.Model()
		if err := u.llmClient.Ping(r.Context()); err == nil {
			connected = true
		}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"connected": connected,
		"model":     model,
		"enabled":   u.chatHandler != nil,
	})
}

func (u *UIServer) handleLLMChat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if u.chatHandler == nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "LLM not configured. Start with --llm http://localhost:11434"})
		return
	}

	var req struct {
		Message   string `json:"message"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
		return
	}
	if req.Message == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "empty message"})
		return
	}
	if req.SessionID == "" {
		req.SessionID = "default"
	}

	response, err := u.chatHandler.Chat(r.Context(), req.SessionID, req.Message)
	if err != nil {
		log.Error().Err(err).Msg("LLM chat error")
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"response": response})
}

func (u *UIServer) handleLLMClear(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if u.chatHandler == nil {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.SessionID == "" {
		req.SessionID = "default"
	}

	u.chatHandler.ClearSession(req.SessionID)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Host management handlers

func (u *UIServer) handleRenameHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if err := u.store.RenameHost(r.Context(), id, name); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "display_name": name})
}

func (u *UIServer) handleHostHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	hours := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		if v, err := fmt.Sscanf(h, "%d", new(int)); v == 1 && err == nil {
			fmt.Sscanf(h, "%d", &hours)
		}
	}
	if hours > 168 {
		hours = 168
	}

	metrics, err := u.store.GetMetricHistory(r.Context(), id, hours)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Downsample if too many points (target ~200 points for smooth charts)
	maxPoints := 200
	step := 1
	if len(metrics) > maxPoints {
		step = len(metrics) / maxPoints
	}

	type chartPoint struct {
		Time         string  `json:"t"`
		CPUPercent   float64 `json:"cpu"`
		MemPercent   float64 `json:"mem"`
		Load1        float64 `json:"load1"`
		Load5        float64 `json:"load5"`
		NetBytesSent uint64  `json:"net_sent"`
		NetBytesRecv uint64  `json:"net_recv"`
	}

	var points []chartPoint
	for i := 0; i < len(metrics); i += step {
		m := metrics[i]
		points = append(points, chartPoint{
			Time:         m.CollectedAt.Format("2006-01-02T15:04:05Z"),
			CPUPercent:   m.CPUPercent,
			MemPercent:   m.MemPercent,
			Load1:        m.Load1,
			Load5:        m.Load5,
			NetBytesSent: m.NetBytesSent,
			NetBytesRecv: m.NetBytesRecv,
		})
	}
	// Always include the last point
	if len(metrics) > 0 && len(points) > 0 {
		last := metrics[len(metrics)-1]
		lastPoint := chartPoint{
			Time:         last.CollectedAt.Format("2006-01-02T15:04:05Z"),
			CPUPercent:   last.CPUPercent,
			MemPercent:   last.MemPercent,
			Load1:        last.Load1,
			Load5:        last.Load5,
			NetBytesSent: last.NetBytesSent,
			NetBytesRecv: last.NetBytesRecv,
		}
		if points[len(points)-1].Time != lastPoint.Time {
			points = append(points, lastPoint)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(points)
}

func (u *UIServer) handleUpdateDeviceType(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		DeviceType string `json:"device_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
		return
	}
	dt := strings.TrimSpace(req.DeviceType)
	if dt == "" {
		dt = "other"
	}
	if err := u.store.UpdateHostDeviceType(r.Context(), id, dt); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "device_type": dt})
}

func (u *UIServer) handleDeleteHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := u.store.DeleteHost(r.Context(), id); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (u *UIServer) handleSaveIntegration(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Name     string `json:"name"`
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
		return
	}

	if req.ID == "" {
		req.ID = req.Type + "-" + fmt.Sprintf("%d", time.Now().Unix())
	}

	ig := &store.Integration{
		ID:      req.ID,
		Type:    req.Type,
		Name:    req.Name,
		Config:  map[string]string{"url": req.URL, "username": req.Username},
		Enabled: true,
		Status:  "unknown",
	}
	if err := u.store.UpsertIntegration(r.Context(), ig); err != nil {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Store password encrypted
	if req.Password != "" {
		key := store.SecretKeyID(req.ID, "password")
		if err := u.store.SetSecret(r.Context(), key, req.Password); err != nil {
			log.Error().Err(err).Msg("encrypt integration password")
		}
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "id": req.ID})
}

func (u *UIServer) handleDeleteIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	u.store.DeleteSecret(r.Context(), store.SecretKeyID(id, "password"))
	u.store.DeleteIntegration(r.Context(), id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (u *UIServer) handleTestIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	w.Header().Set("Content-Type", "text/html")

	ig, err := u.store.GetIntegration(r.Context(), id)
	if err != nil || ig == nil {
		w.Write([]byte(`<span class="text-red-400 text-sm"><i class="fa-solid fa-xmark mr-1"></i>Not found</span>`))
		return
	}

	ctx := r.Context()
	password, _ := u.store.GetSecret(ctx, store.SecretKeyID(ig.ID, "password"))
	url := ig.Config["url"]
	username := ig.Config["username"]

	var pingErr error
	switch ig.Type {
	case "fritzbox":
		fb := integrations.NewFritzBox(url, username, password)
		pingErr = fb.Ping(ctx)
	case "unifi":
		uf := integrations.NewUnifi(url, username, password)
		pingErr = uf.Ping(ctx)
	case "homeassistant":
		ha := integrations.NewHomeAssistant(url, password) // token stored as password
		pingErr = ha.Ping(ctx)
	case "pihole":
		ph := integrations.NewPiHole(url, password) // api_key stored as password
		pingErr = ph.Ping(ctx)
	case "pfsense":
		pf := integrations.NewPfSense(url, username, password)
		pingErr = pf.Ping(ctx)
	default:
		w.Write([]byte(fmt.Sprintf(`<span class="text-red-400 text-sm"><i class="fa-solid fa-xmark mr-1"></i>Unknown type: %s</span>`, ig.Type)))
		return
	}

	if pingErr != nil {
		u.store.UpdateIntegrationStatus(ctx, id, "error", pingErr.Error())
		w.Write([]byte(fmt.Sprintf(`<span class="text-red-400 text-sm"><i class="fa-solid fa-xmark mr-1"></i>%s</span>`, pingErr.Error())))
		return
	}

	u.store.UpdateIntegrationStatus(ctx, id, "ok", "")
	w.Write([]byte(`<span class="text-green-400 text-sm"><i class="fa-solid fa-check mr-1"></i>Connected!</span>`))
}

func (u *UIServer) handleSyncIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	w.Header().Set("Content-Type", "text/html")

	ig, err := u.store.GetIntegration(r.Context(), id)
	if err != nil || ig == nil {
		w.Write([]byte(`<span class="text-red-400 text-sm">Not found</span>`))
		return
	}

	ctx := r.Context()
	password, _ := u.store.GetSecret(ctx, store.SecretKeyID(ig.ID, "password"))
	url := ig.Config["url"]
	username := ig.Config["username"]

	var devices []plugin.DiscoveredDevice
	var syncErr error

	switch ig.Type {
	case "fritzbox":
		fb := integrations.NewFritzBox(url, username, password)
		devices, syncErr = fb.Sync(ctx)
	case "unifi":
		uf := integrations.NewUnifi(url, username, password)
		devices, syncErr = uf.Sync(ctx)
	case "homeassistant":
		ha := integrations.NewHomeAssistant(url, password)
		devices, syncErr = ha.Sync(ctx)
	case "pihole":
		ph := integrations.NewPiHole(url, password)
		devices, syncErr = ph.Sync(ctx)
	case "pfsense":
		pf := integrations.NewPfSense(url, username, password)
		devices, syncErr = pf.Sync(ctx)
	default:
		w.Write([]byte(fmt.Sprintf(`<span class="text-red-400 text-sm">Unknown type: %s</span>`, ig.Type)))
		return
	}

	if syncErr != nil {
		u.store.UpdateIntegrationStatus(ctx, id, "error", syncErr.Error())
		w.Write([]byte(fmt.Sprintf(`<span class="text-red-400 text-sm"><i class="fa-solid fa-xmark mr-1"></i>%s</span>`, syncErr.Error())))
		return
	}

	stored := 0
	for _, d := range devices {
		if d.IP == "" {
			continue
		}
		if err := u.store.UpsertPassiveDevice(ctx, d); err == nil {
			stored++
		}
	}

	u.store.UpdateIntegrationStatus(ctx, id, "ok", "")
	w.Write([]byte(fmt.Sprintf(`<span class="text-green-400 text-sm"><i class="fa-solid fa-check mr-1"></i>Synced %d devices</span>`, stored)))
}

func (u *UIServer) handleOUICheck(w http.ResponseWriter, r *http.Request) {
	devices, _ := u.store.ListPassiveDevices(r.Context())
	updated := 0
	for _, d := range devices {
		if d.Vendor != "" || d.MACAddress == "" {
			continue
		}
		vendor := scanners.LookupVendor(d.MACAddress)
		if vendor == "" {
			continue
		}
		devType := scanners.ClassifyByVendor(vendor)
		u.store.UpdateHostVendor(r.Context(), d.ID, vendor, devType)
		updated++
	}
	w.Header().Set("Content-Type", "text/html")
	if updated > 0 {
		w.Write([]byte(fmt.Sprintf(`<span class="text-sm text-green-400"><i class="fa-solid fa-check mr-1"></i>Updated %d devices</span>`, updated)))
	} else {
		w.Write([]byte(`<span class="text-sm text-gray-400"><i class="fa-solid fa-check mr-1"></i>All vendors already resolved</span>`))
	}
}
