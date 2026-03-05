package integrations

import (
	"context"
	"crypto/md5"
	"encoding/xml"
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

// FritzBox integrates with FRITZ!Box routers via TR-064 SOAP API.
type FritzBox struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

var _ plugin.Integration = (*FritzBox)(nil)

func NewFritzBox(baseURL, username, password string) *FritzBox {
	return &FritzBox{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (f *FritzBox) Name() string                  { return "fritzbox" }
func (f *FritzBox) Type() models.PluginType       { return models.PluginTypeIntegration }
func (f *FritzBox) Detect() bool                  { return f.baseURL != "" }
func (f *FritzBox) Interval() time.Duration       { return 5 * time.Minute }
func (f *FritzBox) Start(_ context.Context) error { return nil }
func (f *FritzBox) Stop() error                   { return nil }

func (f *FritzBox) Configure(config map[string]string) error {
	if url, ok := config["url"]; ok {
		f.baseURL = strings.TrimRight(url, "/")
	}
	if u, ok := config["username"]; ok {
		f.username = u
	}
	if p, ok := config["password"]; ok {
		f.password = p
	}
	return nil
}

// Sync fetches all known hosts from the FRITZ!Box.
func (f *FritzBox) Sync(ctx context.Context) ([]plugin.DiscoveredDevice, error) {
	// 1. Get host count
	count, err := f.getHostCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("get host count: %w", err)
	}

	log.Debug().Int("count", count).Msg("fritzbox host count")

	// 2. Iterate all hosts
	var devices []plugin.DiscoveredDevice
	for i := 0; i < count; i++ {
		dev, err := f.getHostEntry(ctx, i)
		if err != nil {
			log.Debug().Err(err).Int("index", i).Msg("fritzbox skip host")
			continue
		}
		if dev.MAC == "" && dev.IP == "" {
			continue
		}
		devices = append(devices, *dev)
	}

	return devices, nil
}

// Ping tests connectivity to the FRITZ!Box.
func (f *FritzBox) Ping(ctx context.Context) error {
	_, err := f.getHostCount(ctx)
	return err
}

func (f *FritzBox) getHostCount(ctx context.Context) (int, error) {
	body := soapEnvelope("Hosts:1", "GetHostNumberOfEntries", "")
	resp, err := f.soapCall(ctx, "/upnp/control/hosts", "urn:dslforum-org:service:Hosts:1#GetHostNumberOfEntries", body)
	if err != nil {
		return 0, err
	}

	var env struct {
		Body struct {
			Resp struct {
				Count int `xml:"NewHostNumberOfEntries"`
			} `xml:"GetHostNumberOfEntriesResponse"`
		}
	}
	if err := xml.Unmarshal(resp, &env); err != nil {
		return 0, err
	}
	return env.Body.Resp.Count, nil
}

func (f *FritzBox) getHostEntry(ctx context.Context, index int) (*plugin.DiscoveredDevice, error) {
	body := soapEnvelope("Hosts:1", "GetGenericHostEntry",
		fmt.Sprintf("<NewIndex>%d</NewIndex>", index))
	resp, err := f.soapCall(ctx, "/upnp/control/hosts", "urn:dslforum-org:service:Hosts:1#GetGenericHostEntry", body)
	if err != nil {
		return nil, err
	}

	var env struct {
		Body struct {
			Resp struct {
				IP       string `xml:"NewIPAddress"`
				MAC      string `xml:"NewMACAddress"`
				Hostname string `xml:"NewHostName"`
				Active   int    `xml:"NewActive"`
				IFType   string `xml:"NewInterfaceType"`
				Source   string `xml:"NewAddressSource"`
			} `xml:"GetGenericHostEntryResponse"`
		}
	}
	if err := xml.Unmarshal(resp, &env); err != nil {
		return nil, err
	}

	r := env.Body.Resp
	mac := strings.ToLower(r.MAC)

	// Lookup vendor from OUI
	vendor := scanners.LookupVendor(mac)
	devType := ""
	if vendor != "" {
		devType = scanners.ClassifyByVendor(vendor)
	}

	dev := &plugin.DiscoveredDevice{
		IP:         r.IP,
		MAC:        mac,
		Hostname:   r.Hostname,
		Vendor:     vendor,
		DeviceType: devType,
		Source:     "fritzbox",
	}

	return dev, nil
}

// soapCall performs a TR-064 SOAP request with HTTP Digest Auth.
func (f *FritzBox) soapCall(ctx context.Context, path, action, body string) ([]byte, error) {
	url := f.baseURL + path

	// First request (will get 401 with digest challenge)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(body))
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SoapAction", action)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode == 200 {
		// No auth needed for this call (shouldn't happen with GetHostListPath but possible)
		req2, _ := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(body))
		req2.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
		req2.Header.Set("SoapAction", action)
		resp2, err := f.client.Do(req2)
		if err != nil {
			return nil, err
		}
		defer resp2.Body.Close()
		return io.ReadAll(resp2.Body)
	}

	if resp.StatusCode != 401 {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	// Parse digest challenge
	challenge := resp.Header.Get("WWW-Authenticate")
	if challenge == "" {
		return nil, fmt.Errorf("no WWW-Authenticate header")
	}

	digest := parseDigestChallenge(challenge)
	if digest.realm == "" || digest.nonce == "" {
		return nil, fmt.Errorf("invalid digest challenge")
	}

	// Compute digest auth header
	authHeader := computeDigestAuth(f.username, f.password, "POST", path, digest)

	// Retry with auth
	req2, _ := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(body))
	req2.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req2.Header.Set("SoapAction", action)
	req2.Header.Set("Authorization", authHeader)

	resp2, err := f.client.Do(req2)
	if err != nil {
		return nil, err
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != 200 {
		b, _ := io.ReadAll(resp2.Body)
		return nil, fmt.Errorf("status %d: %s", resp2.StatusCode, string(b))
	}

	return io.ReadAll(resp2.Body)
}

func soapEnvelope(service, action, params string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:%s xmlns:u="urn:dslforum-org:service:%s">%s</u:%s>
  </s:Body>
</s:Envelope>`, action, service, params, action)
}

// HTTP Digest Auth implementation

type digestChallenge struct {
	realm  string
	nonce  string
	qop    string
	opaque string
}

func parseDigestChallenge(header string) digestChallenge {
	d := digestChallenge{}
	header = strings.TrimPrefix(header, "Digest ")
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if k, v, ok := strings.Cut(part, "="); ok {
			v = strings.Trim(v, `"`)
			switch strings.TrimSpace(k) {
			case "realm":
				d.realm = v
			case "nonce":
				d.nonce = v
			case "qop":
				d.qop = v
			case "opaque":
				d.opaque = v
			}
		}
	}
	return d
}

func computeDigestAuth(username, password, method, uri string, d digestChallenge) string {
	ha1 := md5hex(fmt.Sprintf("%s:%s:%s", username, d.realm, password))
	ha2 := md5hex(fmt.Sprintf("%s:%s", method, uri))

	nc := "00000001"
	cnonce := md5hex(fmt.Sprintf("%d", time.Now().UnixNano()))[:16]

	var response string
	if d.qop == "auth" || strings.Contains(d.qop, "auth") {
		response = md5hex(fmt.Sprintf("%s:%s:%s:%s:%s:%s", ha1, d.nonce, nc, cnonce, "auth", ha2))
	} else {
		response = md5hex(fmt.Sprintf("%s:%s:%s", ha1, d.nonce, ha2))
	}

	parts := []string{
		fmt.Sprintf(`username="%s"`, username),
		fmt.Sprintf(`realm="%s"`, d.realm),
		fmt.Sprintf(`nonce="%s"`, d.nonce),
		fmt.Sprintf(`uri="%s"`, uri),
		fmt.Sprintf(`response="%s"`, response),
	}
	if d.qop != "" {
		parts = append(parts, `qop=auth`, fmt.Sprintf("nc=%s", nc), fmt.Sprintf(`cnonce="%s"`, cnonce))
	}
	if d.opaque != "" {
		parts = append(parts, fmt.Sprintf(`opaque="%s"`, d.opaque))
	}

	return "Digest " + strings.Join(parts, ", ")
}

func md5hex(s string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(s)))
}
