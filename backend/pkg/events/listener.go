package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"
)

// Channel is the Postgres NOTIFY channel published by the trigger in
// migration 000012. Kept as a constant so it's grep-able from both sides.
const Channel = "uau_events"

// Listener owns a long-running goroutine that consumes pg_notify events
// and forwards them to the broker. It hijacks a single connection from
// the pool — pgx's WaitForNotification needs a dedicated, idle connection,
// since the LISTEN binding lives on the connection itself.
type Listener struct {
	Pool   *pgxpool.Pool
	Broker *Broker
}

func NewListener(pool *pgxpool.Pool, broker *Broker) *Listener {
	return &Listener{Pool: pool, Broker: broker}
}

// Run blocks until ctx is cancelled. On any error it logs, sleeps with
// capped exponential backoff, and reconnects. There is no max-attempts
// limit: a healthy deployment with a flaky network is exactly the case
// where we want to keep trying.
//
// On reconnect we publish a synthetic {table:"", op:"snapshot"} event so
// every subscriber knows to re-fetch authoritative state — see the plan's
// "pgx LISTEN connection drops silently" mitigation.
func (l *Listener) Run(ctx context.Context) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		err := l.runOnce(ctx)
		if ctx.Err() != nil {
			log.Info("events listener: shutting down")
			return
		}
		if err == nil {
			// runOnce only returns nil on ctx cancellation; treat any other
			// nil-return as a benign reset and continue with fresh backoff.
			backoff = time.Second
			continue
		}

		log.Warnf("events listener: %v — reconnecting in %s", err, backoff)
		// Tell every WS to refetch on the next iteration; the listener was
		// down so anything that happened in between was lost.
		l.Broker.Publish(Event{Op: "snapshot"})

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// runOnce holds a single connection and pumps notifications until either
// the context is cancelled or the connection errors out. Designed so the
// outer loop only has to handle the reconnect cadence.
func (l *Listener) runOnce(ctx context.Context) error {
	// Acquire dedicated connection. We don't release it back to the pool
	// until the function returns, because LISTEN bindings are connection-
	// scoped. The pool's max_conns must be at least 2 (one for the listener,
	// one for everything else) — that's already the default.
	conn, err := l.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire listener conn: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+Channel); err != nil {
		return fmt.Errorf("LISTEN %s: %w", Channel, err)
	}
	log.Infof("events listener: subscribed to '%s'", Channel)

	for {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return fmt.Errorf("wait for notification: %w", err)
		}

		ev, err := decodePayload(notification.Payload)
		if err != nil {
			// A malformed payload is the trigger's bug, not a connection
			// problem — log, skip, keep listening.
			log.Warnf("events listener: bad payload %q: %v", notification.Payload, err)
			continue
		}
		l.Broker.Publish(ev)
	}
}

// decodePayload parses one pg_notify payload. The trigger emits a JSON
// object with table, op, id; we tolerate stringified ids in case future
// triggers send larger keys.
func decodePayload(payload string) (Event, error) {
	if payload == "" {
		return Event{}, fmt.Errorf("empty payload")
	}

	var raw struct {
		Table string          `json:"table"`
		Op    string          `json:"op"`
		ID    json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return Event{}, fmt.Errorf("decode json: %w", err)
	}

	ev := Event{Table: raw.Table, Op: raw.Op}
	if len(raw.ID) > 0 && string(raw.ID) != "null" {
		// ID may be a JSON number or a quoted string; try both.
		if err := json.Unmarshal(raw.ID, &ev.ID); err != nil {
			var s string
			if err2 := json.Unmarshal(raw.ID, &s); err2 != nil {
				return Event{}, fmt.Errorf("id not int or string: %w", err)
			}
			parsed, err3 := strconv.ParseInt(s, 10, 64)
			if err3 != nil {
				return Event{}, fmt.Errorf("id string not numeric: %w", err3)
			}
			ev.ID = parsed
		}
	}
	return ev, nil
}

