import React from 'react';
// import { useTheme } from '../design-system/ThemeProvider';

interface CardProps {
  children: React.ReactNode;
  className?: string;
  variant?: 'default' | 'outlined' | 'elevated' | 'filled';
  padding?: 'none' | 'sm' | 'md' | 'lg' | 'xl';
  hover?: boolean;
  clickable?: boolean;
  onClick?: () => void;
}

interface CardHeaderProps {
  children: React.ReactNode;
  className?: string;
  actions?: React.ReactNode;
}

interface CardContentProps {
  children: React.ReactNode;
  className?: string;
}

interface CardFooterProps {
  children: React.ReactNode;
  className?: string;
  divided?: boolean;
}

export function Card({
  children,
  className = '',
  variant = 'default',
  padding = 'md',
  hover = false,
  clickable = false,
  onClick
}: CardProps) {
  // const { theme } = useTheme(); // Available for future theme customization

  const getVariantStyles = () => {
    switch (variant) {
      case 'outlined':
        return 'bg-background border-2 border-border-primary shadow-none';
      case 'elevated':
        return 'bg-surface border border-border-primary shadow-lg';
      case 'filled':
        return 'bg-secondary-50 border border-border-primary shadow-sm';
      default:
        return 'bg-surface border border-border-primary shadow-sm';
    }
  };

  const getPaddingStyles = () => {
    switch (padding) {
      case 'none':
        return '';
      case 'sm':
        return 'p-3';
      case 'md':
        return 'p-4';
      case 'lg':
        return 'p-6';
      case 'xl':
        return 'p-8';
      default:
        return 'p-4';
    }
  };

  const baseStyles = `
    rounded-lg
    transition-all
    duration-200
    ${getVariantStyles()}
    ${getPaddingStyles()}
    ${hover ? 'hover:shadow-md hover:-translate-y-0.5' : ''}
    ${clickable ? 'cursor-pointer hover:shadow-md' : ''}
    ${className}
  `;

  const handleClick = () => {
    if (clickable && onClick) {
      onClick();
    }
  };

  return (
    <div
      className={baseStyles}
      onClick={handleClick}
      role={clickable ? 'button' : undefined}
      tabIndex={clickable ? 0 : undefined}
      onKeyDown={clickable ? (e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          handleClick();
        }
      } : undefined}
    >
      {children}
    </div>
  );
}

export function CardHeader({ children, className = '', actions }: CardHeaderProps) {
  return (
    <div className={`flex items-center justify-between mb-4 ${className}`}>
      <div className="flex-1">
        {children}
      </div>
      {actions && (
        <div className="flex items-center space-x-2 ml-4">
          {actions}
        </div>
      )}
    </div>
  );
}

export function CardContent({ children, className = '' }: CardContentProps) {
  return (
    <div className={`${className}`}>
      {children}
    </div>
  );
}

export function CardFooter({ children, className = '', divided = true }: CardFooterProps) {
  return (
    <div className={`mt-4 ${divided ? 'pt-4 border-t border-border-primary' : ''} ${className}`}>
      {children}
    </div>
  );
}

// Specialized card variants
interface StatCardProps {
  title: string;
  value: string | number;
  subtitle?: string;
  icon?: React.ReactNode;
  trend?: {
    value: number;
    label: string;
    isPositive?: boolean;
  };
  variant?: 'default' | 'primary' | 'success' | 'warning' | 'error';
}

export function StatCard({
  title,
  value,
  subtitle,
  icon,
  trend,
  variant = 'default'
}: StatCardProps) {
  const getVariantStyles = () => {
    switch (variant) {
      case 'primary':
        return {
          card: 'bg-primary-50 border-primary-200',
          icon: 'bg-primary-100 text-primary-600',
          value: 'text-primary-700'
        };
      case 'success':
        return {
          card: 'bg-success-50 border-success-200',
          icon: 'bg-success-100 text-success-600',
          value: 'text-success-700'
        };
      case 'warning':
        return {
          card: 'bg-warning-50 border-warning-200',
          icon: 'bg-warning-100 text-warning-600',
          value: 'text-warning-700'
        };
      case 'error':
        return {
          card: 'bg-error-50 border-error-200',
          icon: 'bg-error-100 text-error-600',
          value: 'text-error-700'
        };
      default:
        return {
          card: 'bg-surface border-border-primary',
          icon: 'bg-secondary-100 text-secondary-600',
          value: 'text-text-primary'
        };
    }
  };

  const styles = getVariantStyles();

  return (
    <Card variant="default" className={styles.card}>
      <div className="flex items-center">
        <div className="flex-1">
          <p className="text-sm font-medium text-text-secondary">{title}</p>
          <div className="flex items-baseline">
            <p className={`text-2xl font-bold ${styles.value}`}>{value}</p>
            {trend && (
              <span className={`ml-2 text-xs font-medium flex items-center ${
                trend.isPositive !== false 
                  ? 'text-success-600' 
                  : 'text-error-600'
              }`}>
                <svg 
                  className={`w-3 h-3 mr-1 ${trend.isPositive !== false ? '' : 'rotate-180'}`} 
                  fill="currentColor" 
                  viewBox="0 0 20 20"
                >
                  <path fillRule="evenodd" d="M12 7a1 1 0 110-2h5a1 1 0 011 1v5a1 1 0 11-2 0V8.414l-4.293 4.293a1 1 0 01-1.414 0L8 10.414l-4.293 4.293a1 1 0 01-1.414-1.414l5-5a1 1 0 011.414 0L11 10.586 14.586 7H12z" clipRule="evenodd" />
                </svg>
                {trend.value}% {trend.label}
              </span>
            )}
          </div>
          {subtitle && (
            <p className="text-xs text-text-secondary mt-1">{subtitle}</p>
          )}
        </div>
        
        {icon && (
          <div className={`h-12 w-12 rounded-lg flex items-center justify-center ${styles.icon}`}>
            {icon}
          </div>
        )}
      </div>
    </Card>
  );
}

interface AlertCardProps {
  type: 'info' | 'success' | 'warning' | 'error';
  title: string;
  message: string;
  action?: React.ReactNode;
  onClose?: () => void;
  closeable?: boolean;
}

export function AlertCard({
  type,
  title,
  message,
  action,
  onClose,
  closeable = true
}: AlertCardProps) {
  const getTypeStyles = () => {
    switch (type) {
      case 'info':
        return {
          card: 'bg-primary-50 border-primary-200',
          icon: 'text-primary-600'
        };
      case 'success':
        return {
          card: 'bg-success-50 border-success-200',
          icon: 'text-success-600'
        };
      case 'warning':
        return {
          card: 'bg-warning-50 border-warning-200',
          icon: 'text-warning-600'
        };
      case 'error':
        return {
          card: 'bg-error-50 border-error-200',
          icon: 'text-error-600'
        };
      default:
        return {
          card: 'bg-secondary-50 border-secondary-200',
          icon: 'text-secondary-600'
        };
    }
  };

  const getIcon = () => {
    switch (type) {
      case 'success':
        return (
          <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
            <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
          </svg>
        );
      case 'warning':
      case 'error':
        return (
          <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
            <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
          </svg>
        );
      default:
        return (
          <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
            <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" />
          </svg>
        );
    }
  };

  const styles = getTypeStyles();

  return (
    <Card variant="default" className={styles.card}>
      <div className="flex">
        <div className={`flex-shrink-0 ${styles.icon}`}>
          {getIcon()}
        </div>
        <div className="ml-3 flex-1">
          <h3 className="text-sm font-medium text-text-primary">{title}</h3>
          <p className="mt-1 text-sm text-text-secondary">{message}</p>
          {action && (
            <div className="mt-3">
              {action}
            </div>
          )}
        </div>
        {closeable && onClose && (
          <div className="ml-auto pl-3">
            <button
              onClick={onClose}
              className="inline-flex text-text-secondary hover:text-text-primary focus:outline-none focus:ring-2 focus:ring-primary-500 rounded-md p-1"
            >
              <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
              </svg>
            </button>
          </div>
        )}
      </div>
    </Card>
  );
}