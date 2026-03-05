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

// PiHole integrates with Pi-hole DNS sinkhole via its admin API.
type PiHole struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

var _ plugin.Integration = (*PiHole)(nil)

func NewPiHole(baseURL, apiKey string) *PiHole {
	return &PiHole{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *PiHole) Name() string                  { return "pihole" }
func (p *PiHole) Type() models.PluginType       { return models.PluginTypeIntegration }
func (p *PiHole) Detect() bool                  { return p.baseURL != "" && p.apiKey != "" }
func (p *PiHole) Interval() time.Duration       { return 5 * time.Minute }
func (p *PiHole) Start(_ context.Context) error { return nil }
func (p *PiHole) Stop() error                   { return nil }

func (p *PiHole) Configure(config map[string]string) error {
	if url, ok := config["url"]; ok {
		p.baseURL = strings.TrimRight(url, "/")
	}
	if key, ok := config["api_key"]; ok {
		p.apiKey = key
	}
	return nil
}

// networkEntry represents a single client from the Pi-hole network API.
type networkEntry struct {
	ID         int    `json:"id"`
	IP         string `json:"ip"`
	MAC        string `json:"mac"`
	HWAddr     string `json:"hwaddr"`
	Name       string `json:"name"`
	LastQuery  int64  `json:"lastQuery"`
	NumQueries int    `json:"numQueries"`
	MACVendor  string `json:"macVendor"`
}

// networkResponse is the top-level response from /admin/api.php?network.
type networkResponse struct {
	Network []networkEntry `json:"network"`
}

// Sync fetches all known network clients from Pi-hole.
func (p *PiHole) Sync(ctx context.Context) ([]plugin.DiscoveredDevice, error) {
	url := fmt.Sprintf("%s/admin/api.php?network&auth=%s", p.baseURL, p.apiKey)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("pihole: create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pihole: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pihole: status %d: %s", resp.StatusCode, string(body))
	}

	var result networkResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("pihole: decode response: %w", err)
	}

	log.Debug().Int("count", len(result.Network)).Msg("pihole network entries")

	var devices []plugin.DiscoveredDevice
	for _, entry := range result.Network {
		mac := strings.ToLower(entry.MAC)
		if mac == "" {
			mac = strings.ToLower(entry.HWAddr)
		}

		ip := entry.IP
		if mac == "" && ip == "" {
			continue
		}

		vendor := entry.MACVendor
		if vendor == "" && mac != "" {
			vendor = scanners.LookupVendor(mac)
		}

		devType := ""
		if vendor != "" {
			devType = scanners.ClassifyByVendor(vendor)
		}

		devices = append(devices, plugin.DiscoveredDevice{
			IP:         ip,
			MAC:        mac,
			Hostname:   entry.Name,
			Vendor:     vendor,
			DeviceType: devType,
			Source:     "pihole",
		})
	}

	return devices, nil
}

// Ping tests connectivity to the Pi-hole API by calling the summary endpoint.
func (p *PiHole) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/admin/api.php?summary&auth=%s", p.baseURL, p.apiKey)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("pihole: create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("pihole: ping failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pihole: ping status %d", resp.StatusCode)
	}

	var summary map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return fmt.Errorf("pihole: decode summary: %w", err)
	}

	if _, ok := summary["status"]; !ok {
		return fmt.Errorf("pihole: summary missing status field")
	}

	return nil
}
