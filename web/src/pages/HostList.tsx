import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { formatDistanceToNow } from 'date-fns';

interface Host {
  id: number;
  hostname: string;
  last_seen: string;
  error: string;
}

export function HostList() {
  const [hosts, setHosts] = useState<Host[]>([]);

  useEffect(() => {
    fetch('http://localhost:8081/api/v1/hosts')
      .then(res => res.json())
      .then(setHosts)
      .catch(err => console.error("Failed to fetch hosts:", err));
  }, []);

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
              <td>{host.error ? <span style={{ color: 'var(--pico-color-red-500)' }}>Error</span> : <span style={{ color: 'var(--pico-color-green-500)' }}>OK</span>}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
