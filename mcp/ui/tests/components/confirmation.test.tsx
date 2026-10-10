import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ConfirmationDialog, ConfirmationDialogDataSchema } from '../../src/components/confirmation-dialog/ConfirmationDialog';
import { ConfirmationRequiredSchema } from '../../src/contract/generated/confirmation';
import { readContractJson, contractAjv } from '../helpers/contract';
import { renderIn } from '../helpers/render';

const human = readContractJson<unknown>('confirmation/examples/ConfirmationRequired/human-mcp-with-token.json');
const nativeCf = readContractJson<unknown>('confirmation/examples/ConfirmationRequired/ui-native-prompt.json');
const a2a = readContractJson<unknown>('confirmation/examples/ConfirmationRequired/a2a-no-token.json');

const statusOf = () => within(screen.getByRole('alertdialog')).getByRole('status');

const withState = (base: unknown, state: 'pending' | 'denied' | 'expired') => ({ ...(base as object), state });

describe('ConfirmationDialog bound to the contract confirmation schema', () => {
  it('accepts the contract examples and rejects a token for an a2a principal', () => {
    expect(ConfirmationRequiredSchema.safeParse(human).success).toBe(true);
    expect(ConfirmationRequiredSchema.safeParse(a2a).success).toBe(true);
    const bad = { ...(a2a as object), confirm_token: 'a'.repeat(40) };
    expect(ConfirmationRequiredSchema.safeParse(bad).success).toBe(false);
    const validate = contractAjv().getSchema('file:///confirmation/confirmation.schema.json#/$defs/ConfirmationRequired');
    expect(validate?.(bad)).toBe(false);
  });

  it('pending and answerable: shows Approve and Deny, focus starts on Deny, names the action', async () => {
    const onApprove = vi.fn();
    const onDeny = vi.fn();
    renderIn(<ConfirmationDialog confirmation={human as never} onApprove={onApprove} onDeny={onDeny} />);
    const dialog = screen.getByRole('alertdialog', { name: /Remove the pin on bafybeigdyrzt5/ });
    expect(dialog).toHaveAttribute('aria-modal', 'false');
    expect(screen.getByText('Awaiting a person')).toBeInTheDocument();
    expect(screen.getByText('Destructive')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Deny' })).toHaveFocus();
    await userEvent.click(screen.getByRole('button', { name: 'Approve removal' }));
    expect(onApprove).toHaveBeenCalledOnce();
    await userEvent.click(screen.getByRole('button', { name: 'Deny' }));
    expect(onDeny).toHaveBeenCalledOnce();
  });

  it('pending but not answerable here: no approve control, says where approval happens', () => {
    renderIn(<ConfirmationDialog confirmation={a2a as never} />);
    expect(screen.queryByRole('button', { name: /Approve/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
    expect(statusOf()).toHaveTextContent(/This panel cannot approve/);
    expect(statusOf()).toHaveTextContent(/IPFS Desktop confirmation window or the macOS system prompt/);
  });

  it('waiting for the system prompt: no approve control even if a handler is passed', () => {
    expect(ConfirmationRequiredSchema.safeParse(nativeCf).success).toBe(true);
    expect((nativeCf as { requires_native_prompt?: boolean }).requires_native_prompt).toBe(true);
    renderIn(<ConfirmationDialog confirmation={nativeCf as never} onApprove={vi.fn()} onDeny={vi.fn()} />);
    expect(screen.getByText('Waiting for system prompt')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Approve/ })).not.toBeInTheDocument();
    expect(statusOf()).toHaveTextContent(/system prompt on this machine/);
    expect(statusOf()).toHaveTextContent(/does not approve/);
    // denial is always safe, so it stays
    expect(screen.getByRole('button', { name: 'Deny' })).toBeInTheDocument();
  });

  it.each([
    ['denied', 'Denied', /Nothing was changed. Deny is final./],
    ['expired', 'Expired', /Expiry is denial/],
  ] as const)('%s: states the outcome and offers no controls', (state, word, text) => {
    renderIn(<ConfirmationDialog confirmation={withState(human, state) as never} onApprove={vi.fn()} onDeny={vi.fn()} />);
    expect(screen.getAllByText(word).length).toBeGreaterThan(0);
    expect(statusOf()).toHaveTextContent(text);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('approved on this surface: says the call runs once and offers no controls', () => {
    renderIn(<ConfirmationDialog confirmation={human as never} resolution="approved" onApprove={vi.fn()} onDeny={vi.fn()} />);
    expect(statusOf()).toHaveTextContent('Approved. The call runs once.');
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('shows the expiry in UTC and the confirmation id', () => {
    renderIn(<ConfirmationDialog confirmation={human as never} />);
    expect(screen.getByText('2026-10-09 12:02:00 UTC')).toBeInTheDocument();
    expect(screen.getByText('cf_3pQm8vK2xR7tLw9a')).toBeInTheDocument();
  });

  it('renders untrusted summary text as text', () => {
    const c = JSON.parse(JSON.stringify(human)) as { summary: { text: string } };
    c.summary.text = 'Remove <img src=x onerror=alert(1)> now ‮';
    renderIn(<ConfirmationDialog confirmation={c as never} />);
    expect(document.querySelector('img')).toBeNull();
    expect(screen.getByRole('alertdialog')).toHaveAccessibleName(expect.stringContaining('\\u{202E}'));
  });

  it('the data schema is strict', () => {
    expect(ConfirmationDialogDataSchema.safeParse({ confirmation: human, surprise: true }).success).toBe(false);
  });
});
