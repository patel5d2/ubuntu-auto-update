import React, { forwardRef } from 'react';
// import { useTheme } from '../design-system/ThemeProvider';

interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
  helperText?: string;
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
  variant?: 'default' | 'filled' | 'minimal';
  inputSize?: 'xs' | 'sm' | 'md' | 'lg' | 'xl';
  fullWidth?: boolean;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(({
  label,
  error,
  helperText,
  leftIcon,
  rightIcon,
  variant = 'default',
  inputSize = 'md',
  fullWidth = false,
  className = '',
  disabled = false,
  required = false,
  ...props
}, ref) => {
  // const { theme } = useTheme(); // Available for future theme customization

  const getVariantStyles = () => {
    switch (variant) {
      case 'filled':
        return `
          bg-secondary-100 border-secondary-100 
          focus:bg-background focus:border-primary-500 focus:ring-primary-500
          hover:bg-secondary-50
        `;
      case 'minimal':
        return `
          bg-transparent border-0 border-b-2 border-secondary-300 rounded-none
          focus:border-primary-500 focus:ring-0
          hover:border-secondary-400
        `;
      default:
        return `
          bg-background border-border-primary
          focus:border-primary-500 focus:ring-primary-500
          hover:border-secondary-400
        `;
    }
  };

  const getSizeStyles = () => {
    switch (inputSize) {
      case 'xs':
        return 'px-2 py-1 text-xs';
      case 'sm':
        return 'px-3 py-1.5 text-sm';
      case 'md':
        return 'px-3 py-2 text-sm';
      case 'lg':
        return 'px-4 py-2.5 text-base';
      case 'xl':
        return 'px-4 py-3 text-lg';
      default:
        return 'px-3 py-2 text-sm';
    }
  };

  const getIconPadding = () => {
    const hasLeftIcon = !!leftIcon;
    const hasRightIcon = !!rightIcon;
    
    switch (inputSize) {
      case 'xs':
        return `${hasLeftIcon ? 'pl-7' : ''} ${hasRightIcon ? 'pr-7' : ''}`;
      case 'sm':
        return `${hasLeftIcon ? 'pl-8' : ''} ${hasRightIcon ? 'pr-8' : ''}`;
      case 'md':
        return `${hasLeftIcon ? 'pl-10' : ''} ${hasRightIcon ? 'pr-10' : ''}`;
      case 'lg':
        return `${hasLeftIcon ? 'pl-11' : ''} ${hasRightIcon ? 'pr-11' : ''}`;
      case 'xl':
        return `${hasLeftIcon ? 'pl-12' : ''} ${hasRightIcon ? 'pr-12' : ''}`;
      default:
        return `${hasLeftIcon ? 'pl-10' : ''} ${hasRightIcon ? 'pr-10' : ''}`;
    }
  };

  const getIconSize = () => {
    switch (inputSize) {
      case 'xs':
        return 'w-3 h-3';
      case 'sm':
        return 'w-4 h-4';
      case 'md':
        return 'w-4 h-4';
      case 'lg':
        return 'w-5 h-5';
      case 'xl':
        return 'w-6 h-6';
      default:
        return 'w-4 h-4';
    }
  };

  const getIconPosition = () => {
    switch (inputSize) {
      case 'xs':
        return 'top-1 left-2';
      case 'sm':
        return 'top-1.5 left-2.5';
      case 'md':
        return 'top-2.5 left-3';
      case 'lg':
        return 'top-3 left-3.5';
      case 'xl':
        return 'top-3.5 left-4';
      default:
        return 'top-2.5 left-3';
    }
  };

  const getRightIconPosition = () => {
    switch (inputSize) {
      case 'xs':
        return 'top-1 right-2';
      case 'sm':
        return 'top-1.5 right-2.5';
      case 'md':
        return 'top-2.5 right-3';
      case 'lg':
        return 'top-3 right-3.5';
      case 'xl':
        return 'top-3.5 right-4';
      default:
        return 'top-2.5 right-3';
    }
  };

  const baseStyles = `
    w-full
    border
    rounded-md
    transition-all
    duration-200
    text-text-primary
    placeholder:text-text-secondary
    disabled:opacity-50
    disabled:cursor-not-allowed
    focus:outline-none
    focus:ring-2
    focus:ring-opacity-50
    ${variant !== 'minimal' ? 'focus:ring-offset-1' : ''}
  `;

  const inputStyles = `
    ${baseStyles}
    ${getVariantStyles()}
    ${getSizeStyles()}
    ${getIconPadding()}
    ${error ? 'border-error-500 focus:border-error-500 focus:ring-error-500' : ''}
    ${disabled ? 'bg-secondary-50' : ''}
    ${fullWidth ? 'w-full' : ''}
    ${className}
  `;

  return (
    <div className={`${fullWidth ? 'w-full' : ''} space-y-1`}>
      {label && (
        <label className="block text-sm font-medium text-text-primary">
          {label}
          {required && <span className="text-error-500 ml-1">*</span>}
        </label>
      )}
      
      <div className="relative">
        {leftIcon && (
          <div className={`absolute ${getIconPosition()} pointer-events-none`}>
            <div className={`${getIconSize()} text-text-secondary`}>
              {leftIcon}
            </div>
          </div>
        )}
        
        <input
          ref={ref}
          disabled={disabled}
          required={required}
          className={inputStyles}
          {...props}
        />
        
        {rightIcon && (
          <div className={`absolute ${getRightIconPosition()} pointer-events-none`}>
            <div className={`${getIconSize()} text-text-secondary`}>
              {rightIcon}
            </div>
          </div>
        )}
      </div>
      
      {(error || helperText) && (
        <div className="text-xs">
          {error ? (
            <span className="text-error-600 flex items-center">
              <svg className="w-3 h-3 mr-1" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
              </svg>
              {error}
            </span>
          ) : (
            <span className="text-text-secondary">{helperText}</span>
          )}
        </div>
      )}
    </div>
  );
});