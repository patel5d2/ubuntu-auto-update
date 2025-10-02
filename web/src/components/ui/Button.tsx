import React, { ButtonHTMLAttributes, forwardRef } from 'react';
// import { useTheme } from '../design-system/ThemeProvider';

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'tertiary' | 'success' | 'warning' | 'error' | 'ghost';
  size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl';
  loading?: boolean;
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
  fullWidth?: boolean;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(({
  variant = 'primary',
  size = 'md',
  loading = false,
  leftIcon,
  rightIcon,
  fullWidth = false,
  disabled,
  children,
  className = '',
  ...props
}, ref) => {
  // const { theme } = useTheme(); // Available for future theme customization

  const baseStyles = `
    inline-flex items-center justify-center font-medium transition-all duration-200 
    focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-50 
    disabled:cursor-not-allowed border
  `;

  const sizeStyles = {
    xs: 'px-2.5 py-1.5 text-xs rounded',
    sm: 'px-3 py-2 text-sm rounded-md',
    md: 'px-4 py-2 text-sm rounded-md',
    lg: 'px-4 py-2 text-base rounded-md',
    xl: 'px-6 py-3 text-base rounded-lg',
  };

  const variantStyles = {
    primary: `
      bg-primary-600 hover:bg-primary-700 active:bg-primary-800
      text-white border-primary-600 hover:border-primary-700
      focus:ring-primary-500 shadow-sm
    `,
    secondary: `
      bg-secondary-100 hover:bg-secondary-200 active:bg-secondary-300
      text-secondary-900 border-secondary-200 hover:border-secondary-300
      focus:ring-secondary-500 shadow-sm
    `,
    tertiary: `
      bg-transparent hover:bg-secondary-100 active:bg-secondary-200
      text-secondary-700 border-secondary-300 hover:border-secondary-400
      focus:ring-secondary-500
    `,
    success: `
      bg-success-600 hover:bg-success-700 active:bg-success-800
      text-white border-success-600 hover:border-success-700
      focus:ring-success-500 shadow-sm
    `,
    warning: `
      bg-warning-600 hover:bg-warning-700 active:bg-warning-800
      text-white border-warning-600 hover:border-warning-700
      focus:ring-warning-500 shadow-sm
    `,
    error: `
      bg-error-600 hover:bg-error-700 active:bg-error-800
      text-white border-error-600 hover:border-error-700
      focus:ring-error-500 shadow-sm
    `,
    ghost: `
      bg-transparent hover:bg-secondary-100 active:bg-secondary-200
      text-secondary-600 border-transparent hover:border-secondary-200
      focus:ring-secondary-500
    `,
  };

  const widthStyle = fullWidth ? 'w-full' : '';

  const combinedClassName = `
    ${baseStyles}
    ${sizeStyles[size]}
    ${variantStyles[variant]}
    ${widthStyle}
    ${className}
  `.replace(/\s+/g, ' ').trim();

  return (
    <button
      ref={ref}
      className={combinedClassName}
      disabled={disabled || loading}
      {...props}
    >
      {loading && (
        <svg 
          className="animate-spin -ml-1 mr-2 h-4 w-4" 
          xmlns="http://www.w3.org/2000/svg" 
          fill="none" 
          viewBox="0 0 24 24"
        >
          <circle 
            className="opacity-25" 
            cx="12" 
            cy="12" 
            r="10" 
            stroke="currentColor" 
            strokeWidth="4"
          />
          <path 
            className="opacity-75" 
            fill="currentColor" 
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
          />
        </svg>
      )}
      {!loading && leftIcon && (
        <span className="mr-2">{leftIcon}</span>
      )}
      {children}
      {!loading && rightIcon && (
        <span className="ml-2">{rightIcon}</span>
      )}
    </button>
  );
});