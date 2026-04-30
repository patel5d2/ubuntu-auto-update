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
  const [enrolling, setEnrolling] = useState(false);
  const [hostnameError, setHostnameError] = useState('');
  const [submitError, setSubmitError] = useState<{ message: string; hint?: string } | null>(null);
  const toast = useToast();
  const firstFieldRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) {
      setHostnameError('');
      setSubmitError(null);
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
    const password = String(data.get('password') ?? '');

    if (!HOSTNAME_PATTERN.test(hostname)) {
      setHostnameError('Use letters, digits, dot, dash, or underscore.');
      return;
    }
    setHostnameError('');
    setSubmitError(null);
    setSubmitting(true);
    setEnrolling(password !== '');

    try {
      const body: Record<string, string> = {
        hostname,
        ssh_user: sshUser || 'root',
      };
      // When a password is supplied the backend does a one-shot enrollment
      // (password SSH → generate keypair → install pubkey → configure
      // passwordless sudo → store encrypted private key). The password is
      // never persisted; it lives in memory for the length of the call.
      if (password !== '') body.password = password;

      const host = await apiPost<Host>('/api/v1/hosts', body);
      toast.show(
        password !== ''
          ? `Host "${host.hostname}" added and configured.`
          : `Host "${host.hostname}" added.`,
        'success',
      );
      onCreated(host);
      onClose();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to add host.';
      // The backend prefixes auto-enroll failures with "Auto-enrollment failed: ".
      // The bootstrap.classifyAuthErr layer below that returns one of a few
      // common, actionable strings — we hint at the right next step here so
      // operators don't have to guess.
      const lower = message.toLowerCase();
      let hint: string | undefined;
      if (lower.includes('authentication failed')) {
        hint = 'Check the SSH user and password. Confirm the host has PasswordAuthentication yes in /etc/ssh/sshd_config.';
      } else if (lower.includes('connection refused')) {
        hint = 'sshd is not running on port 22. Start it on the host (sudo systemctl start ssh) and try again.';
      } else if (lower.includes('could not reach')) {
        hint = 'The backend container cannot reach this host. Check the IP/hostname and any firewalls between them.';
      } else if (lower.includes('verify passwordless sudo')) {
        hint = 'Sudo did not configure passwordlessly. Either run as root, or pre-configure /etc/sudoers.d/ on the host.';
      }
      setSubmitError({ message, hint });
      toast.show(message, 'error');
    } finally {
      setSubmitting(false);
      setEnrolling(false);
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

          <label htmlFor="add-host-password">
            SSH password <small style={{ opacity: 0.7 }}>(optional — auto-configures the host)</small>
            <input
              id="add-host-password"
              type="password"
              name="password"
              placeholder="Leave blank to paste a key later"
              autoComplete="new-password"
            />
          </label>

          <small>
            With a password we sign in once, generate a fresh SSH key, install
            it on the host, set up passwordless sudo, and store the private key
            encrypted. Your password is used in-memory only and never saved.
            Leave it blank to paste a key manually from the host's detail page.
          </small>

          {submitError && (
            <article style={{ marginTop: '1rem', borderLeft: '4px solid #c0392b', padding: '0.75rem 1rem' }}>
              <strong>Couldn't add host.</strong>
              <div style={{ marginTop: '0.25rem' }}>{submitError.message}</div>
              {submitError.hint && (
                <small style={{ display: 'block', marginTop: '0.25rem', opacity: 0.8 }}>
                  Hint: {submitError.hint}
                </small>
              )}
            </article>
          )}

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
              {enrolling ? 'Configuring host…' : submitting ? 'Adding…' : 'Add host'}
            </button>
          </footer>
        </form>
      </article>
    </dialog>
  );
}
