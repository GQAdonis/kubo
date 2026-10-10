import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ApprovalCard } from '../../src/chat/ApprovalCard';
import { ConfirmationDialog } from '../../src/components/confirmation-dialog/ConfirmationDialog';
import { PinList } from '../../src/components/pin-list/PinList';
import { renderToolResult } from '../../src/components/registry';
import { TOOL_META, type ToolName } from '../../src/contract/generated/tool-meta';
import { UiScope } from '../../src/scope/UiScope';
import { ALL_TOOLS, CID_A, OUTPUTS } from '../fixtures/outputs';
import { readContractJson } from '../helpers/contract';
import { renderIn } from '../helpers/render';

const human = readContractJson<unknown>('confirmation/examples/ConfirmationRequired/human-mcp-with-token.json');

const FOCUSABLE = 'a[href], button:not([disabled]), summary, input, select, textarea, [tabindex]:not([tabindex="-1"])';

describe('keyboard operation', () => {
  it('confirmation dialog: focus starts on Deny, Tab reaches Approve, Enter and Space activate', async () => {
    const onApprove = vi.fn();
    const onDeny = vi.fn();
    renderIn(<ConfirmationDialog confirmation={human as never} onApprove={onApprove} onDeny={onDeny} />);
    const deny = screen.getByRole('button', { name: 'Deny' });
    const approve = screen.getByRole('button', { name: 'Approve removal' });
    expect(deny).toHaveFocus();
    await userEvent.tab();
    expect(approve).toHaveFocus();
    await userEvent.tab({ shift: true });
    expect(deny).toHaveFocus();
    await userEvent.keyboard('{Enter}');
    expect(onDeny).toHaveBeenCalledTimes(1);
    expect(onApprove).not.toHaveBeenCalled();
    await userEvent.tab();
    await userEvent.keyboard(' ');
    expect(onApprove).toHaveBeenCalledTimes(1);
  });

  it('the safe action is first in the tab order, so Enter on arrival cannot approve', () => {
    renderIn(<ConfirmationDialog confirmation={human as never} onApprove={vi.fn()} onDeny={vi.fn()} />);
    const order = [...screen.getByRole('alertdialog').querySelectorAll('button')].map((b) => b.textContent);
    expect(order).toEqual(['Deny', 'Approve removal']);
  });

  it('chat approval card does not steal focus on mount', () => {
    const interrupt = {
      id: 'int-1',
      reason: 'tool_call' as const,
      message: 'Pin something',
      toolCallId: 'tc',
      metadata: { 'ipfs-mcp': { confirmation_id: 'cf_Zr4Yt1nB6kq0HdUe', tool: 'pin_add', class: 'write' as const, requires_native_prompt: false } },
    };
    renderIn(<ApprovalCard interrupt={interrupt} onApprove={vi.fn()} onDeny={vi.fn()} />);
    expect(document.body).toHaveFocus();
  });

  it('copy button works from the keyboard and announces the result once in the scope live region', async () => {
    const copy = vi.fn().mockResolvedValue(undefined);
    render(
      <UiScope injectStyles={false} actions={{ copy }}>
        <PinList tool="pin_add" output={OUTPUTS.pin_add[0] as never} />
      </UiScope>,
    );
    const button = screen.getByRole('button', { name: /Copy CID bafkreihdw/ });
    await userEvent.tab();
    // first focusable is the copy button for the only CID
    expect(button).toHaveFocus();
    await userEvent.keyboard('{Enter}');
    expect(copy).toHaveBeenCalledWith(expect.stringContaining('bafkrei'));
    await screen.findByText('Copied');
    const live = document.querySelector('[data-ipfs-ui-live]');
    expect(live).toHaveTextContent(/copied/);
  });

  it('reports a failed copy in words, not silence', async () => {
    const copy = vi.fn().mockRejectedValue(new Error('denied'));
    render(
      <UiScope injectStyles={false} actions={{ copy }}>
        <PinList tool="pin_add" output={OUTPUTS.pin_add[0] as never} />
      </UiScope>,
    );
    await userEvent.click(screen.getByRole('button', { name: /Copy CID/ }));
    await screen.findByText('Copy failed');
    expect(document.querySelector('[data-ipfs-ui-live]')).toHaveTextContent(/copy failed/);
  });

  it('wide tables are keyboard-scrollable regions with a name', () => {
    renderIn(<PinList tool="pin_ls" output={OUTPUTS.pin_ls[0] as never} />);
    const region = screen.getByRole('region', { name: 'Pins' });
    expect(region).toHaveAttribute('tabindex', '0');
    region.focus();
    expect(region).toHaveFocus();
  });

  it('Tab order inside a listing follows reading order', async () => {
    const onOpenPath = vi.fn();
    renderToolResult('ipfs_ls', OUTPUTS.ipfs_ls[0], { onOpenPath });
    const r = renderToolResult('ipfs_ls', OUTPUTS.ipfs_ls[0], { onOpenPath, onLoadMore: vi.fn() });
    if (r.kind !== 'view') throw new Error('no view');
    render(<UiScope injectStyles={false}>{r.node}</UiScope>);
    const stops: string[] = [];
    for (let i = 0; i < 12; i++) {
      await userEvent.tab();
      const el = document.activeElement as HTMLElement;
      stops.push(el.getAttribute('aria-label') ?? el.textContent?.trim() ?? '');
    }
    const iDir = stops.findIndex((s) => s.startsWith('images'));
    const iMore = stops.findIndex((s) => s === 'Load more entries');
    expect(iDir).toBeGreaterThan(-1);
    expect(iMore).toBeGreaterThan(iDir);
  });

  it('every focusable element in every view has an accessible name', () => {
    for (const tool of ALL_TOOLS) {
      if (TOOL_META[tool].component === null) continue;
      for (const [i, fixture] of OUTPUTS[tool].entries()) {
        const r = renderToolResult(tool as ToolName, fixture, { onOpenPath: () => undefined, onLoadMore: () => undefined, onRequestUnpin: () => undefined });
        if (r.kind !== 'view') throw new Error('no view');
        const { container, unmount } = render(<UiScope injectStyles={false}>{r.node}</UiScope>);
        for (const el of container.querySelectorAll(FOCUSABLE)) {
          expect(el, `${tool}[${i}] ${el.outerHTML.slice(0, 80)}`).toHaveAccessibleName();
        }
        unmount();
      }
    }
  });

  it('a Request unpin button names the CID it applies to', async () => {
    const onRequestUnpin = vi.fn();
    renderIn(<PinList tool="pin_ls" output={OUTPUTS.pin_ls[0] as never} onRequestUnpin={onRequestUnpin} />);
    const row = screen.getAllByRole('row')[1] as HTMLElement;
    const btn = within(row).getByRole('button', { name: new RegExp(`Request unpin ${CID_A}`) });
    await userEvent.click(btn);
    expect(onRequestUnpin).toHaveBeenCalledWith(CID_A);
  });
});
