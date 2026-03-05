package integrations

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/dx111ge/homelabmon/internal/agent/scanners"
	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/plugin"
	"github.com/rs/zerolog/log"
)

// Unifi integrates with UniFi Controller via its REST API.
type Unifi struct {
	baseURL  string
	username string
	password string
	site     string
	client   *http.Client
}

var _ plugin.Integration = (*Unifi)(nil)

func NewUnifi(baseURL, username, password string) *Unifi {
	jar, _ := cookiejar.New(nil)
	return &Unifi{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		site:     "default",
		client: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

func (u *Unifi) Name() string                  { return "unifi" }
func (u *Unifi) Type() models.PluginType       { return models.PluginTypeIntegration }
func (u *Unifi) Detect() bool                  { return u.baseURL != "" }
func (u *Unifi) Interval() time.Duration       { return 5 * time.Minute }
func (u *Unifi) Start(_ context.Context) error { return nil }
func (u *Unifi) Stop() error                   { return nil }

func (u *Unifi) Configure(config map[string]string) error {
	if url, ok := config["url"]; ok {
		u.baseURL = strings.TrimRight(url, "/")
	}
	if usr, ok := config["username"]; ok {
		u.username = usr
	}
	if pw, ok := config["password"]; ok {
		u.password = pw
	}
	if site, ok := config["site"]; ok && site != "" {
		u.site = site
	}
	return nil
}

// Sync logs in to the UniFi Controller and fetches all known clients.
func (u *Unifi) Sync(ctx context.Context) ([]plugin.DiscoveredDevice, error) {
	if err := u.login(ctx); err != nil {
		return nil, fmt.Errorf("unifi login: %w", err)
	}

	clients, err := u.fetchClients(ctx)
	if err != nil {
		return nil, fmt.Errorf("unifi fetch clients: %w", err)
	}

	log.Debug().Int("count", len(clients)).Msg("unifi client count")

	var devices []plugin.DiscoveredDevice
	for _, c := range clients {
		mac := strings.ToLower(c.MAC)
		if mac == "" {
			continue
		}

		hostname := c.Hostname
		if hostname == "" {
			hostname = c.Name
		}

		// Use OUI from controller if available; fall back to local lookup.
		vendor := c.OUI
		if vendor == "" {
			vendor = scanners.LookupVendor(mac)
		}

		devType := ""
		if vendor != "" {
			devType = scanners.ClassifyByVendor(vendor)
		}

		devices = append(devices, plugin.DiscoveredDevice{
			IP:         c.IP,
			MAC:        mac,
			Hostname:   hostname,
			Vendor:     vendor,
			DeviceType: devType,
			Source:     "unifi",
		})
	}

	return devices, nil
}

// Ping tests connectivity by performing a login to the controller.
func (u *Unifi) Ping(ctx context.Context) error {
	return u.login(ctx)
}

// login authenticates with the UniFi Controller and stores the session cookie.
func (u *Unifi) login(ctx context.Context) error {
	payload, err := json.Marshal(map[string]string{
		"username": u.username,
		"password": u.password,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", u.baseURL+"/api/login", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// unifiClient represents a single client entry from the UniFi Controller API.
type unifiClient struct {
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	Name     string `json:"name"`
	OUI      string `json:"oui"`
	IsWired  bool   `json:"is_wired"`
	IsGuest  bool   `json:"_is_guest_by_uap"`
	LastSeen int64  `json:"last_seen"`
	Network  string `json:"network"`
}

// fetchClients retrieves the list of connected clients from the controller.
func (u *Unifi) fetchClients(ctx context.Context) ([]unifiClient, error) {
	url := fmt.Sprintf("%s/api/s/%s/stat/sta", u.baseURL, u.site)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch clients: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch clients status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []unifiClient `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode clients: %w", err)
	}

	return result.Data, nil
}
