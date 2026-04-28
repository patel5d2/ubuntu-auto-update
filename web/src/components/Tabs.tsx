import type { CSSProperties, ReactNode } from 'react';

export interface TabDef {
  id: string;
  label: ReactNode;
  badge?: ReactNode;
}

interface TabsProps {
  tabs: TabDef[];
  active: string;
  onChange: (id: string) => void;
}

const navStyle: CSSProperties = {
  display: 'flex',
  gap: '0.25rem',
  borderBottom: '1px solid var(--pico-muted-border-color)',
  marginBottom: '1rem',
};

const buttonBaseStyle: CSSProperties = {
  background: 'transparent',
  border: 'none',
  borderBottom: '2px solid transparent',
  borderRadius: 0,
  margin: 0,
  padding: '0.5rem 0.75rem',
  cursor: 'pointer',
  fontWeight: 500,
  width: 'auto',
  color: 'var(--pico-color)',
};

/**
 * Minimal accessible tab bar. Renders a `role="tablist"` with `role="tab"`
 * buttons; the parent is responsible for rendering the active panel and
 * giving it `role="tabpanel"`.
 */
export function Tabs({ tabs, active, onChange }: TabsProps) {
  return (
    <nav role="tablist" aria-label="Host sections" style={navStyle}>
      {tabs.map(tab => {
        const isActive = tab.id === active;
        const style: CSSProperties = {
          ...buttonBaseStyle,
          borderBottomColor: isActive ? 'var(--pico-primary)' : 'transparent',
          opacity: isActive ? 1 : 0.7,
        };
        return (
          <button
            key={tab.id}
            role="tab"
            type="button"
            aria-selected={isActive}
            aria-controls={`panel-${tab.id}`}
            id={`tab-${tab.id}`}
            tabIndex={isActive ? 0 : -1}
            onClick={() => onChange(tab.id)}
            style={style}
          >
            {tab.label}
            {tab.badge !== undefined && (
              <span style={{ marginLeft: '0.4rem', opacity: 0.7 }}>{tab.badge}</span>
            )}
          </button>
        );
      })}
    </nav>
  );
}
