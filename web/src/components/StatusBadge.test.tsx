import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { StatusBadge } from './StatusBadge';

describe('StatusBadge', () => {
  it('renders the default label for each status', () => {
    const cases = [
      ['online', 'Online'],
      ['offline', 'Offline'],
      ['updating', 'Updating'],
      ['error', 'Error'],
      ['unknown', 'Unknown'],
    ] as const;

    for (const [status, expected] of cases) {
      const { unmount } = render(<StatusBadge status={status} />);
      expect(screen.getByText(expected)).toBeInTheDocument();
      unmount();
    }
  });

  it('overrides the default label when one is provided', () => {
    render(<StatusBadge status="online" label="2m ago" />);
    expect(screen.getByText('2m ago')).toBeInTheDocument();
    expect(screen.queryByText('Online')).not.toBeInTheDocument();
  });

  it('exposes status as a data-status attribute for styling and tests', () => {
    render(<StatusBadge status="error" label="Down" />);
    const badge = screen.getByText('Down');
    expect(badge).toHaveAttribute('data-status', 'error');
  });

  it('puts the title on the element for tooltips', () => {
    render(<StatusBadge status="updating" title="apt upgrade in progress" />);
    expect(screen.getByText('Updating')).toHaveAttribute('title', 'apt upgrade in progress');
  });

  it('uses role="status" so screen readers announce changes', () => {
    render(<StatusBadge status="online" />);
    expect(screen.getByRole('status')).toBeInTheDocument();
  });
});
