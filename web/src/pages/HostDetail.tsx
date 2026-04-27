import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { apiGet, apiPost, createWebSocket } from '../api';
import type { Host } from '../types';

type SaveKeyState =
  | { kind: 'idle' }
  | { kind: 'saving' }
  | { kind: 'success'; message: string }
  | { kind: 'error'; message: string };

export function HostDetail() {
  const { hostId } = useParams<{ hostId: string }>();
  const [host, setHost] = useState<Host | null>(null);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [updateOutput, setUpdateOutput] = useState<string[]>([]);
  const [saveKey, setSaveKey] = useState<SaveKeyState>({ kind: 'idle' });

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
    setSaveKey({ kind: 'saving' });

    const data = new FormData(event.currentTarget);
    const sshUser = String(data.get('sshUser') ?? '');
    const privateKey = String(data.get('privateKey') ?? '');

    try {
      await apiPost(`/api/v1/hosts/${hostId}/ssh-key`, {
        ssh_user: sshUser,
        private_key: privateKey,
      });
      setSaveKey({ kind: 'success', message: 'Key saved successfully.' });
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to save key.';
      setSaveKey({ kind: 'error', message });
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
          <button type="submit" disabled={saveKey.kind === 'saving'}>
            {saveKey.kind === 'saving' ? 'Saving...' : 'Save Key'}
          </button>
          {saveKey.kind === 'success' && (
            <aside role="status" style={{ color: 'var(--pico-color-green-500)' }}>{saveKey.message}</aside>
          )}
          {saveKey.kind === 'error' && (
            <aside role="alert" style={{ color: 'var(--pico-color-red-500)' }}>{saveKey.message}</aside>
          )}
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
