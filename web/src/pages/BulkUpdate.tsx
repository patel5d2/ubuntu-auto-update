import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { apiGet } from '../api';
import type { Host, UpdateRun } from '../types';
import { StatusBadge } from '../components/StatusBadge';
import { RelativeTime } from '../components/RelativeTime';
import { useEvent } from '../hooks/useEvents';

export function BulkUpdate() {
  const { groupId } = useParams<{ groupId: string }>();
  const [runs, setRuns] = useState<UpdateRun[]>([]);
  const [hosts, setHosts] = useState<Record<number, Host>>({});
  const [error, setError] = useState('');

  const fetchRuns = useCallback(async () => {
    if (!groupId) return;
    try {
      const data = await apiGet<UpdateRun[]>(
        `/api/v1/runs?group_id=${encodeURIComponent(groupId)}`,
      );
      setRuns(data);
      setError('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load run group');
    }
  }, [groupId]);

  useEffect(() => {
    if (!groupId) return;

    // Fetch the host map once so we can show hostnames instead of IDs.
    apiGet<Host[]>('/api/v1/hosts')
      .then(list => setHosts(Object.fromEntries(list.map(h => [h.id, h]))))
      .catch(() => { /* non-fatal — we'll fall back to host_id labels */ });

    fetchRuns();
  }, [groupId, fetchRuns]);

  // Each update_runs change in our group fans into a refetch. We debounce so
  // the chunk-by-chunk apt output doesn't bombard the API while a fan-out
  // is underway. The DB trigger fires on every UPDATE (incl. AppendRunOutput),
  // so without debounce we'd re-GET on every 4 KiB of stdout.
  const allRunIds = useMemo(() => new Set(runs.map(r => r.id)), [runs]);
  const refetchTimerRef = useRef<number | null>(null);
  const scheduleRefetch = useCallback(() => {
    if (refetchTimerRef.current != null) return;
    refetchTimerRef.current = window.setTimeout(() => {
      refetchTimerRef.current = null;
      fetchRuns();
    }, 400);
  }, [fetchRuns]);
  useEffect(() => {
    return () => {
      if (refetchTimerRef.current != null) window.clearTimeout(refetchTimerRef.current);
    };
  }, []);

  useEvent({ table: 'update_runs' }, ev => {
    // The pg_notify payload only carries the row id, so we accept any run
    // we already know about plus a "snapshot" hint from reconnects.
    if (ev.op === 'snapshot' || allRunIds.has(ev.id)) {
      scheduleRefetch();
    }
  });

  const summary = useMemo(() => {
    const byStatus = { running: 0, succeeded: 0, failed: 0, cancelled: 0 };
    for (const r of runs) byStatus[r.status]++;
    return byStatus;
  }, [runs]);

  if (!groupId) return <div>Missing run group id.</div>;

  return (
    <article>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', flexWrap: 'wrap', gap: '0.5rem' }}>
        <div>
          <h2 style={{ margin: 0 }}>
            {runs[0]?.kind === 'playbook' ? 'Bulk playbook' : runs[0]?.kind === 'reboot' ? 'Bulk reboot' : 'Bulk update'}
          </h2>
          <small style={{ opacity: 0.7 }}>Group <code>{groupId}</code></small>
        </div>
        <Link to="/hosts" role="button" className="secondary" style={{ width: 'auto' }}>
          Back to hosts
        </Link>
      </header>

      <section aria-label="summary" style={{ display: 'flex', gap: '1rem', flexWrap: 'wrap', marginTop: '1rem' }}>
        <SummaryPill label="Total" value={runs.length} />
        <SummaryPill label="Running" value={summary.running} tone="azure" />
        <SummaryPill label="Succeeded" value={summary.succeeded} tone="green" />
        <SummaryPill label="Failed" value={summary.failed} tone="red" />
        {summary.cancelled > 0 && <SummaryPill label="Cancelled" value={summary.cancelled} tone="grey" />}
      </section>

      {error && (
        <article style={{ borderLeft: '4px solid var(--pico-color-red-500)', marginTop: '1rem' }}>
          <p>{error}</p>
        </article>
      )}

      <section style={{ marginTop: '1rem' }}>
        {runs.length === 0 ? (
          <p aria-busy="true">Loading runs...</p>
        ) : (
          <div>
            {runs.map(run => {
              const host = hosts[run.host_id];
              return (
                <details key={run.id} style={{ marginBottom: '0.5rem' }}>
                  <summary>
                    <span style={{ display: 'inline-flex', alignItems: 'center', gap: '0.5rem', flexWrap: 'wrap' }}>
                      <strong>{host?.hostname ?? `host #${run.host_id}`}</strong>
                      <StatusBadge status={mapStatus(run.status)} label={run.status} title={run.error ?? undefined} />
                      <small style={{ opacity: 0.7 }}>
                        started <RelativeTime time={run.started_at} />
                        {run.exit_code != null && <> · exit {run.exit_code}</>}
                      </small>
                    </span>
                  </summary>
                  <pre style={{ maxHeight: '20rem', overflow: 'auto', marginTop: '0.5rem' }}>
                    <code>{run.output || '(waiting for output...)'}</code>
                  </pre>
                </details>
              );
            })}
          </div>
        )}
      </section>
    </article>
  );
}

type Tone = 'azure' | 'green' | 'red' | 'grey';

function SummaryPill({ label, value, tone }: { label: string; value: number; tone?: Tone }) {
  const color = tone ? `var(--pico-color-${tone}-600)` : undefined;
  return (
    <div style={{ minWidth: '6rem' }}>
      <small style={{ opacity: 0.7 }}>{label}</small>
      <div style={{ fontSize: '1.5rem', fontWeight: 600, color }}>{value}</div>
    </div>
  );
}

function mapStatus(s: UpdateRun['status']) {
  switch (s) {
    case 'running':   return 'updating';
    case 'succeeded': return 'online';
    case 'failed':    return 'error';
    case 'cancelled': return 'offline';
    default:          return 'unknown';
  }
}
