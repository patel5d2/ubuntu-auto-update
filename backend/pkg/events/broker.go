// Package events fans Postgres LISTEN/NOTIFY notifications out to many
// in-process subscribers (typically WebSocket connections). One Listener
// goroutine owns the LISTEN side; Broker handles the fan-out and is safe
// for concurrent use.
//
// The schema is intentionally tiny: the publisher trigger pushes only
// {table, op, id}, the subscriber re-fetches via REST. That keeps the
// channel small (under the 8 KB pg_notify limit by a wide margin) and
// avoids races around in-flight transactions.
package events

import (
	"sync"
	"sync/atomic"
)

// Event is the payload pushed onto every subscriber channel. JSON-encoded
// directly when forwarded over WebSockets, so field tags are part of the
// public contract — don't rename without updating useEvents.ts.
type Event struct {
	Table string `json:"table"`
	Op    string `json:"op"` // INSERT, UPDATE, DELETE, or "snapshot" on reconnect
	ID    int64  `json:"id"`
}

// SubscriberBufferSize is the per-subscriber channel depth. Bursts of agent
// reports during a fleet-wide update can produce hundreds of events in a
// short window; 64 is comfortably more than the eye can perceive but still
// bounded so a slow client doesn't pin memory. If a subscriber lags past
// the buffer we drop events for that subscriber only — it'll resync on the
// next REST refresh.
const SubscriberBufferSize = 64

// Broker is a fan-out pub/sub. New subscribers see only events that arrive
// after they call Subscribe; there is no replay buffer. That's intentional:
// LISTEN is best-effort by design (notifications can be coalesced and
// dropped on disconnect), so we never pretend to give exactly-once
// semantics. Clients always re-fetch authoritative state from REST.

type Broker struct {
	mu          sync.RWMutex
	subscribers map[chan Event]struct{}
	// dropped is bumped under RLock by Publish, so it must be atomic.
	dropped atomic.Uint64
}

func NewBroker() *Broker {
	return &Broker{subscribers: make(map[chan Event]struct{})}
}

// Subscribe registers a new subscriber and returns the read side of its
// channel plus a release function. The caller must call release exactly
// once — typically on WebSocket close. The returned channel is never
// closed by the broker; if it were, slow producers would race with
// release and we'd panic on send.
func (b *Broker) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, SubscriberBufferSize)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	release := func() {
		b.mu.Lock()
		delete(b.subscribers, ch)
		b.mu.Unlock()
	}
	return ch, release
}

// Publish broadcasts an event to all current subscribers without blocking.
// A subscriber whose buffer is full simply misses the event; the broker
// keeps a rolling counter (visible via DroppedCount) for diagnostics. We
// never block on a slow client — the listener loop must keep draining the
// pgx connection or new notifications start queueing in Postgres.
func (b *Broker) Publish(ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers {
		select {
		case ch <- ev:
		default:
			b.dropped.Add(1)
		}
	}
}

// SubscriberCount is mainly for /api/v1/health-style debugging so the
// operator can confirm the WebSocket plumbing is working.
func (b *Broker) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}

// DroppedCount returns the running total of events that were skipped
// because at least one subscriber was full. Reset by restart only.
func (b *Broker) DroppedCount() uint64 {
	return b.dropped.Load()
}
