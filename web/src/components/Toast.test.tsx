import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ToastProvider, useToast, type ToastKind } from './Toast';

function ShowButton({ message, kind, duration }: { message: string; kind?: ToastKind; duration?: number }) {
  const { show } = useToast();
  return (
    <button onClick={() => show(message, kind, duration)}>fire</button>
  );
}

describe('Toast', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('shows a toast when useToast().show() is called', () => {
    render(
      <ToastProvider>
        <ShowButton message="Saved." kind="success" />
      </ToastProvider>,
    );

    expect(screen.queryByText('Saved.')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'fire' }));

    expect(screen.getByText('Saved.')).toBeInTheDocument();
    const toast = screen.getByText('Saved.').closest('[data-kind]');
    expect(toast).toHaveAttribute('data-kind', 'success');
  });

  it('auto-dismisses after the configured duration', () => {
    render(
      <ToastProvider>
        <ShowButton message="ephemeral" duration={1000} />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'fire' }));
    expect(screen.getByText('ephemeral')).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(1100);
    });
    expect(screen.queryByText('ephemeral')).not.toBeInTheDocument();
  });

  it('keeps the toast on screen when duration is 0', () => {
    render(
      <ToastProvider>
        <ShowButton message="sticky" duration={0} />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'fire' }));
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(screen.getByText('sticky')).toBeInTheDocument();
  });

  it('lets the user dismiss a toast manually', () => {
    render(
      <ToastProvider>
        <ShowButton message="dismiss me" duration={0} />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'fire' }));
    expect(screen.getByText('dismiss me')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Dismiss notification' }));
    expect(screen.queryByText('dismiss me')).not.toBeInTheDocument();
  });

  it('uses aria-live="assertive" for errors and "polite" for everything else', () => {
    render(
      <ToastProvider>
        <ShowButton message="bad" kind="error" duration={0} />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'fire' }));
    const toast = screen.getByText('bad').closest('[data-kind]');
    expect(toast).toHaveAttribute('aria-live', 'assertive');
  });

  it('throws if useToast is called outside the provider', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
    expect(() =>
      render(<ShowButton message="x" />),
    ).toThrow(/useToast must be used inside <ToastProvider>/);
    spy.mockRestore();
  });
});
