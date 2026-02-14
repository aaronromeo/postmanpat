package announcer

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const webhookAnnouncePath = "/announcements"

type Option func(*ppAnnoucer)

type Service interface {
	Do(action, ruleName, mailbox string, count int) error
}

func WithWebhookURL(webhookURL string) Option {
	// opt := Option()
	return func(ppa *ppAnnoucer) {
		ppa.baseURL = strings.TrimSpace(webhookURL)
	}
}

type ppAnnoucer struct {
	baseURL string
}

func New(opts ...Option) *ppAnnoucer {
	announcer := &ppAnnoucer{}
	for _, opt := range opts {
		opt(announcer)
	}
	return announcer
}

func (p *ppAnnoucer) Do(action, ruleName, mailbox string, count int) error {
	if p.baseURL == "" {
		return nil
	}
	baseURL := strings.TrimRight(p.baseURL, "/")
	message := fmt.Sprintf("%s: Rule %q mailbox %q matched %d messages\n", action, ruleName, mailbox, count)
	payload := fmt.Sprintf("{\"message\": %q}", message)
	req, err := http.NewRequest("POST", baseURL+webhookAnnouncePath, strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("reporting webhook returned status %s", resp.Status)
	}
	return nil
}
