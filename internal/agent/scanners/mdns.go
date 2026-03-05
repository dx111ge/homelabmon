package scanners

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/mdns"
	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/plugin"
)

// Common mDNS service types to browse.
var mdnsServiceTypes = []string{
	"_http._tcp",
	"_https._tcp",
	"_airplay._tcp",
	"_raop._tcp",
	"_smb._tcp",
	"_ssh._tcp",
	"_printer._tcp",
	"_ipp._tcp",
	"_googlecast._tcp",
	"_spotify-connect._tcp",
	"_hap._tcp",           // HomeKit
	"_companion-link._tcp", // Apple devices
	"_sleep-proxy._udp",
}

// MDNSScanner discovers devices advertising via mDNS/Bonjour.
type MDNSScanner struct{}

var _ plugin.Scanner = (*MDNSScanner)(nil)

func (s *MDNSScanner) Name() string                  { return "mdns" }
func (s *MDNSScanner) Type() models.PluginType       { return models.PluginTypeScanner }
func (s *MDNSScanner) Detect() bool                  { return true }
func (s *MDNSScanner) Interval() time.Duration       { return 5 * time.Minute }
func (s *MDNSScanner) Start(_ context.Context) error { return nil }
func (s *MDNSScanner) Stop() error                   { return nil }

func (s *MDNSScanner) Scan(ctx context.Context) ([]plugin.DiscoveredDevice, error) {
	seen := make(map[string]*plugin.DiscoveredDevice) // keyed by IP

	for _, svcType := range mdnsServiceTypes {
		entries := make(chan *mdns.ServiceEntry, 32)

		go func() {
			params := &mdns.QueryParam{
				Service:             svcType,
				Domain:              "local",
				Timeout:             3 * time.Second,
				Entries:             entries,
				WantUnicastResponse: false,
			}
			mdns.Query(params)
		}()

		timer := time.NewTimer(4 * time.Second)
	collect:
		for {
			select {
			case entry, ok := <-entries:
				if !ok {
					break collect
				}
				ip := ""
				if entry.AddrV4 != nil {
					ip = entry.AddrV4.String()
				} else if entry.AddrV6 != nil {
					ip = entry.AddrV6.String()
				}
				if ip == "" {
					continue
				}

				hostname := strings.TrimSuffix(entry.Host, ".")
				if hostname == "" {
					hostname = entry.Name
				}

				dev, exists := seen[ip]
				if !exists {
					dev = &plugin.DiscoveredDevice{
						IP:       ip,
						Hostname: hostname,
						Source:   "mdns",
					}
					seen[ip] = dev
				}
				// Classify based on service type
				if dev.DeviceType == "" {
					dev.DeviceType = classifyMDNS(svcType, entry.Name)
				}
			case <-timer.C:
				break collect
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
		}
		timer.Stop()
	}

	var devices []plugin.DiscoveredDevice
	for _, d := range seen {
		devices = append(devices, *d)
	}
	return devices, nil
}

// classifyMDNS guesses device type from mDNS service type and name.
func classifyMDNS(svcType, name string) string {
	lower := strings.ToLower(fmt.Sprintf("%s %s", svcType, name))
	switch {
	case strings.Contains(lower, "_airplay") || strings.Contains(lower, "_raop"):
		if strings.Contains(lower, "apple tv") || strings.Contains(lower, "appletv") {
			return "tv"
		}
		return "media"
	case strings.Contains(lower, "_googlecast"):
		if strings.Contains(lower, "chromecast") {
			return "media"
		}
		if strings.Contains(lower, "tv") || strings.Contains(lower, "display") {
			return "tv"
		}
		return "media"
	case strings.Contains(lower, "_printer") || strings.Contains(lower, "_ipp"):
		return "printer"
	case strings.Contains(lower, "_hap") || strings.Contains(lower, "_companion-link"):
		return "phone"
	case strings.Contains(lower, "_spotify"):
		return "media"
	default:
		return ""
	}
}
