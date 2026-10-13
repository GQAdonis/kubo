import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ApprovalCard, approvalModelFromInterrupt, interruptExpired, type ApprovalCardData } from '../../src/chat/ApprovalCard';
import { EgressBadge } from '../../src/chat/EgressBadge';
import { ChatMessageItem, MessageStream } from '../../src/chat/MessageStream';
import { ToolCallCard } from '../../src/chat/ToolCallCard';
import { applyRunInput, EMPTY_CHAT_STATE, foldAguiEvents, RUN_ERROR_CODES, runErrorCopy, toWire, type ChatState } from '../../src/chat/agui';
import type { AguiInterrupt } from '../../src/contract/generated/agui';
import { readAguiFixtures, validateAgui, type AguiFixtureFile } from '../helpers/contract';
import { renderIn } from '../helpers/render';

const fixtures = Object.fromEntries(readAguiFixtures().map((f) => [f.name, f.fixture])) as Record<string, AguiFixtureFile>;
const fx = (name: string): AguiFixtureFile => {
  const f = fixtures[name];
  if (!f) throw new Error(`missing AG-UI fixture ${name}`);
  return f;
};

const run = (name: string, initial: ChatState = EMPTY_CHAT_STATE) =>
  foldAguiEvents(fx(name).events as never, applyRunInput(initial, fx(name).input as never));

const interruptOf = (state: ChatState): AguiInterrupt => {
  const i = state.interrupts[0];
  if (!i) throw new Error('no interrupt in state');
  return i.interrupt;
};

describe('AG-UI fixtures (mcp/contract/fixtures/agui)', () => {
  it('has the fourteen contract fixtures, all valid against agui.schema.json', () => {
    expect(Object.keys(fixtures).sort()).toEqual([
      'destructive-native-prompt',
      'destructive-native-prompt-wait-approved',
      'destructive-native-prompt-wait-denied',
      'destructive-native-prompt-wait-expired',
      'egress-refused',
      'read-pin-ls',
      'run-error-backend-failure',
      'run-error-step-limit',
      'two-calls-native-wait-approved',
      'two-calls-resume-then-native-interrupt',
      'two-calls-write-interrupt',
      'write-interrupt',
      'write-resume-approved',
      'write-resume-denied',
    ]);
    for (const [name, f] of Object.entries(fixtures)) expect(validateAgui(f).errors, name).toBe('');
  });

  it('folds read-pin-ls into messages and a finished tool call', () => {
    const s = run('read-pin-ls');
    expect(s.run.status).toBe('finished');
    expect(s.toolCalls).toHaveLength(1);
    expect(s.toolCalls[0]).toMatchObject({ id: 'tc-1', name: 'pin_ls', status: 'done' });
    // The MESSAGES_SNAPSHOT after TOOL_CALL_END introduces msg-a1, the assistant message that made the
    // call. It has no text, so it is kept for the history and drawn as nothing.
    expect(s.messages.map((m) => [m.id, m.role])).toEqual([
      ['msg-u1', 'user'],
      ['msg-a1', 'assistant'],
      ['msg-a2', 'assistant'],
    ]);
    expect(s.messages[1]?.content).toBe('');
    expect(s.toolCalls[0]?.parentMessageId).toBe('msg-a1');
    expect(s.messages[2]?.content).toBe('You have one recursive pin, named wiki.');
    expect(s.messages[2]?.streaming).toBe(false);
  });

  it('folds the deny resume: denied, and confirmation_declined lands on the original call', () => {
    const first = run('write-interrupt');
    const denied = run('write-resume-denied', first);
    expect(denied.interrupts[0]?.resolution).toBe('denied');
    expect(denied.toolCalls).toHaveLength(1);
    expect(denied.toolCalls[0]).toMatchObject({ id: 'tc-7', status: 'error' });
    expect(denied.toolCalls[0]?.resultText).toContain('confirmation_declined');
    expect(denied.run.status).toBe('finished');
  });

  it('folds each native-prompt observation outcome onto the original call', () => {
    const first = run('destructive-native-prompt');
    const waited = (name: string) => run(name, first);
    // {wait: true} answers nothing: the interrupt stays open until the observation run reports the outcome.
    expect(applyRunInput(first, fx('destructive-native-prompt-wait-approved').input as never).interrupts[0]?.resolution).toBeUndefined();
    expect(waited('destructive-native-prompt-wait-approved').interrupts[0]?.resolution).toBe('approved');
    expect(waited('destructive-native-prompt-wait-denied').interrupts[0]?.resolution).toBe('denied');
    expect(waited('destructive-native-prompt-wait-expired').interrupts[0]?.resolution).toBe('expired');
    expect(waited('destructive-native-prompt-wait-approved').toolCalls[0]).toMatchObject({ id: 'tc-3', status: 'done' });
    for (const name of ['destructive-native-prompt-wait-denied', 'destructive-native-prompt-wait-expired']) {
      const s = waited(name);
      expect(s.toolCalls, name).toHaveLength(1);
      expect(s.toolCalls[0], name).toMatchObject({ id: 'tc-3', status: 'error' });
    }
    expect(waited('destructive-native-prompt-wait-denied').toolCalls[0]?.resultText).toContain('confirmation_declined');
    expect(waited('destructive-native-prompt-wait-expired').toolCalls[0]?.resultText).toContain('confirmation_expired');
  });

  it('folds write-interrupt into an interrupted call and a pending interrupt, with nothing run', () => {
    const s = run('write-interrupt');
    expect(s.run.status).toBe('interrupted');
    expect(s.toolCalls[0]).toMatchObject({ name: 'pin_add', status: 'interrupted' });
    expect(s.toolCalls[0]?.resultText).toBeUndefined();
    expect(s.interrupts).toHaveLength(1);
    expect(s.interrupts[0]?.resolution).toBeUndefined();
  });

  it('resumes: the answer marks the interrupt approved and the result lands on the original call', () => {
    const first = run('write-interrupt');
    const resumed = run('write-resume-approved', first);
    expect(resumed.interrupts[0]?.resolution).toBe('approved');
    expect(resumed.toolCalls).toHaveLength(1);
    expect(resumed.toolCalls[0]).toMatchObject({ id: 'tc-7', status: 'done' });
    expect(resumed.toolCalls[0]?.resultText).toContain('bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku');
  });

  it('treats cancelled and approved:false as denial', () => {
    const first = run('write-interrupt');
    const id = interruptOf(first).id;
    expect(applyRunInput(first, { resume: [{ interruptId: id, status: 'cancelled' }] }).interrupts[0]?.resolution).toBe('denied');
    expect(applyRunInput(first, { resume: [{ interruptId: id, status: 'resolved', payload: { approved: false } }] }).interrupts[0]?.resolution).toBe('denied');
  });

  it('ignores unknown events and malformed interrupts instead of rendering them', () => {
    const s = foldAguiEvents([
      { type: 'SOMETHING_NEW', x: 1 },
      { type: 'RUN_FINISHED', outcome: { type: 'interrupt', interrupts: [{ id: 'x', reason: 'tool_call', message: 'no metadata' }] } },
    ]);
    expect(s.interrupts).toHaveLength(0);
    expect(s.run.status).toBe('finished');
  });

  it('folds run-error-step-limit: the call is reported, then the run ends in error by code', () => {
    const s = run('run-error-step-limit');
    expect(s.toolCalls[0]).toMatchObject({ id: 'tc-1', name: 'pin_ls', status: 'done' });
    expect(s.run).toMatchObject({ status: 'error', errorCode: 'max_steps', error: runErrorCopy('max_steps').body });
  });

  it('folds run-error-backend-failure without reading the wire message', () => {
    const s = run('run-error-backend-failure');
    expect(s.run.status).toBe('error');
    expect(s.run.errorCode).toBe('backend_error');
    expect(s.run.error).toBe(runErrorCopy('backend_error').body);
    expect(s.run.error).not.toContain('HTTP 500');
  });

  it('has fixed, distinct, non-empty copy for every RunErrorCode, and generic copy for the rest', () => {
    const bodies = RUN_ERROR_CODES.map((c) => runErrorCopy(c).body);
    expect(new Set(bodies).size).toBe(RUN_ERROR_CODES.length);
    for (const c of RUN_ERROR_CODES) expect(runErrorCopy(c).label).not.toBe('');
    expect(bodies).not.toContain(runErrorCopy(undefined).body);
    const unknown = foldAguiEvents([{ type: 'RUN_ERROR', message: 'leaky backend text', code: 'brand_new' }]);
    expect(unknown.run).toEqual({ status: 'error', error: runErrorCopy(undefined).body });
  });

  it('folds two-calls-native-wait-approved: the result lands on tc-8 and the run finishes', () => {
    const first = run('two-calls-write-interrupt');
    const second = run('two-calls-resume-then-native-interrupt', first);
    const third = run('two-calls-native-wait-approved', second);
    expect(third.toolCalls.map((c) => [c.id, c.status])).toEqual([['tc-7', 'done'], ['tc-8', 'done']]);
    expect(third.interrupts.map((i) => i.resolution)).toEqual(['approved', 'approved']);
    expect(third.messages.at(-1)?.content).toBe('Pinned it, and garbage collection removed 42 blocks.');
    expect(third.run.status).toBe('finished');
  });
});

describe('MessageStream', () => {
  it('renders the fixture conversation as a labelled log of plain text', () => {
    const s = run('read-pin-ls');
    renderIn(<MessageStream messages={s.messages} />);
    const log = screen.getByRole('log', { name: 'Conversation' });
    expect(log).toHaveAttribute('aria-live', 'polite');
    expect(within(log).getByText('You')).toBeInTheDocument();
    expect(within(log).getByText('Agent')).toBeInTheDocument();
    expect(within(log).getByText('You have one recursive pin, named wiki.')).toBeInTheDocument();
  });

  it('never interprets model output as markup and hides tool messages', () => {
    renderIn(
      <MessageStream
        messages={[
          { id: 'a', role: 'assistant', content: '<img src=x onerror=alert(1)> **not bold**' },
          { id: 't', role: 'tool', content: '{"secret":"x"}' },
        ]}
      />,
    );
    expect(document.querySelector('img')).toBeNull();
    expect(screen.getByText(/\*\*not bold\*\*/)).toBeInTheDocument();
    expect(screen.queryByText(/secret/)).not.toBeInTheDocument();
  });

  it('marks the log busy while a message is streaming', () => {
    renderIn(<MessageStream messages={[{ id: 'a', role: 'assistant', content: 'Pinning', streaming: true }]} />);
    expect(screen.getByRole('log')).toHaveAttribute('aria-busy', 'true');
    expect(screen.getByText('Writing')).toBeInTheDocument();
  });
});

describe('ChatMessageItem', () => {
  it('draws one message among a host\'s own transcript items, with no log of its own', () => {
    renderIn(
      <div role="log" aria-label="Host transcript">
        <ChatMessageItem message={{ id: 'u', role: 'user', content: 'Pin it' }} />
        <ChatMessageItem message={{ id: 'a', role: 'assistant', content: 'Pinning', streaming: true }} />
      </div>,
    );
    expect(screen.getAllByRole('log')).toHaveLength(1);
    expect(screen.getByText('You')).toBeInTheDocument();
    expect(screen.getByText('Writing')).toBeInTheDocument();
    expect(document.querySelectorAll('div.iu-msg')).toHaveLength(2);
  });

  it('is the element MessageStream draws, and renders nothing for a tool message', () => {
    renderIn(<MessageStream messages={[{ id: 'a', role: 'assistant', content: 'Hi' }]} />);
    expect(document.querySelectorAll('li.iu-msg.iu-msg--assistant')).toHaveLength(1);
    const { container } = renderIn(<ChatMessageItem message={{ id: 't', role: 'tool', content: '{"x":1}' }} />);
    expect(container.querySelector('.iu-msg')).toBeNull();
  });
});

describe('ToolCallCard', () => {
  it('embeds the bound view for a finished read call', () => {
    const call = toWire(run('read-pin-ls').toolCalls[0] as never);
    renderIn(<ToolCallCard call={call} acknowledgedHosts={['model.example.net']} />);
    expect(screen.getByRole('article', { name: 'Tool call pin_ls' })).toBeInTheDocument();
    expect(screen.getByText('Read only')).toBeInTheDocument();
    expect(screen.getByText('Done')).toBeInTheDocument();
    expect(screen.getByRole('table', { name: 'Pins' })).toBeInTheDocument();
    // pin_ls carries node content: the egress badge names the acknowledged host
    expect(screen.getByText(/model\.example\.net/)).toBeInTheDocument();
  });

  it('shows arguments as a disclosure and keeps them as data', () => {
    const call = toWire(run('read-pin-ls').toolCalls[0] as never);
    renderIn(<ToolCallCard call={call} />);
    const args = screen.getByText('Arguments');
    expect(args.closest('details')).not.toHaveAttribute('open');
    expect(screen.getByRole('group', { name: 'Arguments of pin_ls' })).toBeInTheDocument();
  });

  it('says nothing has run for an interrupted write', () => {
    const call = toWire(run('write-interrupt').toolCalls[0] as never);
    renderIn(<ToolCallCard call={call} />);
    expect(screen.getByText('Waiting for approval')).toBeInTheDocument();
    expect(screen.getByText('Write')).toBeInTheDocument();
    expect(screen.getByText(/Nothing has run/)).toBeInTheDocument();
  });

  it('shows the resumed write result through the pin view', () => {
    const resumed = run('write-resume-approved', run('write-interrupt'));
    renderIn(<ToolCallCard call={toWire(resumed.toolCalls[0] as never)} />);
    expect(screen.getByText('Done')).toBeInTheDocument();
    expect(screen.getByText('Pinned')).toBeInTheDocument();
  });

  it('shows the egress refusal as a failure with the refused badge', () => {
    const s = run('egress-refused');
    renderIn(<ToolCallCard call={toWire(s.toolCalls[0] as never)} />);
    expect(screen.getByText('Failed')).toBeInTheDocument();
    expect(screen.getByText(/model\.example\.net is not in egress_ack/)).toBeInTheDocument();
    expect(screen.getByText('Content not sent')).toBeInTheDocument();
    expect(screen.getByText('egress_not_acknowledged')).toBeInTheDocument();
  });

  it('refuses to draw a result the contract does not describe, and shows it as data', () => {
    renderIn(
      <ToolCallCard
        call={{ id: 'x', name: 'pin_ls', arguments: '{}', status: 'done', result: '{"pins":[{"cid":"bafy","type":"sideways"}],"page":{"has_more":false}}' }}
      />,
    );
    expect(screen.getByText('Result does not match the contract.')).toBeInTheDocument();
    expect(screen.queryByRole('table', { name: 'Pins' })).not.toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'Raw result of pin_ls' })).toBeInTheDocument();
  });

  it('shows a confirmation_required result as the approval panel, not as a failure', () => {
    const result = JSON.stringify({
      error: {
        code: 'confirmation_required',
        message: 'Confirmation required.',
        retryable: false,
        tool: 'pin_rm',
        confirmation: {
          confirmation_id: 'cf_3pQm8vK2xR7tLw9a',
          state: 'pending',
          summary: { tool: 'pin_rm', class: 'destructive', text: 'Remove the pin on bafy (recursive=true)', targets: [{ kind: 'cid', value: 'bafy' }] },
          expires_at: '2026-10-09T12:02:00Z',
          resolvers: ['desktop'],
        },
      },
    });
    renderIn(<ToolCallCard call={{ id: 'x', name: 'pin_rm', arguments: '{}', status: 'error', result }} />);
    expect(document.querySelector('.iu-approval')).toHaveAttribute('role', 'group');
    expect(screen.queryByText('Tool error')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Approve/ })).not.toBeInTheDocument();
  });

  it('labels an unknown tool and shows its result as data', () => {
    renderIn(<ToolCallCard call={{ id: 'x', name: 'mystery_tool', arguments: 'not json', status: 'done', result: '{"a":1}' }} />);
    expect(screen.getByText('Unknown tool')).toBeInTheDocument();
    expect(screen.getByLabelText('Arguments text')).toHaveTextContent('not json');
  });

  it('reports an unreadable result in place', () => {
    renderIn(<ToolCallCard call={{ id: 'x', name: 'pin_ls', arguments: '{}', status: 'done', result: 'oops' }} />);
    expect(screen.getByText('Unreadable result.')).toBeInTheDocument();
  });
});

describe('ApprovalCard', () => {
  const pending = () => {
    const i = interruptOf(run('write-interrupt'));
    return { interrupt: i } satisfies ApprovalCardData;
  };
  const native = () => ({ interrupt: interruptOf(run('destructive-native-prompt')) }) satisfies ApprovalCardData;

  it('pending: names the action, and approve and deny work', async () => {
    const onApprove = vi.fn();
    const onDeny = vi.fn();
    renderIn(<ApprovalCard {...pending()} onApprove={onApprove} onDeny={onDeny} />);
    const card = screen.getByRole('group', { name: /Pin bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku \(recursive=true\)/ });
    expect(within(card).getByText('Awaiting a person')).toBeInTheDocument();
    expect(within(card).getByText('Write')).toBeInTheDocument();
    await userEvent.click(within(card).getByRole('button', { name: 'Approve pin' }));
    expect(onApprove).toHaveBeenCalledOnce();
    await userEvent.click(within(card).getByRole('button', { name: 'Deny' }));
    expect(onDeny).toHaveBeenCalledOnce();
  });

  it('pending with no handlers: no controls, and the text says this panel cannot approve', () => {
    renderIn(<ApprovalCard {...pending()} />);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
    expect(within(screen.getByRole('group', { name: /Pin bafk/ })).getByRole('status')).toHaveTextContent(/This panel cannot approve/);
  });

  it('waiting for the system prompt: no approve control, the text names the system prompt', () => {
    renderIn(<ApprovalCard {...native()} onApprove={vi.fn()} onDeny={vi.fn()} />);
    const card = screen.getByRole('group', { name: /Run garbage collection on the repo/ });
    expect(within(card).getByText('Waiting for system prompt')).toBeInTheDocument();
    expect(within(card).getByText('Destructive')).toBeInTheDocument();
    expect(within(card).queryByRole('button', { name: /Approve/ })).not.toBeInTheDocument();
    expect(within(card).getByRole('status')).toHaveTextContent(/system prompt on this machine, not in this panel/);
    expect(within(card).getByRole('button', { name: 'Deny' })).toBeInTheDocument();
  });

  it('shows the server message as sent: it already says to confirm in the system prompt, and no approve text is stripped or added', () => {
    expect(native().interrupt.message).toBe('Run garbage collection on the repo. Confirm in the system prompt; you can deny here.');
    expect(native().interrupt.message).not.toMatch(/Approve here/);
    const model = approvalModelFromInterrupt(native());
    expect(model.summary).toBe(native().interrupt.message);
    expect(model.phase).toBe('waiting_system_prompt');
    renderIn(<ApprovalCard {...native()} />);
    expect(screen.getByRole('heading', { name: /Confirm in the system prompt; you can deny here\./ })).toBeInTheDocument();
    expect(screen.queryByText(/Approve here/)).not.toBeInTheDocument();
  });

  it('a native-prompt call stays waiting until the outcome the system prompt produced is known', () => {
    expect(approvalModelFromInterrupt(native()).phase).toBe('waiting_system_prompt');
    // The outcome comes from the observation run's tool result (resolution), never from a click in the card.
    expect(approvalModelFromInterrupt({ ...native(), resolution: 'approved' }).phase).toBe('approved');
    expect(approvalModelFromInterrupt({ ...native(), resolution: 'denied' }).phase).toBe('denied');
    expect(approvalModelFromInterrupt({ ...native(), resolution: 'expired' }).phase).toBe('expired');
  });

  it('renders a server-reported expiry as Expired, not Denied', () => {
    renderIn(<ApprovalCard {...native()} resolution="expired" />);
    const card = screen.getByRole('group', { name: /Run garbage collection/ });
    expect(card).toHaveAttribute('data-phase', 'expired');
    expect(within(card).getByRole('status')).toHaveTextContent(/Expiry is denial/);
    expect(within(card).queryByRole('button')).not.toBeInTheDocument();
  });

  it('hands the click to its handlers, so a host can require a real press (isTrusted)', async () => {
    const onApprove = vi.fn();
    const onDeny = vi.fn();
    renderIn(<ApprovalCard {...pending()} onApprove={onApprove} onDeny={onDeny} />);
    await userEvent.click(screen.getByRole('button', { name: 'Deny' }));
    const activation = onDeny.mock.calls[0]?.[0] as { isTrusted: boolean; type: string; currentTarget: HTMLElement };
    expect(activation.type).toBe('click');
    expect(activation.isTrusted).toBe(false); // jsdom: every event is script-made, as a scripted click would be
    screen.getByRole('button', { name: 'Approve pin' }).click();
    expect(onApprove.mock.calls[0]?.[0]).toMatchObject({ type: 'click', isTrusted: false });
  });

  it('puts focus on Deny when asked (focusSafeAction), and leaves focus alone otherwise', () => {
    const a = renderIn(<ApprovalCard {...pending()} focusSafeAction onApprove={vi.fn()} onDeny={vi.fn()} />);
    expect(screen.getByRole('button', { name: 'Deny' })).toHaveFocus();
    a.unmount();
    renderIn(<ApprovalCard {...pending()} onApprove={vi.fn()} onDeny={vi.fn()} />);
    expect(screen.getByRole('button', { name: 'Deny' })).not.toHaveFocus();
  });

  it('approved, denied and expired are final and offer no controls', () => {
    const cases = [
      ['approved', { resolution: 'approved' as const }, 'Approved', /The call runs once/],
      ['denied', { resolution: 'denied' as const }, 'Denied', /Deny is final/],
      ['expired', { expired: true }, 'Expired', /Expiry is denial/],
    ] as const;
    for (const [phase, extra, word, text] of cases) {
      const { unmount } = renderIn(<ApprovalCard {...pending()} {...extra} onApprove={vi.fn()} onDeny={vi.fn()} />);
      const card = screen.getByRole('group', { name: /Pin bafk/ });
      expect(card).toHaveAttribute('data-phase', phase);
      expect(within(card).getAllByText(word).length).toBeGreaterThan(0);
      expect(within(card).getByRole('status')).toHaveTextContent(text);
      expect(within(card).queryByRole('button')).not.toBeInTheDocument();
      unmount();
    }
  });

  it('computes expiry from a clock the host supplies', () => {
    const i = pending().interrupt;
    expect(interruptExpired(i, '2026-10-09T12:01:59Z')).toBe(false);
    expect(interruptExpired(i, '2026-10-09T12:02:00Z')).toBe(true);
  });

  it('keeps every fixture interrupt rendering without an approve control unless a handler is given', () => {
    for (const name of ['write-interrupt', 'destructive-native-prompt']) {
      const { unmount } = renderIn(<ApprovalCard interrupt={interruptOf(run(name))} />);
      expect(screen.queryByRole('button', { name: /Approve/ })).not.toBeInTheDocument();
      unmount();
    }
  });
});

describe('EgressBadge', () => {
  it('renders nothing for a tool that carries no node content', () => {
    const { container } = renderIn(<EgressBadge tool="pin_add" acknowledged_hosts={['model.example.net']} />);
    expect(container.querySelector('.iu-egress')).toBeNull();
  });

  it('names the acknowledged hosts that may receive the result', () => {
    renderIn(<EgressBadge tool="ipfs_cat" acknowledged_hosts={['model.example.net', 'embed.example.net']} />);
    expect(screen.getByText('Carries node content')).toBeInTheDocument();
    expect(screen.getByText(/model\.example\.net, embed\.example\.net/)).toBeInTheDocument();
  });

  it('says plainly when no host is acknowledged, and when content was refused', () => {
    const a = renderIn(<EgressBadge tool="ipfs_cat" acknowledged_hosts={[]} />);
    expect(screen.getByText(/No host is acknowledged/)).toBeInTheDocument();
    a.unmount();
    renderIn(<EgressBadge tool="ipfs_cat" acknowledged_hosts={['other.example.net']} refused />);
    expect(screen.getByText('Content not sent')).toBeInTheDocument();
    expect(screen.getByText(/stayed on this machine/)).toBeInTheDocument();
  });
});
