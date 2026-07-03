package events

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"ubuntu-auto-update/backend/pkg/session"
)

// websocket protocol-level constants. Tuned so a typical desktop browser's
// short network blips (Wi-Fi handover, sleep/wake) don't kill the channel
// while still letting a truly dead client be reaped.
const (
	pingPeriod = 30 * time.Second
	pongWait   = 60 * time.Second
	writeWait  = 10 * time.Second
)

// Handler returns an http.HandlerFunc that upgrades to a WebSocket and
// streams broker events as JSON. Used as GET /api/v1/events.
//
// One connection per session is the design — the frontend uses a singleton
// WS shared via a React context. Each connection forwards every event the
// broker emits; client-side filtering decides what each component cares
// about. Server-side filtering would mean per-user authorization checks,
// which we skip in the single-admin model.
func Handler(broker *Broker, upgrader websocket.Upgrader, store session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Errorf("events: upgrade: %v", err)
			return
		}
		defer conn.Close()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// Pong handling — the client sends pongs in response to our pings,
		// which extends the read deadline. Without this an idle client
		// would be reaped after pongWait.
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			_ = conn.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})

		// Reader goroutine: we don't expect any client→server messages, but
		// we still need to drain the socket so close frames and pongs are
		// processed. Cancelling ctx tears down the writer below.
		go func() {
			defer cancel()
			for {
				if _, _, err := conn.NextReader(); err != nil {
					return
				}
			}
		}()

		eventsCh, release := broker.Subscribe()
		defer release()

		// Send an immediate snapshot hint so the freshly-connected client
		// performs a full state refresh — simpler than tracking
		// last-seen-event-id per session.
		_ = writeJSON(conn, Event{Op: "snapshot"})

		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()

		token := r.URL.Query().Get("token")

		for {
			select {
			case <-ctx.Done():
				return

			case ev := <-eventsCh:
				if err := writeJSON(conn, ev); err != nil {
					return
				}

			case <-ticker.C:
				// Re-validate session to ensure a revoked/expired token
				// drops the long-lived WebSocket.
				if store != nil && token != "" {
					if _, valid, _ := store.Validate(r.Context(), token); !valid {
						_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Session expired"))
						return
					}
				}

				_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}
}

func writeJSON(conn *websocket.Conn, ev Event) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
	return conn.WriteMessage(websocket.TextMessage, data)
}
