import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { HostDetail } from './HostDetail';
import { ToastProvider } from '../components/Toast';
import { ConfirmProvider } from '../components/ConfirmDialog';
import * as api from '../api';
import type { Host, UpdateRun } from '../types';

const HOST: Host = {
  id: 7,
  hostname: 'phase-c-test',
  ssh_user: 'ubuntu',
  created_at: '2026-04-28T00:00:00Z',
  updated_at: '2026-04-28T00:00:00Z',
  last_seen: '2026-04-28T00:00:00Z',
  update_output: 'Hit:1 archive ok',
  upgrade_output: '0 upgraded, 0 newly installed',
  error: null, tags: [], reboot_required: false, packages_updated: 0, packages_available: 0, os_version: "", kernel_version: "", agent_version: "",
};

const RUNS: UpdateRun[] = [
  {
    id: 101,
    host_id: 7,
    run_group_id: null,
    triggered_by: 'admin',
    kind: 'update',
    status: 'succeeded',
    exit_code: 0,
    started_at: '2026-04-28T11:00:00Z',
    finished_at: '2026-04-28T11:00:30Z',
    output: 'all good',
    error: null,
  },
  {
    id: 102,
    host_id: 7,
    run_group_id: null,
    triggered_by: 'admin',
    kind: 'preview',
    status: 'failed',
    exit_code: 1,
    started_at: '2026-04-28T10:00:00Z',
    finished_at: '2026-04-28T10:00:05Z',
    output: 'oops',
    error: 'apt died',
  },
];

function renderWithRoute() {
  return render(
    <ToastProvider>
      <ConfirmProvider>
        <MemoryRouter initialEntries={['/hosts/7']}>
          <Routes>
            <Route path="/hosts/:hostId" element={<HostDetail />} />
          </Routes>
        </MemoryRouter>
      </ConfirmProvider>
    </ToastProvider>,
  );
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('HostDetail', () => {
  it('loads host + runs and renders the overview tab by default', async () => {
    const apiGet = vi.spyOn(api, 'apiGet').mockImplementation(async (url: string) => {
      if (url === '/api/v1/hosts/7') return HOST as unknown as never;
      if (url.startsWith('/api/v1/hosts/7/runs')) return RUNS as unknown as never;
      throw new Error(`unexpected GET ${url}`);
    });

    renderWithRoute();

    await screen.findByText('phase-c-test');
    expect(apiGet).toHaveBeenCalledWith('/api/v1/hosts/7');
    expect(apiGet).toHaveBeenCalledWith(expect.stringContaining('/api/v1/hosts/7/runs'));

    // Overview is the default tab.
    expect(screen.getByRole('tab', { name: /Overview/ })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByText(/0 upgraded, 0 newly installed/)).toBeInTheDocument();
  });

  it('renders run history when the History tab is selected', async () => {
    vi.spyOn(api, 'apiGet').mockImplementation(async (url: string) => {
      if (url === '/api/v1/hosts/7') return HOST as unknown as never;
      if (url.startsWith('/api/v1/hosts/7/runs')) return RUNS as unknown as never;
      throw new Error(`unexpected GET ${url}`);
    });

    renderWithRoute();
    await screen.findByText('phase-c-test');

    // Click History; the runs table should appear.
    fireEvent.click(screen.getByRole('tab', { name: /History/ }));
    await waitFor(() => {
      expect(screen.getByRole('tab', { name: /History/ })).toHaveAttribute('aria-selected', 'true');
    });

    // Both runs render with their statuses.
    expect(screen.getByText('succeeded')).toBeInTheDocument();
    expect(screen.getByText('failed')).toBeInTheDocument();
    // Triggered-by column.
    const adminCells = screen.getAllByText('admin');
    expect(adminCells.length).toBeGreaterThanOrEqual(2);
  });

  it('shows an empty-state message in History when there are no runs', async () => {
    vi.spyOn(api, 'apiGet').mockImplementation(async (url: string) => {
      if (url === '/api/v1/hosts/7') return HOST as unknown as never;
      if (url.startsWith('/api/v1/hosts/7/runs')) return [] as unknown as never;
      throw new Error(`unexpected GET ${url}`);
    });

    renderWithRoute();
    await screen.findByText('phase-c-test');
    fireEvent.click(screen.getByRole('tab', { name: /History/ }));

    await waitFor(() => {
      expect(screen.getByText(/No runs yet/i)).toBeInTheDocument();
    });
  });

  it('renders the SSH form on the SSH tab with the current ssh_user prefilled', async () => {
    vi.spyOn(api, 'apiGet').mockImplementation(async (url: string) => {
      if (url === '/api/v1/hosts/7') return HOST as unknown as never;
      if (url.startsWith('/api/v1/hosts/7/runs')) return [] as unknown as never;
      throw new Error(`unexpected GET ${url}`);
    });

    renderWithRoute();
    await screen.findByText('phase-c-test');
    fireEvent.click(screen.getByRole('tab', { name: /SSH/ }));

    // Two "SSH User" inputs live on the SSH tab (auto-configure + paste-key);
    // both should reflect the host's current user.
    await waitFor(() => {
      const inputs = screen.getAllByLabelText(/SSH User/i) as HTMLInputElement[];
      expect(inputs.length).toBeGreaterThan(0);
      for (const input of inputs) {
        expect(input).toHaveValue('ubuntu');
      }
    });
  });
});
