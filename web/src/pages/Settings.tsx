import { useCallback, useEffect, useState } from 'react';
import { apiDelete, apiGet, apiPatch, apiPost, canDoAdmin } from '../api';
import type { ApiToken, AuditRecord, Role, User, Webhook } from '../types';
import { RelativeTime } from '../components/RelativeTime';
import { useToast } from '../components/Toast';
import { useConfirm } from '../components/ConfirmDialog';

// Settings brings the previously UI-less admin/operator backend endpoints into
// reach: user management, the audit log, and webhook subscriptions. Sections
// are gated by role — a viewer sees nothing actionable here.
export function Settings() {
  const isAdmin = canDoAdmin();
  return (
    <div>
      <h2>Settings</h2>
      {isAdmin && <UsersSection />}
      {isAdmin && <ApiTokensSection />}
      <WebhooksSection />
      {isAdmin && <AuditSection />}
      {!isAdmin && (
        <p style={{ opacity: 0.7 }}>
          User management and the audit log require an admin account.
        </p>
      )}
    </div>
  );
}

// ApiTokensSection: mint role-scoped long-lived tokens for automation.
// The secret is shown exactly once, right after creation.
function ApiTokensSection() {
  const [tokens, setTokens] = useState<ApiToken[]>([]);
  const [name, setName] = useState('');
  const [role, setRole] = useState<Role>('operator');
  const [freshSecret, setFreshSecret] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  const confirm = useConfirm();

  const refresh = useCallback(() => {
    apiGet<ApiToken[]>('/api/v1/tokens').then(setTokens).catch(err =>
      console.error('Failed to load tokens:', err),
    );
  }, []);
  useEffect(() => { refresh(); }, [refresh]);

  const create = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    setBusy(true);
    try {
      const created = await apiPost<ApiToken & { secret: string }>('/api/v1/tokens', {
        name: name.trim(),
        role,
      });
      setFreshSecret(created.secret);
      setName('');
      refresh();
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Failed to create token.', 'error');
    } finally {
      setBusy(false);
    }
  };

  const revoke = async (t: ApiToken) => {
    const ok = await confirm({
      title: `Revoke token "${t.name}"?`,
      message: 'Anything using it loses access immediately.',
      destructive: true,
      confirmLabel: 'Revoke',
    });
    if (!ok) return;
    try {
      await apiDelete(`/api/v1/tokens/${t.id}`);
      toast.show('Token revoked.', 'success');
      refresh();
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Failed to revoke token.', 'error');
    }
  };

  return (
    <section style={{ marginBottom: '2rem' }}>
      <h3>API tokens</h3>
      <p style={{ opacity: 0.75, fontSize: '0.9rem' }}>
        Long-lived tokens for scripts and CI. Send as{' '}
        <code>Authorization: Bearer uat_…</code> — the secret is stored hashed
        and shown only once.
      </p>

      {freshSecret && (
        <article style={{ borderLeft: '4px solid var(--accent)' }}>
          <strong>Copy this token now — it will not be shown again.</strong>
          <pre style={{ marginTop: '0.5rem' }}><code>{freshSecret}</code></pre>
          <div style={{ display: 'flex', gap: '0.5rem' }}>
            <button
              type="button"
              style={{ width: 'auto' }}
              onClick={() => {
                navigator.clipboard?.writeText(freshSecret);
                toast.show('Copied.', 'success');
              }}
            >
              Copy
            </button>
            <button type="button" className="secondary" style={{ width: 'auto' }} onClick={() => setFreshSecret(null)}>
              Done
            </button>
          </div>
        </article>
      )}

      <form onSubmit={create} style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap', alignItems: 'flex-end' }}>
        <label style={{ flex: '1 1 12rem', marginBottom: 0 }}>
          Name
          <input value={name} onChange={e => setName(e.target.value)} placeholder="ci-deploy" required />
        </label>
        <label style={{ marginBottom: 0 }}>
          Role
          <select value={role} onChange={e => setRole(e.target.value as Role)} style={{ width: 'auto' }}>
            <option value="viewer">viewer</option>
            <option value="operator">operator</option>
            <option value="admin">admin</option>
          </select>
        </label>
        <button type="submit" disabled={busy} aria-busy={busy || undefined} style={{ width: 'auto' }}>
          Create token
        </button>
      </form>

      {tokens.length > 0 && (
        <table>
          <thead>
            <tr><th>Name</th><th>Role</th><th>Created</th><th>Last used</th><th></th></tr>
          </thead>
          <tbody>
            {tokens.map(t => (
              <tr key={t.id}>
                <td>{t.name}</td>
                <td>{t.role}</td>
                <td><RelativeTime time={t.created_at} /></td>
                <td>{t.last_used_at ? <RelativeTime time={t.last_used_at} /> : 'never'}</td>
                <td>
                  <button type="button" className="secondary" onClick={() => revoke(t)} style={{ width: 'auto', padding: '0.2rem 0.6rem' }}>
                    Revoke
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

function UsersSection() {
  const [users, setUsers] = useState<User[]>([]);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [role, setRole] = useState<Role>('viewer');
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  const confirm = useConfirm();

  const refresh = useCallback(() => {
    apiGet<User[]>('/api/v1/users').then(setUsers).catch(err =>
      console.error('Failed to load users:', err),
    );
  }, []);
  useEffect(() => { refresh(); }, [refresh]);

  const create = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      await apiPost<User>('/api/v1/users', { username: username.trim(), password, role });
      toast.show(`User "${username.trim()}" created.`, 'success');
      setUsername(''); setPassword(''); setRole('viewer');
      refresh();
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Failed to create user.', 'error');
    } finally {
      setBusy(false);
    }
  };

  const toggleDisabled = async (u: User) => {
    try {
      await apiPatch<User>(`/api/v1/users/${u.id}`, { disabled: !u.disabled_at });
      refresh();
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Failed to update user.', 'error');
    }
  };

  const remove = async (u: User) => {
    const ok = await confirm({
      title: `Delete user "${u.username}"?`,
      message: 'They will lose access immediately. Audit history is retained.',
      destructive: true, confirmLabel: 'Delete',
    });
    if (!ok) return;
    try {
      await apiDelete(`/api/v1/users/${u.id}`);
      toast.show('User deleted.', 'success');
      refresh();
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Failed to delete user.', 'error');
    }
  };

  return (
    <section style={{ marginBottom: '2rem' }}>
      <h3>Users</h3>
      <form onSubmit={create} style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap', alignItems: 'flex-end' }}>
        <label style={{ flex: '1 1 10rem', marginBottom: 0 }}>Username
          <input value={username} onChange={e => setUsername(e.target.value)} required />
        </label>
        <label style={{ flex: '1 1 10rem', marginBottom: 0 }}>Password <small style={{ opacity: 0.6 }}>(min 12)</small>
          <input type="password" value={password} onChange={e => setPassword(e.target.value)} required minLength={12} />
        </label>
        <label style={{ flex: '0 1 8rem', marginBottom: 0 }}>Role
          <select value={role} onChange={e => setRole(e.target.value as Role)}>
            <option value="viewer">viewer</option>
            <option value="operator">operator</option>
            <option value="admin">admin</option>
          </select>
        </label>
        <button type="submit" disabled={busy} aria-busy={busy || undefined} style={{ width: 'auto' }}>Add user</button>
      </form>

      <table style={{ marginTop: '1rem' }}>
        <thead>
          <tr><th>Username</th><th>Role</th><th>Status</th><th>Last login</th><th></th></tr>
        </thead>
        <tbody>
          {users.map(u => (
            <tr key={u.id} style={u.disabled_at ? { opacity: 0.55 } : undefined}>
              <td>{u.username}</td>
              <td><code>{u.role}</code></td>
              <td>{u.disabled_at ? 'disabled' : 'active'}</td>
              <td>{u.last_login_at ? <RelativeTime time={u.last_login_at} /> : '—'}</td>
              <td style={{ display: 'flex', gap: '0.4rem' }}>
                <button type="button" className="secondary" style={btnSm} onClick={() => toggleDisabled(u)}>
                  {u.disabled_at ? 'Enable' : 'Disable'}
                </button>
                <button type="button" className="secondary" style={btnSm} onClick={() => remove(u)}>Delete</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

function WebhooksSection() {
  const [hooks, setHooks] = useState<Webhook[]>([]);
  const [url, setUrl] = useState('');
  const [event, setEvent] = useState('update_failure');
  const [busy, setBusy] = useState(false);
  const toast = useToast();

  const refresh = useCallback(() => {
    apiGet<Webhook[]>('/api/v1/webhooks').then(setHooks).catch(err =>
      console.error('Failed to load webhooks:', err),
    );
  }, []);
  useEffect(() => { refresh(); }, [refresh]);

  const add = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      await apiPost('/api/v1/webhooks', { url: url.trim(), event });
      toast.show('Webhook added.', 'success');
      setUrl('');
      refresh();
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Failed to add webhook.', 'error');
    } finally {
      setBusy(false);
    }
  };

  const remove = async (h: Webhook) => {
    try {
      await apiDelete(`/api/v1/webhooks/${h.id}`);
      refresh();
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Failed to delete webhook.', 'error');
    }
  };

  return (
    <section style={{ marginBottom: '2rem' }}>
      <h3>Webhooks</h3>
      <p style={{ opacity: 0.7, marginTop: 0 }}>
        POSTed a JSON payload when an event fires (e.g. an update fails). Point it at Slack, Discord, or your own endpoint.
      </p>
      <form onSubmit={add} style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap', alignItems: 'flex-end' }}>
        <label style={{ flex: '1 1 18rem', marginBottom: 0 }}>URL
          <input type="url" value={url} onChange={e => setUrl(e.target.value)} placeholder="https://hooks.example.com/…" required />
        </label>
        <label style={{ flex: '0 1 12rem', marginBottom: 0 }}>Event
          <select value={event} onChange={e => setEvent(e.target.value)}>
            <option value="update_success">update_success</option>
            <option value="update_failure">update_failure</option>
            <option value="preview_success">preview_success</option>
          </select>
        </label>
        <button type="submit" disabled={busy} aria-busy={busy || undefined} style={{ width: 'auto' }}>Add webhook</button>
      </form>

      {hooks.length === 0 ? (
        <p style={{ marginTop: '1rem', opacity: 0.7 }}>No webhooks configured.</p>
      ) : (
        <table style={{ marginTop: '1rem' }}>
          <thead><tr><th>URL</th><th>Event</th><th></th></tr></thead>
          <tbody>
            {hooks.map(h => (
              <tr key={h.id}>
                <td style={{ wordBreak: 'break-all' }}>{h.url}</td>
                <td><code>{h.event}</code></td>
                <td><button type="button" className="secondary" style={btnSm} onClick={() => remove(h)}>Delete</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

function AuditSection() {
  const [records, setRecords] = useState<AuditRecord[]>([]);
  useEffect(() => {
    apiGet<AuditRecord[]>('/api/v1/audit?limit=50').then(setRecords).catch(err =>
      console.error('Failed to load audit log:', err),
    );
  }, []);

  return (
    <section>
      <h3>Audit log</h3>
      {records.length === 0 ? (
        <p style={{ opacity: 0.7 }}>No audit records yet.</p>
      ) : (
        <table>
          <thead><tr><th>When</th><th>Actor</th><th>Action</th><th>Target</th></tr></thead>
          <tbody>
            {records.map(rec => (
              <tr key={rec.id}>
                <td><RelativeTime time={rec.occurred_at} /></td>
                <td>{rec.actor_label ?? '—'}</td>
                <td><code>{rec.action}</code></td>
                <td>{rec.target_type ? `${rec.target_type}:${rec.target_id ?? ''}` : '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

const btnSm: React.CSSProperties = { width: 'auto', padding: '0.15rem 0.6rem', margin: 0 };
