package integrations

import (
	"context"
	"crypto/tls"
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

// PfSense integrates with pfSense firewalls via the pfSense REST API.
// Authentication uses basic auth (username/password).
// TLS verification is skipped since pfSense commonly uses self-signed certs.
type PfSense struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

var _ plugin.Integration = (*PfSense)(nil)

func NewPfSense(baseURL, username, password string) *PfSense {
	return &PfSense{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}
}

func (p *PfSense) Name() string                  { return "pfsense" }
func (p *PfSense) Type() models.PluginType       { return models.PluginTypeIntegration }
func (p *PfSense) Detect() bool                  { return p.baseURL != "" }
func (p *PfSense) Interval() time.Duration       { return 5 * time.Minute }
func (p *PfSense) Start(_ context.Context) error { return nil }
func (p *PfSense) Stop() error                   { return nil }

func (p *PfSense) Configure(config map[string]string) error {
	if url, ok := config["url"]; ok {
		p.baseURL = strings.TrimRight(url, "/")
	}
	if u, ok := config["username"]; ok {
		p.username = u
	}
	if pw, ok := config["password"]; ok {
		p.password = pw
	}
	return nil
}

// arpResponse represents the pfSense ARP API response.
type arpResponse struct {
	Data []arpEntry `json:"data"`
}

// arpEntry represents a single ARP table entry from pfSense.
type arpEntry struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Hostname  string `json:"hostname"`
	Interface string `json:"interface"`
	Status    string `json:"status"`
	Expires   string `json:"expires"`
}

// Sync fetches the ARP table from pfSense and returns discovered devices.
func (p *PfSense) Sync(ctx context.Context) ([]plugin.DiscoveredDevice, error) {
	body, err := p.apiGet(ctx, "/api/v1/diagnostics/arp")
	if err != nil {
		return nil, fmt.Errorf("fetch arp table: %w", err)
	}

	var resp arpResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse arp response: %w", err)
	}

	log.Debug().Int("count", len(resp.Data)).Msg("pfsense arp entries")

	var devices []plugin.DiscoveredDevice
	for _, entry := range resp.Data {
		mac := strings.ToLower(entry.MAC)
		if mac == "" && entry.IP == "" {
			continue
		}

		vendor := scanners.LookupVendor(mac)
		devType := ""
		if vendor != "" {
			devType = scanners.ClassifyByVendor(vendor)
		}

		devices = append(devices, plugin.DiscoveredDevice{
			IP:         entry.IP,
			MAC:        mac,
			Hostname:   entry.Hostname,
			Vendor:     vendor,
			DeviceType: devType,
			Source:     "pfsense",
		})
	}

	return devices, nil
}

// Ping tests connectivity to the pfSense API by fetching the ARP endpoint.
func (p *PfSense) Ping(ctx context.Context) error {
	_, err := p.apiGet(ctx, "/api/v1/diagnostics/arp")
	return err
}

// apiGet performs an authenticated GET request to the pfSense REST API.
func (p *PfSense) apiGet(ctx context.Context, path string) ([]byte, error) {
	url := p.baseURL + path

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(p.username, p.password)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}

	return io.ReadAll(resp.Body)
}
