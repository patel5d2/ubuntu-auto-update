import { useId, type ReactNode } from 'react';

interface CheckboxProps {
  label: ReactNode;
  checked: boolean;
  onChange?: (e: React.ChangeEvent<HTMLInputElement>) => void;
  disabled?: boolean;
}

/** Labeled checkbox with the accent tint. */
export function Checkbox({ label, checked, onChange, disabled = false }: CheckboxProps) {
  const id = useId();
  return (
    <label htmlFor={id} style={{ display: 'inline-flex', alignItems: 'center', gap: '0.4rem', fontSize: 'var(--text-body)', color: 'var(--ink)', cursor: disabled ? 'not-allowed' : 'pointer', opacity: disabled ? 0.6 : 1 }}>
      <input id={id} type="checkbox" checked={checked} disabled={disabled} onChange={onChange} style={{ width: 15, height: 15, accentColor: 'var(--accent)' }} />
      {label}
    </label>
  );
}
