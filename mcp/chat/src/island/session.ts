import {
  EMPTY_CHAT_STATE,
  applyRunInput,
  foldAgui,
  interruptExpired,
  type ChatInterrupt,
  type ChatState,
  type ResumeEntry,
} from '@gqadonis/ipfs-mcp-ui';
import { AguiError, type AguiClient, type AguiErrorKind } from '../agui/client';
import { buildRunInput } from '../agui/wire';

// The chat session: one thread against `ipfs mcp` /agui. It is a plain store (no React), so the same
// logic runs under the island and under tests that replay recorded AG-UI fixtures. The island reads it
// with useSyncExternalStore. Snapshots are immutable; each change produces a new object.
//
// Interrupts (docs/mcp/0005-agent-chat-ui.md, amendment of 2026-10-09):
//   - A run carries at most one interrupt (ADR-0004 Round 6 amendment), so a resume carries exactly one
//     entry and never mixes a write answer with a native-prompt {wait: true}. A write interrupt is
//     answered here: approved: true or approved: false. Later queued calls surface as the next run's
//     interrupt, never in the same batch.
//   - A native-prompt interrupt is decided in the system prompt. The chat can only deny it
//     ({approved: false}) or watch it ({wait: true}). Once such an interrupt is on screen the session
//     sends {wait: true} by itself: it answers nothing, and the run it opens ends with the tool result,
//     confirmation_declined or confirmation_expired. The result, not a click, marks the card approved,
//     denied or expired.
//   - Expiry is denial. No resume is sent for an interrupt that expired on its own.

export interface SessionError {
  readonly kind: AguiErrorKind | 'incomplete';
  readonly message: string;
  /** Epoch ms before which a send is refused (a 503's Retry-After, clamped). Absent when sending is allowed now. */
  readonly retryNotBefore?: number;
  /**
   * The message that was refused before the stream and can be sent again (409 session_busy, 503 resource_exhausted,
   * 503 backend_not_configured once a backend is set).
   * It is out of the transcript while this is set; retry() sends it again.
   */
  readonly retryText?: string;
}

export interface SessionSnapshot {
  readonly chat: ChatState;
  readonly threadId: string;
  readonly running: boolean;
  /** Native-prompt interrupts whose outcome the in-flight run is waiting for. Empty when idle. */
  readonly observing: readonly string[];
  readonly error?: SessionError;
}

export interface SessionIds {
  thread(): string;
  run(): string;
  message(): string;
}

export interface SessionDeps {
  /** Null when the island is not configured: nothing can be sent. */
  readonly client: AguiClient | null;
  readonly ids: SessionIds;
  /** The host clock in epoch milliseconds. Defaults to Date.now. Expiry is judged against it. */
  readonly now?: () => number;
}

export type RefusalReason =
  | 'not_configured'
  | 'busy'
  | 'empty'
  | 'pending_interrupt'
  | 'native_prompt'
  | 'not_native'
  | 'unknown_interrupt'
  | 'already_answered'
  | 'expired'
  | 'retry_later';

export type Outcome = { readonly ok: true } | { readonly ok: false; readonly reason: RefusalReason };

export interface ChatSession {
  getSnapshot(): SessionSnapshot;
  subscribe(listener: () => void): () => void;
  /** Start a run with a new user message. */
  send(text: string): Promise<Outcome>;
  /** Send again the message a refusal took back (session_busy, resource_exhausted, backend_not_configured). */
  retry(): Promise<Outcome>;
  /**
   * Answer an interrupt. Approval of a call that needs the native prompt is refused here, whatever the
   * caller says: the chat cannot approve it. A denial of a native-prompt call that is being observed
   * stops the observation first. The caller must already have checked the user gesture.
   */
  answer(interruptId: string, decision: 'approve' | 'deny'): Promise<Outcome>;
  /**
   * Watch a native-prompt interrupt again, after the observation stopped or its stream dropped. The
   * confirmation stays pending on the server until it expires, so a new {wait: true} picks it up.
   */
  observe(interruptId: string): Promise<Outcome>;
  stop(): void;
  reset(): void;
}

const ok: Outcome = { ok: true };
const refuse = (reason: RefusalReason): Outcome => ({ ok: false, reason });

const isNative = (i: ChatInterrupt): boolean => i.interrupt.metadata['ipfs-mcp'].requires_native_prompt;
const waitEntry = (i: ChatInterrupt): ResumeEntry => ({ interruptId: i.interrupt.id, status: 'resolved', payload: { wait: true } });

function toSessionError(e: unknown, nowMs: number): SessionError {
  if (!(e instanceof AguiError)) return { kind: 'network', message: 'The run failed.' };
  if (e.kind === 'resource_exhausted' && e.retryAfterSeconds !== undefined) {
    const n = e.retryAfterSeconds;
    return {
      kind: e.kind,
      message: `The node is at its session limit. Try again in ${n} ${n === 1 ? 'second' : 'seconds'}.`,
      retryNotBefore: nowMs + n * 1000,
    };
  }
  return { kind: e.kind, message: e.message };
}

export function createChatSession(deps: SessionDeps): ChatSession {
  let snapshot: SessionSnapshot = { chat: EMPTY_CHAT_STATE, threadId: deps.ids.thread(), running: false, observing: [] };
  let controller: AbortController | undefined;
  let current: Promise<void> = Promise.resolve();
  /** Set when the person's denial stops an observation, so the stopped run is not reported as a failure. */
  let quietAbort = false;
  /** Interrupts the session has already watched on its own: a server that repeats one cannot make a loop. */
  const watched = new Set<string>();
  const listeners = new Set<() => void>();

  const emit = () => {
    for (const l of listeners) l();
  };
  const set = (next: Partial<SessionSnapshot>) => {
    snapshot = { ...snapshot, ...next };
    emit();
  };
  const setChat = (fn: (c: ChatState) => ChatState) => set({ chat: fn(snapshot.chat) });

  const nowIso = () => new Date(deps.now?.() ?? Date.now()).toISOString();
  const isExpired = (i: ChatInterrupt) => interruptExpired(i.interrupt, nowIso());
  const unresolved = () => snapshot.chat.interrupts.filter((i) => i.resolution === undefined);
  /** Not answered and not past its expiry. Expiry is denial, so an expired one does not block. */
  const awaiting = () => unresolved().filter((i) => !isExpired(i));

  async function runOnce(input: ReturnType<typeof buildRunInput>, observed: readonly string[]): Promise<void> {
    const client = deps.client;
    if (!client) return;
    controller = new AbortController();
    const mine = controller;
    const { error: _cleared, ...rest } = snapshot;
    snapshot = { ...rest, running: true, observing: observed };
    emit();
    try {
      for await (const event of client.run(input, mine.signal)) {
        if (mine.signal.aborted) break;
        setChat((c) => foldAgui(c, event));
      }
      if (!mine.signal.aborted && snapshot.chat.run.status === 'running') {
        set({ error: { kind: 'incomplete', message: 'The stream ended before the run finished.' } });
      }
    } catch (e) {
      if (!mine.signal.aborted) {
        set({ error: toSessionError(e, deps.now?.() ?? Date.now()) });
      }
    } finally {
      if (controller === mine) controller = undefined;
      if (snapshot.chat.run.status === 'running') {
        if (quietAbort && mine.signal.aborted) {
          // The person denied while the system prompt was being watched. The thread is waiting, not broken.
          setChat((c) => ({ ...c, run: { ...c.run, status: unresolved().length > 0 ? 'interrupted' : 'finished' } }));
        } else {
          // A run that ended without an outcome must not leave the chat looking busy.
          setChat((c) => ({ ...c, run: { ...c.run, status: 'error', error: c.run.error ?? 'The run did not finish.' } }));
        }
      }
      quietAbort = false;
      set({ running: false, observing: [] });
    }
  }

  /** Run and remember the promise, so a denial can wait for an aborted observation to settle. */
  const run = (input: ReturnType<typeof buildRunInput>, observed: readonly string[] = []): Promise<void> => {
    current = runOnce(input, observed);
    return current;
  };

  const resumeInput = (resume: readonly ResumeEntry[]) =>
    buildRunInput(snapshot.chat, { threadId: snapshot.chat.run.threadId ?? snapshot.threadId, runId: deps.ids.run(), resume });

  /**
   * When a run ends with nothing but native-prompt interrupts waiting, watch them. {wait: true} answers
   * nothing, so no gesture is needed. A run that failed, or that ended with a write interrupt still
   * waiting for a person, does not start a watch: the write answer carries the waits.
   */
  async function watchNative(): Promise<void> {
    if (snapshot.running || snapshot.error || snapshot.chat.run.status !== 'interrupted') return;
    const open = awaiting();
    const only = open[0];
    if (open.length !== 1 || !only || !isNative(only) || watched.has(only.interrupt.id)) return;
    watched.add(only.interrupt.id);
    await run(resumeInput([waitEntry(only)]), [only.interrupt.id]);
  }

  return {
    getSnapshot: () => snapshot,
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },

    async send(text) {
      if (!deps.client) return refuse('not_configured');
      if (snapshot.running) return refuse('busy');
      const holdUntil = snapshot.error?.retryNotBefore;
      if (holdUntil !== undefined && (deps.now?.() ?? Date.now()) < holdUntil) return refuse('retry_later');
      const trimmed = text.trim();
      if (trimmed === '') return refuse('empty');
      if (awaiting().length > 0) return refuse('pending_interrupt');
      const id = deps.ids.message();
      const input = buildRunInput(snapshot.chat, {
        threadId: snapshot.chat.run.threadId ?? snapshot.threadId,
        runId: deps.ids.run(),
        user: { id, text: trimmed },
      });
      setChat((c) => ({ ...c, messages: [...c.messages, { id, role: 'user', content: trimmed, streaming: false }] }));
      await run(input);
      const refused = snapshot.error;
      if (refused && (refused.kind === 'session_busy' || refused.kind === 'resource_exhausted' || refused.kind === 'backend_not_configured')) {
        // Refused before the stream: the server took nothing. Take the message back out, keep the text.
        setChat((c) => ({ ...c, messages: c.messages.filter((m) => m.id !== id) }));
        set({ error: { ...refused, retryText: trimmed } });
        return ok;
      }
      await watchNative();
      return ok;
    },

    async retry() {
      const text = snapshot.error?.retryText;
      return text === undefined ? refuse('empty') : this.send(text);
    },

    async answer(interruptId, decision) {
      if (!deps.client) return refuse('not_configured');
      const target = snapshot.chat.interrupts.find((i) => i.interrupt.id === interruptId);
      // The chat never approves a call the system prompt decides, in any state and whoever asks.
      if (target && decision === 'approve' && isNative(target)) return refuse('native_prompt');
      const observed = target !== undefined && snapshot.observing.includes(interruptId);
      // The one thing allowed while a run is in flight is to deny a call whose system prompt is being watched.
      if (snapshot.running && !observed) return refuse('busy');
      if (!target) return refuse('unknown_interrupt');
      if (target.resolution !== undefined) return refuse('already_answered');
      if (isExpired(target)) return refuse('expired');

      if (observed) {
        // Stop watching, then deny: the denial resolves the confirmation as denied on the server.
        quietAbort = true;
        controller?.abort();
        await current;
      }

      const entry: ResumeEntry = { interruptId, status: 'resolved', payload: { approved: decision === 'approve' } };
      setChat((c) => applyRunInput(c, { resume: [entry] }));
      // One interrupt per run: this is the only entry the resume carries.
      await run(resumeInput([entry]));
      await watchNative();
      return ok;
    },

    async observe(interruptId) {
      if (!deps.client) return refuse('not_configured');
      if (snapshot.running) return refuse('busy');
      const target = snapshot.chat.interrupts.find((i) => i.interrupt.id === interruptId);
      if (!target) return refuse('unknown_interrupt');
      if (target.resolution !== undefined) return refuse('already_answered');
      if (isExpired(target)) return refuse('expired');
      if (!isNative(target)) return refuse('not_native');
      if (awaiting().length !== 1) return refuse('pending_interrupt');
      await run(resumeInput([waitEntry(target)]), [target.interrupt.id]);
      return ok;
    },

    stop() {
      controller?.abort();
    },

    reset() {
      controller?.abort();
      watched.clear();
      snapshot = { chat: EMPTY_CHAT_STATE, threadId: deps.ids.thread(), running: false, observing: [] };
      emit();
    },
  };
}
