package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

// DefaultTimeout is the maximum time to wait for a webhook delivery
const DefaultTimeout = 10 * time.Second

// Send delivers a webhook payload to the specified URL with proper timeout and context.
func Send(url string, payload interface{}) error {
	return SendWithContext(context.Background(), url, payload)
}

// SendWithContext delivers a webhook payload with context support for cancellation.
func SendWithContext(ctx context.Context, url string, payload interface{}) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("Failed to marshal webhook payload: %v", err)
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Errorf("Failed to create webhook request: %v", err)
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ubuntu-auto-update/1.0")

	client := &http.Client{
		Timeout: DefaultTimeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("Failed to send webhook to %s: %v", url, err)
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Errorf("Webhook to %s returned non-success status code: %d", url, resp.StatusCode)
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	log.Debugf("Webhook delivered to %s successfully", url)
	return nil
}