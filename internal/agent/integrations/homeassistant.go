package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dx111ge/homelabmon/internal/agent/scanners"
	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/plugin"
	"github.com/rs/zerolog/log"
)

// HomeAssistant integrates with Home Assistant via its REST API.
type HomeAssistant struct {
	baseURL string
	token   string
	client  *http.Client
}

var _ plugin.Integration = (*HomeAssistant)(nil)

func NewHomeAssistant(url, token string) *HomeAssistant {
	return &HomeAssistant{
		baseURL: strings.TrimRight(url, "/"),
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (h *HomeAssistant) Name() string                  { return "homeassistant" }
func (h *HomeAssistant) Type() models.PluginType       { return models.PluginTypeIntegration }
func (h *HomeAssistant) Detect() bool                  { return h.baseURL != "" && h.token != "" }
func (h *HomeAssistant) Interval() time.Duration       { return 5 * time.Minute }
func (h *HomeAssistant) Start(_ context.Context) error { return nil }
func (h *HomeAssistant) Stop() error                   { return nil }

func (h *HomeAssistant) Configure(config map[string]string) error {
	if url, ok := config["url"]; ok {
		h.baseURL = strings.TrimRight(url, "/")
	}
	if t, ok := config["token"]; ok {
		h.token = t
	}
	return nil
}

// haState represents a single entity state from the Home Assistant API.
type haState struct {
	EntityID string                 `json:"entity_id"`
	State    string                 `json:"state"`
	Attrs    map[string]interface{} `json:"attributes"`
}

// Sync fetches all device_tracker entities from Home Assistant and converts
// them to DiscoveredDevice entries.
func (h *HomeAssistant) Sync(ctx context.Context) ([]plugin.DiscoveredDevice, error) {
	states, err := h.getStates(ctx)
	if err != nil {
		return nil, fmt.Errorf("get states: %w", err)
	}

	log.Debug().Int("total_entities", len(states)).Msg("homeassistant fetched states")

	var devices []plugin.DiscoveredDevice
	for _, s := range states {
		if !strings.HasPrefix(s.EntityID, "device_tracker.") {
			continue
		}

		dev := h.stateToDevice(s)
		if dev.MAC == "" && dev.IP == "" {
			continue
		}
		devices = append(devices, dev)
	}

	log.Debug().Int("count", len(devices)).Msg("homeassistant device_tracker devices")
	return devices, nil
}

// Ping tests connectivity to the Home Assistant API.
func (h *HomeAssistant) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", h.baseURL+"/api/", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)

	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (h *HomeAssistant) getStates(ctx context.Context) ([]haState, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", h.baseURL+"/api/states", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}

	var states []haState
	if err := json.NewDecoder(resp.Body).Decode(&states); err != nil {
		return nil, fmt.Errorf("decode states: %w", err)
	}
	return states, nil
}

func (h *HomeAssistant) stateToDevice(s haState) plugin.DiscoveredDevice {
	ip := attrString(s.Attrs, "ip")
	mac := strings.ToLower(attrString(s.Attrs, "mac"))
	hostname := attrString(s.Attrs, "host_name")

	// Fall back to friendly_name if no hostname
	if hostname == "" {
		hostname = attrString(s.Attrs, "friendly_name")
	}

	// OUI vendor lookup
	vendor := ""
	devType := ""
	if mac != "" {
		vendor = scanners.LookupVendor(mac)
		if vendor != "" {
			devType = scanners.ClassifyByVendor(vendor)
		}
	}

	return plugin.DiscoveredDevice{
		IP:         ip,
		MAC:        mac,
		Hostname:   hostname,
		Vendor:     vendor,
		DeviceType: devType,
		Source:     "homeassistant",
	}
}

// attrString extracts a string value from an attributes map.
func attrString(attrs map[string]interface{}, key string) string {
	v, ok := attrs[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
