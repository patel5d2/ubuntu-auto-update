import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';

export interface ConfirmOptions {
  title: string;
  /** Body text. Falls back to a plain warning if omitted. */
  message?: ReactNode;
  /** Defaults to "Confirm". */
  confirmLabel?: string;
  /** Defaults to "Cancel". */
  cancelLabel?: string;
  /** Renders the confirm button as a destructive (red) action. */
  destructive?: boolean;
  /**
   * If set, the user must type this exact string into a verification field
   * before the confirm button is enabled. Used for things like host deletion
   * where typing the hostname proves intent.
   */
  requireTypedConfirmation?: string;
}

interface ConfirmContextValue {
  confirm: (options: ConfirmOptions) => Promise<boolean>;
}

const ConfirmContext = createContext<ConfirmContextValue | null>(null);

interface PendingState {
  options: ConfirmOptions;
  resolve: (ok: boolean) => void;
}

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [pending, setPending] = useState<PendingState | null>(null);

  const confirm = useCallback((options: ConfirmOptions): Promise<boolean> => {
    return new Promise<boolean>(resolve => {
      setPending({ options, resolve });
    });
  }, []);

  const handleClose = useCallback(
    (ok: boolean) => {
      if (!pending) return;
      pending.resolve(ok);
      setPending(null);
    },
    [pending],
  );

  const ctx = useMemo<ConfirmContextValue>(() => ({ confirm }), [confirm]);

  return (
    <ConfirmContext.Provider value={ctx}>
      {children}
      {pending && (
        <ConfirmDialog
          options={pending.options}
          onConfirm={() => handleClose(true)}
          onCancel={() => handleClose(false)}
        />
      )}
    </ConfirmContext.Provider>
  );
}

export function useConfirm(): (options: ConfirmOptions) => Promise<boolean> {
  const ctx = useContext(ConfirmContext);
  if (!ctx) throw new Error('useConfirm must be used inside <ConfirmProvider>');
  return ctx.confirm;
}

interface DialogProps {
  options: ConfirmOptions;
  onConfirm: () => void;
  onCancel: () => void;
}

function ConfirmDialog({ options, onConfirm, onCancel }: DialogProps) {
  const {
    title,
    message,
    confirmLabel = 'Confirm',
    cancelLabel = 'Cancel',
    destructive = false,
    requireTypedConfirmation,
  } = options;

  const [typed, setTyped] = useState('');
  const cancelRef = useRef<HTMLButtonElement>(null);

  // Focus the safest action by default (cancel) — destructive flows shouldn't
  // be one keystroke away.
  useEffect(() => {
    cancelRef.current?.focus();
  }, []);

  // Esc cancels.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onCancel();
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onCancel]);

  const typedMatches =
    !requireTypedConfirmation || typed === requireTypedConfirmation;
  const confirmDisabled = !typedMatches;

  return (
    <dialog open aria-labelledby="confirm-title">
      <article>
        <header>
          <button
            type="button"
            aria-label="Close"
            rel="prev"
            onClick={onCancel}
          />
          <strong id="confirm-title">{title}</strong>
        </header>

        {message && <p>{message}</p>}

        {requireTypedConfirmation && (
          <label>
            Type <code>{requireTypedConfirmation}</code> to confirm:
            <input
              type="text"
              value={typed}
              onChange={e => setTyped(e.target.value)}
              aria-invalid={typed.length > 0 && !typedMatches}
              autoComplete="off"
              autoFocus
            />
          </label>
        )}

        <footer>
          <button
            type="button"
            className="secondary"
            ref={cancelRef}
            onClick={onCancel}
          >
            {cancelLabel}
          </button>
          <button
            type="button"
            className={destructive ? 'contrast' : ''}
            onClick={onConfirm}
            disabled={confirmDisabled}
            data-destructive={destructive ? 'true' : undefined}
          >
            {confirmLabel}
          </button>
        </footer>
      </article>
    </dialog>
  );
}
