import type { CSSProperties, ReactNode } from 'react';

interface BadgeProps {
  /** `neutral` = accent chip (tag-chip); `alert` = red semantic pill. */
  variant?: 'neutral' | 'alert';
  active?: boolean;
  onClick?: () => void;
  children: ReactNode;
  title?: string;
}

/** Formalizes the app's `.tag-chip` / `.badge-alert` pills. */
export function Badge({ variant = 'neutral', active = false, onClick, children, title }: BadgeProps) {
  if (variant === 'alert') {
    const alertStyle: CSSProperties = {
      display: 'inline-block', padding: '0.05rem 0.5rem', borderRadius: 'var(--radius-pill)',
      fontSize: 'var(--text-xs)', fontWeight: 600, whiteSpace: 'nowrap',
      background: 'color-mix(in srgb, var(--bad) 16%, transparent)', color: 'var(--bad)',
    };
    return <span style={alertStyle} title={title}>{children}</span>;
  }

  const clickable = typeof onClick === 'function';
  const style: CSSProperties = {
    display: 'inline-block', padding: '0.1rem 0.55rem', borderRadius: 'var(--radius-pill)',
    fontSize: 'var(--text-xs)', fontWeight: 500, whiteSpace: 'nowrap',
    cursor: clickable ? 'pointer' : 'default',
    background: active ? 'var(--accent)' : 'var(--accent-soft)',
    color: active ? 'var(--primary-inverse)' : 'var(--accent-strong)',
    transition: 'opacity 0.12s ease',
  };
  return <span onClick={onClick} style={style} title={title}>{children}</span>;
}
