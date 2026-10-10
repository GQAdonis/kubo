// @vitest-environment node
import { EMPTY_CHAT_STATE, foldAgui, type AguiEvent } from '@gqadonis/ipfs-mcp-ui';
import { describe, expect, it } from 'vitest';
import { buildRunInput } from '../../src/agui/wire';
import { buildTimeline } from '../../src/island/timeline';
import { fixture } from '../helpers/fixtures';

const fold = (events: readonly AguiEvent[]) => events.reduce(foldAgui, EMPTY_CHAT_STATE);

describe('timeline', () => {
  it('lays out a message, then its tool calls, then the approval each interrupted call waits on', () => {
    const f = fixture('write-interrupt');
    const chat = fold(f.events);
    const items = buildTimeline(chat);
    expect(items.map((i) => i.kind)).toEqual(['message', 'call']);
    const call = items[1];
    expect(call?.kind === 'call' && call.call.id).toBe('tc-7');
    expect(call?.kind === 'call' && call.interrupt?.interrupt.metadata['ipfs-mcp'].tool).toBe('pin_add');
  });

  it('draws no bubble for an assistant turn that only called tools', () => {
    const chat = fold(fixture('write-interrupt').events);
    expect(buildTimeline(chat).filter((i) => i.kind === 'message').map((i) => i.kind === 'message' && i.message.role)).toEqual(['user']);
  });

  it('keeps calls whose parent message is unknown', () => {
    const chat = fold([
      { type: 'TOOL_CALL_START', toolCallId: 'tc-x', toolCallName: 'pin_ls' },
      { type: 'TOOL_CALL_ARGS', toolCallId: 'tc-x', delta: '{}' },
    ]);
    expect(buildTimeline(chat)).toHaveLength(1);
  });
});

describe('wire input', () => {
  it('rebuilds exactly the input of the recorded resume run from the state of the interrupted run', () => {
    const interrupted = fixture('write-interrupt');
    const resumed = fixture('write-resume-approved');
    const chat = fold(interrupted.events);
    const rebuilt = buildRunInput(chat, {
      threadId: 'thread-pin-2',
      runId: 'run-2',
      resume: [{ interruptId: 'int-cf_Zr4Yt1nB6kq0HdUe', status: 'resolved', payload: { approved: true } }],
    });
    expect(rebuilt).toEqual(resumed.input);
  });

  it('includes finished tool results as tool messages on the next turn', () => {
    const chat = fold(fixture('read-pin-ls').events);
    const input = buildRunInput(chat, { threadId: 't', runId: 'r', user: { id: 'msg-u2', text: 'and ls again' } });
    const roles = input.messages.map((m) => m.role);
    // The user turn and msg-a1 (with its call) come from the run's MESSAGES_SNAPSHOT, then the tool result.
    expect(roles).toEqual(['user', 'assistant', 'tool', 'assistant', 'user']);
    expect(input.messages.map((m) => m.id)).toEqual(['msg-u1', 'msg-a1', 'msg-t1', 'msg-a2', 'msg-u2']);
    expect(input.messages[1]).toMatchObject({ id: 'msg-a1', toolCalls: [{ id: 'tc-1' }] });
    // The tool message carries the id the server gave the result, so the history matches what it sent.
    expect(input.messages[2]).toMatchObject({ id: 'msg-t1', role: 'tool', toolCallId: 'tc-1' });
    expect(input.tools).toEqual([]);
    expect(input.context).toEqual([]);
  });
});
