// Package notify delivers event notifications to channels. It supports three
// providers per the house standard: Shoutrrr (Slack/Discord/Telegram/SMTP/...),
// GreenAPI (WhatsApp cloud) and a self-hosted WhatsApp Web gateway. Channel
// credentials are encrypted at rest by the caller; the Notifier receives
// already-decrypted channels.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/containrrr/shoutrrr"
)

// Provider identifies a channel's delivery backend.
type Provider string

const (
	ProviderShoutrrr    Provider = "shoutrrr"
	ProviderGreenAPI    Provider = "greenapi"
	ProviderWhatsAppWeb Provider = "whatsapp_web"
)

// Channel is a notification target. Secret fields (URL, Token, Password) are
// stored encrypted; the Notifier is given decrypted copies.
type Channel struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Provider        Provider `json:"provider"`
	Enabled         bool     `json:"enabled"`
	NotifyOnSuccess bool     `json:"notify_on_success"`
	NotifyOnFailure bool     `json:"notify_on_failure"`

	// Shoutrrr.
	URL string `json:"url,omitempty"`

	// GreenAPI (WhatsApp cloud).
	InstanceID string `json:"instance_id,omitempty"`
	Token      string `json:"token,omitempty"`
	Phone      string `json:"phone,omitempty"`
	APIURL     string `json:"api_url,omitempty"`

	// WhatsApp Web (self-hosted).
	BaseURL  string `json:"base_url,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// SecretFields returns the names of the encrypted-at-rest fields.
func SecretFields() []string { return []string{"url", "token", "password"} }

// Notifier sends messages to channels.
type Notifier struct {
	client *http.Client
}

// New returns a Notifier.
func New() *Notifier {
	return &Notifier{client: &http.Client{Timeout: 20 * time.Second}}
}

// Send delivers message to a single channel.
func (n *Notifier) Send(ctx context.Context, ch Channel, message string) error {
	switch ch.Provider {
	case ProviderShoutrrr:
		return sendShoutrrr(ch.URL, message)
	case ProviderGreenAPI:
		return n.sendGreenAPI(ctx, ch, message)
	case ProviderWhatsAppWeb:
		return n.sendWhatsAppWeb(ctx, ch, message)
	default:
		return fmt.Errorf("unknown provider %q", ch.Provider)
	}
}

func sendShoutrrr(url, message string) error {
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("shoutrrr url is empty")
	}
	if err := shoutrrr.Send(url, message); err != nil {
		return fmt.Errorf("shoutrrr send: %w", err)
	}
	return nil
}

func (n *Notifier) sendGreenAPI(ctx context.Context, ch Channel, message string) error {
	apiURL := strings.TrimSpace(ch.APIURL)
	if apiURL == "" {
		apiURL = "https://api.green-api.com"
	}
	instance := strings.TrimSpace(ch.InstanceID)
	token := strings.TrimSpace(ch.Token)
	phone := strings.TrimSpace(ch.Phone)
	if instance == "" || token == "" || phone == "" {
		return fmt.Errorf("greenapi requires instance_id, token and phone")
	}
	chatID := phone
	if !strings.Contains(chatID, "@") {
		chatID += "@c.us"
	}
	url := fmt.Sprintf("%s/waInstance%s/sendMessage/%s", strings.TrimRight(apiURL, "/"), instance, token)
	body, _ := json.Marshal(map[string]string{"chatId": chatID, "message": message})
	return n.postJSON(ctx, url, body, "", "")
}

func (n *Notifier) sendWhatsAppWeb(ctx context.Context, ch Channel, message string) error {
	base := strings.TrimRight(strings.TrimSpace(ch.BaseURL), "/")
	if base == "" || strings.TrimSpace(ch.Phone) == "" {
		return fmt.Errorf("whatsapp_web requires base_url and phone")
	}
	url := base + "/send/message"
	body, _ := json.Marshal(map[string]string{"phone": strings.TrimSpace(ch.Phone), "message": message})
	return n.postJSON(ctx, url, body, ch.Username, ch.Password)
}

func (n *Notifier) postJSON(ctx context.Context, url string, body []byte, user, pass string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notification endpoint returned %d", resp.StatusCode)
	}
	return nil
}
