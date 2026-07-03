import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AddHostModal } from './AddHostModal';
import { ToastProvider } from './Toast';
import * as api from '../api';
import type { Host } from '../types';

function renderModal(overrides: { open?: boolean } = {}) {
  const onClose = vi.fn();
  const onCreated = vi.fn<(host: Host) => void>();
  render(
    <ToastProvider>
      <AddHostModal open={overrides.open ?? true} onClose={onClose} onCreated={onCreated} />
    </ToastProvider>,
  );
  return { onClose, onCreated };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('AddHostModal', () => {
  it('renders nothing when closed', () => {
    renderModal({ open: false });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('rejects an invalid hostname inline without calling the API', () => {
    const apiPost = vi.spyOn(api, 'apiPost');
    const { onCreated } = renderModal();

    fireEvent.change(screen.getByLabelText(/Hostname/i), { target: { value: '!!bad name' } });
    fireEvent.submit(screen.getByRole('button', { name: 'Add host' }).closest('form')!);

    expect(apiPost).not.toHaveBeenCalled();
    expect(onCreated).not.toHaveBeenCalled();
    expect(screen.getByText(/Use letters, digits, dot, dash, or underscore/i)).toBeInTheDocument();
  });

  it('submits to /api/v1/hosts and notifies the parent on success', async () => {
    const created: Host = {
      id: 42,
      hostname: 'prod-web-01',
      ssh_user: 'root',
      created_at: '2026-04-28T00:00:00Z',
      updated_at: '2026-04-28T00:00:00Z',
      last_seen: '2026-04-28T00:00:00Z',
      update_output: '',
      upgrade_output: '',
      error: null, tags: [], reboot_required: false, packages_updated: 0, packages_available: 0, os_version: "", kernel_version: "", agent_version: "",
    };
    const apiPost = vi.spyOn(api, 'apiPost').mockResolvedValue(created);
    const { onClose, onCreated } = renderModal();

    fireEvent.change(screen.getByLabelText(/Hostname/i), { target: { value: 'prod-web-01' } });
    fireEvent.change(screen.getByLabelText(/SSH user/i), { target: { value: 'ubuntu' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add host' }));

    await waitFor(() => expect(apiPost).toHaveBeenCalledWith('/api/v1/hosts', {
      hostname: 'prod-web-01',
      ssh_user: 'ubuntu',
    }));
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(created));
    expect(onClose).toHaveBeenCalled();
  });

  it('defaults ssh_user to "root" when the field is left blank', async () => {
    const created: Host = {
      id: 1, hostname: 'host', ssh_user: 'root',
      created_at: '', updated_at: '', last_seen: '',
      update_output: '', upgrade_output: '', error: null, tags: [], reboot_required: false, packages_updated: 0, packages_available: 0, os_version: "", kernel_version: "", agent_version: "",
    };
    const apiPost = vi.spyOn(api, 'apiPost').mockResolvedValue(created);
    renderModal();

    fireEvent.change(screen.getByLabelText(/Hostname/i), { target: { value: 'host' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add host' }));

    await waitFor(() => expect(apiPost).toHaveBeenCalledWith('/api/v1/hosts', {
      hostname: 'host',
      ssh_user: 'root',
    }));
  });

  it('keeps the modal open and shows a toast when the API returns an error', async () => {
    vi.spyOn(api, 'apiPost').mockRejectedValue(new Error('A host with that hostname already exists'));
    const { onClose, onCreated } = renderModal();

    fireEvent.change(screen.getByLabelText(/Hostname/i), { target: { value: 'host' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add host' }));

    // The error surfaces in two places: the toast and the inline article in the
    // modal. findAllByText accepts >=1 match.
    const matches = await screen.findAllByText(/A host with that hostname already exists/i);
    expect(matches.length).toBeGreaterThanOrEqual(1);
    expect(onCreated).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });
});
