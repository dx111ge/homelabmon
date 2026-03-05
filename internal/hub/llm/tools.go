package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dx111ge/homelabmon/internal/store"
)

// ToolExecutor executes tool calls against the store.
type ToolExecutor struct {
	store *store.Store
}

func NewToolExecutor(s *store.Store) *ToolExecutor {
	return &ToolExecutor{store: s}
}

// ToolDefinitions returns the tools available to the LLM.
func ToolDefinitions() []Tool {
	return []Tool{
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "list_hosts",
				Description: "List all known hosts (servers, devices) with their status, OS, IP, and type. Use this to see what's on the network.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","description":"Filter by status: online, offline, or all (default: all)","enum":["online","offline","all"]},"monitor_type":{"type":"string","description":"Filter by type: agent, passive, or all (default: all)","enum":["agent","passive","all"]}}}`),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "get_host",
				Description: "Get detailed information about a specific host by hostname or ID, including hardware specs, OS, IPs.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"hostname":{"type":"string","description":"Hostname to look up (case-insensitive partial match)"}},"required":["hostname"]}`),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "get_metrics",
				Description: "Get current or historical CPU, memory, disk, and network metrics for a host.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"hostname":{"type":"string","description":"Hostname to get metrics for"},"hours":{"type":"integer","description":"Hours of history (default: 1, max: 168)"}},"required":["hostname"]}`),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "list_services",
				Description: "List all discovered services (Docker containers, web servers, databases, etc.) across all hosts or for a specific host.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"hostname":{"type":"string","description":"Filter by hostname (optional)"},"category":{"type":"string","description":"Filter by category: container, web, database, media, monitoring, etc. (optional)"}}}`),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "get_summary",
				Description: "Get a high-level summary of the entire homelab: host counts, online/offline, service counts, resource usage.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
	}
}

// Execute runs a tool call and returns the result as a string.
func (e *ToolExecutor) Execute(ctx context.Context, name string, args json.RawMessage) (string, error) {
	switch name {
	case "list_hosts":
		return e.listHosts(ctx, args)
	case "get_host":
		return e.getHost(ctx, args)
	case "get_metrics":
		return e.getMetrics(ctx, args)
	case "list_services":
		return e.listServices(ctx, args)
	case "get_summary":
		return e.getSummary(ctx)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func (e *ToolExecutor) listHosts(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Status      string `json:"status"`
		MonitorType string `json:"monitor_type"`
	}
	json.Unmarshal(args, &params)

	hosts, err := e.store.ListHosts(ctx)
	if err != nil {
		return "", err
	}

	type hostSummary struct {
		Hostname    string   `json:"hostname"`
		Status      string   `json:"status"`
		MonitorType string   `json:"monitor_type"`
		DeviceType  string   `json:"device_type"`
		OS          string   `json:"os,omitempty"`
		IPs         []string `json:"ips,omitempty"`
		Vendor      string   `json:"vendor,omitempty"`
		LastSeen    string   `json:"last_seen"`
	}

	var result []hostSummary
	for _, h := range hosts {
		if params.Status != "" && params.Status != "all" && h.Status != params.Status {
			continue
		}
		if params.MonitorType != "" && params.MonitorType != "all" && h.MonitorType != params.MonitorType {
			continue
		}
		hs := hostSummary{
			Hostname:    h.Hostname,
			Status:      h.Status,
			MonitorType: h.MonitorType,
			DeviceType:  h.DeviceType,
			OS:          h.OS,
			IPs:         h.IPAddresses,
			Vendor:      h.Vendor,
			LastSeen:    h.LastSeen.Format("2006-01-02 15:04:05"),
		}
		result = append(result, hs)
	}

	b, _ := json.Marshal(result)
	return string(b), nil
}

func (e *ToolExecutor) getHost(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Hostname string `json:"hostname"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", err
	}

	hosts, err := e.store.ListHosts(ctx)
	if err != nil {
		return "", err
	}

	// Find by partial hostname match (case-insensitive)
	for _, h := range hosts {
		if containsCI(h.Hostname, params.Hostname) || h.ID == params.Hostname {
			b, _ := json.Marshal(h)
			return string(b), nil
		}
	}

	return fmt.Sprintf(`{"error":"host %q not found"}`, params.Hostname), nil
}

func (e *ToolExecutor) getMetrics(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Hostname string `json:"hostname"`
		Hours    int    `json:"hours"`
	}
	json.Unmarshal(args, &params)
	if params.Hours <= 0 {
		params.Hours = 1
	}
	if params.Hours > 168 {
		params.Hours = 168
	}

	// Find host
	hosts, err := e.store.ListHosts(ctx)
	if err != nil {
		return "", err
	}

	var hostID string
	for _, h := range hosts {
		if containsCI(h.Hostname, params.Hostname) || h.ID == params.Hostname {
			hostID = h.ID
			break
		}
	}
	if hostID == "" {
		return fmt.Sprintf(`{"error":"host %q not found"}`, params.Hostname), nil
	}

	if params.Hours <= 1 {
		// Return latest snapshot
		m, err := e.store.GetLatestMetric(ctx, hostID)
		if err != nil {
			return `{"error":"no metrics available"}`, nil
		}
		type metricResult struct {
			CollectedAt  string  `json:"collected_at"`
			CPUPercent   float64 `json:"cpu_percent"`
			MemPercent   float64 `json:"mem_percent"`
			MemUsedGB    float64 `json:"mem_used_gb"`
			MemTotalGB   float64 `json:"mem_total_gb"`
			NetSentMB    float64 `json:"net_sent_mb"`
			NetRecvMB    float64 `json:"net_recv_mb"`
			Disks        interface{} `json:"disks"`
		}
		r := metricResult{
			CollectedAt: m.CollectedAt.Format("2006-01-02 15:04:05"),
			CPUPercent:  m.CPUPercent,
			MemPercent:  m.MemPercent,
			MemUsedGB:   float64(m.MemUsed) / (1024 * 1024 * 1024),
			MemTotalGB:  float64(m.MemTotal) / (1024 * 1024 * 1024),
			NetSentMB:   float64(m.NetBytesSent) / (1024 * 1024),
			NetRecvMB:   float64(m.NetBytesRecv) / (1024 * 1024),
			Disks:       m.Disks(),
		}
		b, _ := json.Marshal(r)
		return string(b), nil
	}

	// Return history summary
	metrics, err := e.store.GetMetricHistory(ctx, hostID, params.Hours)
	if err != nil {
		return `{"error":"no metrics available"}`, nil
	}
	if len(metrics) == 0 {
		return `{"error":"no metrics in time range"}`, nil
	}

	var avgCPU, avgMem, maxCPU, maxMem float64
	for _, m := range metrics {
		avgCPU += m.CPUPercent
		avgMem += m.MemPercent
		if m.CPUPercent > maxCPU {
			maxCPU = m.CPUPercent
		}
		if m.MemPercent > maxMem {
			maxMem = m.MemPercent
		}
	}
	avgCPU /= float64(len(metrics))
	avgMem /= float64(len(metrics))

	type historySummary struct {
		DataPoints int     `json:"data_points"`
		Hours      int     `json:"hours"`
		AvgCPU     float64 `json:"avg_cpu_percent"`
		MaxCPU     float64 `json:"max_cpu_percent"`
		AvgMem     float64 `json:"avg_mem_percent"`
		MaxMem     float64 `json:"max_mem_percent"`
		FirstAt    string  `json:"first_at"`
		LastAt     string  `json:"last_at"`
	}
	r := historySummary{
		DataPoints: len(metrics),
		Hours:      params.Hours,
		AvgCPU:     avgCPU,
		MaxCPU:     maxCPU,
		AvgMem:     avgMem,
		MaxMem:     maxMem,
		FirstAt:    metrics[0].CollectedAt.Format("2006-01-02 15:04:05"),
		LastAt:     metrics[len(metrics)-1].CollectedAt.Format("2006-01-02 15:04:05"),
	}
	b, _ := json.Marshal(r)
	return string(b), nil
}

func (e *ToolExecutor) listServices(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Hostname string `json:"hostname"`
		Category string `json:"category"`
	}
	json.Unmarshal(args, &params)

	var services []struct {
		Name         string `json:"name"`
		Port         int    `json:"port"`
		Category     string `json:"category"`
		Source       string `json:"source"`
		Status       string `json:"status"`
		Host         string `json:"host"`
		ContainerImg string `json:"container_image,omitempty"`
	}

	hosts, _ := e.store.ListHosts(ctx)
	hostNames := make(map[string]string)
	for _, h := range hosts {
		hostNames[h.ID] = h.Hostname
	}

	allSvcs, err := e.store.ListAllServices(ctx)
	if err != nil {
		return "", err
	}

	for _, s := range allSvcs {
		if s.Category == "unknown" {
			continue
		}
		hostName := hostNames[s.HostID]
		if params.Hostname != "" && !containsCI(hostName, params.Hostname) {
			continue
		}
		if params.Category != "" && s.Category != params.Category {
			continue
		}
		services = append(services, struct {
			Name         string `json:"name"`
			Port         int    `json:"port"`
			Category     string `json:"category"`
			Source       string `json:"source"`
			Status       string `json:"status"`
			Host         string `json:"host"`
			ContainerImg string `json:"container_image,omitempty"`
		}{
			Name:         s.Name,
			Port:         s.Port,
			Category:     s.Category,
			Source:       s.Source,
			Status:       s.Status,
			Host:         hostName,
			ContainerImg: s.ContainerImg,
		})
	}

	b, _ := json.Marshal(services)
	return string(b), nil
}

func (e *ToolExecutor) getSummary(ctx context.Context) (string, error) {
	hosts, _ := e.store.ListHosts(ctx)

	var agentCount, passiveCount, onlineCount, offlineCount int
	for _, h := range hosts {
		switch h.MonitorType {
		case "agent":
			agentCount++
		case "passive":
			passiveCount++
		}
		switch h.Status {
		case "online":
			onlineCount++
		case "offline":
			offlineCount++
		}
	}

	allSvcs, _ := e.store.ListAllServices(ctx)
	var activeServices int
	categories := make(map[string]int)
	for _, s := range allSvcs {
		if s.Status == "active" && s.Category != "unknown" {
			activeServices++
			categories[s.Category]++
		}
	}

	type summary struct {
		TotalHosts     int            `json:"total_hosts"`
		AgentNodes     int            `json:"agent_nodes"`
		PassiveDevices int            `json:"passive_devices"`
		Online         int            `json:"online"`
		Offline        int            `json:"offline"`
		ActiveServices int            `json:"active_services"`
		ServicesByType map[string]int `json:"services_by_type"`
	}

	r := summary{
		TotalHosts:     len(hosts),
		AgentNodes:     agentCount,
		PassiveDevices: passiveCount,
		Online:         onlineCount,
		Offline:        offlineCount,
		ActiveServices: activeServices,
		ServicesByType: categories,
	}
	b, _ := json.Marshal(r)
	return string(b), nil
}

func containsCI(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	// Simple case-insensitive contains
	sl := make([]byte, len(s))
	for i := range s {
		if s[i] >= 'A' && s[i] <= 'Z' {
			sl[i] = s[i] + 32
		} else {
			sl[i] = s[i]
		}
	}
	subl := make([]byte, len(substr))
	for i := range substr {
		if substr[i] >= 'A' && substr[i] <= 'Z' {
			subl[i] = substr[i] + 32
		} else {
			subl[i] = substr[i]
		}
	}
	return bytes_contains(sl, subl)
}

func bytes_contains(s, sub []byte) bool {
	if len(sub) == 0 {
		return true
	}
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		match := true
		for j := range sub {
			if s[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
