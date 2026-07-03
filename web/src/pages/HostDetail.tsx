import { useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { apiDelete, apiGet, apiPatch, apiPost, createWebSocket } from '../api';
import type { Host, TestConnectionResult, UpdateRun } from '../types';
import { useToast } from '../components/Toast';
import { useConfirm } from '../components/ConfirmDialog';
import { Tabs } from '../components/Tabs';
import { StatusBadge } from '../components/StatusBadge';
import { RelativeTime } from '../components/RelativeTime';

type TabId = 'overview' | 'history' | 'ssh';

export function HostDetail() {
  const { hostId } = useParams<{ hostId: string }>();
  const navigate = useNavigate();
  const [host, setHost] = useState<Host | null>(null);
  const [runs, setRuns] = useState<UpdateRun[]>([]);
  const [tab, setTab] = useState<TabId>('overview');
  const [savingKey, setSavingKey] = useState(false);
  const [autoConfiguring, setAutoConfiguring] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [testing, setTesting] = useState(false);

  // Live-run state. Driven by the websocket while a preview/update is active.
  // lastKind keeps the finished output on screen after the socket closes —
  // previously the panel unmounted the instant the stream ended, so a fast
  // run looked like "nothing happened".
  const [liveLines, setLiveLines] = useState<string[]>([]);
  const [liveKind, setLiveKind] = useState<'preview' | 'update' | null>(null);
  const [lastKind, setLastKind] = useState<'preview' | 'update' | null>(null);
  const liveSocketRef = useRef<WebSocket | null>(null);

  const toast = useToast();
  const confirm = useConfirm();

  useEffect(() => {
    if (!hostId) return;
    apiGet<Host>(`/api/v1/hosts/${hostId}`).then(setHost).catch(err => {
      console.error('Failed to fetch host:', err);
    });
    refreshRuns(hostId);

    return () => {
      // Close any in-flight stream on unmount so the websocket goroutine
      // server-side hangs up cleanly.
      liveSocketRef.current?.close();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hostId]);

  const refreshRuns = (id: string) => {
    apiGet<UpdateRun[]>(`/api/v1/hosts/${id}/runs?limit=20`)
      .then(setRuns)
      .catch(err => console.error('Failed to fetch runs:', err));
  };

  const startStream = (kind: 'preview' | 'update') => {
    if (!hostId || liveSocketRef.current) return;
    setLiveKind(kind);
    setLastKind(kind);
    setLiveLines([]);

    const path =
      kind === 'preview'
        ? `/api/v1/hosts/${hostId}/preview-updates`
        : `/api/v1/hosts/${hostId}/run-update`;

    const ws = createWebSocket(path);
    liveSocketRef.current = ws;

    ws.onmessage = ev => setLiveLines(prev => [...prev, String(ev.data)]);
    ws.onerror = err => console.error('WebSocket error:', err);
    ws.onclose = () => {
      liveSocketRef.current = null;
      setLiveKind(null);
      // History updated; refetch the list and the host (last_seen, error).
      if (hostId) {
        refreshRuns(hostId);
        apiGet<Host>(`/api/v1/hosts/${hostId}`).then(setHost).catch(() => {});
      }
    };
  };

  const handlePreview = () => startStream('preview');

  const handleRunUpdate = async () => {
    if (!host) return;
    const ok = await confirm({
      title: `Run apt-get upgrade on ${host.hostname}?`,
      message:
        'This will install all available package updates on the host. Output streams here in real time and is also kept in update history.',
      destructive: true,
      confirmLabel: 'Run update',
      cancelLabel: 'Cancel',
    });
    if (!ok) return;
    startStream('update');
  };

  const handleSaveKey = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSavingKey(true);

    const data = new FormData(event.currentTarget);
    const sshUser = String(data.get('sshUser') ?? '');
    const privateKey = String(data.get('privateKey') ?? '');

    try {
      await apiPost(`/api/v1/hosts/${hostId}/ssh-key`, {
        ssh_user: sshUser,
        private_key: privateKey,
      });
      toast.show('SSH key saved.', 'success');
      // Re-fetch host so any ssh_user change is reflected immediately.
      apiGet<Host>(`/api/v1/hosts/${hostId}`).then(setHost).catch(() => {});
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Failed to save key.', 'error');
    } finally {
      setSavingKey(false);
    }
  };

  const handleAutoConfigure = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!hostId) return;

    // Capture the form element BEFORE await — React's synthetic event recycles
    // currentTarget after the handler returns, so accessing it after a network
    // round trip can throw or silently no-op.
    const form = event.currentTarget;
    const data = new FormData(form);
    const sshUser = String(data.get('autoSshUser') ?? '').trim();
    const password = String(data.get('autoPassword') ?? '');
    if (password === '') {
      toast.show('Password is required for auto-configure.', 'error');
      return;
    }

    setAutoConfiguring(true);
    try {
      const body: Record<string, string> = { password };
      if (sshUser) body.ssh_user = sshUser;
      const result = await apiPost<{ ok: boolean; sudo_configured: boolean }>(
        `/api/v1/hosts/${hostId}/auto-configure`,
        body,
      );
      const sudoNote = result.sudo_configured ? '' : ' (root user — no sudo needed)';
      toast.show(`Configured. Key generated and installed${sudoNote}.`, 'success');
      // Reset the captured form so the password isn't left sitting in the DOM.
      form.reset();
      apiGet<Host>(`/api/v1/hosts/${hostId}`).then(setHost).catch(() => {});
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Auto-configure failed.', 'error');
    } finally {
      setAutoConfiguring(false);
    }
  };

  const handleTestConnection = async () => {
    if (!hostId) return;
    setTesting(true);
    try {
      const result = await apiPost<TestConnectionResult>(
        `/api/v1/hosts/${hostId}/test-connection`,
        {},
      );
      if (!result.ok) {
        toast.show(`Test failed: ${result.error ?? 'unknown error'}`, 'error');
        return;
      }
      const sudoNote =
        result.sudo_state === 'available' ? ' · sudo OK'
          : result.sudo_state === 'unavailable' ? ' · sudo MISSING (apt-get upgrade will fail)'
          : result.sudo_state === 'root' ? ' · running as root'
          : '';
      const tone = result.sudo_state === 'unavailable' ? 'error' : 'success';
      toast.show(`Connected in ${result.latency_ms}ms${sudoNote}`, tone);
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Test failed', 'error');
    } finally {
      setTesting(false);
    }
  };

  const handleDelete = async () => {
    if (!host) return;
    const ok = await confirm({
      title: `Delete host "${host.hostname}"?`,
      message:
        'This removes the host record, its stored SSH key, and its update history. Update logs on the host itself are unaffected.',
      destructive: true,
      confirmLabel: 'Delete host',
      requireTypedConfirmation: host.hostname,
    });
    if (!ok) return;

    setDeleting(true);
    try {
      await apiDelete(`/api/v1/hosts/${host.id}`, {
        'X-Confirm-Hostname': host.hostname,
      });
      toast.show(`Deleted "${host.hostname}".`, 'success');
      navigate('/hosts');
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Failed to delete host.', 'error');
      setDeleting(false);
    }
  };

  if (!host) return <div aria-busy="true">Loading host details...</div>;

  const isStreaming = liveKind !== null;
  const tabs = [
    { id: 'overview', label: 'Overview' },
    { id: 'history', label: 'History', badge: runs.length || undefined },
    { id: 'ssh', label: 'SSH' },
  ] as const;

  return (
    <article>
      <header>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '0.5rem', flexWrap: 'wrap' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', flexWrap: 'wrap' }}>
            <h2 style={{ margin: 0 }}>{host.hostname}</h2>
            <StatusBadge status={host.error ? 'error' : 'online'} />
          </div>
          <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
            <button
              type="button"
              onClick={handleRunUpdate}
              disabled={isStreaming}
              aria-busy={liveKind === 'update' || undefined}
              style={{ width: 'auto' }}
            >
              {liveKind === 'update' ? 'Updating…' : 'Run Update'}
            </button>
            <button
              type="button"
              className="secondary"
              onClick={handlePreview}
              disabled={isStreaming}
              aria-busy={liveKind === 'preview' || undefined}
              style={{ width: 'auto' }}
            >
              {liveKind === 'preview' ? 'Previewing…' : 'Preview Updates'}
            </button>
            <Link to={`/hosts/${hostId}/execute-script`}>
              <button type="button" className="secondary" disabled={isStreaming} style={{ width: 'auto' }}>
                Execute Script
              </button>
            </Link>
            <button
              type="button"
              className="contrast"
              onClick={handleDelete}
              disabled={deleting || isStreaming}
              aria-busy={deleting || undefined}
              style={{ width: 'auto' }}
            >
              {deleting ? 'Deleting…' : 'Delete'}
            </button>
          </div>
        </div>
      </header>

      <Tabs
        tabs={tabs as unknown as { id: TabId; label: string; badge?: number }[]}
        active={tab}
        onChange={id => setTab(id as TabId)}
      />

      {tab === 'overview' && (
        <section role="tabpanel" id="panel-overview" aria-labelledby="tab-overview">
          <TagEditor host={host} onSaved={setHost} />
          {(host.os_version || host.kernel_version || host.agent_version) && (
            <div style={{ display: 'flex', gap: '1.5rem', flexWrap: 'wrap', margin: '0 0 1rem', fontSize: '0.9rem' }}>
              {host.os_version && <span><strong>OS:</strong> {host.os_version}</span>}
              {host.kernel_version && <span><strong>Kernel:</strong> <code>{host.kernel_version}</code></span>}
              {host.agent_version && <span><strong>Agent:</strong> {host.agent_version}</span>}
              <span><strong>Updates available:</strong> {host.packages_available}</span>
              {host.reboot_required && <span style={{ color: 'var(--bad)', fontWeight: 600 }}>⟳ Reboot required</span>}
            </div>
          )}
          {host.error && (
            <article style={{ borderLeft: '4px solid var(--pico-color-red-500)' }}>
              <strong>Last error</strong>
              <pre style={{ marginTop: '0.5rem' }}><code>{host.error}</code></pre>
            </article>
          )}
          <details open>
            <summary>Latest upgrade output</summary>
            <pre><code>{host.upgrade_output || 'No output captured.'}</code></pre>
          </details>
          <details>
            <summary>Latest update output (apt-get update)</summary>
            <pre><code>{host.update_output || 'No output captured.'}</code></pre>
          </details>
        </section>
      )}

      {tab === 'history' && (
        <section role="tabpanel" id="panel-history" aria-labelledby="tab-history">
          <RunsTable runs={runs} />
        </section>
      )}

      {tab === 'ssh' && (
        <section role="tabpanel" id="panel-ssh" aria-labelledby="tab-ssh">
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '0.5rem' }}>
            <p style={{ margin: 0 }}>
              Current SSH user: <code>{host.ssh_user}</code>
            </p>
            <button
              type="button"
              className="secondary"
              onClick={handleTestConnection}
              disabled={testing}
              aria-busy={testing || undefined}
              style={{ width: 'auto' }}
            >
              {testing ? 'Testing…' : 'Test connection'}
            </button>
          </div>
          <details open style={{ marginTop: '1rem' }}>
            <summary><strong>Auto-configure with password</strong> (recommended)</summary>
            <form onSubmit={handleAutoConfigure}>
              <div className="grid">
                <label htmlFor="autoSshUser">
                  SSH User
                  <input
                    type="text"
                    id="autoSshUser"
                    name="autoSshUser"
                    placeholder="root"
                    defaultValue={host.ssh_user}
                  />
                </label>
                <label htmlFor="autoPassword">
                  SSH Password
                  <input
                    type="password"
                    id="autoPassword"
                    name="autoPassword"
                    placeholder="Used once, never stored"
                    autoComplete="new-password"
                    required
                  />
                </label>
              </div>
              <small style={{ display: 'block', opacity: 0.7, marginBottom: '0.5rem' }}>
                We sign in once with this password, generate a fresh SSH key,
                install it on the host, set up passwordless sudo for non-root
                users, and store the private key encrypted. The password lives
                in memory for the request only.
              </small>
              <button
                type="submit"
                disabled={autoConfiguring}
                aria-busy={autoConfiguring || undefined}
              >
                {autoConfiguring ? 'Configuring…' : 'Configure host'}
              </button>
            </form>
          </details>

          <details style={{ marginTop: '1rem' }}>
            <summary>Paste an existing private key (advanced)</summary>
            <form onSubmit={handleSaveKey}>
              <div className="grid">
                <label htmlFor="sshUser">
                  SSH User
                  <input type="text" id="sshUser" name="sshUser" placeholder="root" defaultValue={host.ssh_user} required />
                </label>
                <label htmlFor="privateKey">
                  Private Key
                  <textarea id="privateKey" name="privateKey" placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" required rows={10} />
                </label>
              </div>
              <small style={{ display: 'block', opacity: 0.7, marginBottom: '0.5rem' }}>
                The matching public key must already be in the SSH user's
                <code>~/.ssh/authorized_keys</code> on the target. For non-root
                users, passwordless sudo must also be configured. Use
                <strong> Test connection</strong> to verify.
              </small>
              <button type="submit" disabled={savingKey} aria-busy={savingKey || undefined}>
                {savingKey ? 'Saving…' : 'Save Key'}
              </button>
            </form>
          </details>
        </section>
      )}

      {/* Live output panel — stays visible after the stream finishes so the
          result is readable; a new run resets it. */}
      {lastKind !== null && (
        <article style={{ marginTop: '1.5rem', borderLeft: '4px solid var(--pico-color-azure-500)' }}>
          <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <strong>{lastKind === 'update' ? 'Update output' : 'Preview output'}</strong>
            {isStreaming ? (
              <span aria-busy="true">streaming…</span>
            ) : (
              <span style={{ display: 'flex', gap: '0.75rem', alignItems: 'center' }}>
                <small>finished — full log saved in History</small>
                <button
                  type="button"
                  className="secondary"
                  style={{ width: 'auto', padding: '0.1rem 0.6rem' }}
                  onClick={() => { setLastKind(null); setLiveLines([]); }}
                >
                  Dismiss
                </button>
              </span>
            )}
          </header>
          <pre style={{ maxHeight: '24rem', overflow: 'auto' }}>
            <code>{liveLines.length > 0 ? liveLines.join('') : 'Waiting for output…'}</code>
          </pre>
        </article>
      )}
    </article>
  );
}

function RunsTable({ runs }: { runs: UpdateRun[] }) {
  if (runs.length === 0) {
    return <p>No runs yet. Click <strong>Preview Updates</strong> or <strong>Run Update</strong> to start one.</p>;
  }

  return (
    <table>
      <thead>
        <tr>
          <th>Started</th>
          <th>Kind</th>
          <th>Status</th>
          <th>By</th>
          <th>Exit</th>
          <th>Duration</th>
        </tr>
      </thead>
      <tbody>
        {runs.map(run => {
          const finished = run.finished_at ? new Date(run.finished_at) : null;
          const duration = finished ? formatDuration(finished.getTime() - new Date(run.started_at).getTime()) : '—';
          return (
            <tr key={run.id}>
              <td>
                <RelativeTime time={run.started_at} />
              </td>
              <td>{run.kind}</td>
              <td>
                <StatusBadge
                  status={mapRunStatus(run.status)}
                  label={run.status}
                  title={run.error ?? undefined}
                />
              </td>
              <td>{run.triggered_by}</td>
              <td>{run.exit_code ?? '—'}</td>
              <td>{duration}</td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

function mapRunStatus(s: UpdateRun['status']) {
  switch (s) {
    case 'running':   return 'updating';
    case 'succeeded': return 'online';
    case 'failed':    return 'error';
    case 'cancelled': return 'offline';
    default:          return 'unknown';
  }
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms} ms`;
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  const rs = s % 60;
  return `${m}m ${rs}s`;
}

// TagEditor: chips + comma-separated edit box. Tags drive filtering on the
// host list and (eventually) schedule targeting.
function TagEditor({ host, onSaved }: { host: Host; onSaved: (h: Host) => void }) {
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState('');
  const [saving, setSaving] = useState(false);
  const toast = useToast();

  const save = async () => {
    setSaving(true);
    try {
      const tags = value.split(',').map(t => t.trim()).filter(Boolean);
      const updated = await apiPatch<Host>(`/api/v1/hosts/${host.id}`, { tags });
      onSaved(updated);
      setEditing(false);
      toast.show('Tags saved.', 'success');
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Failed to save tags.', 'error');
    } finally {
      setSaving(false);
    }
  };

  if (!editing) {
    return (
      <p style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', flexWrap: 'wrap' }}>
        <strong>Tags:</strong>
        {(host.tags ?? []).length === 0 && <small>none</small>}
        {(host.tags ?? []).map(t => (
          <span key={t} className="tag-chip">{t}</span>
        ))}
        <button
          type="button"
          className="secondary"
          style={{ width: 'auto', padding: '0.1rem 0.6rem' }}
          onClick={() => {
            setValue((host.tags ?? []).join(', '));
            setEditing(true);
          }}
        >
          Edit
        </button>
      </p>
    );
  }

  return (
    <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center', marginBottom: '1rem' }}>
      <input
        value={value}
        onChange={e => setValue(e.target.value)}
        placeholder="prod, web, us-east (comma-separated)"
        style={{ flex: 1, marginBottom: 0 }}
      />
      <button type="button" onClick={save} disabled={saving} aria-busy={saving || undefined} style={{ width: 'auto' }}>
        Save
      </button>
      <button type="button" className="secondary" onClick={() => setEditing(false)} style={{ width: 'auto' }}>
        Cancel
      </button>
    </div>
  );
}
