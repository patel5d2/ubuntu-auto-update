import { useCallback, useEffect, useState } from 'react';
import { apiDelete, apiGet, apiPatch, apiPost } from '../api';
import type { Host, Playbook, Schedule } from '../types';
import { RelativeTime } from '../components/RelativeTime';
import { useToast } from '../components/Toast';
import { useConfirm } from '../components/ConfirmDialog';

// Schedules: recurring server-side update runs. Interval-based ("every N
// hours from a start time") — matches the backend scheduler.
export function Schedules() {
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [hosts, setHosts] = useState<Host[]>([]);
  const [playbooks, setPlaybooks] = useState<Playbook[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);

  // Create-form state
  const [name, setName] = useState('');
  const [intervalHours, setIntervalHours] = useState(24);
  const [startAt, setStartAt] = useState(''); // datetime-local value
  const [playbookId, setPlaybookId] = useState<number | ''>(''); // '' = apt upgrade
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [submitting, setSubmitting] = useState(false);

  const toast = useToast();
  const confirm = useConfirm();

  const refresh = useCallback(() => {
    return Promise.all([
      apiGet<Schedule[]>('/api/v1/schedules'),
      apiGet<Host[]>('/api/v1/hosts'),
      apiGet<Playbook[]>('/api/v1/playbooks'),
    ])
      .then(([s, h, p]) => {
        setSchedules(s);
        setHosts(h);
        setPlaybooks(p);
      })
      .catch(err => console.error('Failed to load schedules:', err));
  }, []);

  useEffect(() => {
    refresh().finally(() => setLoading(false));
  }, [refresh]);

  const toggleHost = (id: number) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim() || selected.size === 0) {
      toast.show('Name and at least one host are required.', 'error');
      return;
    }
    setSubmitting(true);
    try {
      const body: Record<string, unknown> = {
        name: name.trim(),
        host_ids: Array.from(selected),
        interval_minutes: Math.round(intervalHours * 60),
      };
      if (startAt) body.start_at = new Date(startAt).toISOString();
      if (playbookId !== '') body.playbook_id = playbookId;
      await apiPost<Schedule>('/api/v1/schedules', body);
      toast.show('Schedule created.', 'success');
      setName('');
      setSelected(new Set());
      setStartAt('');
      setPlaybookId('');
      setCreating(false);
      refresh();
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Failed to create schedule.', 'error');
    } finally {
      setSubmitting(false);
    }
  };

  const handleToggle = async (s: Schedule) => {
    try {
      await apiPatch<Schedule>(`/api/v1/schedules/${s.id}`, { enabled: !s.enabled });
      refresh();
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Failed to update schedule.', 'error');
    }
  };

  const handleDelete = async (s: Schedule) => {
    const ok = await confirm({
      title: `Delete schedule "${s.name}"?`,
      message: 'Future automatic runs stop; past run history is kept.',
      destructive: true,
      confirmLabel: 'Delete',
    });
    if (!ok) return;
    try {
      await apiDelete(`/api/v1/schedules/${s.id}`);
      toast.show('Schedule deleted.', 'success');
      refresh();
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Failed to delete schedule.', 'error');
    }
  };

  const hostName = (id: number) => hosts.find(h => h.id === id)?.hostname ?? `#${id}`;

  if (loading) {
    return (
      <div>
        <h2>Schedules</h2>
        <article aria-busy="true">Loading…</article>
      </div>
    );
  }

  return (
    <div>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '0.5rem', marginBottom: '1rem' }}>
        <h2 style={{ margin: 0 }}>Schedules</h2>
        <button onClick={() => setCreating(v => !v)} style={{ width: 'auto' }}>
          {creating ? 'Cancel' : '+ New Schedule'}
        </button>
      </header>

      {creating && (
        <article>
          <form onSubmit={handleCreate}>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(14rem, 1fr))', gap: '0.75rem' }}>
              <label>
                Name
                <input value={name} onChange={e => setName(e.target.value)} placeholder="Nightly security patch" required />
              </label>
              <label>
                Repeat every (hours)
                <input
                  type="number"
                  min={1}
                  max={720}
                  value={intervalHours}
                  onChange={e => setIntervalHours(Number(e.target.value))}
                />
              </label>
              <label>
                First run (optional)
                <input type="datetime-local" value={startAt} onChange={e => setStartAt(e.target.value)} />
              </label>
              <label>
                Runs
                <select
                  value={playbookId}
                  onChange={e => setPlaybookId(e.target.value === '' ? '' : Number(e.target.value))}
                >
                  <option value="">apt upgrade (default)</option>
                  {playbooks.map(pb => (
                    <option key={pb.id} value={pb.id}>{pb.name}</option>
                  ))}
                </select>
              </label>
            </div>

            <fieldset>
              <legend>Hosts ({selected.size} selected)</legend>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: '0.25rem 1rem', maxHeight: '10rem', overflowY: 'auto' }}>
                {hosts.map(h => (
                  <label key={h.id} style={{ display: 'inline-flex', alignItems: 'center', gap: '0.3rem' }}>
                    <input type="checkbox" checked={selected.has(h.id)} onChange={() => toggleHost(h.id)} />
                    {h.hostname}
                  </label>
                ))}
                {hosts.length === 0 && <small>No hosts registered yet.</small>}
              </div>
            </fieldset>

            <button type="submit" disabled={submitting} aria-busy={submitting || undefined} style={{ width: 'auto' }}>
              Create schedule
            </button>
          </form>
        </article>
      )}

      {schedules.length === 0 ? (
        <article style={{ textAlign: 'center', padding: '2rem' }}>
          <h3 style={{ marginTop: 0 }}>No schedules yet</h3>
          <p>Create one to run <code>apt-get upgrade</code> across selected hosts automatically.</p>
        </article>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Hosts</th>
              <th>Runs</th>
              <th>Every</th>
              <th>Next run</th>
              <th>Enabled</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {schedules.map(s => (
              <tr key={s.id} style={s.enabled ? undefined : { opacity: 0.55 }}>
                <td>{s.name}</td>
                <td title={s.host_ids.map(hostName).join(', ')}>{s.host_ids.length}</td>
                <td>
                  {s.playbook_id != null
                    ? playbooks.find(p => p.id === s.playbook_id)?.name ?? `playbook #${s.playbook_id}`
                    : 'apt upgrade'}
                </td>
                <td>{formatInterval(s.interval_minutes)}</td>
                <td>{s.enabled ? <RelativeTime time={s.next_run_at} /> : '—'}</td>
                <td>
                  <input
                    type="checkbox"
                    role="switch"
                    checked={s.enabled}
                    onChange={() => handleToggle(s)}
                    aria-label={`Enable ${s.name}`}
                  />
                </td>
                <td>
                  <button type="button" className="secondary" onClick={() => handleDelete(s)} style={{ width: 'auto', padding: '0.2rem 0.6rem' }}>
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

function formatInterval(minutes: number): string {
  if (minutes % 1440 === 0) return `${minutes / 1440} day${minutes === 1440 ? '' : 's'}`;
  if (minutes % 60 === 0) return `${minutes / 60} hour${minutes === 60 ? '' : 's'}`;
  return `${minutes} min`;
}
