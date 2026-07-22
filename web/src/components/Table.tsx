import type { CSSProperties, ReactNode } from 'react';

export interface Column<T> {
  key: string;
  label: ReactNode;
  width?: string;
  render?: (row: T) => ReactNode;
}

interface TableProps<T> {
  columns: Column<T>[];
  rows: T[];
  rowKey?: (row: T) => string | number;
  onRowClick?: (row: T) => void;
  /** Shown in a single spanned row when `rows` is empty. */
  emptyText?: ReactNode;
}

const tableStyle: CSSProperties = {
  width: '100%', borderCollapse: 'collapse', fontFamily: 'var(--font-sans)', fontSize: 'var(--text-body)',
  borderRadius: 'var(--radius-lg)', overflow: 'hidden', boxShadow: 'var(--shadow-sm)', border: '1px solid var(--border)',
};
const thStyle: CSSProperties = {
  textAlign: 'left', padding: '0.6rem 0.9rem', background: 'var(--surface-2)',
  fontSize: 'var(--text-xs)', fontWeight: 600, textTransform: 'uppercase', letterSpacing: 'var(--tracking-wide)',
  color: 'var(--ink-muted)',
};

/** Data-driven table primitive matching the app's `.content table` styling. */
export function Table<T>({ columns, rows, rowKey, onRowClick, emptyText = 'No data.' }: TableProps<T>) {
  return (
    <table style={tableStyle}>
      <thead>
        <tr>
          {columns.map(c => (
            <th key={c.key} style={{ ...thStyle, width: c.width }}>{c.label}</th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((row, i) => (
          <tr
            key={rowKey ? rowKey(row) : i}
            onClick={onRowClick ? () => onRowClick(row) : undefined}
            style={{ cursor: onRowClick ? 'pointer' : 'default', borderTop: '1px solid var(--border)' }}
          >
            {columns.map(c => (
              <td key={c.key} style={{ padding: '0.6rem 0.9rem', color: 'var(--ink)' }}>
                {c.render ? c.render(row) : (row as Record<string, ReactNode>)[c.key]}
              </td>
            ))}
          </tr>
        ))}
        {rows.length === 0 && (
          <tr><td colSpan={columns.length} style={{ textAlign: 'center', padding: '1.5rem', color: 'var(--ink-muted)' }}>{emptyText}</td></tr>
        )}
      </tbody>
    </table>
  );
}
