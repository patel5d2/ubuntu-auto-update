import { useId, type CSSProperties } from 'react';

interface TextareaProps {
  label: string;
  value?: string;
  defaultValue?: string;
  onChange?: (e: React.ChangeEvent<HTMLTextAreaElement>) => void;
  placeholder?: string;
  rows?: number;
  helperText?: string;
  required?: boolean;
  /** Monospace + smaller text — for scripts / config bodies. */
  mono?: boolean;
}

/** Labeled multi-line input with an optional monospace mode. */
export function Textarea({
  label, value, defaultValue, onChange, placeholder,
  rows = 5, helperText, required = false, mono = false,
}: TextareaProps) {
  const id = useId();
  const style: CSSProperties = {
    display: 'block', width: '100%', marginTop: 'var(--space-2)', resize: 'vertical',
    fontFamily: mono ? 'var(--font-mono)' : 'var(--font-sans)', fontSize: mono ? 'var(--text-sm)' : 'var(--text-body)',
    color: 'var(--ink)', background: 'var(--card-bg)', border: '1px solid var(--border)',
    borderRadius: 'var(--radius-md)', padding: '0.55rem 0.75rem', outline: 'none', boxSizing: 'border-box',
  };
  return (
    <label htmlFor={id} style={{ display: 'block', marginBottom: 'var(--space-4)', fontSize: 'var(--text-body)', fontWeight: 500, color: 'var(--ink)' }}>
      {label}
      <textarea id={id} value={value} defaultValue={defaultValue} onChange={onChange} placeholder={placeholder} rows={rows} required={required} style={style} />
      {helperText && <small style={{ display: 'block', marginTop: 'var(--space-1)', color: 'var(--ink-muted)' }}>{helperText}</small>}
    </label>
  );
}
