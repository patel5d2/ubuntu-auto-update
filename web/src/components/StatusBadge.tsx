import type { CSSProperties } from 'react';

export type HostStatus = 'online' | 'offline' | 'updating' | 'error' | 'unknown';

interface StatusBadgeProps {
  status: HostStatus;
  /** Visible text. Defaults to a capitalized form of the status. */
  label?: string;
  /** Tooltip text shown on hover. */
  title?: string;
}

// Colors come from the theme token layer (shell.css) via color-mix so the
// pills render in both light and dark. The previous --pico-color-* names were
// Pico v2 tokens that don't exist in this v1 project, so pills had no fill.
const mix = (token: string, pct: number) => `color-mix(in srgb, var(${token}) ${pct}%, transparent)`;

const PALETTE: Record<HostStatus, { bg: string; fg: string; defaultLabel: string }> = {
  online:   { bg: mix('--good', 15),       fg: 'var(--good)',       defaultLabel: 'Online' },
  updating: { bg: mix('--accent', 15),     fg: 'var(--accent-strong)', defaultLabel: 'Updating' },
  offline:  { bg: mix('--ink-muted', 15),  fg: 'var(--ink-muted)',  defaultLabel: 'Offline' },
  error:    { bg: mix('--bad', 15),        fg: 'var(--bad)',        defaultLabel: 'Error' },
  unknown:  { bg: mix('--ink-muted', 15),  fg: 'var(--ink-muted)',  defaultLabel: 'Unknown' },
};

/**
 * Inline status pill. Pure presentational — no state, no fetch.
 */
export function StatusBadge({ status, label, title }: StatusBadgeProps) {
  const palette = PALETTE[status];
  const style: CSSProperties = {
    display: 'inline-block',
    padding: '0.15rem 0.55rem',
    borderRadius: '999px',
    fontSize: '0.75rem',
    fontWeight: 600,
    lineHeight: 1.4,
    backgroundColor: palette.bg,
    color: palette.fg,
    whiteSpace: 'nowrap',
  };

  return (
    <span
      role="status"
      data-status={status}
      title={title}
      style={style}
    >
      {label ?? palette.defaultLabel}
    </span>
  );
}
