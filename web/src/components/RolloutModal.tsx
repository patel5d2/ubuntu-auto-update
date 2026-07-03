import { useState } from 'react';

export interface RolloutOptions {
  concurrency: number;
  canary_count: number;
  canary_wait_seconds: number;
  abort_on_failure_pct: number;
}

// RolloutModal surfaces the staged-rollout knobs the backend has always
// supported (canary wave, wait, abort threshold) but the UI never exposed.
export function RolloutModal({
  hostCount,
  submitting,
  onCancel,
  onSubmit,
}: {
  hostCount: number;
  submitting: boolean;
  onCancel: () => void;
  onSubmit: (opts: RolloutOptions) => void;
}) {
  const [concurrency, setConcurrency] = useState(5);
  const [canaryCount, setCanaryCount] = useState(0);
  const [canaryWait, setCanaryWait] = useState(120);
  const [abortPct, setAbortPct] = useState(50);

  const canary = Math.min(Math.max(canaryCount, 0), hostCount);

  return (
    <dialog open>
      <article style={{ maxWidth: '32rem' }}>
        <header>
          <strong>
            Run update on {hostCount} host{hostCount === 1 ? '' : 's'}
          </strong>
        </header>

        <p style={{ marginBottom: '1rem' }}>
          Each host gets a real <code>apt-get upgrade</code>. Optionally stage the rollout: update a
          canary wave first, wait, then continue only if the canaries succeed.
        </p>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0.75rem' }}>
          <label>
            Concurrency
            <input
              type="number"
              min={1}
              max={20}
              value={concurrency}
              onChange={e => setConcurrency(Number(e.target.value))}
            />
            <small>Parallel SSH sessions (max 20)</small>
          </label>
          <label>
            Canary hosts
            <input
              type="number"
              min={0}
              max={hostCount}
              value={canaryCount}
              onChange={e => setCanaryCount(Number(e.target.value))}
            />
            <small>0 = no canary, update all at once</small>
          </label>
          <label>
            Wait after canary (sec)
            <input
              type="number"
              min={0}
              max={600}
              value={canaryWait}
              onChange={e => setCanaryWait(Number(e.target.value))}
              disabled={canary === 0}
            />
            <small>Pause before the rest of the fleet</small>
          </label>
          <label>
            Abort if failures ≥ (%)
            <input
              type="number"
              min={0}
              max={100}
              value={abortPct}
              onChange={e => setAbortPct(Number(e.target.value))}
              disabled={canary === 0}
            />
            <small>Skip remaining hosts past this rate</small>
          </label>
        </div>

        <footer style={{ display: 'flex', gap: '0.5rem', justifyContent: 'flex-end' }}>
          <button type="button" className="secondary" onClick={onCancel} style={{ width: 'auto' }}>
            Cancel
          </button>
          <button
            type="button"
            disabled={submitting}
            aria-busy={submitting || undefined}
            onClick={() =>
              onSubmit({
                concurrency,
                canary_count: canary,
                canary_wait_seconds: canary > 0 ? canaryWait : 0,
                abort_on_failure_pct: canary > 0 ? abortPct : 0,
              })
            }
            style={{ width: 'auto' }}
          >
            {canary > 0 ? `Start canary rollout (${canary} first)` : 'Run update'}
          </button>
        </footer>
      </article>
    </dialog>
  );
}
