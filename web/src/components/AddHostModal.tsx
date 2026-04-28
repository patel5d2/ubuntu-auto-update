import { useEffect, useRef, useState } from 'react';
import { apiPost } from '../api';
import type { Host } from '../types';
import { useToast } from './Toast';

interface AddHostModalProps {
  open: boolean;
  onClose: () => void;
  /** Called with the newly-created host so the parent can update its list. */
  onCreated: (host: Host) => void;
}

// Hostnames here are application-level identifiers, not strict DNS labels.
// Allow the common subset: letters, digits, dot, dash, underscore.
const HOSTNAME_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]*$/;

export function AddHostModal({ open, onClose, onCreated }: AddHostModalProps) {
  const [submitting, setSubmitting] = useState(false);
  const [hostnameError, setHostnameError] = useState('');
  const toast = useToast();
  const firstFieldRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) {
      setHostnameError('');
      // Focus the first field on open so the user can start typing immediately.
      queueMicrotask(() => firstFieldRef.current?.focus());
    }
  }, [open]);

  // Esc closes the dialog (only when not actively submitting).
  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape' && !submitting) onClose();
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, submitting, onClose]);

  if (!open) return null;

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();

    const data = new FormData(event.currentTarget);
    const hostname = String(data.get('hostname') ?? '').trim();
    const sshUser = String(data.get('ssh_user') ?? '').trim();

    if (!HOSTNAME_PATTERN.test(hostname)) {
      setHostnameError('Use letters, digits, dot, dash, or underscore.');
      return;
    }
    setHostnameError('');
    setSubmitting(true);

    try {
      const host = await apiPost<Host>('/api/v1/hosts', {
        hostname,
        ssh_user: sshUser || 'root',
      });
      toast.show(`Host "${host.hostname}" added.`, 'success');
      onCreated(host);
      onClose();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to add host.';
      toast.show(message, 'error');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <dialog open aria-labelledby="add-host-title">
      <article>
        <header>
          <button
            type="button"
            aria-label="Close"
            rel="prev"
            onClick={onClose}
            disabled={submitting}
          />
          <strong id="add-host-title">Add a host</strong>
        </header>

        <form onSubmit={handleSubmit}>
          <label htmlFor="add-host-hostname">
            Hostname
            <input
              id="add-host-hostname"
              ref={firstFieldRef}
              type="text"
              name="hostname"
              placeholder="prod-web-01"
              required
              aria-invalid={hostnameError ? true : undefined}
              aria-describedby={hostnameError ? 'add-host-hostname-error' : undefined}
            />
            {hostnameError && (
              <small id="add-host-hostname-error">{hostnameError}</small>
            )}
          </label>

          <label htmlFor="add-host-ssh-user">
            SSH user <small style={{ opacity: 0.7 }}>(default: root)</small>
            <input
              id="add-host-ssh-user"
              type="text"
              name="ssh_user"
              placeholder="root"
              autoComplete="off"
            />
          </label>

          <small>
            You can attach an SSH private key from the host's detail page after
            it appears in the list.
          </small>

          <footer style={{ marginTop: '1rem' }}>
            <button
              type="button"
              className="secondary"
              onClick={onClose}
              disabled={submitting}
            >
              Cancel
            </button>
            <button type="submit" disabled={submitting} aria-busy={submitting || undefined}>
              {submitting ? 'Adding…' : 'Add host'}
            </button>
          </footer>
        </form>
      </article>
    </dialog>
  );
}
