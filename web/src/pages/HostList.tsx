import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { apiDelete, apiGet, apiPost } from '../api';
import type { BulkRunResult, Host, Playbook } from '../types';
import { AddHostModal } from '../components/AddHostModal';
import { StatusBadge, type HostStatus } from '../components/StatusBadge';
import { RelativeTime } from '../components/RelativeTime';
import { useToast } from '../components/Toast';
import { useConfirm } from '../components/ConfirmDialog';
import { useEvent } from '../hooks/useEvents';
import { RolloutModal, type RolloutOptions } from '../components/RolloutModal';

type StatusFilter = 'all' | HostStatus;

// hostStatus maps a raw host record to a StatusBadge state. Last-seen older
// than OFFLINE_THRESHOLD_MS without an explicit error is "offline" — the agent
// hasn't reported recently, but we don't have evidence of an actual failure.
const OFFLINE_THRESHOLD_MS = 15 * 60 * 1000;

function hostStatus(host: Host): HostStatus {
  if (host.error) return 'error';
  // Server-side sweep is the source of truth; the client-side threshold
  // below remains as a fallback for rows the sweep hasn't evaluated yet.
  if (host.offline_since) return 'offline';
  const lastSeen = new Date(host.last_seen).getTime();
  if (Number.isFinite(lastSeen) && Date.now() - lastSeen > OFFLINE_THRESHOLD_MS) {
    return 'offline';
  }
  return 'online';
}

export function HostList() {
  const [hosts, setHosts] = useState<Host[]>([]);
  const [loading, setLoading] = useState(true);
  const [fetchError, setFetchError] = useState('');
  const [addOpen, setAddOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [tagFilter, setTagFilter] = useState<string | null>(null);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [bulkSubmitting, setBulkSubmitting] = useState(false);
  const [rolloutOpen, setRolloutOpen] = useState(false);
  // Playbook picked in the bulk bar; the rollout modal runs it instead of
  // apt-get upgrade when non-null.
  const [playbooks, setPlaybooks] = useState<Playbook[]>([]);
  const [selectedPlaybook, setSelectedPlaybook] = useState<number | ''>('');
  const [rolloutPlaybook, setRolloutPlaybook] = useState<Playbook | null>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const navigate = useNavigate();
  const toast = useToast();
  const confirm = useConfirm();

  const refresh = useCallback(() => {
    return apiGet<Host[]>('/api/v1/hosts')
      .then(data => {
        setHosts(data);
        setFetchError('');
      })
      .catch(err => {
        console.error('Failed to fetch hosts:', err);
        setFetchError('Failed to load hosts. Is the backend running?');
      });
  }, []);

  useEffect(() => {
    refresh().finally(() => setLoading(false));
    apiGet<Playbook[]>('/api/v1/playbooks').then(setPlaybooks).catch(() => {});
  }, [refresh]);

  // Live update: any host change (agent report, manual edit, bulk update
  // finishing) refetches the table. The backend coalesces inserts/updates/
  // deletes onto one channel; we just refetch for simplicity. The 200 ms
  // debounce keeps a flurry of trigger fires from each producing a request.
  const refetchTimerRef = useRef<number | null>(null);
  const scheduleRefetch = useCallback(() => {
    if (refetchTimerRef.current != null) return;
    refetchTimerRef.current = window.setTimeout(() => {
      refetchTimerRef.current = null;
      refresh();
    }, 200);
  }, [refresh]);
  useEffect(() => {
    return () => {
      if (refetchTimerRef.current != null) window.clearTimeout(refetchTimerRef.current);
    };
  }, []);

  useEvent({ table: 'hosts' }, scheduleRefetch);
  // update_runs.UPDATE flips a host's "last error" / "last seen" indirectly,
  // so refetch on those too. Polling-free real-time UX falls out of this.
  useEvent({ table: 'update_runs' }, scheduleRefetch);

  // Global keyboard shortcuts: / focus search, n open Add Host. Ignore when
  // typing in form fields so we don't hijack normal input.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const tag = (e.target as HTMLElement | null)?.tagName?.toLowerCase();
      if (tag === 'input' || tag === 'textarea' || tag === 'select') return;
      if (e.key === '/') {
        e.preventDefault();
        searchRef.current?.focus();
      } else if (e.key === 'n') {
        e.preventDefault();
        setAddOpen(true);
      }
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  const filtered = useMemo(() => {
    const term = search.trim().toLowerCase();
    return hosts.filter(host => {
      if (term && !host.hostname.toLowerCase().includes(term)) return false;
      if (statusFilter !== 'all' && hostStatus(host) !== statusFilter) return false;
      if (tagFilter && !(host.tags ?? []).includes(tagFilter)) return false;
      return true;
    });
  }, [hosts, search, statusFilter, tagFilter]);

  // Keep selection in sync with the visible list — if a filtered-out host
  // gets removed it shouldn't linger in the bulk count.
  useEffect(() => {
    setSelected(prev => {
      const visible = new Set(filtered.map(h => h.id));
      const next = new Set<number>();
      for (const id of prev) if (visible.has(id)) next.add(id);
      return next.size === prev.size ? prev : next;
    });
  }, [filtered]);

  const allVisibleSelected = filtered.length > 0 && filtered.every(h => selected.has(h.id));
  const someVisibleSelected = filtered.some(h => selected.has(h.id));

  const toggleAll = () => {
    setSelected(prev => {
      if (allVisibleSelected) {
        const next = new Set(prev);
        for (const h of filtered) next.delete(h.id);
        return next;
      }
      const next = new Set(prev);
      for (const h of filtered) next.add(h.id);
      return next;
    });
  };

  const toggleOne = (id: number) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleCreated = (host: Host) => {
    setHosts(prev => [...prev, host].sort((a, b) => a.hostname.localeCompare(b.hostname)));
  };

  const handleRolloutSubmit = async (opts: RolloutOptions) => {
    const ids = Array.from(selected);
    const pb = rolloutPlaybook;
    setBulkSubmitting(true);
    try {
      const result = pb
        ? await apiPost<BulkRunResult>('/api/v1/hosts/bulk/run-playbook', {
            host_ids: ids,
            playbook_id: pb.id,
            ...opts,
          })
        : await apiPost<BulkRunResult>('/api/v1/hosts/bulk/run-update', {
            host_ids: ids,
            ...opts,
          });
      toast.show(
        `Bulk ${pb ? `playbook "${pb.name}"` : 'update'} started for ${ids.length} host${ids.length === 1 ? '' : 's'}.`,
        'success',
      );
      closeRollout();
      navigate(`/hosts/bulk/${result.group_id}`);
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Failed to start bulk run.', 'error');
    } finally {
      setBulkSubmitting(false);
    }
  };

  const closeRollout = () => {
    setRolloutOpen(false);
    setRolloutPlaybook(null);
  };

  const handleBulkReboot = async () => {
    const ids = Array.from(selected);
    const ok = await confirm({
      title: `Reboot ${ids.length} host${ids.length === 1 ? '' : 's'}?`,
      message:
        'Each host reboots and its run succeeds once it comes back online (up to 10 minutes each).',
      destructive: true,
      confirmLabel: 'Reboot',
    });
    if (!ok) return;
    setBulkSubmitting(true);
    try {
      const result = await apiPost<BulkRunResult>('/api/v1/hosts/bulk/reboot', { host_ids: ids });
      toast.show(`Reboot started for ${ids.length} host${ids.length === 1 ? '' : 's'}.`, 'success');
      navigate(`/hosts/bulk/${result.group_id}`);
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Failed to start reboot.', 'error');
    } finally {
      setBulkSubmitting(false);
    }
  };

  const handleBulkDelete = async () => {
    if (selected.size === 0) return;
    const targets = hosts.filter(h => selected.has(h.id));
    const ok = await confirm({
      title: `Delete ${targets.length} host${targets.length === 1 ? '' : 's'}?`,
      message:
        'This removes the host records, their stored SSH keys, and update history. Type DELETE to confirm.',
      destructive: true,
      confirmLabel: 'Delete',
      requireTypedConfirmation: 'DELETE',
    });
    if (!ok) return;

    // Delete sequentially to keep error reporting clear. With single-admin
    // auth and small selections this is fine; for larger fleets we'd batch.
    let succeeded = 0;
    const failures: string[] = [];
    for (const host of targets) {
      try {
        await apiDelete(`/api/v1/hosts/${host.id}`, {
          'X-Confirm-Hostname': host.hostname,
        });
        succeeded++;
      } catch (err) {
        failures.push(`${host.hostname}: ${err instanceof Error ? err.message : 'unknown error'}`);
      }
    }

    setHosts(prev => prev.filter(h => !targets.find(t => t.id === h.id && !failures.some(f => f.startsWith(h.hostname + ':')))));
    setSelected(new Set());

    if (failures.length === 0) {
      toast.show(`Deleted ${succeeded} host${succeeded === 1 ? '' : 's'}.`, 'success');
    } else {
      toast.show(
        `Deleted ${succeeded}; ${failures.length} failed. ${failures[0]}`,
        'error',
      );
    }
  };

  if (loading) {
    return (
      <div>
        <h2>Managed Hosts</h2>
        <article aria-busy="true">Loading hosts...</article>
      </div>
    );
  }

  return (
    <div>
      <header style={hostListHeaderStyle}>
        <h2 style={{ margin: 0 }}>Managed Hosts</h2>
        <button onClick={() => setAddOpen(true)} style={{ width: 'auto' }} title="Add host (n)">
          + Add Host
        </button>
      </header>

      {fetchError ? (
        <article style={{ color: 'var(--pico-color-red-500)' }}>
          <p>{fetchError}</p>
        </article>
      ) : null}

      {/* Filter bar — search + status. Both reset selection via the effect above. */}
      <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap', alignItems: 'center', margin: '0 0 1rem' }}>
        <input
          ref={searchRef}
          type="search"
          placeholder="Search hostname (press / to focus)"
          value={search}
          onChange={e => setSearch(e.target.value)}
          style={{ flex: '1 1 16rem', minWidth: '12rem' }}
        />
        <select
          value={statusFilter}
          onChange={e => setStatusFilter(e.target.value as StatusFilter)}
          style={{ width: 'auto', minWidth: '9rem' }}
          aria-label="Status filter"
        >
          <option value="all">All statuses</option>
          <option value="online">Online</option>
          <option value="offline">Offline</option>
          <option value="error">Error</option>
        </select>
        {tagFilter && (
          <span className="tag-chip active" onClick={() => setTagFilter(null)} title="Clear tag filter">
            {tagFilter} ✕
          </span>
        )}
      </div>

      {/* Sticky bulk-action bar — only renders while something is selected. */}
      {selected.size > 0 && (
        <div role="toolbar" aria-label="Bulk actions" style={bulkBarStyle}>
          <strong>{selected.size} selected</strong>
          <button
            type="button"
            onClick={() => setRolloutOpen(true)}
            disabled={bulkSubmitting}
            aria-busy={bulkSubmitting || undefined}
            style={{ width: 'auto' }}
            title="Run update on selected (u)"
          >
            {bulkSubmitting ? 'Starting…' : `Update ${selected.size}`}
          </button>
          {playbooks.length > 0 && (
            <>
              <select
                value={selectedPlaybook}
                onChange={e => setSelectedPlaybook(e.target.value === '' ? '' : Number(e.target.value))}
                aria-label="Playbook"
                style={{ width: 'auto', marginBottom: 0 }}
              >
                <option value="">Playbook…</option>
                {playbooks.map(pb => (
                  <option key={pb.id} value={pb.id}>{pb.name}</option>
                ))}
              </select>
              <button
                type="button"
                className="secondary"
                onClick={() => {
                  const pb = playbooks.find(p => p.id === selectedPlaybook);
                  if (!pb) return;
                  setRolloutPlaybook(pb);
                  setRolloutOpen(true);
                }}
                disabled={bulkSubmitting || selectedPlaybook === ''}
                style={{ width: 'auto' }}
              >
                Run playbook
              </button>
            </>
          )}
          <button
            type="button"
            className="secondary"
            onClick={handleBulkReboot}
            disabled={bulkSubmitting}
            style={{ width: 'auto' }}
          >
            Reboot {selected.size}
          </button>
          <button
            type="button"
            className="contrast"
            onClick={handleBulkDelete}
            disabled={bulkSubmitting}
            style={{ width: 'auto' }}
          >
            Delete {selected.size}
          </button>
          <button
            type="button"
            className="secondary"
            onClick={() => setSelected(new Set())}
            style={{ width: 'auto' }}
          >
            Clear
          </button>
        </div>
      )}

      {hosts.length === 0 && !fetchError ? (
        <article style={{ textAlign: 'center', padding: '2rem' }}>
          <h3 style={{ marginTop: 0 }}>No hosts yet</h3>
          <p>
            Click <strong>+ Add Host</strong> to register one manually, or run the install
            script on a machine — it'll auto-enroll using the configured token.
          </p>
        </article>
      ) : (
        <table>
          <thead>
            <tr>
              <th style={{ width: '2rem' }}>
                <input
                  type="checkbox"
                  checked={allVisibleSelected}
                  ref={el => {
                    if (el) el.indeterminate = !allVisibleSelected && someVisibleSelected;
                  }}
                  onChange={toggleAll}
                  aria-label="Select all visible hosts"
                />
              </th>
              <th>Hostname</th>
              <th>Status</th>
              <th>Tags</th>
              <th>Last seen</th>
              <th>SSH user</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map(host => {
              const status = hostStatus(host);
              return (
                <tr key={host.id} data-status={status}>
                  <td>
                    <input
                      type="checkbox"
                      checked={selected.has(host.id)}
                      onChange={() => toggleOne(host.id)}
                      aria-label={`Select ${host.hostname}`}
                    />
                  </td>
                  <td><Link to={`/hosts/${host.id}`}>{host.hostname}</Link></td>
                  <td>
                    <StatusBadge status={status} title={host.error ?? undefined} />
                    {host.reboot_required && (
                      <span className="badge-alert" title="Kernel/package update needs a reboot">
                        ⟳ reboot
                      </span>
                    )}
                  </td>
                  <td>
                    {(host.tags ?? []).map(tag => (
                      <span
                        key={tag}
                        className={`tag-chip${tagFilter === tag ? ' active' : ''}`}
                        onClick={() => setTagFilter(prev => (prev === tag ? null : tag))}
                        title={`Filter by ${tag}`}
                      >
                        {tag}
                      </span>
                    ))}
                  </td>
                  <td><RelativeTime time={host.last_seen} /></td>
                  <td><code>{host.ssh_user}</code></td>
                </tr>
              );
            })}
            {filtered.length === 0 && (
              <tr>
                <td colSpan={6} style={{ textAlign: 'center' }}>
                  No hosts match the current filters.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      )}

      <AddHostModal open={addOpen} onClose={() => setAddOpen(false)} onCreated={handleCreated} />
      {rolloutOpen && (
        <RolloutModal
          hostCount={selected.size}
          submitting={bulkSubmitting}
          onCancel={closeRollout}
          onSubmit={handleRolloutSubmit}
          securityToggle={!rolloutPlaybook}
          {...(rolloutPlaybook && {
            title: `Run playbook "${rolloutPlaybook.name}" on ${selected.size} host${selected.size === 1 ? '' : 's'}`,
            description: (
              <>
                Each host runs the playbook's {rolloutPlaybook.steps.length} step
                {rolloutPlaybook.steps.length === 1 ? '' : 's'} over SSH, stopping at the first
                failure. Optionally stage the rollout with a canary wave.
              </>
            ),
            submitLabel: 'Run playbook',
          })}
        />
      )}
    </div>
  );
}

const hostListHeaderStyle: React.CSSProperties = {
  display: 'flex',
  justifyContent: 'space-between',
  alignItems: 'center',
  marginBottom: '1rem',
  gap: '0.5rem',
};

const bulkBarStyle: React.CSSProperties = {
  position: 'sticky',
  top: 0,
  zIndex: 10,
  display: 'flex',
  alignItems: 'center',
  gap: '0.75rem',
  padding: '0.5rem 0.75rem',
  marginBottom: '0.75rem',
  backgroundColor: 'var(--pico-card-background-color)',
  borderRadius: '0.5rem',
  boxShadow: '0 2px 8px rgba(0,0,0,0.08)',
  flexWrap: 'wrap',
};
