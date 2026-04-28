import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  type ReactNode,
} from 'react';
import { createWebSocket, isAuthenticated } from '../api';

// Event mirrors backend pkg/events.Event. Component subscribers usually
// only care about table/op/id; the WS payload is JSON-encoded directly so
// the shape is the public contract.
export interface ServerEvent {
  table: string;
  op: 'INSERT' | 'UPDATE' | 'DELETE' | 'snapshot';
  id: number;
}

// Filter is applied client-side: subscribers receive only events that match
// every present field. An undefined field matches anything. The "snapshot"
// event always reaches every subscriber regardless of filter — it's the
// "go re-fetch your state" hint.
export interface EventFilter {
  table?: string;
  op?: ServerEvent['op'];
  id?: number;
}

type Listener = (event: ServerEvent) => void;

// Manager owns the singleton WebSocket. One instance per EventsProvider.
// Reconnects with capped exponential backoff so a brief network blip
// doesn't permanently break the live UI. The first connect attempt is
// deferred until at least one subscriber exists — pages that never use
// useEvent (e.g. /login) shouldn't open a socket.
class Manager {
  private ws: WebSocket | null = null;
  private listeners = new Map<symbol, { filter: EventFilter; cb: Listener }>();
  private backoffMs = 1000;
  private readonly maxBackoffMs = 30_000;
  private reconnectTimer: number | null = null;
  private closedByApp = false;

  subscribe(filter: EventFilter, cb: Listener): () => void {
    const key = Symbol('events-listener');
    this.listeners.set(key, { filter, cb });
    this.ensureOpen();
    return () => {
      this.listeners.delete(key);
      // Don't proactively close on listener count = 0. Pages flip in and out
      // (especially under StrictMode's double-invoke) and re-opening the
      // WS each time would be wasteful. The provider's unmount handler is
      // the authoritative shutdown.
    };
  }

  shutdown(): void {
    this.closedByApp = true;
    if (this.reconnectTimer != null) {
      window.clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.listeners.clear();
  }

  private ensureOpen(): void {
    if (this.ws || this.reconnectTimer != null) return;
    if (!isAuthenticated()) return; // login flow opens the WS once auth succeeds
    this.connect();
  }

  private connect(): void {
    this.closedByApp = false;
    let ws: WebSocket;
    try {
      ws = createWebSocket('/api/v1/events');
    } catch (err) {
      console.warn('events: failed to open WS', err);
      this.scheduleReconnect();
      return;
    }
    this.ws = ws;

    ws.onopen = () => {
      this.backoffMs = 1000;
    };

    ws.onmessage = (msg: MessageEvent) => {
      let event: ServerEvent;
      try {
        event = JSON.parse(String(msg.data));
      } catch {
        return; // malformed payload — server should never send these
      }
      this.dispatch(event);
    };

    ws.onerror = () => {
      // 'error' fires immediately before 'close' on most browsers; the close
      // handler does the actual reconnect bookkeeping.
    };

    ws.onclose = () => {
      this.ws = null;
      if (this.closedByApp) return;
      this.scheduleReconnect();
    };
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer != null) return;
    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null;
      // Bump backoff for next round; capped so a long outage doesn't push
      // us into multi-minute waits.
      this.backoffMs = Math.min(this.backoffMs * 2, this.maxBackoffMs);
      this.connect();
    }, this.backoffMs);
  }

  private dispatch(event: ServerEvent): void {
    for (const { filter, cb } of this.listeners.values()) {
      if (event.op === 'snapshot') {
        cb(event);
        continue;
      }
      if (filter.table && filter.table !== event.table) continue;
      if (filter.op && filter.op !== event.op) continue;
      if (filter.id != null && filter.id !== event.id) continue;
      try {
        cb(event);
      } catch (err) {
        console.error('events listener threw:', err);
      }
    }
  }
}

interface EventsContextValue {
  subscribe: Manager['subscribe'];
}

const EventsContext = createContext<EventsContextValue | null>(null);

// EventsProvider is mounted once near the app root. It owns the singleton
// Manager via a ref so its identity survives StrictMode double-invocation;
// the lifecycle effect handles connect-on-mount, shutdown-on-unmount.
export function EventsProvider({ children }: { children: ReactNode }) {
  const managerRef = useRef<Manager | null>(null);
  if (managerRef.current === null) {
    managerRef.current = new Manager();
  }

  useEffect(() => {
    return () => {
      managerRef.current?.shutdown();
      managerRef.current = null;
    };
    // Empty deps: lifecycle is bound to provider mount, not to any prop.
  }, []);

  const subscribe = useCallback<Manager['subscribe']>((filter, cb) => {
    return managerRef.current?.subscribe(filter, cb) ?? (() => {});
  }, []);

  return (
    <EventsContext.Provider value={{ subscribe }}>
      {children}
    </EventsContext.Provider>
  );
}

// useEvent registers a callback against the singleton WS. Subscribe in an
// effect so React's cleanup function (per StrictMode docs) handles the
// double-invoke correctly. Filter changes re-subscribe; cb changes do not
// (the cb ref is captured stably via a ref to avoid resubscribe churn on
// every render).
export function useEvent(filter: EventFilter, cb: Listener) {
  const ctx = useContext(EventsContext);
  const cbRef = useRef(cb);
  cbRef.current = cb;

  useEffect(() => {
    if (!ctx) return; // outside provider — useful in tests
    const stable: Listener = ev => cbRef.current(ev);
    return ctx.subscribe(filter, stable);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ctx, filter.table, filter.op, filter.id]);
}
