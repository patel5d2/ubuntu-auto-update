import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { apiGet, apiPost, createWebSocket } from '../api';
import type { Host } from '../types';
import { useToast } from '../components/Toast';

export function HostDetail() {
  const { hostId } = useParams<{ hostId: string }>();
  const [host, setHost] = useState<Host | null>(null);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [updateOutput, setUpdateOutput] = useState<string[]>([]);
  const [savingKey, setSavingKey] = useState(false);
  const toast = useToast();

  useEffect(() => {
    if (!hostId) return;
    apiGet<Host>(`/api/v1/hosts/${hostId}`)
      .then(setHost)
      .catch(err => console.error('Failed to fetch host details:', err));
  }, [hostId]);

  const handlePreviewUpdate = () => {
    setIsModalOpen(true);
    setUpdateOutput([]);
    const ws = createWebSocket(`/api/v1/hosts/${hostId}/preview-updates`);
    ws.onmessage = (event) => setUpdateOutput(prev => [...prev, event.data]);
    ws.onerror = (error) => console.error('WebSocket error:', error);
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
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to save key.';
      toast.show(message, 'error');
    } finally {
      setSavingKey(false);
    }
  };

  if (!host) return <div aria-busy="true">Loading host details...</div>;

  return (
    <article>
      <header>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '0.5rem' }}>
          <h2 style={{ margin: 0 }}>{host.hostname}</h2>
          <button onClick={handlePreviewUpdate} style={{ width: 'auto' }}>Preview Updates</button>
          <Link to={`/hosts/${hostId}/execute-script`}>
            <button style={{ width: 'auto' }}>Execute Script</button>
          </Link>
        </div>
      </header>
      <details>
        <summary>Update Output (apt-get update)</summary>
        <pre><code>{host.update_output || 'No output captured.'}</code></pre>
      </details>
      <details open>
        <summary>Upgrade Output (apt-get --dry-run upgrade)</summary>
        <pre><code>{host.upgrade_output || 'No output captured.'}</code></pre>
      </details>

      {host.error && (
        <details open>
          <summary>Error</summary>
          <pre><code>{host.error}</code></pre>
        </details>
      )}

      <details>
        <summary>SSH Private Key</summary>
        <form onSubmit={handleSaveKey}>
          <div className="grid">
            <label htmlFor="sshUser">
              SSH User
              <input type="text" id="sshUser" name="sshUser" placeholder="root" defaultValue={host.ssh_user} required />
            </label>
            <label htmlFor="privateKey">
              Private Key
              <textarea id="privateKey" name="privateKey" placeholder="Private Key" required rows={10} />
            </label>
          </div>
          <button type="submit" disabled={savingKey} aria-busy={savingKey || undefined}>
            {savingKey ? 'Saving...' : 'Save Key'}
          </button>
        </form>
      </details>

      {isModalOpen && (
        <dialog open>
          <article>
            <header>
              <a href="#" aria-label="Close" className="close" onClick={(e) => { e.preventDefault(); setIsModalOpen(false); }}></a>
              Update Output
            </header>
            <pre><code>{updateOutput.join('\n')}</code></pre>
          </article>
        </dialog>
      )}
    </article>
  );
}
