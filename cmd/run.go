package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/dx111ge/homelabmon/configs"
	"github.com/dx111ge/homelabmon/internal/agent"
	"github.com/dx111ge/homelabmon/internal/agent/discovery"
	"github.com/dx111ge/homelabmon/internal/agent/observers"
	"github.com/dx111ge/homelabmon/internal/agent/integrations"
	"github.com/dx111ge/homelabmon/internal/agent/scanners"
	hubapi "github.com/dx111ge/homelabmon/internal/hub/api"
	"github.com/dx111ge/homelabmon/internal/hub/llm"
	"github.com/dx111ge/homelabmon/internal/mesh"
	"github.com/dx111ge/homelabmon/internal/notify"
	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/plugin"
	"github.com/dx111ge/homelabmon/internal/store"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the HomeMonitor node",
	RunE:  runAgent,
}

func init() {
	rootCmd.AddCommand(runCmd)

	// Use PersistentFlags on root so they work with both `homelabmon --ui` and `homelabmon run --ui`
	rootCmd.PersistentFlags().Bool("ui", false, "enable web dashboard")
	rootCmd.PersistentFlags().String("llm", "", "Ollama API URL (e.g., http://localhost:11434)")
	rootCmd.PersistentFlags().String("llm-model", "qwen2.5:7b", "Ollama model for chat (default: qwen2.5:7b)")
	rootCmd.PersistentFlags().StringSlice("peer", nil, "initial peer addresses (e.g., 192.168.1.10:9600)")
	rootCmd.PersistentFlags().String("bind", ":9600", "bind address for API and UI")
	rootCmd.PersistentFlags().Int("collect-interval", 30, "metric collection interval in seconds")
	rootCmd.PersistentFlags().Bool("scan", false, "enable network scanning")
	rootCmd.PersistentFlags().String("notify-ntfy", "", "ntfy.sh topic URL (e.g., https://ntfy.sh/homelabmon-alerts)")
	rootCmd.PersistentFlags().String("notify-webhook", "", "webhook URL for notifications (Discord, Slack, custom)")
	rootCmd.PersistentFlags().Float64("notify-cpu-threshold", 90, "CPU % threshold for alert notifications")
	rootCmd.PersistentFlags().Float64("notify-mem-threshold", 90, "memory % threshold for alert notifications")
	rootCmd.PersistentFlags().Float64("notify-disk-threshold", 90, "disk % threshold for alert notifications")

	viper.BindPFlag("ui", rootCmd.PersistentFlags().Lookup("ui"))
	viper.BindPFlag("llm", rootCmd.PersistentFlags().Lookup("llm"))
	viper.BindPFlag("llm-model", rootCmd.PersistentFlags().Lookup("llm-model"))
	viper.BindPFlag("peers", rootCmd.PersistentFlags().Lookup("peer"))
	viper.BindPFlag("bind", rootCmd.PersistentFlags().Lookup("bind"))
	viper.BindPFlag("collect-interval", rootCmd.PersistentFlags().Lookup("collect-interval"))
	viper.BindPFlag("scan", rootCmd.PersistentFlags().Lookup("scan"))
	viper.BindPFlag("notify-ntfy", rootCmd.PersistentFlags().Lookup("notify-ntfy"))
	viper.BindPFlag("notify-webhook", rootCmd.PersistentFlags().Lookup("notify-webhook"))
	viper.BindPFlag("notify-cpu-threshold", rootCmd.PersistentFlags().Lookup("notify-cpu-threshold"))
	viper.BindPFlag("notify-mem-threshold", rootCmd.PersistentFlags().Lookup("notify-mem-threshold"))
	viper.BindPFlag("notify-disk-threshold", rootCmd.PersistentFlags().Lookup("notify-disk-threshold"))
}

func runAgent(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir := dataDir()

	// 1. Load or generate node identity
	nodeID := loadOrCreateNodeID(dir)
	log.Info().Str("node_id", nodeID).Str("data_dir", dir).Msg("starting homelabmon")

	// 2. Open store
	st, err := store.New(dir)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	// 3. Collect host info
	hostInfo, err := agent.CollectHostInfo(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("collect host info: %w", err)
	}
	if err := st.UpsertHost(ctx, hostInfo); err != nil {
		return fmt.Errorf("store host info: %w", err)
	}

	identity := &models.NodeIdentity{
		ID:       nodeID,
		Hostname: hostInfo.Hostname,
		BindAddr: viper.GetString("bind"),
		Version:  Version,
		DataDir:  dir,
	}

	log.Info().
		Str("hostname", hostInfo.Hostname).
		Str("os", hostInfo.OS).
		Str("arch", hostInfo.Arch).
		Str("platform", hostInfo.Platform).
		Int("cpu_cores", hostInfo.CPUCores).
		Strs("ips", hostInfo.IPAddresses).
		Msg("host info collected")

	// 4. Plugin registry + observers
	registry := plugin.NewRegistry()
	registry.Register(&observers.CPUObserver{})
	registry.Register(&observers.MemoryObserver{})
	registry.Register(&observers.DiskObserver{})
	registry.Register(&observers.NetworkObserver{})

	// Phase 2: service discovery observers
	portsObs := &observers.PortsObserver{}
	procObs := &observers.ProcessObserver{}
	dockerObs := &observers.DockerObserver{}
	registry.Register(portsObs)
	registry.Register(procObs)
	registry.Register(dockerObs)

	registry.DetectAndStart(ctx)

	// Load fingerprints and create discovery engine
	fingerprints, err := discovery.ParseFingerprints(configs.FingerprintsYAML)
	if err != nil {
		log.Warn().Err(err).Msg("failed to parse fingerprints, service discovery disabled")
	}
	var discoveryEngine *discovery.Engine
	if fingerprints != nil {
		discoveryEngine = discovery.NewEngine(nodeID, st, portsObs, procObs, fingerprints)
	}

	// Phase 3: network scanners (when --scan is enabled)
	scanEnabled := viper.GetBool("scan")
	var arpScanner *scanners.ARPScanner
	var mdnsScanner *scanners.MDNSScanner
	if scanEnabled {
		arpScanner = &scanners.ARPScanner{}
		mdnsScanner = &scanners.MDNSScanner{}
		registry.Register(arpScanner)
		registry.Register(mdnsScanner)
		registry.DetectAndStart(ctx)
		log.Info().Msg("network scanning enabled")
	}

	// Phase 5: load external plugins from <data-dir>/plugins/
	pluginDir := filepath.Join(dir, "plugins")
	if entries, err := os.ReadDir(pluginDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			path := filepath.Join(pluginDir, entry.Name())
			if p := plugin.LoadExternalPlugin(path); p != nil {
				registry.Register(p)
			}
		}
		registry.DetectAndStart(ctx)
	}

	// Notifications
	dispatcher := notify.NewDispatcher(10 * time.Minute)
	if url := viper.GetString("notify-ntfy"); url != "" {
		dispatcher.AddSender(notify.NewNtfySender(url))
	}
	if url := viper.GetString("notify-webhook"); url != "" {
		dispatcher.AddSender(notify.NewWebhookSender(url))
	}

	// 5. Collector
	interval := time.Duration(viper.GetInt("collect-interval")) * time.Second
	collector := agent.NewCollector(nodeID, st, registry, interval)
	collector.Start(ctx)
	defer collector.Stop()

	// 6. LLM (if --llm)
	var chatHandler *llm.ChatHandler
	var llmClient *llm.Client
	if llmURL := viper.GetString("llm"); llmURL != "" {
		llmModel := viper.GetString("llm-model")
		if llmModel == "" {
			llmModel = "qwen2.5:7b"
		}
		llmClient = llm.NewClient(llmURL, llmModel)
		if err := llmClient.Ping(ctx); err != nil {
			log.Warn().Err(err).Str("url", llmURL).Msg("Ollama not reachable, chat disabled")
		} else {
			executor := llm.NewToolExecutor(st)
			chatHandler = llm.NewChatHandler(llmClient, executor)
			log.Info().Str("url", llmURL).Str("model", llmModel).Msg("LLM chat enabled")
		}
	}

	// 7. Mesh transport
	transport := mesh.NewTransport(identity, st, collector)

	// 8. Web UI (if --ui)
	if viper.GetBool("ui") {
		uiServer, err := hubapi.NewUIServer(st, collector, identity, scanEnabled, dispatcher, chatHandler, llmClient)
		if err != nil {
			return fmt.Errorf("create UI server: %w", err)
		}
		uiServer.SetupRoutes(transport.Mux())
		log.Info().Msg("web UI enabled")
	}

	// 8. Start transport
	bindAddr := viper.GetString("bind")
	if err := transport.Start(bindAddr); err != nil {
		return fmt.Errorf("start transport: %w", err)
	}

	// 9. Register initial peers
	peers := viper.GetStringSlice("peers")
	for _, addr := range peers {
		now := time.Now().UTC()
		st.UpsertPeer(ctx, &models.PeerInfo{
			ID:            "pending-" + addr,
			Address:       addr,
			Status:        "unknown",
			LastHeartbeat: &now,
		})
		log.Info().Str("peer", addr).Msg("added initial peer")
	}

	// 10. Heartbeat service
	hbService := agent.NewHeartbeatService(identity, collector, st)
	hbService.Start(ctx)
	defer hbService.Stop()

	// 11. Background: service discovery every 60s
	go func() {
		// Wait for first metric collection to populate ports/processes
		time.Sleep(5 * time.Second)
		runDiscovery(ctx, discoveryEngine, dockerObs, nodeID, st)

		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runDiscovery(ctx, discoveryEngine, dockerObs, nodeID, st)
			}
		}
	}()

	// 12. Background: network scanning (when --scan is enabled)
	if scanEnabled {
		go func() {
			// Initial scan after 10s
			time.Sleep(10 * time.Second)
			runNetworkScan(ctx, arpScanner, mdnsScanner, st)

			ticker := time.NewTicker(3 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					runNetworkScan(ctx, arpScanner, mdnsScanner, st)
				}
			}
		}()
	}

	// 13. Background: purge old metrics every hour
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, _ := st.PurgeOldMetrics(ctx, 7)
				if n > 0 {
					log.Info().Int64("purged", n).Msg("purged old metrics")
				}
			}
		}
	}()

	// 13. Background: mark stale services gone every 5 minutes
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				st.MarkStaleServicesGone(ctx, 10*time.Minute)
			}
		}
	}()

	// 14. Background: mark stale hosts offline every 2 minutes + notify
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stale, _ := st.MarkStaleHostsOffline(ctx, 5*time.Minute)
				for _, h := range stale {
					log.Warn().Str("host", h.Hostname).Msg("host went offline")
					dispatcher.Send(notify.FormatHostOffline(h.ID, h.Hostname))
				}
			}
		}
	}()

	// 15. Background: metric threshold alerts
	if dispatcher.HasSenders() {
		go func() {
			cpuThreshold := viper.GetFloat64("notify-cpu-threshold")
			memThreshold := viper.GetFloat64("notify-mem-threshold")
			diskThreshold := viper.GetFloat64("notify-disk-threshold")

			ticker := time.NewTicker(interval) // check at same rate as collection
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					checkThresholds(ctx, st, dispatcher, cpuThreshold, memThreshold, diskThreshold)
				}
			}
		}()
	}

	// 16. Background: integration auto-sync every 5 minutes
	go func() {
		time.Sleep(15 * time.Second) // initial delay
		runIntegrationSync(ctx, st)

		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runIntegrationSync(ctx, st)
			}
		}
	}()

	log.Info().Str("bind", bindAddr).Bool("ui", viper.GetBool("ui")).Msg("homelabmon running")

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Info().Msg("shutting down...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	transport.Stop(shutdownCtx)
	registry.StopAll()

	log.Info().Msg("homelabmon stopped")
	return nil
}

func runDiscovery(ctx context.Context, engine *discovery.Engine, dockerObs *observers.DockerObserver, nodeID string, st *store.Store) {
	if engine != nil {
		engine.Discover(ctx)
	}

	// Convert running Docker containers to services
	containers := dockerObs.Latest()
	now := time.Now().UTC()
	for _, c := range containers {
		if c.State != "running" {
			continue
		}
		for _, p := range c.Ports {
			if p.PublicPort == 0 {
				continue
			}
			svc := &models.DiscoveredService{
				HostID:       nodeID,
				Name:         c.ContainerName(),
				Port:         p.PublicPort,
				Protocol:     p.Type,
				Process:      c.Image,
				Category:     "container",
				Source:       "docker",
				ContainerID:  c.ID[:12],
				ContainerImg: c.Image,
				Status:       "active",
				LastSeen:     now,
			}
			st.UpsertService(ctx, svc)
		}
	}
}

func runNetworkScan(ctx context.Context, arpScanner *scanners.ARPScanner, mdnsScanner *scanners.MDNSScanner, st *store.Store) {
	var allDevices []plugin.DiscoveredDevice

	// ARP scan
	if arpScanner != nil {
		devices, err := arpScanner.Scan(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("ARP scan failed")
		} else {
			allDevices = append(allDevices, devices...)
		}
	}

	// mDNS scan
	if mdnsScanner != nil {
		devices, err := mdnsScanner.Scan(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("mDNS scan failed")
		} else {
			allDevices = append(allDevices, devices...)
		}
	}

	// Filter out multicast/broadcast addresses
	var filtered []plugin.DiscoveredDevice
	for _, d := range allDevices {
		if strings.HasPrefix(d.IP, "224.") || strings.HasPrefix(d.IP, "239.") || strings.HasPrefix(d.IP, "255.") {
			continue
		}
		if strings.HasPrefix(d.MAC, "01:00:5e") || strings.HasPrefix(d.MAC, "ff:ff:ff") {
			continue
		}
		filtered = append(filtered, d)
	}
	allDevices = filtered

	// Enrich with OUI vendor lookup and classify
	for i := range allDevices {
		if allDevices[i].MAC != "" && allDevices[i].Vendor == "" {
			allDevices[i].Vendor = scanners.LookupVendor(allDevices[i].MAC)
		}
		if allDevices[i].DeviceType == "" && allDevices[i].Vendor != "" {
			allDevices[i].DeviceType = scanners.ClassifyByVendor(allDevices[i].Vendor)
		}
	}

	// Merge by MAC (prefer entry with more info)
	merged := make(map[string]plugin.DiscoveredDevice)
	for _, d := range allDevices {
		key := d.MAC
		if key == "" {
			key = d.IP
		}
		if existing, ok := merged[key]; ok {
			if d.Hostname != "" && existing.Hostname == "" {
				existing.Hostname = d.Hostname
			}
			if d.Vendor != "" && existing.Vendor == "" {
				existing.Vendor = d.Vendor
			}
			if d.DeviceType != "" && existing.DeviceType == "" {
				existing.DeviceType = d.DeviceType
			}
			if d.Source != existing.Source {
				existing.Source = existing.Source + "," + d.Source
			}
			merged[key] = existing
		} else {
			merged[key] = d
		}
	}

	// Store each device
	stored := 0
	for _, d := range merged {
		if err := st.UpsertPassiveDevice(ctx, d); err != nil {
			log.Warn().Err(err).Str("ip", d.IP).Msg("failed to store passive device")
		} else {
			stored++
		}
	}

	if stored > 0 {
		log.Info().Int("devices", stored).Msg("network scan complete")
	}
}

func checkThresholds(ctx context.Context, st *store.Store, dispatcher *notify.Dispatcher, cpuThresh, memThresh, diskThresh float64) {
	hosts, err := st.ListHosts(ctx)
	if err != nil {
		return
	}
	for _, h := range hosts {
		if h.MonitorType != "agent" || h.Status != "online" {
			continue
		}
		metric, err := st.GetLatestMetric(ctx, h.ID)
		if err != nil || metric == nil {
			continue
		}
		if metric.CPUPercent >= cpuThresh {
			dispatcher.Send(notify.FormatThreshold(h.ID, h.Hostname, "CPU", metric.CPUPercent))
		}
		if metric.MemPercent >= memThresh {
			dispatcher.Send(notify.FormatThreshold(h.ID, h.Hostname, "Memory", metric.MemPercent))
		}
		for _, d := range metric.Disks() {
			if d.UsedPercent >= diskThresh {
				dispatcher.Send(notify.FormatThreshold(h.ID, h.Hostname, "Disk "+d.Path, d.UsedPercent))
			}
		}
	}
}

func runIntegrationSync(ctx context.Context, st *store.Store) {
	igs, err := st.ListIntegrations(ctx)
	if err != nil || len(igs) == 0 {
		return
	}

	for _, ig := range igs {
		if !ig.Enabled {
			continue
		}

		url := ig.Config["url"]
		username := ig.Config["username"]
		password, _ := st.GetSecret(ctx, store.SecretKeyID(ig.ID, "password"))

		var devices []plugin.DiscoveredDevice
		var err error

		switch ig.Type {
		case "fritzbox":
			fb := integrations.NewFritzBox(url, username, password)
			devices, err = fb.Sync(ctx)
		case "unifi":
			uf := integrations.NewUnifi(url, username, password)
			devices, err = uf.Sync(ctx)
		case "homeassistant":
			ha := integrations.NewHomeAssistant(url, password)
			devices, err = ha.Sync(ctx)
		case "pihole":
			ph := integrations.NewPiHole(url, password)
			devices, err = ph.Sync(ctx)
		case "pfsense":
			pf := integrations.NewPfSense(url, username, password)
			devices, err = pf.Sync(ctx)
		default:
			continue
		}

		if err != nil {
			st.UpdateIntegrationStatus(ctx, ig.ID, "error", err.Error())
			log.Warn().Err(err).Str("integration", ig.Name).Msg("integration sync failed")
			continue
		}

		stored := 0
		for _, d := range devices {
			if d.IP == "" {
				continue
			}
			if err := st.UpsertPassiveDevice(ctx, d); err == nil {
				stored++
			}
		}
		st.UpdateIntegrationStatus(ctx, ig.ID, "ok", "")
		log.Info().Str("integration", ig.Name).Int("devices", stored).Msg("integration sync complete")
	}
}

func loadOrCreateNodeID(dir string) string {
	idPath := filepath.Join(dir, "node-id")
	if data, err := os.ReadFile(idPath); err == nil {
		id := string(data)
		if len(id) > 0 {
			return id
		}
	}
	os.MkdirAll(dir, 0700)
	id := uuid.New().String()
	os.WriteFile(idPath, []byte(id), 0600)
	return id
}
