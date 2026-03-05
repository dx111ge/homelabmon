package notify

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Severity levels for notifications.
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
	SeverityResolved = "resolved"
)

// Notification represents an alert to send.
type Notification struct {
	Title    string `json:"title"`
	Message  string `json:"message"`
	Severity string `json:"severity"` // info, warning, critical, resolved
	HostID   string `json:"host_id,omitempty"`
	Hostname string `json:"hostname,omitempty"`
	Category string `json:"category,omitempty"` // host_offline, host_online, cpu, memory, disk, service
}

// Sender sends notifications to an external service.
type Sender interface {
	Name() string
	Send(n Notification) error
}

// Dispatcher manages multiple senders and deduplicates notifications.
type Dispatcher struct {
	mu       sync.Mutex
	senders  []Sender
	cooldown map[string]time.Time // key -> last sent time
	interval time.Duration        // minimum interval between same notification
}

// NewDispatcher creates a dispatcher with a dedup cooldown interval.
func NewDispatcher(cooldownInterval time.Duration) *Dispatcher {
	return &Dispatcher{
		cooldown: make(map[string]time.Time),
		interval: cooldownInterval,
	}
}

// AddSender registers a notification sender.
func (d *Dispatcher) AddSender(s Sender) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.senders = append(d.senders, s)
	log.Info().Str("sender", s.Name()).Msg("notification sender registered")
}

// HasSenders returns true if any senders are registered.
func (d *Dispatcher) HasSenders() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.senders) > 0
}

// Send dispatches a notification to all senders, with dedup.
func (d *Dispatcher) Send(n Notification) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := dedupKey(n)
	if last, ok := d.cooldown[key]; ok && time.Since(last) < d.interval {
		return // cooldown active, skip
	}
	d.cooldown[key] = time.Now()

	for _, s := range d.senders {
		if err := s.Send(n); err != nil {
			log.Warn().Err(err).Str("sender", s.Name()).Msg("notification send failed")
		} else {
			log.Debug().Str("sender", s.Name()).Str("title", n.Title).Msg("notification sent")
		}
	}
}

// SenderNames returns the names of all registered senders.
func (d *Dispatcher) SenderNames() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	names := make([]string, len(d.senders))
	for i, s := range d.senders {
		names[i] = s.Name()
	}
	return names
}

func dedupKey(n Notification) string {
	return fmt.Sprintf("%s:%s:%s", n.Category, n.HostID, n.Severity)
}

// FormatHostOffline creates a host-offline notification.
func FormatHostOffline(hostID, hostname string) Notification {
	return Notification{
		Title:    fmt.Sprintf("%s went offline", hostname),
		Message:  fmt.Sprintf("Host %s (%s) is no longer responding.", hostname, shortID(hostID)),
		Severity: SeverityCritical,
		HostID:   hostID,
		Hostname: hostname,
		Category: "host_offline",
	}
}

// FormatHostOnline creates a host-back-online notification.
func FormatHostOnline(hostID, hostname string) Notification {
	return Notification{
		Title:    fmt.Sprintf("%s is back online", hostname),
		Message:  fmt.Sprintf("Host %s (%s) is responding again.", hostname, shortID(hostID)),
		Severity: SeverityResolved,
		HostID:   hostID,
		Hostname: hostname,
		Category: "host_online",
	}
}

// FormatThreshold creates a resource threshold notification.
func FormatThreshold(hostID, hostname, resource string, percent float64) Notification {
	return Notification{
		Title:    fmt.Sprintf("%s: %s at %.0f%%", hostname, resource, percent),
		Message:  fmt.Sprintf("Host %s has %s usage at %.1f%%.", hostname, strings.ToLower(resource), percent),
		Severity: SeverityWarning,
		HostID:   hostID,
		Hostname: hostname,
		Category: strings.ToLower(resource),
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
