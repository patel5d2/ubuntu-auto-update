import { useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { formatDistanceToNow } from 'date-fns';
import { apiDelete, apiGet, apiPost, createWebSocket } from '../api';
import type { Host, UpdateRun } from '../types';
import { useToast } from '../components/Toast';
import { useConfirm } from '../components/ConfirmDialog';
import { Tabs } from '../components/Tabs';
import { StatusBadge } from '../components/StatusBadge';

type TabId = 'overview' | 'history' | 'ssh';

export function HostDetail() {
  const { hostId } = useParams<{ hostId: string }>();
  const navigate = useNavigate();
  const [host, setHost] = useState<Host | null>(null);
  const [runs, setRuns] = useState<UpdateRun[]>([]);
  const [tab, setTab] = useState<TabId>('overview');
  const [savingKey, setSavingKey] = useState(false);
  const [deleting, setDeleting] = useState(false);

  // Live-run state. Driven by the websocket while a preview/update is active.
  const [liveLines, setLiveLines] = useState<string[]>([]);
  const [liveKind, setLiveKind] = useState<'preview' | 'update' | null>(null);
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
          <p>
            Current SSH user: <code>{host.ssh_user}</code>
          </p>
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
            <button type="submit" disabled={savingKey} aria-busy={savingKey || undefined}>
              {savingKey ? 'Saving…' : 'Save Key'}
            </button>
          </form>
        </section>
      )}

      {/* Live output overlay — visible whenever a stream is active. */}
      {isStreaming && (
        <article style={{ marginTop: '1.5rem', borderLeft: '4px solid var(--pico-color-azure-500)' }}>
          <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <strong>{liveKind === 'update' ? 'Live update output' : 'Live preview output'}</strong>
            <span aria-busy="true">streaming…</span>
          </header>
          <pre style={{ maxHeight: '24rem', overflow: 'auto' }}><code>{liveLines.join('')}</code></pre>
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
          const started = new Date(run.started_at);
          const finished = run.finished_at ? new Date(run.finished_at) : null;
          const duration = finished ? formatDuration(finished.getTime() - started.getTime()) : '—';
          return (
            <tr key={run.id}>
              <td title={started.toISOString()}>
                {formatDistanceToNow(started, { addSuffix: true })}
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
