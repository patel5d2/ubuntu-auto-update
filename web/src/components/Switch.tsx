import { useId, type ReactNode } from 'react';

interface SwitchProps {
  label: ReactNode;
  checked: boolean;
  onChange?: (e: React.ChangeEvent<HTMLInputElement>) => void;
  disabled?: boolean;
}

/** Accent-filled toggle switch (visual sugar over a checkbox). */
export function Switch({ label, checked, onChange, disabled = false }: SwitchProps) {
  const id = useId();
  return (
    <label htmlFor={id} style={{ display: 'inline-flex', alignItems: 'center', gap: '0.5rem', fontSize: 'var(--text-body)', color: 'var(--ink)', cursor: disabled ? 'not-allowed' : 'pointer', opacity: disabled ? 0.6 : 1 }}>
      <span style={{ position: 'relative', width: 34, height: 20, flexShrink: 0 }}>
        <input id={id} type="checkbox" role="switch" checked={checked} disabled={disabled} onChange={onChange} style={{ position: 'absolute', opacity: 0, width: '100%', height: '100%', margin: 0, cursor: 'inherit' }} />
        <span style={{ position: 'absolute', inset: 0, borderRadius: 'var(--radius-pill)', background: checked ? 'var(--accent)' : 'var(--border)', transition: 'background 0.15s ease' }} />
        <span style={{ position: 'absolute', top: 2, left: checked ? 16 : 2, width: 16, height: 16, borderRadius: '50%', background: '#fff', boxShadow: 'var(--shadow-sm)', transition: 'left 0.15s ease' }} />
      </span>
      {label}
    </label>
  );
}
