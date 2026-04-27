import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { formatDistanceToNow } from 'date-fns';
import { apiGet } from '../api';
import type { Host } from '../types';

export function HostList() {
  const [hosts, setHosts] = useState<Host[]>([]);
  const [loading, setLoading] = useState(true);
  const [fetchError, setFetchError] = useState('');

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

  if (loading) return <div aria-busy="true">Loading hosts...</div>;

  if (fetchError) {
    return (
      <div>
        <h2>Managed Hosts</h2>
        <article style={{ color: 'var(--pico-color-red-500)' }}>
          <p>{fetchError}</p>
        </article>
      </div>
    );
  }

  return (
    <div>
      <h2>Managed Hosts</h2>
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
              <td colSpan={3} style={{ textAlign: 'center' }}>No hosts registered yet.</td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
