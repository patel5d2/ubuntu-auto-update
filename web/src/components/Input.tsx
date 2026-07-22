import { useId, type CSSProperties } from 'react';

interface InputProps {
  label: string;
  type?: string;
  value?: string;
  defaultValue?: string;
  onChange?: (e: React.ChangeEvent<HTMLInputElement>) => void;
  placeholder?: string;
  error?: string;
  helperText?: string;
  required?: boolean;
  disabled?: boolean;
  name?: string;
  autoComplete?: string;
}

/** Labeled text input with inline error / helper text. */
export function Input({
  label, type = 'text', value, defaultValue, onChange, placeholder,
  error, helperText, required = false, disabled = false, name, autoComplete,
}: InputProps) {
  const id = useId();
  const inputStyle: CSSProperties = {
    display: 'block', width: '100%', marginTop: 'var(--space-2)',
    fontFamily: 'var(--font-sans)', fontSize: 'var(--text-body)', color: 'var(--ink)',
    background: disabled ? 'var(--hover-bg)' : 'var(--card-bg)',
    border: `1px solid ${error ? 'var(--bad)' : 'var(--border)'}`,
    borderRadius: 'var(--radius-md)', padding: '0.55rem 0.75rem',
    outline: 'none', boxSizing: 'border-box',
  };
  return (
    <label htmlFor={id} style={{ display: 'block', marginBottom: 'var(--space-4)', fontSize: 'var(--text-body)', fontWeight: 500, color: 'var(--ink)' }}>
      {label}
      <input
        id={id} name={name} type={type} value={value} defaultValue={defaultValue}
        onChange={onChange} placeholder={placeholder} required={required} disabled={disabled}
        autoComplete={autoComplete} aria-invalid={error ? true : undefined} style={inputStyle}
      />
      {error ? (
        <small style={{ display: 'block', marginTop: 'var(--space-1)', color: 'var(--bad)' }}>{error}</small>
      ) : helperText ? (
        <small style={{ display: 'block', marginTop: 'var(--space-1)', color: 'var(--ink-muted)' }}>{helperText}</small>
      ) : null}
    </label>
  );
}
