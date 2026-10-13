import { z } from 'zod';
import { AguiInterruptSchema, type AguiInterrupt, type AguiMessage } from '../contract/generated/agui';
import { ToolErrorSchema, type ToolError } from '../contract/generated/common';

// A pure reducer from AG-UI events to the state the chat primitives render. It has no transport:
// the chat island (or a test) feeds it events that came from POST /agui. Event and field names follow
// the subset ipfs mcp emits (mcp/contract/fixtures/agui.schema.json).

export type ChatRole = 'developer' | 'system' | 'assistant' | 'user' | 'tool';

export interface ChatMessage {
  readonly id: string;
  readonly role: ChatRole;
  readonly content: string;
  /** True while TEXT_MESSAGE_CONTENT events are still arriving. */
  readonly streaming: boolean;
}

export type ToolCallStatus = 'running' | 'interrupted' | 'done' | 'error';

export interface ChatToolCall {
  readonly id: string;
  readonly name: string;
  /** The raw JSON text of the arguments, exactly as streamed. */
  readonly argumentsText: string;
  readonly parentMessageId?: string;
  /** The raw TOOL_CALL_RESULT content, a JSON text. */
  readonly resultText?: string;
  /** The messageId the server gave the TOOL_CALL_RESULT; the history echoes it back as the tool message's id. */
  readonly resultMessageId?: string;
  readonly status: ToolCallStatus;
}

/**
 * How an interrupt ended. 'expired' is a denial the server reported (confirmation_expired); the card
 * shows it as Expired, not Denied, because nobody denied it.
 */
export type InterruptResolution = 'approved' | 'denied' | 'expired';

export interface ChatInterrupt {
  readonly interrupt: AguiInterrupt;
  readonly resolution?: InterruptResolution;
}

export type RunStatus = 'idle' | 'running' | 'finished' | 'interrupted' | 'error';

/**
 * Why a run that had started ended with RUN_ERROR (agui.schema.json RunErrorCode). The codes are the
 * whole vocabulary: the RUN_ERROR message is never read, because for backend_error it comes from a
 * model backend and may quote the conversation.
 */
export const RUN_ERROR_CODES = ['max_steps', 'max_tokens', 'wall_clock', 'backend_error', 'session_busy', 'session_full', 'interrupt_open'] as const;
export type RunErrorCode = (typeof RUN_ERROR_CODES)[number];

export interface RunErrorCopy {
  /** The state word of the notice. */
  readonly label: string;
  /** What happened and what the person can do. Fixed text; nothing from the wire is interpolated. */
  readonly body: string;
}

const RUN_ERROR_COPY: Record<RunErrorCode, RunErrorCopy> = {
  max_steps: { label: 'Stopped', body: 'The agent reached its step limit before it finished. Ask again with a narrower request.' },
  max_tokens: { label: 'Stopped', body: 'The agent reached its token limit before it finished. Ask for something shorter.' },
  wall_clock: { label: 'Stopped', body: 'The agent ran out of time before it finished. Ask again, or ask for less at once.' },
  backend_error: { label: 'Failed', body: 'The model backend failed. Nothing on your node changed because of this error. Try again in a moment.' },
  session_busy: { label: 'Busy', body: 'A reply is still streaming on this thread. Send again when it ends.' },
  session_full: { label: 'Full', body: 'This conversation reached its length limit. Start a new chat to continue.' },
  interrupt_open: { label: 'Waiting', body: 'Answer the open question first. This thread has an approval waiting.' },
};

const UNKNOWN_RUN_ERROR: RunErrorCopy = { label: 'Failed', body: 'The run failed for a reason this panel does not recognize.' };

export const isRunErrorCode = (v: unknown): v is RunErrorCode => typeof v === 'string' && (RUN_ERROR_CODES as readonly string[]).includes(v);

/** The fixed copy for a RUN_ERROR code. An unknown or missing code gets the generic copy. */
export function runErrorCopy(code: RunErrorCode | undefined): RunErrorCopy {
  return code ? RUN_ERROR_COPY[code] : UNKNOWN_RUN_ERROR;
}

export interface ChatState {
  readonly messages: readonly ChatMessage[];
  readonly toolCalls: readonly ChatToolCall[];
  readonly interrupts: readonly ChatInterrupt[];
  readonly run: {
    readonly status: RunStatus;
    readonly threadId?: string;
    readonly runId?: string;
    /** The fixed copy for `errorCode`; never the RUN_ERROR message. */
    readonly error?: string;
    /** Set by RUN_ERROR when the wire carried a known RunErrorCode. */
    readonly errorCode?: RunErrorCode;
  };
}

export const EMPTY_CHAT_STATE: ChatState = {
  messages: [],
  toolCalls: [],
  interrupts: [],
  run: { status: 'idle' },
};

/** The subset of AG-UI events the reducer reads. Unknown event types are ignored, never fatal. */
export interface AguiEvent {
  readonly type: string;
  readonly [key: string]: unknown;
}

const str = (v: unknown): string | undefined => (typeof v === 'string' ? v : undefined);

function upsert<T extends { id: string }>(list: readonly T[], id: string, make: () => T, update: (t: T) => T): T[] {
  const i = list.findIndex((x) => x.id === id);
  if (i < 0) return [...list, update(make())];
  return list.map((x, j) => (j === i ? update(x) : x));
}

const roleOf = (v: unknown): ChatRole =>
  v === 'user' || v === 'system' || v === 'developer' || v === 'tool' ? v : 'assistant';

/** Fold one AG-UI event into the chat state. Returns a new state; the input is never mutated. */
export function foldAgui(state: ChatState, event: AguiEvent): ChatState {
  switch (event.type) {
    case 'RUN_STARTED': {
      const threadId = str(event['threadId']);
      const runId = str(event['runId']);
      return { ...state, run: { status: 'running', ...(threadId && { threadId }), ...(runId && { runId }) } };
    }
    case 'RUN_FINISHED': {
      const outcome = event['outcome'] as { type?: string; interrupts?: unknown[] } | undefined;
      const interrupts = outcome?.type === 'interrupt' ? parseInterrupts(outcome.interrupts) : [];
      const merged = [...state.interrupts];
      for (const i of interrupts) {
        if (!merged.some((m) => m.interrupt.id === i.id)) merged.push({ interrupt: i });
      }
      const interruptedIds = new Set(interrupts.map((i) => i.toolCallId));
      return {
        ...state,
        interrupts: merged,
        toolCalls: state.toolCalls.map((c) => (interruptedIds.has(c.id) ? { ...c, status: 'interrupted' } : c)),
        run: { ...state.run, status: interrupts.length > 0 ? 'interrupted' : 'finished' },
      };
    }
    case 'RUN_ERROR': {
      // Rendered by code, with fixed copy. The message is deliberately not read.
      const code = isRunErrorCode(event['code']) ? event['code'] : undefined;
      const { errorCode: _drop, ...run } = state.run;
      return { ...state, run: { ...run, status: 'error', error: runErrorCopy(code).body, ...(code && { errorCode: code }) } };
    }
    case 'TEXT_MESSAGE_START': {
      const id = str(event['messageId']);
      if (!id) return state;
      return {
        ...state,
        messages: upsert(state.messages, id, () => ({ id, role: roleOf(event['role']), content: '', streaming: true }), (m) => ({
          ...m,
          streaming: true,
        })),
      };
    }
    case 'TEXT_MESSAGE_CONTENT': {
      const id = str(event['messageId']);
      const delta = str(event['delta']) ?? '';
      if (!id) return state;
      return {
        ...state,
        messages: upsert(state.messages, id, () => ({ id, role: 'assistant', content: '', streaming: true }), (m) => ({
          ...m,
          content: m.content + delta,
        })),
      };
    }
    case 'TEXT_MESSAGE_END': {
      const id = str(event['messageId']);
      if (!id) return state;
      return { ...state, messages: state.messages.map((m) => (m.id === id ? { ...m, streaming: false } : m)) };
    }
    case 'TOOL_CALL_START': {
      const id = str(event['toolCallId']);
      const name = str(event['toolCallName']);
      if (!id || !name) return state;
      const parent = str(event['parentMessageId']);
      return {
        ...state,
        toolCalls: upsert(state.toolCalls, id, () => ({ id, name, argumentsText: '', status: 'running' }), (c) => ({
          ...c,
          name,
          ...(parent && { parentMessageId: parent }),
        })),
      };
    }
    case 'TOOL_CALL_ARGS': {
      const id = str(event['toolCallId']);
      const delta = str(event['delta']) ?? '';
      if (!id) return state;
      return {
        ...state,
        toolCalls: state.toolCalls.map((c) => (c.id === id ? { ...c, argumentsText: c.argumentsText + delta } : c)),
      };
    }
    case 'TOOL_CALL_END':
      return state;
    case 'TOOL_CALL_RESULT': {
      const id = str(event['toolCallId']);
      const content = str(event['content']);
      if (!id || content === undefined) return state;
      const messageId = str(event['messageId']);
      const error = parseToolError(content);
      const status: ToolCallStatus = error ? 'error' : 'done';
      // The result of a call that still has an unanswered interrupt is how its outcome reaches the chat:
      // a native-prompt call is decided outside this chat, and the observation run reports it here.
      const outcome = outcomeOfResult(error);
      return {
        ...state,
        toolCalls: state.toolCalls.map((c) => (c.id === id ? { ...c, resultText: content, status, ...(messageId && { resultMessageId: messageId }) } : c)),
        interrupts: state.interrupts.map((i) =>
          i.interrupt.toolCallId === id && i.resolution === undefined ? { ...i, resolution: outcome } : i,
        ),
      };
    }
    case 'MESSAGES_SNAPSHOT': {
      const snap = Array.isArray(event['messages']) ? (event['messages'] as AguiMessage[]) : [];
      return { ...state, messages: mergeSnapshot(state.messages, snap), toolCalls: mergeSnapshotCalls(state.toolCalls, snap) };
    }
    default:
      return state;
  }
}

/** The tool result that ends an interrupt: declined and expired are denials, anything else means the call ran. */
function outcomeOfResult(error: ToolError | null): InterruptResolution {
  if (error?.error.code === 'confirmation_declined') return 'denied';
  if (error?.error.code === 'confirmation_expired') return 'expired';
  return 'approved';
}

/** A malformed interrupt is dropped, never rendered as something it is not. */
function parseInterrupts(raw: unknown[] | undefined): AguiInterrupt[] {
  const out: AguiInterrupt[] = [];
  for (const r of raw ?? []) {
    const parsed = AguiInterruptSchema.safeParse(r);
    if (parsed.success) out.push(parsed.data);
  }
  return out;
}

function mergeSnapshot(current: readonly ChatMessage[], snap: readonly AguiMessage[]): ChatMessage[] {
  const known = new Map(current.map((m) => [m.id, m]));
  return snap
    .filter((m) => m.role !== 'tool')
    .map((m) => {
      const prev = known.get(m.id);
      return {
        id: m.id,
        role: m.role,
        content: m.content ?? prev?.content ?? '',
        streaming: prev?.streaming ?? false,
      };
    });
}

function mergeSnapshotCalls(current: readonly ChatToolCall[], snap: readonly AguiMessage[]): ChatToolCall[] {
  const calls = [...current];
  for (const m of snap) {
    for (const tc of m.toolCalls ?? []) {
      if (!calls.some((c) => c.id === tc.id)) {
        calls.push({
          id: tc.id,
          name: tc.function.name,
          argumentsText: tc.function.arguments,
          parentMessageId: m.id,
          status: 'running',
        });
      }
    }
  }
  return calls;
}

export function foldAguiEvents(events: readonly AguiEvent[], initial: ChatState = EMPTY_CHAT_STATE): ChatState {
  return events.reduce(foldAgui, initial);
}

/**
 * A RunAgentInput.resume entry. `{approved}` answers an interrupt. `{wait: true}` is only for a
 * native-prompt interrupt: it asks the server to report the system prompt's outcome and answers nothing.
 */
export interface ResumeEntry {
  readonly interruptId: string;
  readonly status: 'resolved' | 'cancelled';
  readonly payload?: { readonly approved: boolean } | { readonly wait: true };
}

/**
 * Apply the input of a resumed run: the messages it carries, and the answers to earlier interrupts.
 * `cancelled`, or `approved: false`, is a denial; `{wait: true}` answers nothing. The state keeps tool calls from the earlier run so
 * the resumed run's TOOL_CALL_RESULT lands on the original call without re-emitting it.
 */
export function applyRunInput(
  state: ChatState,
  input: { readonly messages?: readonly AguiMessage[]; readonly resume?: readonly ResumeEntry[] },
): ChatState {
  const resume = input.resume ?? [];
  const interrupts = state.interrupts.map((i) => {
    const r = resume.find((x) => x.interruptId === i.interrupt.id);
    if (!r || (r.payload && 'wait' in r.payload)) return i;
    const approved = r.status === 'resolved' && r.payload !== undefined && 'approved' in r.payload && r.payload.approved === true;
    return { ...i, resolution: (approved ? 'approved' : 'denied') as InterruptResolution };
  });
  const snap = input.messages ?? [];
  return {
    ...state,
    interrupts,
    messages: snap.length > 0 ? mergeSnapshot(state.messages, snap) : state.messages,
    toolCalls: snap.length > 0 ? mergeSnapshotCalls(state.toolCalls, snap) : state.toolCalls,
  };
}

// ----------------------------------------------------------------- tool call text helpers

/** Parse text as JSON, returning undefined instead of throwing. */
export function parseJson(text: string): unknown {
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return undefined;
  }
}

/** The ToolError carried by a tool result text, if it is one. */
export function parseToolError(text: string): ToolError | null {
  const parsed = ToolErrorSchema.safeParse(parseJson(text));
  return parsed.success ? parsed.data : null;
}

export const ChatToolCallWireSchema = z.strictObject({
  id: z.string().min(1),
  name: z.string().min(1),
  arguments: z.string(),
  result: z.string().optional(),
  status: z.enum(['running', 'interrupted', 'done', 'error']),
});
export type ChatToolCallWire = z.infer<typeof ChatToolCallWireSchema>;

export function toWire(call: ChatToolCall): ChatToolCallWire {
  return {
    id: call.id,
    name: call.name,
    arguments: call.argumentsText,
    ...(call.resultText !== undefined && { result: call.resultText }),
    status: call.status,
  };
}
