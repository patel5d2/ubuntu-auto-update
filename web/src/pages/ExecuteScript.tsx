import { useState } from 'react';
import { useParams } from 'react-router-dom';

export function ExecuteScript() {
  const { hostId } = useParams<{ hostId: string }>();
  const [script, setScript] = useState('');
  const [output, setOutput] = useState<string[]>([]);
  const [isModalOpen, setIsModalOpen] = useState(false);

  const handleExecuteScript = () => {
    setIsModalOpen(true);
    setOutput([]);
    const ws = new WebSocket(`ws://${window.location.host}/api/v1/hosts/${hostId}/execute-script`);

    ws.onopen = () => {
      ws.send(script);
    };

    ws.onmessage = (event) => {
      setOutput(prev => [...prev, event.data]);
    };

    ws.onclose = () => {
      console.log("WebSocket connection closed.");
    };

    ws.onerror = (error) => {
      console.error("WebSocket error:", error);
    };
  };

  return (
    <article>
      <header>
        <h2>Execute Script on Host {hostId}</h2>
      </header>
      <textarea
        value={script}
        onChange={(e) => setScript(e.target.value)}
        placeholder="Enter your script here"
        rows={10}
        style={{ width: '100%' }}
      />
      <button onClick={handleExecuteScript} style={{ marginTop: '10px' }}>Execute Script</button>

      {isModalOpen && (
        <dialog open>
          <article>
            <header>
              <a href="#" aria-label="Close" className="close" onClick={() => setIsModalOpen(false)}></a>
              Script Output
            </header>
            <pre><code>{output.join('\n')}</code></pre>
          </article>
        </dialog>
      )}
    </article>
  );
}
