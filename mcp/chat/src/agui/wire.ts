import type { ChatState, ResumeEntry } from '@gqadonis/ipfs-mcp-ui';

// Build the RunAgentInput the island POSTs. The shape is the one in mcp/contract/fixtures/agui.schema.json:
// The island declares no tools of its own and sends `tools: []`, the shape AG-UI clients use. That is
// a choice, not a requirement: ipfs mcp ignores client-declared `tools`, `context`, `state` and
// `forwardedProps` and offers the model only the contract tools enabled for the principal. A resumed
// run carries `resume` entries.

export interface WireToolCall {
  readonly id: string;
  readonly type: 'function';
  readonly function: { readonly name: string; readonly arguments: string };
}

export interface WireMessage {
  readonly id: string;
  readonly role: 'developer' | 'system' | 'assistant' | 'user' | 'tool';
  readonly content?: string;
  readonly toolCallId?: string;
  readonly toolCalls?: readonly WireToolCall[];
}

export interface RunAgentInput {
  readonly threadId: string;
  readonly runId: string;
  readonly messages: readonly WireMessage[];
  readonly tools: readonly unknown[];
  readonly context: readonly unknown[];
  readonly resume?: readonly ResumeEntry[];
}

export interface BuildRunInput {
  readonly threadId: string;
  readonly runId: string;
  /** A new user turn to append. Mutually exclusive in practice with `resume`. */
  readonly user?: { readonly id: string; readonly text: string };
  readonly resume?: readonly ResumeEntry[];
}

/** The history the server needs to continue the thread: messages, their tool calls, and finished tool results. */
export function buildRunInput(state: ChatState, args: BuildRunInput): RunAgentInput {
  const callsByParent = new Map<string, WireToolCall[]>();
  const orphans: WireToolCall[] = [];
  const known = new Set(state.messages.map((m) => m.id));
  for (const c of state.toolCalls) {
    const wire: WireToolCall = { id: c.id, type: 'function', function: { name: c.name, arguments: c.argumentsText } };
    if (c.parentMessageId !== undefined && known.has(c.parentMessageId)) {
      callsByParent.set(c.parentMessageId, [...(callsByParent.get(c.parentMessageId) ?? []), wire]);
    } else orphans.push(wire);
  }

  const messages: WireMessage[] = [];
  const results = (calls: readonly WireToolCall[]) => {
    for (const w of calls) {
      const call = state.toolCalls.find((c) => c.id === w.id);
      if (call?.resultText !== undefined) {
        messages.push({ id: call.resultMessageId ?? `msg-tool-${w.id}`, role: 'tool', toolCallId: w.id, content: call.resultText });
      }
    }
  };

  for (const m of state.messages) {
    const calls = callsByParent.get(m.id);
    messages.push({
      id: m.id,
      role: m.role,
      ...(m.content !== '' && { content: m.content }),
      ...(calls && { toolCalls: calls }),
    });
    if (calls) results(calls);
  }
  if (orphans.length > 0) {
    messages.push({ id: `msg-asst-${orphans[0]?.id ?? 'orphan'}`, role: 'assistant', toolCalls: orphans });
    results(orphans);
  }
  if (args.user) messages.push({ id: args.user.id, role: 'user', content: args.user.text });

  return {
    threadId: args.threadId,
    runId: args.runId,
    messages,
    tools: [],
    context: [],
    ...(args.resume && args.resume.length > 0 && { resume: args.resume }),
  };
}
