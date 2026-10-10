import { render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ApprovalCard } from '../../src/chat/ApprovalCard';
import { EgressBadge } from '../../src/chat/EgressBadge';
import { MessageStream } from '../../src/chat/MessageStream';
import { ToolCallCard } from '../../src/chat/ToolCallCard';
import { applyRunInput, EMPTY_CHAT_STATE, foldAguiEvents, toWire, type ChatState } from '../../src/chat/agui';
import { ConfirmationDialog } from '../../src/components/confirmation-dialog/ConfirmationDialog';
import { renderToolResult } from '../../src/components/registry';
import { ToolErrorNotice } from '../../src/components/tool-error/ToolErrorNotice';
import { TOOL_META, type ToolName } from '../../src/contract/generated/tool-meta';
import { UiScope, type UiTheme } from '../../src/scope/UiScope';
import { ALL_TOOLS, OUTPUTS } from '../fixtures/outputs';
import { axeViolations, describeViolations } from '../helpers/axe';
import { readAguiFixtures, readContractJson } from '../helpers/contract';

const THEMES: UiTheme[] = ['dark', 'light'];

async function expectAccessible(node: React.ReactNode, theme: UiTheme, label: string) {
  const { container, unmount } = render(
    <UiScope theme={theme} injectStyles={false}>
      {node}
    </UiScope>,
  );
  const violations = await axeViolations(container);
  expect(violations, `${label} (${theme})\n${describeViolations(violations)}`).toEqual([]);
  unmount();
}

describe.each(THEMES)('axe, WCAG 2.2 AA rules that run without layout: %s theme', (theme) => {
  it('every view for every tool fixture has no violation', async () => {
    for (const tool of ALL_TOOLS) {
      if (TOOL_META[tool].component === null) continue;
      for (const [i, fixture] of OUTPUTS[tool].entries()) {
        const r = renderToolResult(tool as ToolName, fixture, {
          onOpenPath: () => undefined,
          onLoadMore: () => undefined,
          onRequestUnpin: () => undefined,
        });
        if (r.kind !== 'view') throw new Error(`${tool} did not render a view`);
        await expectAccessible(r.node, theme, `${tool}[${i}]`);
      }
    }
  }, 60_000);

  it('the confirmation dialog in each state has no violation', async () => {
    const human = readContractJson<{ state: string }>('confirmation/examples/ConfirmationRequired/human-mcp-with-token.json');
    const nativeCf = readContractJson<{ state: string }>('confirmation/examples/ConfirmationRequired/ui-native-prompt.json');
    const variants = [
      <ConfirmationDialog key="p" confirmation={human as never} onApprove={vi.fn()} onDeny={vi.fn()} />,
      <ConfirmationDialog key="n" confirmation={nativeCf as never} onApprove={vi.fn()} onDeny={vi.fn()} />,
      <ConfirmationDialog key="e" confirmation={human as never} />,
      <ConfirmationDialog key="a" confirmation={human as never} resolution="approved" />,
      <ConfirmationDialog key="d" confirmation={{ ...human, state: 'denied' } as never} />,
      <ConfirmationDialog key="x" confirmation={{ ...human, state: 'expired' } as never} />,
    ];
    for (const v of variants) await expectAccessible(v, theme, `confirmation ${v.key}`);
  });

  it('the chat primitives rendered from every AG-UI fixture have no violation', async () => {
    for (const { name, fixture } of readAguiFixtures()) {
      let state: ChatState = EMPTY_CHAT_STATE;
      if (name === 'write-resume-approved') {
        const first = readAguiFixtures().find((f) => f.name === 'write-interrupt');
        state = foldAguiEvents((first?.fixture.events ?? []) as never, applyRunInput(EMPTY_CHAT_STATE, first?.fixture.input as never));
      }
      const s = foldAguiEvents(fixture.events as never, applyRunInput(state, fixture.input as never));
      const node = (
        <>
          <MessageStream messages={s.messages} />
          {s.toolCalls.map((c) => (
            <ToolCallCard key={c.id} call={toWire(c)} acknowledgedHosts={['model.example.net']} />
          ))}
          {s.interrupts.map((i) => (
            <ApprovalCard key={i.interrupt.id} interrupt={i.interrupt} {...(i.resolution && { resolution: i.resolution })} onApprove={vi.fn()} onDeny={vi.fn()} />
          ))}
        </>
      );
      await expectAccessible(node, theme, `fixture ${name}`);
    }
  });

  it('the egress badge and tool error have no violation', async () => {
    await expectAccessible(
      <>
        <EgressBadge tool="ipfs_cat" acknowledged_hosts={['a.example.net']} />
        <EgressBadge tool="ipfs_cat" acknowledged_hosts={[]} refused />
        <ToolErrorNotice error={{ error: { code: 'daemon_unreachable', message: 'The daemon is not running.', retryable: true, tool: 'pin_ls' } }} />
      </>,
      theme,
      'egress and error',
    );
  });
});
