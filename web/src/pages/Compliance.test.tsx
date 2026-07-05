import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Compliance } from './Compliance';
import { ToastProvider } from '../components/Toast';
import * as api from '../api';

const ROWS = [
  {
    host_id: 1,
    hostname: 'web-1',
    tags: ['prod'],
    os_version: 'Ubuntu 24.04',
    packages_available: 0,
    reboot_required: false,
    last_seen: '2026-07-05T00:00:00Z',
    offline_since: null,
    last_success_at: '2026-07-04T02:00:00Z',
    last_attempt_at: '2026-07-04T02:00:00Z',
    last_attempt_status: 'succeeded',
  },
  {
    host_id: 2,
    hostname: 'db-1',
    tags: [],
    os_version: 'Ubuntu 22.04',
    packages_available: 7,
    reboot_required: true,
    last_seen: '2026-07-05T00:00:00Z',
    offline_since: '2026-07-05T01:00:00Z',
    last_success_at: null,
    last_attempt_at: '2026-07-04T02:00:00Z',
    last_attempt_status: 'failed',
  },
];

function renderPage() {
  return render(
    <ToastProvider>
      <Compliance />
    </ToastProvider>,
  );
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('Compliance', () => {
  it('renders the report with a patched summary', async () => {
    vi.spyOn(api, 'apiGet').mockResolvedValue(ROWS as unknown as never);

    renderPage();

    await screen.findByText('web-1');
    // 1 of 2 fully patched (db-1 has pending updates + reboot flag).
    expect(screen.getByText(/1 of 2 hosts fully patched/)).toBeInTheDocument();
    expect(screen.getByText('db-1')).toBeInTheDocument();
    expect(screen.getByText('7')).toBeInTheDocument(); // pending count
    expect(screen.getByText(/⟳ required/)).toBeInTheDocument();
    expect(screen.getByText('(failed)')).toBeInTheDocument();
    // Host with no successful update ever.
    expect(screen.getByText('never')).toBeInTheDocument();
  });

  it('shows the empty state without hosts', async () => {
    vi.spyOn(api, 'apiGet').mockResolvedValue([] as unknown as never);

    renderPage();

    await screen.findByText(/No hosts yet/i);
    expect(screen.getByRole('button', { name: /Download CSV/i })).toBeDisabled();
  });
});
