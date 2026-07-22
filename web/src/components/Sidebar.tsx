import type { CSSProperties } from 'react';

export interface SidebarItem {
  label: string;
  href?: string;
  active?: boolean;
  onClick?: (e: React.MouseEvent) => void;
}

interface SidebarProps {
  items: SidebarItem[];
  role?: string;
  theme?: 'light' | 'dark';
  onToggleTheme?: () => void;
  onLogout?: (e: React.MouseEvent) => void;
}

const asideStyle: CSSProperties = {
  width: '15rem', flexShrink: 0, display: 'flex', flexDirection: 'column',
  padding: '1.25rem 0.85rem', borderRight: '1px solid var(--border)', background: 'var(--card-bg)',
  minHeight: '100%', boxSizing: 'border-box', fontFamily: 'var(--font-sans)',
};

/**
 * Token-styled port of the app's `.sidebar`. Standalone (routing-agnostic) —
 * the live app's Layout keeps its react-router NavLinks + responsive shell.css
 * rules; this is the design-system reference primitive.
 */
export function Sidebar({ items, role = 'viewer', theme = 'light', onToggleTheme, onLogout }: SidebarProps) {
  return (
    <aside style={asideStyle}>
      <div style={{ fontWeight: 700, fontSize: 'var(--text-base)', letterSpacing: 'var(--tracking-tight)', padding: '0.25rem 0.5rem 1.25rem', display: 'flex', alignItems: 'center', gap: '0.5rem', color: 'var(--ink)' }}>
        <span style={{ width: '0.55rem', height: '0.55rem', borderRadius: '50%', background: 'var(--accent)', flexShrink: 0 }} />
        Ubuntu Auto-Update
      </div>
      <nav style={{ display: 'flex', flexDirection: 'column', gap: '0.15rem' }}>
        {items.map(item => (
          <a
            key={item.label}
            href={item.href ?? '#'}
            onClick={item.onClick}
            style={{
              display: 'flex', alignItems: 'center', padding: '0.5rem 0.7rem', borderRadius: 'var(--radius-md)',
              textDecoration: 'none', fontWeight: item.active ? 600 : 500, fontSize: '0.92rem',
              background: item.active ? 'var(--accent-soft)' : 'transparent',
              color: item.active ? 'var(--accent-strong)' : 'var(--ink-muted)',
            }}
          >
            {item.label}
          </a>
        ))}
      </nav>
      <div style={{ marginTop: 'auto', display: 'flex', flexDirection: 'column', gap: '0.5rem', padding: '0.75rem 0.5rem 0.25rem', borderTop: '1px solid var(--border)', fontSize: 'var(--text-sm)', color: 'var(--ink-muted)' }}>
        <button type="button" onClick={onToggleTheme} aria-label="Toggle dark mode" style={{ width: 'auto', alignSelf: 'flex-start', padding: '0.3rem 0.6rem', fontSize: '0.82rem', background: 'transparent', color: 'var(--ink-muted)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', cursor: 'pointer' }}>
          {theme === 'dark' ? '☀ Light' : '☾ Dark'}
        </button>
        <span>Signed in as <code>{role}</code></span>
        <a href="#" onClick={onLogout} style={{ color: 'var(--ink-muted)' }}>Log out</a>
      </div>
    </aside>
  );
}
