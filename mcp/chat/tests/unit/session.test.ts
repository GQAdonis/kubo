// @vitest-environment node
import type { AguiEvent } from '@gqadonis/ipfs-mcp-ui';
import { describe, expect, it, vi } from 'vitest';
import type { AguiClient } from '../../src/agui/client';
import { AguiError } from '../../src/agui/client';
import type { RunAgentInput } from '../../src/agui/wire';
import { createChatSession, type SessionIds } from '../../src/island/session';

// Session behavior that the recorded fixtures do not cover: one interrupt per run, expiry, stop,
// failure, and the refusals that keep the chat from sending when it must not.

const ids = (): SessionIds => {
  let n = 0;
  return { thread: () => 'thread-1', run: () => `run-${++n}`, message: () => `msg-u${n}` };
};

const interrupt = (n: number, extra: { native?: boolean; expiresAt?: string } = {}) => ({
  id: `int-cf_Aaaaaaaaaaaaaaa${n}`,
  reason: 'tool_call',
  message: `Pin item ${n}`,
  toolCallId: `tc-${n}`,
  ...(extra.expiresAt && { expiresAt: extra.expiresAt }),
  metadata: {
    'ipfs-mcp': { confirmation_id: `cf_Aaaaaaaaaaaaaaa${n}`, tool: 'pin_add', class: 'write', requires_native_prompt: extra.native ?? false },
  },
});

const interruptedRun = (interrupts: unknown[]): AguiEvent[] => [
  { type: 'RUN_STARTED', threadId: 'thread-1', runId: 'run-1' },
  ...interrupts.map((_, i) => ({ type: 'TOOL_CALL_START', toolCallId: `tc-${i + 1}`, toolCallName: 'pin_add', parentMessageId: 'msg-a1' })),
  ...interrupts.map((_, i) => ({ type: 'TOOL_CALL_ARGS', toolCallId: `tc-${i + 1}`, delta: '{}' })),
  { type: 'RUN_FINISHED', threadId: 'thread-1', runId: 'run-1', outcome: { type: 'interrupt', interrupts } },
];

/** 'hold' is a run that starts and then waits, like a system prompt nobody has answered, until it is aborted. */
type Script = AguiEvent[] | 'hold';

function scripted(runs: Script[]): { client: AguiClient; inputs: RunAgentInput[]; signals: AbortSignal[] } {
  const inputs: RunAgentInput[] = [];
  const signals: AbortSignal[] = [];
  const queue = [...runs];
  return {
    inputs,
    signals,
    client: {
      async *run(input, signal) {
        inputs.push(input);
        if (signal) signals.push(signal);
        const next = queue.shift() ?? [];
        if (next === 'hold') {
          yield { type: 'RUN_STARTED', threadId: input.threadId, runId: input.runId } as AguiEvent;
          await new Promise<void>((_, reject) => signal?.addEventListener('abort', () => reject(new AguiError('aborted', 'Stopped.'))));
          return;
        }
        for (const e of next) yield e;
      },
    },
  };
}

const finished = (runId: string, extra: AguiEvent[] = []): AguiEvent[] => [
  { type: 'RUN_STARTED', threadId: 'thread-1', runId },
  ...extra,
  { type: 'RUN_FINISHED', threadId: 'thread-1', runId, outcome: { type: 'success' } },
];
const result = (callId: string, content: string): AguiEvent => ({ type: 'TOOL_CALL_RESULT', messageId: `msg-t-${callId}`, toolCallId: callId, role: 'tool', content });
const declined = (callId: string, tool = 'pin_add') => result(callId, JSON.stringify({ error: { code: 'confirmation_declined', message: 'Declined.', retryable: false, tool } }));
const expiredResult = (callId: string, tool = 'pin_add') => result(callId, JSON.stringify({ error: { code: 'confirmation_expired', message: 'Expired.', retryable: true, tool } }));

describe('chat session', () => {
  it('sends nothing when there is no client', async () => {
    const s = createChatSession({ client: null, ids: ids() });
    expect(await s.send('hello')).toEqual({ ok: false, reason: 'not_configured' });
    expect(await s.answer('x', 'deny')).toEqual({ ok: false, reason: 'not_configured' });
  });

  it('ignores an empty message', async () => {
    const { client, inputs } = scripted([]);
    const s = createChatSession({ client, ids: ids() });
    expect(await s.send('   ')).toEqual({ ok: false, reason: 'empty' });
    expect(inputs).toHaveLength(0);
  });

  it('refuses a new message while an approval is waiting', async () => {
    const { client, inputs } = scripted([interruptedRun([interrupt(1)])]);
    const s = createChatSession({ client, ids: ids() });
    await s.send('pin it');
    expect(await s.send('and another')).toEqual({ ok: false, reason: 'pending_interrupt' });
    expect(inputs).toHaveLength(1);
  });

  it('sends exactly one resume entry per answer; the queued second call arrives as the next run\'s interrupt', async () => {
    const second = [{ type: 'RUN_STARTED', threadId: 'thread-1', runId: 'run-2' }, { type: 'RUN_FINISHED', threadId: 'thread-1', runId: 'run-2', outcome: { type: 'interrupt', interrupts: [interrupt(2)] } }] as AguiEvent[];
    const { client, inputs } = scripted([interruptedRun([interrupt(1)]), second, [{ type: 'RUN_STARTED', runId: 'run-3', threadId: 'thread-1' }, { type: 'RUN_FINISHED', threadId: 'thread-1', runId: 'run-3', outcome: { type: 'success' } }]]);
    const s = createChatSession({ client, ids: ids() });
    await s.send('pin both');
    const a = s.getSnapshot().chat.interrupts[0]?.interrupt.id ?? '';
    expect(await s.answer(a, 'approve')).toEqual({ ok: true });
    expect(inputs).toHaveLength(2);
    expect(inputs[1]?.resume).toEqual([{ interruptId: a, status: 'resolved', payload: { approved: true } }]);
    const b = interrupt(2).id;
    expect(await s.answer(b, 'deny')).toEqual({ ok: true });
    expect(inputs).toHaveLength(3);
    expect(inputs[2]?.resume).toEqual([{ interruptId: b, status: 'resolved', payload: { approved: false } }]);
    for (const i of inputs) expect(i.resume?.length ?? 0).toBeLessThanOrEqual(1);
  });

  it('refuses to answer the same interrupt twice', async () => {
    const { client } = scripted([interruptedRun([interrupt(1)]), []]);
    const s = createChatSession({ client, ids: ids() });
    await s.send('pin');
    const id = s.getSnapshot().chat.interrupts[0]?.interrupt.id ?? '';
    await s.answer(id, 'deny');
    expect(await s.answer(id, 'approve')).toEqual({ ok: false, reason: 'already_answered' });
  });

  it('refuses an unknown interrupt id', async () => {
    const { client } = scripted([interruptedRun([interrupt(1)])]);
    const s = createChatSession({ client, ids: ids() });
    await s.send('pin');
    expect(await s.answer('int-nope', 'approve')).toEqual({ ok: false, reason: 'unknown_interrupt' });
  });

  describe('native-prompt interrupts', () => {
    const nativeId = (n: number) => interrupt(n, { native: true }).id;

    it('sends {wait: true} by itself once the card is showing, and answers nothing', async () => {
      const { client, inputs } = scripted([interruptedRun([interrupt(1, { native: true })]), 'hold']);
      const s = createChatSession({ client, ids: ids() });
      const sending = s.send('run gc');
      await vi.waitFor(() => expect(inputs).toHaveLength(2));
      expect(inputs[1]?.resume).toEqual([{ interruptId: nativeId(1), status: 'resolved', payload: { wait: true } }]);
      expect(s.getSnapshot().observing).toEqual([nativeId(1)]);
      expect(s.getSnapshot().chat.interrupts[0]?.resolution).toBeUndefined(); // a wait is not an answer
      s.stop();
      await sending;
    });

    it('never approves, in any state, and says why', async () => {
      const { client, inputs } = scripted([interruptedRun([interrupt(1, { native: true })]), 'hold']);
      const s = createChatSession({ client, ids: ids() });
      const sending = s.send('run gc');
      await vi.waitFor(() => expect(s.getSnapshot().observing).toHaveLength(1));
      expect(await s.answer(nativeId(1), 'approve')).toEqual({ ok: false, reason: 'native_prompt' });
      s.stop();
      await sending;
      expect(await s.answer(nativeId(1), 'approve')).toEqual({ ok: false, reason: 'native_prompt' });
      expect(inputs).toHaveLength(2); // the first run and the wait; no approval ever went out
      expect(JSON.stringify(inputs)).not.toContain('"approved":true');
    });

    it('a denial in the chat stops the watch, then is sent as resolved with approved: false', async () => {
      const { client, inputs, signals } = scripted([interruptedRun([interrupt(1, { native: true })]), 'hold', finished('run-3', [declined('tc-1')])]);
      const s = createChatSession({ client, ids: ids() });
      const sending = s.send('run gc');
      await vi.waitFor(() => expect(s.getSnapshot().observing).toHaveLength(1));

      expect(await s.answer(nativeId(1), 'deny')).toEqual({ ok: true });
      await sending;
      expect(signals[1]?.aborted).toBe(true);
      expect(inputs).toHaveLength(3);
      expect(inputs[2]?.resume).toEqual([{ interruptId: nativeId(1), status: 'resolved', payload: { approved: false } }]);
      const snap = s.getSnapshot();
      expect(snap.error).toBeUndefined(); // stopping the watch is not a failure
      expect(snap.running).toBe(false);
      expect(snap.chat.interrupts[0]?.resolution).toBe('denied');
      expect(snap.chat.run.status).toBe('finished');
    });

    it('takes the outcome from the observation: approved, declined and expired end the card without a click', async () => {
      const cases = [
        ['approved', result('tc-1', '{"removed_count":42}')],
        ['denied', declined('tc-1')],
        ['expired', expiredResult('tc-1')],
      ] as const;
      for (const [resolution, event] of cases) {
        const { client } = scripted([interruptedRun([interrupt(1, { native: true })]), finished('run-2', [event])]);
        const s = createChatSession({ client, ids: ids() });
        await s.send('run gc');
        expect(s.getSnapshot().chat.interrupts[0]?.resolution, resolution).toBe(resolution);
        // Nothing is left waiting, so the person can write again.
        expect(await s.send('again'), resolution).toEqual({ ok: true });
      }
    });

    it('a write approval resumes with that one entry only; the native interrupt that follows is watched in its own run', async () => {
      const afterWrite = [
        { type: 'RUN_STARTED', threadId: 'thread-1', runId: 'run-2' },
        { type: 'RUN_FINISHED', threadId: 'thread-1', runId: 'run-2', outcome: { type: 'interrupt', interrupts: [interrupt(2, { native: true })] } },
      ] as AguiEvent[];
      const { client, inputs } = scripted([interruptedRun([interrupt(1)]), afterWrite, finished('run-3', [result('tc-2', '{"removed_count":1}')])]);
      const s = createChatSession({ client, ids: ids() });
      await s.send('do both');
      expect(inputs).toHaveLength(1); // the write is waiting for a person: no watch yet
      expect(await s.answer(interrupt(1).id, 'approve')).toEqual({ ok: true });
      expect(inputs[1]?.resume).toEqual([{ interruptId: interrupt(1).id, status: 'resolved', payload: { approved: true } }]);
      expect(inputs[2]?.resume).toEqual([{ interruptId: nativeId(2), status: 'resolved', payload: { wait: true } }]);
      expect(s.getSnapshot().chat.interrupts.map((i) => i.resolution)).toEqual(['approved', 'approved']);
    });

    it('does not start a second watch for an interrupt it already watched', async () => {
      // A server that repeats the same interrupt on the observation run must not make the session loop.
      const again = [{ type: 'RUN_STARTED', threadId: 'thread-1', runId: 'run-2' }, { type: 'RUN_FINISHED', threadId: 'thread-1', runId: 'run-2', outcome: { type: 'interrupt', interrupts: [interrupt(1, { native: true })] } }] as AguiEvent[];
      const { client, inputs } = scripted([interruptedRun([interrupt(1, { native: true })]), again, again]);
      const s = createChatSession({ client, ids: ids() });
      await s.send('run gc');
      expect(inputs).toHaveLength(2);
    });

    it('watches again on request after a dropped stream, and only for native-prompt interrupts', async () => {
      const dropped: AguiEvent[] = [{ type: 'RUN_STARTED', threadId: 'thread-1', runId: 'run-2' }]; // ends before the run does
      const { client, inputs } = scripted([interruptedRun([interrupt(1, { native: true })]), dropped, finished('run-3', [declined('tc-1')])]);
      const s = createChatSession({ client, ids: ids() });
      await s.send('run gc');
      expect(s.getSnapshot().error?.kind).toBe('incomplete');
      expect(s.getSnapshot().chat.interrupts[0]?.resolution).toBeUndefined(); // still pending on the server
      expect(await s.observe('int-nope')).toEqual({ ok: false, reason: 'unknown_interrupt' });
      expect(await s.observe(nativeId(1))).toEqual({ ok: true });
      expect(inputs[2]?.resume).toEqual([{ interruptId: nativeId(1), status: 'resolved', payload: { wait: true } }]);
      expect(s.getSnapshot().chat.interrupts[0]?.resolution).toBe('denied');

      const w = scripted([interruptedRun([interrupt(1)])]);
      const write = createChatSession({ client: w.client, ids: ids() });
      await write.send('pin');
      expect(await write.observe(interrupt(1).id)).toEqual({ ok: false, reason: 'not_native' });
    });
  });

  it('treats an expired interrupt as denied: it cannot be approved and does not block a new message', async () => {
    let now = Date.parse('2026-10-09T12:00:00Z');
    const { client, inputs } = scripted([
      interruptedRun([interrupt(1, { expiresAt: '2026-10-09T12:02:00Z' })]),
      [{ type: 'RUN_STARTED', runId: 'run-2', threadId: 'thread-1' }, { type: 'RUN_FINISHED', threadId: 'thread-1', runId: 'run-2', outcome: { type: 'success' } }],
    ]);
    const s = createChatSession({ client, ids: ids(), now: () => now });
    await s.send('pin');
    const id = s.getSnapshot().chat.interrupts[0]?.interrupt.id ?? '';
    expect(await s.send('again')).toEqual({ ok: false, reason: 'pending_interrupt' });

    now = Date.parse('2026-10-09T12:02:01Z');
    expect(await s.answer(id, 'approve')).toEqual({ ok: false, reason: 'expired' });
    expect(await s.send('again')).toEqual({ ok: true });
    expect(inputs).toHaveLength(2);
    expect(inputs[1]?.resume).toBeUndefined(); // a new message, not a resume of the expired approval
  });

  it('reports a failed run and leaves the chat usable', async () => {
    const client: AguiClient = {
      // eslint-disable-next-line require-yield
      async *run() {
        throw new AguiError('unauthorized', 'The token was refused.', 401);
      },
    };
    const s = createChatSession({ client, ids: ids() });
    await s.send('hi');
    const snap = s.getSnapshot();
    expect(snap.error).toEqual({ kind: 'unauthorized', message: 'The token was refused.' });
    expect(snap.running).toBe(false);
  });

  it('reports a stream that ends before the run finishes', async () => {
    const { client } = scripted([[{ type: 'RUN_STARTED', threadId: 'thread-1', runId: 'run-1' }]]);
    const s = createChatSession({ client, ids: ids() });
    await s.send('hi');
    const snap = s.getSnapshot();
    expect(snap.error?.kind).toBe('incomplete');
    expect(snap.chat.run.status).toBe('error');
    expect(snap.running).toBe(false);
  });

  it('stops a run in flight without reporting an error', async () => {
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const client: AguiClient = {
      async *run(_input, signal) {
        yield { type: 'RUN_STARTED', threadId: 'thread-1', runId: 'run-1' } as AguiEvent;
        await gate;
        if (signal?.aborted) throw new AguiError('aborted', 'Stopped.');
        yield { type: 'RUN_FINISHED', threadId: 'thread-1', runId: 'run-1', outcome: { type: 'success' } } as AguiEvent;
      },
    };
    const s = createChatSession({ client, ids: ids() });
    const sending = s.send('long task');
    await Promise.resolve();
    expect(s.getSnapshot().running).toBe(true);
    expect(await s.send('another')).toEqual({ ok: false, reason: 'busy' });
    s.stop();
    release();
    await sending;
    expect(s.getSnapshot().running).toBe(false);
    expect(s.getSnapshot().error).toBeUndefined();
  });

  it('starts a clean thread on reset', async () => {
    const { client } = scripted([interruptedRun([interrupt(1)])]);
    const s = createChatSession({ client, ids: ids() });
    await s.send('pin');
    s.reset();
    expect(s.getSnapshot().chat.messages).toEqual([]);
    expect(s.getSnapshot().chat.interrupts).toEqual([]);
  });
});
