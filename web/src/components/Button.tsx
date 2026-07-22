import { useEffect, useState, type CSSProperties, type ReactNode } from 'react';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';
export type ButtonSize = 'sm' | 'md';

interface ButtonProps {
  children: ReactNode;
  variant?: ButtonVariant;
  size?: ButtonSize;
  /** Shows a spinner and disables the button while an action is in flight. */
  busy?: boolean;
  disabled?: boolean;
  fullWidth?: boolean;
  onClick?: (e: React.MouseEvent<HTMLButtonElement>) => void;
  type?: 'button' | 'submit' | 'reset';
  title?: string;
  'aria-label'?: string;
}

// One shared keyframe for the busy spinner — injected once, not per button.
function injectSpinnerKeyframes() {
  if (typeof document === 'undefined' || document.getElementById('dsui-btn-kf')) return;
  const s = document.createElement('style');
  s.id = 'dsui-btn-kf';
  s.textContent = '@keyframes dsui-spin{to{transform:rotate(360deg)}}';
  document.head.appendChild(s);
}

const VARIANTS: Record<ButtonVariant, CSSProperties> = {
  primary:   { background: 'var(--accent)', color: 'var(--primary-inverse)', border: '1px solid transparent' },
  secondary: { background: 'transparent', color: 'var(--ink)', border: '1px solid var(--border)' },
  ghost:     { background: 'transparent', color: 'var(--ink-muted)', border: '1px solid transparent' },
  danger:    { background: 'var(--bad)', color: '#fff', border: '1px solid transparent' },
};

const HOVER_BG: Record<ButtonVariant, string> = {
  primary: 'var(--accent-strong)',
  secondary: 'var(--hover-bg)',
  ghost: 'var(--hover-bg)',
  danger: 'color-mix(in srgb, var(--bad) 85%, black)',
};

/**
 * Formalizes the app's Pico `<button>` usage. `danger` (red) makes destructive
 * intent explicit, replacing Pico `.contrast` for Delete/Log-out. `busy` swaps
 * the old `aria-busy` text juggling for a spinner.
 */
export function Button({
  children,
  variant = 'primary',
  size = 'md',
  busy = false,
  disabled = false,
  fullWidth = false,
  onClick,
  type = 'button',
  title,
  'aria-label': ariaLabel,
}: ButtonProps) {
  useEffect(() => { injectSpinnerKeyframes(); }, []);
  const [hover, setHover] = useState(false);
  const palette = VARIANTS[variant];
  const isDisabled = disabled || busy;

  const style: CSSProperties = {
    display: 'inline-flex', alignItems: 'center', justifyContent: 'center', gap: '0.45rem',
    width: fullWidth ? '100%' : 'auto',
    fontFamily: 'var(--font-sans)', fontWeight: 600,
    fontSize: size === 'sm' ? 'var(--text-sm)' : 'var(--text-body)',
    padding: size === 'sm' ? '0.3rem 0.75rem' : '0.6rem 1.1rem',
    borderRadius: 'var(--radius-md)',
    cursor: isDisabled ? 'not-allowed' : 'pointer',
    opacity: isDisabled ? 0.55 : 1,
    boxShadow: variant === 'primary' && !isDisabled ? 'var(--shadow-sm)' : 'none',
    transition: 'background 0.12s ease, color 0.12s ease, border-color 0.12s ease',
    ...palette,
    background: hover && !isDisabled ? HOVER_BG[variant] : palette.background,
  };

  return (
    <button
      type={type}
      onClick={isDisabled ? undefined : onClick}
      disabled={isDisabled}
      title={title}
      aria-label={ariaLabel}
      aria-busy={busy || undefined}
      style={style}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
    >
      {busy && (
        <span
          aria-hidden="true"
          style={{
            width: 13, height: 13, border: '2px solid currentColor', borderRightColor: 'transparent',
            borderRadius: '50%', display: 'inline-block', animation: 'dsui-spin .6s linear infinite',
          }}
        />
      )}
      {children}
    </button>
  );
}
