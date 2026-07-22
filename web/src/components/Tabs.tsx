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
  borderBottom: '1px solid var(--border)',
  marginBottom: '1rem',
};

// Theme tokens (--border/--accent/--ink) instead of --pico-* names: those were
// Pico v2 tokens that don't exist in this v1 project, so the tab colors never
// resolved. See the same fix in StatusBadge.
const buttonBaseStyle: CSSProperties = {
  background: 'transparent',
  border: 'none',
  borderBottom: '2px solid transparent',
  borderRadius: 0,
  margin: 0,
  padding: '0.5rem 0.75rem',
  cursor: 'pointer',
  fontFamily: 'var(--font-sans)',
  fontWeight: 500,
  fontSize: 'var(--text-body)',
  width: 'auto',
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
          borderBottomColor: isActive ? 'var(--accent)' : 'transparent',
          color: isActive ? 'var(--ink)' : 'var(--ink-muted)',
          opacity: isActive ? 1 : 0.85,
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
