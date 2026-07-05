package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	log "github.com/sirupsen/logrus"
)

// DefaultTimeout is the maximum time to wait for a webhook delivery
const DefaultTimeout = 10 * time.Second

// skipSSRFCheck is a test-only hook. When true, IsSafeURL is not called.
// This is never set in production — only by webhook_test.go.
var skipSSRFCheck bool

// IsSafeURL validates that the target URL doesn't point to localhost, private networks,
// or AWS metadata endpoints. Prevents Server-Side Request Forgery (SSRF).
func IsSafeURL(target string) error {
	u, err := url.Parse(target)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q (must be http or https)", u.Scheme)
	}

	// Resolve IP to check against private ranges
	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("could not resolve hostname: %w", err)
	}

	for _, ip := range ips {
		// Reject loopback (127.0.0.0/8, ::1)
		if ip.IsLoopback() {
			return fmt.Errorf("loopback addresses are not allowed")
		}
		// Reject private networks (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
		if ip.IsPrivate() {
			return fmt.Errorf("private IP addresses are not allowed")
		}
		// Reject link-local (169.254.0.0/16) which hits AWS IMDS
		if ip.IsLinkLocalUnicast() {
			return fmt.Errorf("link-local addresses are not allowed")
		}
		// Reject unspecified (0.0.0.0)
		if ip.IsUnspecified() {
			return fmt.Errorf("unspecified addresses are not allowed")
		}
	}
	return nil
}

// SendWithContext delivers a webhook payload with context support for cancellation.
func SendWithContext(ctx context.Context, url string, payload interface{}) error {
	if !skipSSRFCheck {
		if err := IsSafeURL(url); err != nil {
			log.Warnf("Refused to send webhook to %s: %v", url, err)
			return fmt.Errorf("unsafe webhook URL: %w", err)
		}
	}

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
