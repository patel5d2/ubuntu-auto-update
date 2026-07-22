import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { apiGet } from '../api';
import type { Host, Overview as OverviewStats, Schedule } from '../types';
import { RelativeTime } from '../components/RelativeTime';
import { StatusBadge } from '../components/StatusBadge';
import { StatCard } from '../components/StatCard';
import { useEvent } from '../hooks/useEvents';

const OFFLINE_THRESHOLD_MS = 15 * 60 * 1000;

// Fleet Overview: the landing dashboard. Stat cards from /overview, plus a
// "needs attention" list (error/offline hosts) and upcoming schedules —
// everything an operator wants before drilling into a host.
export function Overview() {
  const [stats, setStats] = useState<OverviewStats | null>(null);
  const [hosts, setHosts] = useState<Host[]>([]);
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [error, setError] = useState('');

  const refresh = useCallback(() => {
    Promise.all([
      apiGet<OverviewStats>('/api/v1/overview'),
      apiGet<Host[]>('/api/v1/hosts'),
      apiGet<Schedule[]>('/api/v1/schedules'),
    ])
      .then(([s, h, sch]) => {
        setStats(s);
        setHosts(h);
        setSchedules(sch);
        setError('');
      })
      .catch(err => {
        console.error('Failed to load overview:', err);
        setError('Failed to load overview. Is the backend running?');
      });
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  useEvent({ table: 'hosts' }, refresh);
  useEvent({ table: 'update_runs' }, refresh);

  const attention = hosts.filter(h => {
    if (h.error) return true;
    const seen = new Date(h.last_seen).getTime();
    return Number.isFinite(seen) && Date.now() - seen > OFFLINE_THRESHOLD_MS;
  });

  const upcoming = schedules.filter(s => s.enabled).slice(0, 5);

  if (!stats && !error) {
    return (
      <div>
        <h2>Fleet Overview</h2>
        <article aria-busy="true">Loading…</article>
      </div>
    );
  }

  return (
    <div>
      <h2>Fleet Overview</h2>
      {error && <article style={{ color: 'var(--bad)' }}>{error}</article>}

      {stats && (
        <div className="stat-grid">
          <StatCard label="Hosts" value={stats.total_hosts} />
          <StatCard label="Online (24h)" value={stats.online_hosts} tone="good" />
          <StatCard label="Hosts with errors" value={stats.error_hosts} tone={stats.error_hosts > 0 ? 'bad' : undefined} />
          <StatCard label="Reboot required" value={stats.reboot_hosts} tone={stats.reboot_hosts > 0 ? 'bad' : undefined} />
          <StatCard label="Runs (7 days)" value={stats.runs_7d} />
          <StatCard label="Failed runs (7 days)" value={stats.failed_7d} tone={stats.failed_7d > 0 ? 'bad' : undefined} />
          <StatCard label="Running now" value={stats.running_now} />
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(20rem, 1fr))', gap: '1rem' }}>
        <section>
          <h4>Needs attention</h4>
          {attention.length === 0 ? (
            <article>All hosts healthy. 🎉</article>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Host</th>
                  <th>Status</th>
                  <th>Last seen</th>
                </tr>
              </thead>
              <tbody>
                {attention.slice(0, 10).map(h => (
                  <tr key={h.id}>
                    <td><Link to={`/hosts/${h.id}`}>{h.hostname}</Link></td>
                    <td><StatusBadge status={h.error ? 'error' : 'offline'} title={h.error ?? undefined} /></td>
                    <td><RelativeTime time={h.last_seen} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>

        <section>
          <h4>Upcoming schedules</h4>
          {upcoming.length === 0 ? (
            <article>
              No schedules yet. <Link to="/schedules">Create one</Link> to run updates automatically.
            </article>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Hosts</th>
                  <th>Next run</th>
                </tr>
              </thead>
              <tbody>
                {upcoming.map(s => (
                  <tr key={s.id}>
                    <td><Link to="/schedules">{s.name}</Link></td>
                    <td>{s.host_ids.length}</td>
                    <td><RelativeTime time={s.next_run_at} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>
      </div>
    </div>
  );
}
