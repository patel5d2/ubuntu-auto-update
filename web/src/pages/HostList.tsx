import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { formatDistanceToNow } from 'date-fns';
import { apiGet } from '../api';
import type { Host } from '../types';
import { AddHostModal } from '../components/AddHostModal';

export function HostList() {
  const [hosts, setHosts] = useState<Host[]>([]);
  const [loading, setLoading] = useState(true);
  const [fetchError, setFetchError] = useState('');
  const [addOpen, setAddOpen] = useState(false);

  useEffect(() => {
    apiGet<Host[]>('/api/v1/hosts')
      .then(data => {
        setHosts(data);
        setFetchError('');
      })
      .catch(err => {
        console.error('Failed to fetch hosts:', err);
        setFetchError('Failed to load hosts. Is the backend running?');
      })
      .finally(() => setLoading(false));
  }, []);

  const handleCreated = (host: Host) => {
    // Optimistic insert + sort by hostname so it lands in the right place.
    setHosts(prev => [...prev, host].sort((a, b) => a.hostname.localeCompare(b.hostname)));
  };

  if (loading) return <div aria-busy="true">Loading hosts...</div>;

  if (fetchError) {
    return (
      <div>
        <header style={hostListHeaderStyle}>
          <h2 style={{ margin: 0 }}>Managed Hosts</h2>
          <button onClick={() => setAddOpen(true)} style={{ width: 'auto' }}>+ Add Host</button>
        </header>
        <article style={{ color: 'var(--pico-color-red-500)' }}>
          <p>{fetchError}</p>
        </article>
        <AddHostModal open={addOpen} onClose={() => setAddOpen(false)} onCreated={handleCreated} />
      </div>
    );
  }

  return (
    <div>
      <header style={hostListHeaderStyle}>
        <h2 style={{ margin: 0 }}>Managed Hosts</h2>
        <button onClick={() => setAddOpen(true)} style={{ width: 'auto' }}>+ Add Host</button>
      </header>

      <table>
        <thead>
          <tr>
            <th>Hostname</th>
            <th>Last Seen</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {hosts.map(host => (
            <tr key={host.id}>
              <td><Link to={`/hosts/${host.id}`}>{host.hostname}</Link></td>
              <td>{formatDistanceToNow(new Date(host.last_seen), { addSuffix: true })}</td>
              <td>
                {host.error
                  ? <span style={{ color: 'var(--pico-color-red-500)' }}>Error</span>
                  : <span style={{ color: 'var(--pico-color-green-500)' }}>OK</span>}
              </td>
            </tr>
          ))}
          {hosts.length === 0 && (
            <tr>
              <td colSpan={3} style={{ textAlign: 'center' }}>
                No hosts registered yet. Click <strong>+ Add Host</strong> to add one.
              </td>
            </tr>
          )}
        </tbody>
      </table>

      <AddHostModal open={addOpen} onClose={() => setAddOpen(false)} onCreated={handleCreated} />
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
