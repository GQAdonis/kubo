import { screen, waitFor } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { RUN_ERROR_CODES, runErrorCopy } from '@gqadonis/ipfs-mcp-ui';
import { AguiError, createAguiClient, FAILED_PRECONDITION_MESSAGE, SESSION_BUSY_MESSAGE } from '../../src/agui/client';
import { checkEndpoint } from '../../src/agui/endpoint';
import { parseRetryAfter, RETRY_AFTER_DEFAULT_SECONDS, RETRY_AFTER_MAX_SECONDS } from '../../src/agui/retry-after';
import { buildRunInput } from '../../src/agui/wire';
import { createChatSession } from '../../src/island/session';
import { fixture, FIXTURE_NOW, sseText } from '../helpers/fixtures';
import { TOKEN, idsFor, mountIsland, randomEndpoint, say, untilIdle } from '../helpers/island';
import { EMPTY_CHAT_STATE } from '@gqadonis/ipfs-mcp-ui';

// Task 2.3 (mcp-agui-endpoint): a RUN_ERROR is drawn by its code with fixed copy, never by its message;
// HTTP 409 and 503 are put into words by the kind of conflict; a 503 honours Retry-After.

const SECRET = 'SECRET-BACKEND-BODY: the model quoted your conversation';

const sse = (events: readonly Record<string, unknown>[]): Response =>
  new Response(sseText(events as never), { status: 200, headers: { 'content-type': 'text/event-stream' } });
const json = (status: number, body: unknown, headers: Record<string, string> = {}): Response =>
  new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json', ...headers } });
const toolError = (code: string) => ({ error: { code, message: SECRET, retryable: code !== 'failed_precondition' } });
const runError = (code: string | undefined) => [
  { type: 'RUN_STARTED', threadId: 'thread-x', runId: 'run-1' },
  { type: 'RUN_ERROR', message: SECRET, ...(code && { code }) },
];

/** A fetch that answers each POST with the next scripted response. */
function scripted(...responses: (() => Response)[]) {
  const calls: unknown[] = [];
  const queue = [...responses];
  const impl = (async (_url: unknown, init: RequestInit = {}) => {
    calls.push(typeof init.body === 'string' ? JSON.parse(init.body) : undefined);
    const next = queue.shift();
    return next ? next() : json(500, { error: 'script exhausted' });
  }) as typeof fetch;
  return { fetch: impl, calls };
}

const endpoint = () => {
  const c = checkEndpoint(randomEndpoint());
  if (!c.ok) throw new Error(c.reason);
  return c.url;
};

async function refusal(response: Response, now?: () => number): Promise<AguiError> {
  const client = createAguiClient({ endpoint: endpoint(), getToken: () => TOKEN, fetch: (async () => response) as typeof fetch, ...(now && { now }) });
  try {
    for await (const _ of client.run(buildRunInput(EMPTY_CHAT_STATE, { threadId: 't', runId: 'r', user: { id: 'u', text: 'hi' } }))) void _;
  } catch (e) {
    if (e instanceof AguiError) return e;
    throw e;
  }
  throw new Error('expected a refusal');
}

describe('Retry-After', () => {
  it('reads delay-seconds and an HTTP-date, clamps, and falls back', () => {
    expect(parseRetryAfter('7', 0)).toBe(7);
    expect(parseRetryAfter(' 30 ', 0)).toBe(30);
    expect(parseRetryAfter('0', 0)).toBe(1);
    expect(parseRetryAfter('86400', 0)).toBe(RETRY_AFTER_MAX_SECONDS);
    expect(parseRetryAfter(new Date(FIXTURE_NOW + 12_000).toUTCString(), FIXTURE_NOW)).toBe(12);
    expect(parseRetryAfter(new Date(FIXTURE_NOW - 60_000).toUTCString(), FIXTURE_NOW)).toBe(1);
    for (const bad of [null, '', 'soon', '-5', '1e9']) expect(parseRetryAfter(bad, 0)).toBe(RETRY_AFTER_DEFAULT_SECONDS);
  });
});

describe('client: HTTP conflicts before the stream', () => {
  it('maps 409 session_busy to a retryable kind, and shows fixed text, not the body', async () => {
    const e = await refusal(json(409, toolError('session_busy')));
    expect(e).toMatchObject({ kind: 'session_busy', status: 409, message: SESSION_BUSY_MESSAGE });
    expect(e.message).not.toContain('SECRET');
  });
  it('maps 409 failed_precondition to its own kind', async () => {
    expect(await refusal(json(409, toolError('failed_precondition')))).toMatchObject({ kind: 'failed_precondition', message: FAILED_PRECONDITION_MESSAGE });
  });
  it('treats a 409 with no readable body as session_busy, the case waiting can fix', async () => {
    expect(await refusal(new Response('not json', { status: 409 }))).toMatchObject({ kind: 'session_busy' });
  });
  it('maps 503 to resource_exhausted and carries Retry-After', async () => {
    const e = await refusal(json(503, toolError('resource_exhausted'), { 'retry-after': '7' }));
    expect(e).toMatchObject({ kind: 'resource_exhausted', status: 503, retryAfterSeconds: 7 });
    expect(e.message).not.toContain('SECRET');
  });
  it('uses the default wait when a 503 has no Retry-After, and reads an HTTP-date against the injected clock', async () => {
    expect((await refusal(json(503, toolError('resource_exhausted')))).retryAfterSeconds).toBe(RETRY_AFTER_DEFAULT_SECONDS);
    const date = new Date(FIXTURE_NOW + 20_000).toUTCString();
    expect((await refusal(json(503, {}, { 'retry-after': date }), () => FIXTURE_NOW)).retryAfterSeconds).toBe(20);
  });
});

describe('wire: the client tools array', () => {
  it('may carry tools the island does not declare itself: no empty-array assumption in the type or the builder', () => {
    const input = buildRunInput(EMPTY_CHAT_STATE, { threadId: 't', runId: 'r', user: { id: 'u', text: 'hi' } });
    const widened: readonly unknown[] = input.tools;
    expect(widened).toEqual([]); // the island sends none; the server ignores any
  });
});

describe('session: retryable conflicts', () => {
  const ids = () => {
    let n = 0;
    return { thread: () => 'thread-r', run: () => `run-${++n}`, message: () => `msg-${++n}` };
  };
  const make = (clientFetch: typeof fetch, now?: () => number) =>
    createChatSession({ client: createAguiClient({ endpoint: endpoint(), getToken: () => TOKEN, fetch: clientFetch }), ids: ids(), ...(now && { now }) });

  it('takes the refused message out of the transcript, keeps its text, and sends it again on retry', async () => {
    const f = scripted(() => json(409, toolError('session_busy')), () => sse(fixture('read-pin-ls').events as never));
    const s = make(f.fetch);
    await s.send('Which recursive pins do I have?');
    let snap = s.getSnapshot();
    expect(snap.error).toMatchObject({ kind: 'session_busy', retryText: 'Which recursive pins do I have?' });
    expect(snap.chat.messages).toHaveLength(0);
    expect(await s.retry()).toEqual({ ok: true });
    snap = s.getSnapshot();
    expect(snap.error).toBeUndefined();
    expect(snap.chat.run.status).toBe('finished');
    expect(f.calls).toHaveLength(2);
  });

  it('holds sends until Retry-After passes on a 503, then lets them through', async () => {
    let clock = FIXTURE_NOW;
    const f = scripted(() => json(503, toolError('resource_exhausted'), { 'retry-after': '30' }), () => sse(fixture('read-pin-ls').events as never));
    const s = make(f.fetch, () => clock);
    await s.send('first');
    expect(s.getSnapshot().error).toMatchObject({ kind: 'resource_exhausted', retryNotBefore: FIXTURE_NOW + 30_000, message: 'The node is at its session limit. Try again in 30 seconds.' });
    clock = FIXTURE_NOW + 29_999;
    expect(await s.send('too soon')).toEqual({ ok: false, reason: 'retry_later' });
    expect(f.calls).toHaveLength(1); // nothing was sent
    clock = FIXTURE_NOW + 30_000;
    expect(await s.retry()).toEqual({ ok: true });
    expect(f.calls).toHaveLength(2);
  });

  it('does not offer a retry for failed_precondition', async () => {
    const s = make(scripted(() => json(409, toolError('failed_precondition'))).fetch);
    await s.send('hello');
    expect(s.getSnapshot().error).toMatchObject({ kind: 'failed_precondition', message: FAILED_PRECONDITION_MESSAGE });
    expect(s.getSnapshot().error?.retryText).toBeUndefined();
    expect(await s.retry()).toEqual({ ok: false, reason: 'empty' });
  });
});

describe('panel: RUN_ERROR by code', () => {
  const idsForRun = () => {
    const f = fixture('read-pin-ls');
    return idsFor(f, ['run-1', 'run-2']);
  };

  it.each(RUN_ERROR_CODES)('draws %s with its fixed copy and never the wire message', async (code) => {
    const f = scripted(() => sse(runError(code)));
    const m = mountIsland({ fake: { fetch: f.fetch, calls: [] }, props: { ids: idsForRun() } });
    await say(m, 'hello');
    await untilIdle();
    const copy = runErrorCopy(code);
    await screen.findByText(copy.body);
    expect(screen.getByText(`${copy.label}.`)).toBeInTheDocument();
    expect(document.body.textContent).not.toContain('SECRET');
    // The composer is usable again: a RUN_ERROR ends the run, it does not lock the thread.
    expect(screen.getByRole('textbox', { name: 'Message to the agent' })).toBeEnabled();
  });

  it('gives an unknown or missing code generic copy, still without the message', async () => {
    for (const code of ['something_new', undefined]) {
      const f = scripted(() => sse(runError(code)));
      const m = mountIsland({ fake: { fetch: f.fetch, calls: [] }, props: { ids: idsForRun() } });
      await say(m, 'hello');
      await screen.findByText(runErrorCopy(undefined).body);
      expect(document.body.textContent).not.toContain('SECRET');
      m.view.unmount();
    }
  });

  it('uses a stop state for limits and a failure state for the backend', async () => {
    const limit = mountIsland({ fake: { fetch: scripted(() => sse(runError('max_steps'))).fetch, calls: [] }, props: { ids: idsForRun() } });
    await say(limit, 'a');
    await screen.findByText(runErrorCopy('max_steps').body);
    expect(screen.getByText('Stopped', { selector: '.iu-badge' })).toBeInTheDocument();
    expect(screen.getByText('Stopped.').closest('[role]')).toHaveAttribute('role', 'status');
    limit.view.unmount();

    const backend = mountIsland({ fake: { fetch: scripted(() => sse(runError('backend_error'))).fetch, calls: [] }, props: { ids: idsForRun() } });
    await say(backend, 'a');
    await screen.findByText(runErrorCopy('backend_error').body);
    expect(screen.getByText('Failed', { selector: '.iu-badge' })).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('Failed.');
  });
});

describe('panel: HTTP conflicts', () => {
  it('409 session_busy says a reply is still streaming and sends the same message again on request', async () => {
    const f = scripted(() => json(409, toolError('session_busy')), () => sse(fixture('read-pin-ls').events as never));
    const m = mountIsland({ fake: { fetch: f.fetch, calls: [] }, props: { ids: idsFor(fixture('read-pin-ls'), ['run-1', 'run-2']) } });
    await say(m, String(fixture('read-pin-ls').input.messages[0]?.['content']));
    await screen.findByText(SESSION_BUSY_MESSAGE);
    expect(screen.getByText('Busy', { selector: '.iu-badge' })).toBeInTheDocument();
    expect(screen.getByText('A reply is still streaming. Send again when it ends.')).toBeInTheDocument();
    await m.user.click(screen.getByRole('button', { name: 'Send message again' }));
    await screen.findByText('You have one recursive pin, named wiki.');
    expect(screen.queryByText(SESSION_BUSY_MESSAGE)).not.toBeInTheDocument();
    expect(f.calls).toHaveLength(2);
  });

  it('409 failed_precondition asks for the open question first and offers no retry', async () => {
    const f = scripted(() => json(409, toolError('failed_precondition')));
    const m = mountIsland({ fake: { fetch: f.fetch, calls: [] }, props: { ids: idsFor(fixture('read-pin-ls'), ['run-1']) } });
    await say(m, 'hello');
    await screen.findByText('Answer the open question first.');
    expect(screen.queryByRole('button', { name: 'Send message again' })).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain('SECRET');
  });

  it('503 states the wait, blocks the composer, and disables the retry until it passes', async () => {
    const f = scripted(() => json(503, toolError('resource_exhausted'), { 'retry-after': '30' }));
    const m = mountIsland({ fake: { fetch: f.fetch, calls: [] }, props: { ids: idsFor(fixture('read-pin-ls'), ['run-1']) } });
    await say(m, 'hello');
    await screen.findByText('The node is at its session limit. Try again in 30 seconds.');
    await waitFor(() => expect(screen.getByRole('button', { name: 'Send message again' })).toBeDisabled());
    expect(screen.getByRole('textbox', { name: 'Message to the agent' })).toBeDisabled();
    expect(f.calls).toHaveLength(1);
  });
});

// agent-chat-island-integration 1.2: a node mounted with /agui and no backend answers 503
// backend_not_configured (mcp/server TestAGUIWithoutABackendAnswers503). The panel shows a "pick a model"
// state with a link to Settings, not the session-limit notice, and a retry is not held by Retry-After.
describe('503 backend_not_configured', () => {
  const noBackend = () => json(503, { error: { code: 'backend_not_configured', message: 'no model backend is configured', retryable: false } });

  it('is its own kind, not the session limit, and shows fixed text', async () => {
    const e = await refusal(noBackend());
    expect(e).toMatchObject({ kind: 'backend_not_configured', status: 503 });
    expect(e.retryAfterSeconds).toBeUndefined();
    expect(e.message).toBe('The node has no model backend yet. Choose one in Settings.');
  });

  it('keeps a 503 with another code, or no readable body, as the session limit', async () => {
    expect(await refusal(json(503, toolError('resource_exhausted')))).toMatchObject({ kind: 'resource_exhausted' });
    expect(await refusal(new Response('upstream down', { status: 503 }))).toMatchObject({ kind: 'resource_exhausted' });
  });

  it('takes the message back out and keeps its text, with no wait imposed', async () => {
    const f = scripted(noBackend, () => sse(fixture('read-pin-ls').events as never));
    const s = createChatSession({ client: createAguiClient({ endpoint: endpoint(), getToken: () => TOKEN, fetch: f.fetch }), ids: { thread: () => 't', run: () => 'r', message: () => 'm' } });
    await s.send('Which pins do I have?');
    const snap = s.getSnapshot();
    expect(snap.error).toMatchObject({ kind: 'backend_not_configured', retryText: 'Which pins do I have?' });
    expect(snap.error?.retryNotBefore).toBeUndefined();
    expect(snap.chat.messages).toHaveLength(0);
    expect(await s.retry()).toEqual({ ok: true });
    expect(s.getSnapshot().error).toBeUndefined();
  });

  it('replays the 503 in the panel: a pick-a-model state, a link to Settings, no failure notice', async () => {
    const m = mountIsland({ props: { fetchImpl: (async () => noBackend()) as typeof fetch, ids: idsFor(fixture('read-pin-ls'), ['run-1']) } });
    await say(m, 'Which pins do I have?');
    expect(await screen.findByText('Choose a backend to start')).toBeInTheDocument();
    expect(screen.getByText(/The agent needs a backend before it can answer. Choose one in Settings./)).toBeInTheDocument();
    const link = screen.getByRole('link', { name: 'Choose a backend' });
    expect(link).toHaveAttribute('href', '#/settings/agent');
    expect(screen.getByText('No backend', { selector: '.iu-badge' })).toBeInTheDocument();
    expect(screen.queryByText(/session limit/)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Send message again' })).toBeEnabled();
    expect(screen.getByRole('textbox', { name: 'Message to the agent' })).toBeEnabled();
  });

  it('names the unsent message with a visible lead-in and announces only the explanation', async () => {
    const m = mountIsland({ props: { fetchImpl: (async () => noBackend()) as typeof fetch, ids: idsFor(fixture('read-pin-ls'), ['run-1']) } });
    await say(m, 'Which pins do I have?');
    const unsent = await screen.findByTestId('backend-empty-unsent');
    expect(unsent).toHaveTextContent('Not sent: Which pins do I have?');
    const block = screen.getByTestId('backend-empty');
    expect(block).not.toHaveAttribute('role');
    expect(block.querySelectorAll('[role="status"]')).toHaveLength(1);
    expect(unsent).not.toHaveAttribute('role');
  });

  it('closes the panel when the link is followed, so the Settings page is in view', async () => {
    let open = true;
    const m = mountIsland({ props: { fetchImpl: (async () => noBackend()) as typeof fetch, ids: idsFor(fixture('read-pin-ls'), ['run-1']), onOpenChange: (o) => { open = o; } } });
    await say(m, 'hello');
    const link = await screen.findByRole('link', { name: 'Choose a backend' });
    link.addEventListener('click', (e) => e.preventDefault());
    await m.user.click(link);
    expect(open).toBe(false);
  });
});
