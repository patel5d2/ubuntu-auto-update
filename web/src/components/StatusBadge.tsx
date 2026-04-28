import type { CSSProperties } from 'react';

export type HostStatus = 'online' | 'offline' | 'updating' | 'error' | 'unknown';

interface StatusBadgeProps {
  status: HostStatus;
  /** Visible text. Defaults to a capitalized form of the status. */
  label?: string;
  /** Tooltip text shown on hover. */
  title?: string;
}

const PALETTE: Record<HostStatus, { bg: string; fg: string; defaultLabel: string }> = {
  online:   { bg: 'var(--pico-color-green-100)',  fg: 'var(--pico-color-green-700)',  defaultLabel: 'Online' },
  updating: { bg: 'var(--pico-color-azure-100)',  fg: 'var(--pico-color-azure-700)',  defaultLabel: 'Updating' },
  offline:  { bg: 'var(--pico-color-grey-150)',   fg: 'var(--pico-color-grey-700)',   defaultLabel: 'Offline' },
  error:    { bg: 'var(--pico-color-red-100)',    fg: 'var(--pico-color-red-700)',    defaultLabel: 'Error' },
  unknown:  { bg: 'var(--pico-color-grey-150)',   fg: 'var(--pico-color-grey-600)',   defaultLabel: 'Unknown' },
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
