import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from 'react';

export type ToastKind = 'success' | 'error' | 'info' | 'warning';

interface Toast {
  id: number;
  kind: ToastKind;
  message: string;
  /** ms before auto-dismiss; 0 disables it. */
  duration: number;
}

interface ToastContextValue {
  show: (message: string, kind?: ToastKind, durationMs?: number) => number;
  dismiss: (id: number) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

const DEFAULT_DURATION_MS = 4000;

const KIND_STYLE: Record<ToastKind, { bg: string; fg: string; border: string }> = {
  success: { bg: 'var(--pico-color-green-100)', fg: 'var(--pico-color-green-800)', border: 'var(--pico-color-green-500)' },
  error:   { bg: 'var(--pico-color-red-100)',   fg: 'var(--pico-color-red-800)',   border: 'var(--pico-color-red-500)' },
  info:    { bg: 'var(--pico-color-azure-100)', fg: 'var(--pico-color-azure-800)', border: 'var(--pico-color-azure-500)' },
  warning: { bg: 'var(--pico-color-amber-100)', fg: 'var(--pico-color-amber-800)', border: 'var(--pico-color-amber-500)' },
};

interface ToastProviderProps {
  children: ReactNode;
  /** Override per-toast default duration (ms). */
  defaultDuration?: number;
}

export function ToastProvider({ children, defaultDuration = DEFAULT_DURATION_MS }: ToastProviderProps) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(1);
  const timers = useRef(new Map<number, ReturnType<typeof setTimeout>>());

  const dismiss = useCallback((id: number) => {
    setToasts(prev => prev.filter(t => t.id !== id));
    const handle = timers.current.get(id);
    if (handle) {
      clearTimeout(handle);
      timers.current.delete(id);
    }
  }, []);

  const show = useCallback(
    (message: string, kind: ToastKind = 'info', durationMs?: number): number => {
      const id = nextId.current++;
      const duration = durationMs ?? defaultDuration;
      setToasts(prev => [...prev, { id, kind, message, duration }]);
      if (duration > 0) {
        const handle = setTimeout(() => dismiss(id), duration);
        timers.current.set(id, handle);
      }
      return id;
    },
    [defaultDuration, dismiss],
  );

  // Clear timers on unmount so jsdom tests don't leak handles.
  useEffect(
    () => () => {
      const map = timers.current;
      for (const handle of map.values()) clearTimeout(handle);
      map.clear();
    },
    [],
  );

  const ctx = useMemo<ToastContextValue>(() => ({ show, dismiss }), [show, dismiss]);

  return (
    <ToastContext.Provider value={ctx}>
      {children}
      <ToastViewport toasts={toasts} onDismiss={dismiss} />
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast must be used inside <ToastProvider>');
  return ctx;
}

interface ViewportProps {
  toasts: Toast[];
  onDismiss: (id: number) => void;
}

const VIEWPORT_STYLE: CSSProperties = {
  position: 'fixed',
  top: '1rem',
  right: '1rem',
  display: 'flex',
  flexDirection: 'column',
  gap: '0.5rem',
  zIndex: 9999,
  maxWidth: 'min(28rem, calc(100vw - 2rem))',
  pointerEvents: 'none',
};

function ToastViewport({ toasts, onDismiss }: ViewportProps) {
  return (
    <div role="region" aria-label="Notifications" style={VIEWPORT_STYLE}>
      {toasts.map(t => (
        <ToastItem key={t.id} toast={t} onDismiss={() => onDismiss(t.id)} />
      ))}
    </div>
  );
}

function ToastItem({ toast, onDismiss }: { toast: Toast; onDismiss: () => void }) {
  const palette = KIND_STYLE[toast.kind];
  const style: CSSProperties = {
    backgroundColor: palette.bg,
    color: palette.fg,
    borderLeft: `4px solid ${palette.border}`,
    borderRadius: '0.375rem',
    padding: '0.625rem 0.875rem',
    boxShadow: '0 4px 12px rgba(0,0,0,0.08)',
    display: 'flex',
    alignItems: 'flex-start',
    gap: '0.75rem',
    pointerEvents: 'auto',
  };
  const closeStyle: CSSProperties = {
    background: 'transparent',
    border: 'none',
    color: 'inherit',
    cursor: 'pointer',
    padding: 0,
    fontSize: '1.1rem',
    lineHeight: 1,
    width: 'auto',
  };

  return (
    <div role="status" aria-live={toast.kind === 'error' ? 'assertive' : 'polite'} data-kind={toast.kind} style={style}>
      <span style={{ flex: 1 }}>{toast.message}</span>
      <button type="button" aria-label="Dismiss notification" onClick={onDismiss} style={closeStyle}>
        ×
      </button>
    </div>
  );
}
