package webhook

import (
	"context"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Dispatcher fans out webhook deliveries asynchronously with bounded retries
// and exponential backoff. The HTTP handler returns immediately; the
// dispatcher tracks in-flight deliveries so they can be drained on shutdown.
type Dispatcher struct {
	maxAttempts int
	baseBackoff time.Duration
	wg          sync.WaitGroup
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		maxAttempts: 3,
		baseBackoff: 500 * time.Millisecond,
	}
}

// Deliver enqueues an asynchronous delivery to url with the given payload.
// Failures are retried up to maxAttempts times with exponential backoff;
// final failures are logged but not surfaced to the caller.
func (d *Dispatcher) Deliver(ctx context.Context, url string, payload interface{}) {
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		backoff := d.baseBackoff
		for attempt := 1; attempt <= d.maxAttempts; attempt++ {
			err := SendWithContext(ctx, url, payload)
			if err == nil {
				return
			}
			if attempt == d.maxAttempts {
				log.WithError(err).Errorf("webhook to %s failed after %d attempts", url, attempt)
				return
			}
			log.WithError(err).Warnf("webhook to %s attempt %d/%d failed, retrying in %s", url, attempt, d.maxAttempts, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			backoff *= 2
		}
	}()
}

// Wait blocks until all in-flight deliveries finish (or fail terminally).
// Use during graceful shutdown.
func (d *Dispatcher) Wait() {
	d.wg.Wait()
}
