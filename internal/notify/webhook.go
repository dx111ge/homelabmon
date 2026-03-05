package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookSender sends JSON notifications to a generic webhook URL.
// Compatible with Discord, Slack (via incoming webhooks), or any custom endpoint.
type WebhookSender struct {
	url    string
	client *http.Client
}

// NewWebhookSender creates a generic webhook sender.
func NewWebhookSender(url string) *WebhookSender {
	return &WebhookSender{
		url: url,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *WebhookSender) Name() string { return "webhook" }

type webhookPayload struct {
	Title    string `json:"title"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
	HostID   string `json:"host_id,omitempty"`
	Hostname string `json:"hostname,omitempty"`
	Category string `json:"category,omitempty"`
	Time     string `json:"time"`
	// Discord/Slack compatibility
	Content string `json:"content,omitempty"`
	Text    string `json:"text,omitempty"`
}

func (s *WebhookSender) Send(n Notification) error {
	icon := severityIcon(n.Severity)
	formatted := fmt.Sprintf("%s **%s** - %s", icon, n.Title, n.Message)

	payload := webhookPayload{
		Title:    n.Title,
		Message:  n.Message,
		Severity: n.Severity,
		HostID:   n.HostID,
		Hostname: n.Hostname,
		Category: n.Category,
		Time:     time.Now().UTC().Format(time.RFC3339),
		Content:  formatted, // Discord
		Text:     formatted, // Slack
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(s.url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func severityIcon(severity string) string {
	switch severity {
	case SeverityCritical:
		return "[CRITICAL]"
	case SeverityWarning:
		return "[WARNING]"
	case SeverityResolved:
		return "[RESOLVED]"
	default:
		return "[INFO]"
	}
}
