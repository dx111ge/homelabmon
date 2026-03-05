package scanners

import (
	"context"
	"fmt"
	"strings"
	"sync"
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
	// Hard deadline for the entire scan — all queries run in parallel.
	// hashicorp/mdns.Query can deadlock; QueryContext + deadline ensures
	// we always return in bounded time even if the library hangs.
	scanCtx, scanCancel := context.WithTimeout(ctx, 15*time.Second)
	defer scanCancel()

	var mu sync.Mutex
	seen := make(map[string]*plugin.DiscoveredDevice)

	var wg sync.WaitGroup
	for _, svcType := range mdnsServiceTypes {
		wg.Add(1)
		go func(svc string) {
			defer wg.Done()
			queryMDNS(scanCtx, svc, &mu, seen)
		}(svcType)
	}

	// Wait for all queries or context cancellation.
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-scanCtx.Done():
		// Some queries may still be stuck — they'll be cleaned up when
		// scanCtx cancels their QueryContext. We proceed with what we have.
	}

	mu.Lock()
	devices := make([]plugin.DiscoveredDevice, 0, len(seen))
	for _, d := range seen {
		devices = append(devices, *d)
	}
	mu.Unlock()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return devices, nil
}

// queryMDNS runs a single mDNS query for one service type with its own
// context deadline. Results are merged into the shared seen map.
func queryMDNS(ctx context.Context, svcType string, mu *sync.Mutex, seen map[string]*plugin.DiscoveredDevice) {
	// Per-query timeout — if the library hangs, context cancellation
	// closes the underlying multicast client.
	qCtx, qCancel := context.WithTimeout(ctx, 5*time.Second)
	defer qCancel()

	entries := make(chan *mdns.ServiceEntry, 32)

	go func() {
		params := &mdns.QueryParam{
			Service:             svcType,
			Domain:              "local",
			Timeout:             3 * time.Second,
			Entries:             entries,
			WantUnicastResponse: false,
		}
		// QueryContext will close the client when qCtx is cancelled,
		// unblocking any stuck internal goroutines.
		mdns.QueryContext(qCtx, params)
		close(entries)
	}()

	for {
		select {
		case entry, ok := <-entries:
			if !ok {
				return
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

			mu.Lock()
			dev, exists := seen[ip]
			if !exists {
				dev = &plugin.DiscoveredDevice{
					IP:       ip,
					Hostname: hostname,
					Source:   "mdns",
				}
				seen[ip] = dev
			}
			if dev.DeviceType == "" {
				dev.DeviceType = classifyMDNS(svcType, entry.Name)
			}
			mu.Unlock()
		case <-qCtx.Done():
			return
		}
	}
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
