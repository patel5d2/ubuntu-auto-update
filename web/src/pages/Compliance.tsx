import { useEffect, useState } from 'react';
import { apiGet, authHeaders } from '../api';
import { RelativeTime } from '../components/RelativeTime';
import { StatusBadge } from '../components/StatusBadge';
import { useToast } from '../components/Toast';

// Compliance: one row per host answering "is this machine patched?".
// The CSV export is the same data for auditors and spreadsheets.
interface ComplianceRow {
  host_id: number;
  hostname: string;
  tags: string[];
  os_version: string;
  packages_available: number;
  reboot_required: boolean;
  last_seen: string;
  offline_since: string | null;
  last_success_at: string | null;
  last_attempt_at: string | null;
  last_attempt_status: string | null;
}

export function Compliance() {
  const [rows, setRows] = useState<ComplianceRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [downloading, setDownloading] = useState(false);
  const toast = useToast();

  useEffect(() => {
    apiGet<ComplianceRow[]>('/api/v1/reports/compliance')
      .then(setRows)
      .catch(err => console.error('Failed to load compliance report:', err))
      .finally(() => setLoading(false));
  }, []);

  const downloadCsv = async () => {
    setDownloading(true);
    try {
      const res = await fetch('/api/v1/reports/compliance?format=csv', {
        headers: authHeaders(),
        credentials: 'include',
      });
      if (!res.ok) throw new Error(`Export failed (HTTP ${res.status})`);
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `compliance-${new Date().toISOString().slice(0, 10)}.csv`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Export failed.', 'error');
    } finally {
      setDownloading(false);
    }
  };

  if (loading) {
    return (
      <div>
        <h2>Compliance</h2>
        <article aria-busy="true">Loading…</article>
      </div>
    );
  }

  const patched = rows.filter(r => r.packages_available === 0 && !r.reboot_required).length;

  return (
    <div>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '0.5rem', marginBottom: '1rem' }}>
        <h2 style={{ margin: 0 }}>Compliance</h2>
        <button
          type="button"
          className="secondary"
          onClick={downloadCsv}
          disabled={downloading || rows.length === 0}
          aria-busy={downloading || undefined}
          style={{ width: 'auto' }}
        >
          Download CSV
        </button>
      </header>

      {rows.length === 0 ? (
        <article style={{ textAlign: 'center', padding: '2rem' }}>
          <h3 style={{ marginTop: 0 }}>No hosts yet</h3>
          <p>Register a host and the report fills in.</p>
        </article>
      ) : (
        <>
          <p style={{ opacity: 0.75 }}>
            {patched} of {rows.length} host{rows.length === 1 ? '' : 's'} fully patched
            (no pending updates, no reboot required).
          </p>
          <table>
            <thead>
              <tr>
                <th>Host</th>
                <th>Status</th>
                <th>OS</th>
                <th>Pending</th>
                <th>Reboot</th>
                <th>Last successful update</th>
                <th>Last attempt</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(r => (
                <tr key={r.host_id}>
                  <td>
                    {r.hostname}
                    {r.tags.map(t => <span key={t} className="tag-chip" style={{ marginLeft: '0.3rem' }}>{t}</span>)}
                  </td>
                  <td><StatusBadge status={r.offline_since ? 'offline' : 'online'} /></td>
                  <td>{r.os_version || '—'}</td>
                  <td style={r.packages_available > 0 ? { color: 'var(--bad)', fontWeight: 600 } : undefined}>
                    {r.packages_available}
                  </td>
                  <td>{r.reboot_required ? <span className="badge-alert">⟳ required</span> : '—'}</td>
                  <td>{r.last_success_at ? <RelativeTime time={r.last_success_at} /> : 'never'}</td>
                  <td>
                    {r.last_attempt_at ? (
                      <>
                        <RelativeTime time={r.last_attempt_at} />
                        {r.last_attempt_status === 'failed' && (
                          <span style={{ color: 'var(--bad)', marginLeft: '0.3rem' }}>(failed)</span>
                        )}
                      </>
                    ) : 'never'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </div>
  );
}
