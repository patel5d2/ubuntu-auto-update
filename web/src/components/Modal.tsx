import { useEffect, type CSSProperties, type ReactNode } from 'react';

export interface ModalAction {
  label: string;
  onClick: () => void;
  variant?: 'primary' | 'secondary' | 'danger';
  disabled?: boolean;
}

interface ModalProps {
  open: boolean;
  title: string;
  children: ReactNode;
  onClose: () => void;
  actions?: ModalAction[];
}

/**
 * Generalized overlay dialog — the token-styled counterpart to the app's
 * native `<dialog>` patterns (ConfirmDialog, AddHostModal, RolloutModal).
 * Esc and backdrop-click close.
 */
export function Modal({ open, title, children, onClose, actions = [] }: ModalProps) {
  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) { if (e.key === 'Escape') onClose(); }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;

  const actionStyle = (variant: ModalAction['variant']): CSSProperties => ({
    fontFamily: 'var(--font-sans)', fontWeight: 600, fontSize: 'var(--text-body)',
    padding: '0.55rem 1.05rem', borderRadius: 'var(--radius-md)', cursor: 'pointer',
    background: variant === 'danger' ? 'var(--bad)' : variant === 'secondary' ? 'transparent' : 'var(--accent)',
    color: variant === 'secondary' ? 'var(--ink)' : '#fff',
    border: variant === 'secondary' ? '1px solid var(--border)' : '1px solid transparent',
  });

  return (
    <div
      style={{ position: 'fixed', inset: 0, background: 'rgba(15,18,24,0.45)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000, padding: '1rem' }}
      onClick={onClose}
    >
      <div
        role="dialog" aria-modal="true" aria-label={title}
        onClick={e => e.stopPropagation()}
        style={{ width: '100%', maxWidth: '32rem', background: 'var(--card-bg)', color: 'var(--ink)', borderRadius: 'var(--radius-lg)', boxShadow: 'var(--shadow-md)', border: '1px solid var(--border)' }}
      >
        <header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '1.1rem 1.25rem', borderBottom: '1px solid var(--border)' }}>
          <strong style={{ fontSize: 'var(--text-md)' }}>{title}</strong>
          <button onClick={onClose} aria-label="Close" style={{ background: 'transparent', border: 'none', color: 'var(--ink-muted)', fontSize: '1.2rem', cursor: 'pointer', lineHeight: 1 }}>×</button>
        </header>
        <div style={{ padding: '1.25rem' }}>{children}</div>
        {actions.length > 0 && (
          <footer style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem', padding: '1rem 1.25rem', borderTop: '1px solid var(--border)' }}>
            {actions.map((a, i) => (
              <button key={i} onClick={a.onClick} disabled={a.disabled} style={{ ...actionStyle(a.variant), cursor: a.disabled ? 'not-allowed' : 'pointer', opacity: a.disabled ? 0.55 : 1 }}>
                {a.label}
              </button>
            ))}
          </footer>
        )}
      </div>
    </div>
  );
}
