import { useId, type CSSProperties } from 'react';

export interface SelectOption {
  value: string;
  label: string;
}

interface SelectProps {
  label?: string;
  value?: string;
  onChange?: (e: React.ChangeEvent<HTMLSelectElement>) => void;
  options: SelectOption[];
  helperText?: string;
  disabled?: boolean;
  placeholder?: string;
  'aria-label'?: string;
}

/** Labeled (or bare) native select styled to match the token layer. */
export function Select({
  label, value, onChange, options, helperText, disabled = false, placeholder,
  'aria-label': ariaLabel,
}: SelectProps) {
  const id = useId();
  const style: CSSProperties = {
    display: 'block', width: label ? '100%' : 'auto', marginTop: label ? 'var(--space-2)' : 0,
    fontFamily: 'var(--font-sans)', fontSize: 'var(--text-body)', color: 'var(--ink)',
    background: 'var(--card-bg)', border: '1px solid var(--border)',
    borderRadius: 'var(--radius-md)', padding: '0.55rem 0.75rem', boxSizing: 'border-box',
  };
  const content = (
    <select id={id} value={value} onChange={onChange} disabled={disabled} aria-label={label ? undefined : ariaLabel} style={style}>
      {placeholder && <option value="">{placeholder}</option>}
      {options.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
    </select>
  );
  if (!label) return content;
  return (
    <label htmlFor={id} style={{ display: 'block', marginBottom: 'var(--space-4)', fontSize: 'var(--text-body)', fontWeight: 500, color: 'var(--ink)' }}>
      {label}
      {content}
      {helperText && <small style={{ display: 'block', marginTop: 'var(--space-1)', color: 'var(--ink-muted)' }}>{helperText}</small>}
    </label>
  );
}
