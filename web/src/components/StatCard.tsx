import type { ReactNode } from 'react';

interface StatCardProps {
  label: string;
  value: ReactNode;
  /** Colors the value: green for good, red for bad. Neutral (ink) otherwise. */
  tone?: 'good' | 'bad';
}

/**
 * Overview stat tile. Renders with the existing `.stat-card` CSS (shell.css)
 * so the hover-lift and theming come for free — extracted from Overview's
 * inline copy so every dashboard reuses one component.
 */
export function StatCard({ label, value, tone }: StatCardProps) {
  return (
    <div className={`stat-card${tone ? ` stat-${tone}` : ''}`}>
      <div className="stat-value">{value}</div>
      <div className="stat-label">{label}</div>
    </div>
  );
}
