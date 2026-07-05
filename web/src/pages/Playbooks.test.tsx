import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Playbooks } from './Playbooks';
import { ToastProvider } from '../components/Toast';
import { ConfirmProvider } from '../components/ConfirmDialog';
import * as api from '../api';
import type { Playbook } from '../types';

const PLAYBOOKS: Playbook[] = [
  {
    id: 1,
    name: 'Patch nginx',
    description: 'Update and restart nginx',
    steps: ['apt-get update', 'apt-get install -y nginx', 'systemctl restart nginx'],
    use_sudo: true,
    created_by: 'admin',
    created_at: '2026-07-01T00:00:00Z',
    updated_at: '2026-07-01T00:00:00Z',
  },
];

function renderPage() {
  return render(
    <ToastProvider>
      <ConfirmProvider>
        <Playbooks />
      </ConfirmProvider>
    </ToastProvider>,
  );
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('Playbooks', () => {
  it('lists playbooks with step count and sudo badge', async () => {
    vi.spyOn(api, 'apiGet').mockResolvedValue(PLAYBOOKS as unknown as never);

    renderPage();

    await screen.findByText('Patch nginx');
    expect(screen.getByText('Update and restart nginx')).toBeInTheDocument();
    expect(screen.getByText('3')).toBeInTheDocument(); // step count
    expect(screen.getByText('sudo')).toBeInTheDocument();
  });

  it('creates a playbook from the form, dropping blank step lines', async () => {
    vi.spyOn(api, 'apiGet').mockResolvedValue([] as unknown as never);
    const apiPost = vi.spyOn(api, 'apiPost').mockResolvedValue(PLAYBOOKS[0] as unknown as never);

    renderPage();
    await screen.findByText(/No playbooks yet/i);

    fireEvent.click(screen.getByRole('button', { name: /New Playbook/i }));
    fireEvent.change(screen.getByLabelText(/Name/i), { target: { value: 'Patch nginx' } });
    fireEvent.change(screen.getByLabelText(/Steps/i), {
      target: { value: 'apt-get update\n\n  systemctl restart nginx  \n' },
    });
    fireEvent.click(screen.getByRole('button', { name: /Create playbook/i }));

    await waitFor(() => {
      expect(apiPost).toHaveBeenCalledWith('/api/v1/playbooks', {
        name: 'Patch nginx',
        description: '',
        steps: ['apt-get update', 'systemctl restart nginx'],
        use_sudo: true,
      });
    });
  });

  it('rejects submission without any steps', async () => {
    vi.spyOn(api, 'apiGet').mockResolvedValue([] as unknown as never);
    const apiPost = vi.spyOn(api, 'apiPost');

    renderPage();
    await screen.findByText(/No playbooks yet/i);

    fireEvent.click(screen.getByRole('button', { name: /New Playbook/i }));
    fireEvent.change(screen.getByLabelText(/Name/i), { target: { value: 'Empty' } });
    fireEvent.click(screen.getByRole('button', { name: /Create playbook/i }));

    await screen.findByText(/at least one step/i);
    expect(apiPost).not.toHaveBeenCalled();
  });
});
