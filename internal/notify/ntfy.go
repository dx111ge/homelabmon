package notify

import (
	"bytes"
	"fmt"
	"net/http"
	"time"
)

// NtfySender sends notifications to an ntfy.sh topic.
type NtfySender struct {
	url    string // e.g. "https://ntfy.sh/homelabmon-alerts"
	client *http.Client
}

// NewNtfySender creates a sender for ntfy.sh (or a self-hosted ntfy instance).
// url should be the full topic URL, e.g. "https://ntfy.sh/my-topic" or "http://localhost:8080/my-topic".
func NewNtfySender(url string) *NtfySender {
	return &NtfySender{
		url: url,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *NtfySender) Name() string { return "ntfy" }

func (s *NtfySender) Send(n Notification) error {
	req, err := http.NewRequest("POST", s.url, bytes.NewBufferString(n.Message))
	if err != nil {
		return err
	}
	req.Header.Set("Title", n.Title)
	req.Header.Set("Tags", ntfyTags(n.Severity))
	req.Header.Set("Priority", ntfyPriority(n.Severity))

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ntfy returned status %d", resp.StatusCode)
	}
	return nil
}

func ntfyPriority(severity string) string {
	switch severity {
	case SeverityCritical:
		return "high"
	case SeverityWarning:
		return "default"
	case SeverityResolved:
		return "low"
	default:
		return "default"
	}
}

func ntfyTags(severity string) string {
	switch severity {
	case SeverityCritical:
		return "rotating_light"
	case SeverityWarning:
		return "warning"
	case SeverityResolved:
		return "white_check_mark"
	default:
		return "information_source"
	}
}
