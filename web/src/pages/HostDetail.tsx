import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

interface Host {
  id: number;
  hostname: string;
  ssh_user: string;
  update_output: string;
  upgrade_output: string;
  error: string;
}

export function HostDetail() {
  const { hostId } = useParams<{ hostId: string }>();
  const [host, setHost] = useState<Host | null>(null);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [updateOutput, setUpdateOutput] = useState<string[]>([]);

  useEffect(() => {
    if (hostId) {
      fetch(`http://localhost:8081/api/v1/hosts/${hostId}`)
        .then(res => res.json())
        .then(setHost)
        .catch(err => console.error("Failed to fetch host details:", err));
    }
  }, [hostId]);

  const handleRunUpdate = () => {
    setIsModalOpen(true);
    setUpdateOutput([]);
    const ws = new WebSocket(`ws://localhost:8081/api/v1/hosts/${hostId}/run-update`);

    ws.onmessage = (event) => {
      setUpdateOutput(prev => [...prev, event.data]);
    };

    ws.onclose = () => {
      console.log("WebSocket connection closed.");
    };

    ws.onerror = (error) => {
      console.error("WebSocket error:", error);
    };
  };

  // const handleScanKey = async () => { // Keeping for future functionality
  //   const response = await fetch(`http://localhost:8081/api/v1/hosts/${hostId}/ssh-key`, {
  //     method: 'POST',
  //   });
  //
  //   if (response.ok) {
  //     alert('Host key scanned successfully!');
  //   } else {
  //     alert('Failed to scan host key.');
  //   }
  // };

  const handleSaveKey = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = event.currentTarget;
    const sshUser = (form.elements.namedItem('sshUser') as HTMLInputElement).value;
    const privateKey = (form.elements.namedItem('privateKey') as HTMLTextAreaElement).value;

    const response = await fetch(`http://localhost:8081/api/v1/hosts/${hostId}/ssh-key`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ssh_user: sshUser, private_key: privateKey }),
      }
    );

    if (response.ok) {
      alert('Key saved successfully!');
    } else {
      alert('Failed to save key.');
    }
  };

  if (!host) {
    return <div aria-busy="true">Loading host details...</div>;
  }

  return (
    <article>
      <header>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 style={{ margin: 0 }}>{host.hostname}</h2>
          <button onClick={handleRunUpdate} style={{ width: 'auto' }}>Run Update</button>
          <Link to={`/hosts/${hostId}/execute-script`}><button style={{ width: 'auto' }}>Execute Script</button></Link>
        </div>
      </header>
      <details>
        <summary>Update Output (apt-get update)</summary>
        <pre><code>{host.update_output || "No output captured."}</code></pre>
      </details>
      <details open>
        <summary>Upgrade Output (apt-get --dry-run upgrade)</summary>
        <pre><code>{host.upgrade_output || "No output captured."}</code></pre>
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
          <button type="submit">Save Key</button>
        </form>
      </details>

      {isModalOpen && (
        <dialog open>
          <article>
            <header>
              <a href="#" aria-label="Close" className="close" onClick={() => setIsModalOpen(false)}></a>
              Update Output
            </header>
            <pre><code>{updateOutput.join('\n')}</code></pre>
          </article>
        </dialog>
      )}
    </article>
  );
}