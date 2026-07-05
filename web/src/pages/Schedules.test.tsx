import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Schedules } from './Schedules';
import { ToastProvider } from '../components/Toast';
import { ConfirmProvider } from '../components/ConfirmDialog';
import * as api from '../api';
import type { Schedule } from '../types';

const SCHEDULES: Schedule[] = [
  {
    id: 1,
    name: 'nightly-security',
    host_ids: [1],
    interval_minutes: 1440,
    next_run_at: '2026-07-06T02:00:00Z',
    enabled: true,
    created_by: 'admin',
    created_at: '2026-07-01T00:00:00Z',
    playbook_id: null,
    concurrency: 0,
    canary_count: 0,
    canary_wait_seconds: 0,
    abort_on_failure_pct: 0,
    window_start_minute: 120,
    window_end_minute: 300,
    window_days: 127,
    security_only: true,
  },
];

const HOSTS = [
  {
    id: 1, hostname: 'web-1', ssh_user: 'root', created_at: '', updated_at: '',
    last_seen: '', update_output: '', upgrade_output: '', error: null, tags: [],
    reboot_required: false, packages_updated: 0, packages_available: 0,
    os_version: '', kernel_version: '', agent_version: '', offline_since: null,
  },
];

function mockGets() {
  vi.spyOn(api, 'apiGet').mockImplementation(async (url: string) => {
    if (url === '/api/v1/schedules') return SCHEDULES as unknown as never;
    if (url === '/api/v1/hosts') return HOSTS as unknown as never;
    if (url === '/api/v1/playbooks') return [] as unknown as never;
    throw new Error(`unexpected GET ${url}`);
  });
}

function renderPage() {
  return render(
    <ToastProvider>
      <ConfirmProvider>
        <Schedules />
      </ConfirmProvider>
    </ToastProvider>,
  );
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('Schedules', () => {
  it('renders schedules with window summary and security-only mode', async () => {
    mockGets();
    renderPage();

    await screen.findByText('nightly-security');
    // Maintenance window rendered as HH:MM–HH:MM UTC.
    expect(screen.getByText(/02:00–05:00 UTC/)).toBeInTheDocument();
    // Runs column shows the security mode instead of plain apt upgrade.
    expect(screen.getByText('security updates')).toBeInTheDocument();
  });

  it('rejects a maintenance window with no days selected', async () => {
    mockGets();
    const apiPost = vi.spyOn(api, 'apiPost');
    renderPage();
    await screen.findByText('nightly-security');

    fireEvent.click(screen.getByRole('button', { name: /New Schedule/i }));
    fireEvent.change(screen.getByLabelText(/^Name$/i), { target: { value: 'windowed' } });
    fireEvent.click(screen.getByRole('checkbox', { name: 'web-1' }));
    fireEvent.click(screen.getByRole('switch', { name: /maintenance window/i }));
    // Uncheck all seven days.
    for (const d of ['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa']) {
      fireEvent.click(screen.getByRole('checkbox', { name: d }));
    }
    fireEvent.click(screen.getByRole('button', { name: /Create schedule/i }));

    await waitFor(() => {
      expect(screen.getByText(/at least one day/i)).toBeInTheDocument();
    });
    expect(apiPost).not.toHaveBeenCalled();
  });
});
