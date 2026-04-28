import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ConfirmProvider, useConfirm, type ConfirmOptions } from './ConfirmDialog';

function Trigger({ options, onResult }: { options: ConfirmOptions; onResult: (ok: boolean) => void }) {
  const confirm = useConfirm();
  return (
    <button
      onClick={async () => {
        const ok = await confirm(options);
        onResult(ok);
      }}
    >
      ask
    </button>
  );
}

function setup(options: ConfirmOptions) {
  const onResult = vi.fn<(ok: boolean) => void>();
  render(
    <ConfirmProvider>
      <Trigger options={options} onResult={onResult} />
    </ConfirmProvider>,
  );
  fireEvent.click(screen.getByRole('button', { name: 'ask' }));
  return { onResult };
}

describe('ConfirmDialog', () => {
  it('opens with the provided title and message', () => {
    setup({ title: 'Delete host?', message: 'This cannot be undone.' });
    expect(screen.getByText('Delete host?')).toBeInTheDocument();
    expect(screen.getByText('This cannot be undone.')).toBeInTheDocument();
  });

  it('resolves true when Confirm is clicked', async () => {
    const { onResult } = setup({ title: 'Sure?' });
    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));

    // Microtask: let the promise settle.
    await Promise.resolve();
    expect(onResult).toHaveBeenCalledWith(true);
  });

  it('resolves false when Cancel is clicked', async () => {
    const { onResult } = setup({ title: 'Sure?' });
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    await Promise.resolve();
    expect(onResult).toHaveBeenCalledWith(false);
  });

  it('resolves false when Escape is pressed', async () => {
    const { onResult } = setup({ title: 'Sure?' });
    fireEvent.keyDown(window, { key: 'Escape' });

    await Promise.resolve();
    expect(onResult).toHaveBeenCalledWith(false);
  });

  it('uses custom labels when provided', () => {
    setup({ title: 'Run update?', confirmLabel: 'Run it', cancelLabel: 'Not now' });
    expect(screen.getByRole('button', { name: 'Run it' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Not now' })).toBeInTheDocument();
  });

  it('marks the confirm button as destructive when requested', () => {
    setup({ title: 'Delete?', destructive: true, confirmLabel: 'Delete' });
    expect(screen.getByRole('button', { name: 'Delete' })).toHaveAttribute('data-destructive', 'true');
  });

  it('disables Confirm until the typed-confirmation matches', () => {
    setup({
      title: 'Delete prod-web-01?',
      destructive: true,
      requireTypedConfirmation: 'prod-web-01',
      confirmLabel: 'Delete',
    });

    const confirmBtn = screen.getByRole('button', { name: 'Delete' });
    const input = screen.getByRole('textbox');
    expect(confirmBtn).toBeDisabled();

    fireEvent.change(input, { target: { value: 'prod-web-0' } });
    expect(confirmBtn).toBeDisabled();
    expect(input).toHaveAttribute('aria-invalid', 'true');

    fireEvent.change(input, { target: { value: 'prod-web-01' } });
    expect(confirmBtn).toBeEnabled();
    expect(input).toHaveAttribute('aria-invalid', 'false');
  });

  it('throws if useConfirm is called outside the provider', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
    function Bare() {
      useConfirm();
      return null;
    }
    expect(() => render(<Bare />)).toThrow(/useConfirm must be used inside <ConfirmProvider>/);
    spy.mockRestore();
  });
});
