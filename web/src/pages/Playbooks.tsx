import { useCallback, useEffect, useState } from 'react';
import { apiDelete, apiGet, apiPatch, apiPost } from '../api';
import type { Playbook } from '../types';
import { RelativeTime } from '../components/RelativeTime';
import { useToast } from '../components/Toast';
import { useConfirm } from '../components/ConfirmDialog';

// Playbooks: named, ordered shell steps run over SSH one-by-one, stop-on-failure.
// Runs happen from HostDetail (single host) and the Hosts bulk bar; this page is
// just the library (CRUD).
export function Playbooks() {
  const [playbooks, setPlaybooks] = useState<Playbook[]>([]);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<Playbook | null>(null); // non-null = edit, 'new' via open flag
  const [open, setOpen] = useState(false);

  // Form state
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [stepsText, setStepsText] = useState('');
  const [useSudo, setUseSudo] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  const toast = useToast();
  const confirm = useConfirm();

  const refresh = useCallback(() => {
    return apiGet<Playbook[]>('/api/v1/playbooks')
      .then(setPlaybooks)
      .catch(err => console.error('Failed to load playbooks:', err));
  }, []);

  useEffect(() => {
    refresh().finally(() => setLoading(false));
  }, [refresh]);

  const openCreate = () => {
    setEditing(null);
    setName('');
    setDescription('');
    setStepsText('');
    setUseSudo(true);
    setOpen(true);
  };

  const openEdit = (pb: Playbook) => {
    setEditing(pb);
    setName(pb.name);
    setDescription(pb.description);
    setStepsText(pb.steps.join('\n'));
    setUseSudo(pb.use_sudo);
    setOpen(true);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const steps = stepsText.split('\n').map(s => s.trim()).filter(Boolean);
    if (!name.trim() || steps.length === 0) {
      toast.show('Name and at least one step are required.', 'error');
      return;
    }
    setSubmitting(true);
    try {
      const body = { name: name.trim(), description: description.trim(), steps, use_sudo: useSudo };
      if (editing) {
        await apiPatch<Playbook>(`/api/v1/playbooks/${editing.id}`, body);
        toast.show('Playbook updated.', 'success');
      } else {
        await apiPost<Playbook>('/api/v1/playbooks', body);
        toast.show('Playbook created.', 'success');
      }
      setOpen(false);
      refresh();
    } catch (err) {
      toast.show(err instanceof Error ? err.message : 'Failed to save playbook.', 'error');
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async (pb: Playbook) => {
    const ok = await confirm({
      title: `Delete playbook "${pb.name}"?`,
      message: 'Past run history is kept. This cannot be undone.',
      destructive: true,
      confirmLabel: 'Delete',
    });
    if (!ok) return;
    try {
      await apiDelete(`/api/v1/playbooks/${pb.id}`);
      toast.show('Playbook deleted.', 'success');
      refresh();
    } catch (err) {
      // Surfaces the 409 "used by N schedule(s)" message from the API.
      toast.show(err instanceof Error ? err.message : 'Failed to delete playbook.', 'error');
    }
  };

  if (loading) {
    return (
      <div>
        <h2>Playbooks</h2>
        <article aria-busy="true">Loading…</article>
      </div>
    );
  }

  return (
    <div>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '0.5rem', marginBottom: '1rem' }}>
        <h2 style={{ margin: 0 }}>Playbooks</h2>
        <button onClick={openCreate} style={{ width: 'auto' }}>+ New Playbook</button>
      </header>

      {open && (
        <article>
          <form onSubmit={handleSubmit}>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(14rem, 1fr))', gap: '0.75rem' }}>
              <label>
                Name
                <input value={name} onChange={e => setName(e.target.value)} placeholder="Harden & patch" required />
              </label>
              <label>
                Description
                <input value={description} onChange={e => setDescription(e.target.value)} placeholder="What this playbook does" />
              </label>
            </div>

            <label>
              Steps (one shell command per line)
              <textarea
                value={stepsText}
                onChange={e => setStepsText(e.target.value)}
                rows={6}
                placeholder={'apt-get update\napt-get upgrade -y\nsystemctl restart nginx'}
                style={{ fontFamily: 'monospace' }}
              />
            </label>

            <label style={{ display: 'inline-flex', alignItems: 'center', gap: '0.4rem' }}>
              <input type="checkbox" role="switch" checked={useSudo} onChange={e => setUseSudo(e.target.checked)} />
              Run with sudo (non-root hosts)
            </label>

            <small style={{ display: 'block', margin: '0.5rem 0 1rem', opacity: 0.75 }}>
              Steps run sequentially over SSH as the host's SSH user and stop at the first
              failure. With <strong>Run with sudo</strong>, each step runs as{' '}
              <code>sudo -n sh -c '…'</code> — non-root hosts need passwordless sudo broad
              enough for the commands you use.
            </small>

            <div style={{ display: 'flex', gap: '0.5rem' }}>
              <button type="submit" disabled={submitting} aria-busy={submitting || undefined} style={{ width: 'auto' }}>
                {editing ? 'Save changes' : 'Create playbook'}
              </button>
              <button type="button" className="secondary" onClick={() => setOpen(false)} style={{ width: 'auto' }}>
                Cancel
              </button>
            </div>
          </form>
        </article>
      )}

      {playbooks.length === 0 ? (
        <article style={{ textAlign: 'center', padding: '2rem' }}>
          <h3 style={{ marginTop: 0 }}>No playbooks yet</h3>
          <p>Create one to run a repeatable sequence of shell steps across your hosts.</p>
        </article>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Description</th>
              <th>Steps</th>
              <th>Sudo</th>
              <th>Updated</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {playbooks.map(pb => (
              <tr key={pb.id}>
                <td>{pb.name}</td>
                <td style={{ opacity: 0.8 }}>{pb.description || '—'}</td>
                <td title={pb.steps.join('\n')}>{pb.steps.length}</td>
                <td>{pb.use_sudo ? <span className="tag-chip">sudo</span> : '—'}</td>
                <td><RelativeTime time={pb.updated_at} /></td>
                <td style={{ display: 'flex', gap: '0.4rem' }}>
                  <button type="button" className="secondary" onClick={() => openEdit(pb)} style={{ width: 'auto', padding: '0.2rem 0.6rem' }}>Edit</button>
                  <button type="button" className="secondary" onClick={() => handleDelete(pb)} style={{ width: 'auto', padding: '0.2rem 0.6rem' }}>Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
