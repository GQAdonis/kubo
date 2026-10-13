// @vitest-environment node
import { mkdirSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { createAguiClient } from '../../src/agui/client';
import { checkEndpoint } from '../../src/agui/endpoint';
import { createChatSession, type ChatSession, type SessionIds } from '../../src/island/session';
import { FIXTURE_NOW, fixture, loadFixtures } from '../helpers/fixtures';
import { startFixtureServer, type FixtureServer, type RecordedRequest } from '../helpers/fixture-server';

// Task 1.4: replay the recorded contract AG-UI fixtures (all fourteen) through the island's client and
// session, against a local server on a free port. The server answers only when the request body equals
// the fixture input, so a pass proves the wire format as well as the event handling. No live `ipfs mcp`
// is involved.
//
// Three fixtures (destructive-native-prompt-wait-approved, -denied, -expired) share one input: the
// outcome belongs to the system prompt, not to the request. The shared server therefore holds only the
// approved one, and the denied and expired outcomes each get their own server.

const TOKEN = 'replay-bearer-0123456789abcdef';
const OUTCOME_ONLY = ['destructive-native-prompt-wait-denied', 'destructive-native-prompt-wait-expired'];
let server: FixtureServer;
const extraServers: FixtureServer[] = [];
/** Every request any server in this file received, for the origin and path check at the end. */
const allRequests = (): RecordedRequest[] => [server, ...extraServers].flatMap((x) => x.requests);

beforeAll(async () => {
  server = await startFixtureServer({
    token: TOKEN,
    fragmentBytes: 64,
    keepAlive: true,
    fixtures: loadFixtures().filter((f) => !OUTCOME_ONLY.includes(f.name)),
  });
});
afterAll(async () => {
  await server.close();
  await Promise.all(extraServers.map((x) => x.close()));
});

/** A server whose only wait outcome is `outcome`. */
async function serverWithOutcome(outcome: string): Promise<FixtureServer> {
  const x = await startFixtureServer({
    token: TOKEN,
    fragmentBytes: 64,
    fixtures: loadFixtures().filter((f) => !f.name.startsWith('destructive-native-prompt-wait-') || f.name === outcome),
  });
  extraServers.push(x);
  return x;
}

/** Ids that reproduce a fixture's thread, run sequence and first user message. */
function idsFor(name: string, runs: string[]): SessionIds {
  const f = fixture(name);
  const queue = [...runs];
  return { thread: () => f.input.threadId, run: () => queue.shift() ?? 'run-exhausted', message: () => String(f.input.messages[0]?.['id']) };
}

function sessionFor(name: string, runs: string[], on: FixtureServer = server): ChatSession {
  const c = checkEndpoint(on.endpoint);
  if (!c.ok) throw new Error(c.reason);
  return createChatSession({ client: createAguiClient({ endpoint: c.url, getToken: () => TOKEN }), ids: idsFor(name, runs), now: () => FIXTURE_NOW });
}

const userText = (name: string) => String(fixture(name).input.messages[0]?.['content']);
const matchedSince = (n: number) => server.requests.slice(n).map((r) => r.matched);

interface Result {
  fixture: string;
  provenance: string;
  requests: number;
  matchedFixtures: (string | undefined)[];
  eventsReplayed: number;
  finalRun: string;
  interrupts: { tool: string; class: string; requiresNativePrompt: boolean; resolution: string }[];
  toolCalls: { name: string; status: string }[];
  messages: string[];
  note?: string;
}
const results: Result[] = [];

function record(name: string, session: ChatSession, before: number, eventsReplayed: number, note?: string, on: FixtureServer = server) {
  const s = session.getSnapshot();
  results.push({
    fixture: name,
    provenance: fixture(name).provenance,
    requests: on.requests.length - before,
    matchedFixtures: on.requests.slice(before).map((r) => r.matched),
    eventsReplayed,
    finalRun: s.chat.run.status,
    interrupts: s.chat.interrupts.map((i) => ({
      tool: i.interrupt.metadata['ipfs-mcp'].tool,
      class: i.interrupt.metadata['ipfs-mcp'].class,
      requiresNativePrompt: i.interrupt.metadata['ipfs-mcp'].requires_native_prompt,
      resolution: i.resolution ?? 'unanswered',
    })),
    toolCalls: s.chat.toolCalls.map((c) => ({ name: c.name, status: c.status })),
    messages: s.chat.messages.filter((m) => m.content).map((m) => `${m.role}: ${m.content}`),
    ...(note && { note }),
  });
}

describe('replay of the recorded contract AG-UI fixtures', () => {
  it('has a fixture for each case the island must handle', () => {
    // File order: '-' sorts before '.', so the wait outcomes come before the interrupt they observe.
    expect(loadFixtures().map((f) => f.name)).toEqual([
      'destructive-native-prompt-wait-approved',
      'destructive-native-prompt-wait-denied',
      'destructive-native-prompt-wait-expired',
      'destructive-native-prompt',
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
  });

  it('read-pin-ls: a read-only run finishes with the tool result and the answer', async () => {
    const before = server.requests.length;
    const s = sessionFor('read-pin-ls', ['run-1']);
    await s.send(userText('read-pin-ls'));
    const { chat, error } = s.getSnapshot();
    expect(error).toBeUndefined();
    expect(matchedSince(before)).toEqual(['read-pin-ls']);
    expect(chat.run.status).toBe('finished');
    expect(chat.interrupts).toHaveLength(0);
    expect(chat.toolCalls).toMatchObject([{ id: 'tc-1', name: 'pin_ls', status: 'done' }]);
    expect(chat.messages.map((m) => m.content).filter(Boolean)).toEqual(['Which recursive pins do I have?', 'You have one recursive pin, named wiki.']);
    // The call hangs off msg-a1, which the MESSAGES_SNAPSHOT introduced; no message was made up for it.
    expect(chat.messages.map((m) => m.id)).toEqual(['msg-u1', 'msg-a1', 'msg-a2']);
    expect(chat.toolCalls[0]?.parentMessageId).toBe('msg-a1');
    record('read-pin-ls', s, before, fixture('read-pin-ls').events.length);
  });

  it('egress-refused: the structured refusal is the tool result, not the content', async () => {
    const before = server.requests.length;
    const s = sessionFor('egress-refused', ['run-1']);
    await s.send(userText('egress-refused'));
    const { chat } = s.getSnapshot();
    expect(matchedSince(before)).toEqual(['egress-refused']);
    expect(chat.toolCalls[0]).toMatchObject({ name: 'ipfs_cat', status: 'error' });
    expect(chat.toolCalls[0]?.resultText).toContain('egress_not_acknowledged');
    expect(chat.run.status).toBe('finished');
    record('egress-refused', s, before, fixture('egress-refused').events.length);
  });

  it('write-interrupt then write-resume-approved: the run ends interrupted, and one approval resumes it', async () => {
    const before = server.requests.length;
    const s = sessionFor('write-interrupt', ['run-1', 'run-2']);
    await s.send(userText('write-interrupt'));

    let snap = s.getSnapshot();
    expect(snap.chat.run.status).toBe('interrupted');
    expect(snap.chat.toolCalls[0]).toMatchObject({ id: 'tc-7', name: 'pin_add', status: 'interrupted' });
    expect(snap.chat.interrupts).toHaveLength(1);
    expect(snap.chat.interrupts[0]?.interrupt.metadata['ipfs-mcp']).toMatchObject({ tool: 'pin_add', class: 'write', requires_native_prompt: false });
    expect(server.requests.length - before).toBe(1); // nothing but the first run has been sent

    const interruptId = snap.chat.interrupts[0]?.interrupt.id ?? '';
    expect(await s.answer(interruptId, 'approve')).toEqual({ ok: true });

    snap = s.getSnapshot();
    expect(matchedSince(before)).toEqual(['write-interrupt', 'write-resume-approved']);
    expect(snap.error).toBeUndefined();
    expect(snap.chat.interrupts[0]?.resolution).toBe('approved');
    expect(snap.chat.toolCalls[0]).toMatchObject({ id: 'tc-7', status: 'done' });
    expect(snap.chat.toolCalls).toHaveLength(1); // the resumed run did not re-emit the call
    expect(snap.chat.messages.at(-1)?.content).toBe('Pinned.');
    expect(snap.chat.run.status).toBe('finished');
    // The resume's history (matched byte for byte above) carried msg-a1 from the snapshot with its call.
    expect(snap.chat.messages.map((m) => m.id)).toEqual(['msg-u1', 'msg-a1', 'msg-a2']);
    record('write-interrupt', s, before, fixture('write-interrupt').events.length + fixture('write-resume-approved').events.length);
  });

  it('write-interrupt then write-resume-denied: a deny resumes with approved: false and the call ends declined', async () => {
    const before = server.requests.length;
    const s = sessionFor('write-interrupt', ['run-1', 'run-2']);
    await s.send(userText('write-interrupt'));
    const id = s.getSnapshot().chat.interrupts[0]?.interrupt.id ?? '';
    expect(await s.answer(id, 'deny')).toEqual({ ok: true });

    const snap = s.getSnapshot();
    expect(matchedSince(before)).toEqual(['write-interrupt', 'write-resume-denied']);
    expect(server.requests.at(-1)?.body).toMatchObject({
      runId: 'run-2',
      tools: [],
      resume: [{ interruptId: id, status: 'resolved', payload: { approved: false } }],
    });
    expect(snap.error).toBeUndefined();
    expect(snap.chat.interrupts[0]?.resolution).toBe('denied');
    expect(snap.chat.toolCalls).toHaveLength(1); // the call is not re-emitted
    expect(snap.chat.toolCalls[0]).toMatchObject({ id: 'tc-7', name: 'pin_add', status: 'error' });
    expect(snap.chat.toolCalls[0]?.resultText).toContain('confirmation_declined');
    expect(snap.chat.messages.at(-1)?.content).toBe('Understood, I did not pin it.');
    expect(snap.chat.run.status).toBe('finished');
    record('write-interrupt', s, before, fixture('write-interrupt').events.length + fixture('write-resume-denied').events.length, 'deny answered by write-resume-denied');
  });

  it('two-calls-write-interrupt then two-calls-resume-then-native-interrupt: one interrupt per run, one resume entry, the queued native call is the next run\'s interrupt', async () => {
    const before = server.requests.length;
    const s = sessionFor('two-calls-write-interrupt', ['run-1', 'run-2', 'run-3']);
    await s.send(userText('two-calls-write-interrupt'));

    let snap = s.getSnapshot();
    expect(snap.chat.run.status).toBe('interrupted');
    expect(snap.chat.toolCalls.map((c) => c.name)).toEqual(['pin_add', 'repo_gc']); // both announced in run-1
    expect(snap.chat.interrupts).toHaveLength(1); // at most one per run
    expect(snap.chat.interrupts[0]?.interrupt.metadata['ipfs-mcp']).toMatchObject({ tool: 'pin_add', class: 'write', requires_native_prompt: false });
    expect(server.requests.length - before).toBe(1);

    const writeId = snap.chat.interrupts[0]?.interrupt.id ?? '';
    expect(await s.answer(writeId, 'approve')).toEqual({ ok: true });

    // The resume carries exactly the approval: no {wait: true} rides along with it.
    const resume = server.requests[before + 1]?.body as { resume?: unknown[] };
    expect(server.requests[before + 1]?.matched).toBe('two-calls-resume-then-native-interrupt');
    expect(resume.resume).toHaveLength(1);
    expect(resume.resume).toEqual([{ interruptId: writeId, status: 'resolved', payload: { approved: true } }]);

    // The session then watches the native interrupt in a run of its own, again with a single entry. That
    // third request is two-calls-native-wait-approved: {wait: true} on the queued repo_gc interrupt,
    // answered by the system prompt, so the run ends with the tool result and the final text.
    const third = server.requests[before + 2];
    expect(third?.matched).toBe('two-calls-native-wait-approved');
    expect(third?.body).toMatchObject({ runId: 'run-3', resume: [{ interruptId: 'int-cf_Tw0CallsGcNat1veXy', status: 'resolved', payload: { wait: true } }] });
    expect((third?.body as { resume: unknown[] }).resume).toHaveLength(1);
    expect(matchedSince(before)).toEqual(['two-calls-write-interrupt', 'two-calls-resume-then-native-interrupt', 'two-calls-native-wait-approved']);

    snap = s.getSnapshot();
    expect(snap.error).toBeUndefined();
    expect(snap.observing).toEqual([]);
    expect(snap.chat.interrupts.map((i) => [i.interrupt.metadata['ipfs-mcp'].tool, i.resolution])).toEqual([
      ['pin_add', 'approved'],
      ['repo_gc', 'approved'], // reported by the result on tc-8, not by a click
    ]);
    expect(snap.chat.toolCalls).toHaveLength(2); // tc-8 was not re-emitted
    expect(snap.chat.toolCalls[1]).toMatchObject({ id: 'tc-8', name: 'repo_gc', status: 'done' });
    expect(snap.chat.toolCalls[1]?.resultText).toContain('removed_count');
    expect(snap.chat.messages.at(-1)?.content).toBe('Pinned it, and garbage collection removed 42 blocks.');
    expect(snap.chat.run.status).toBe('finished');
    record(
      'two-calls-write-interrupt',
      s,
      before,
      fixture('two-calls-write-interrupt').events.length + fixture('two-calls-resume-then-native-interrupt').events.length + fixture('two-calls-native-wait-approved').events.length,
      'single-entry resumes; the third request ({wait: true}) is answered by two-calls-native-wait-approved',
    );
  });

  for (const [name, code, text] of [
    ['run-error-step-limit', 'max_steps', 'The agent reached its step limit before it finished. Ask again with a narrower request.'],
    ['run-error-backend-failure', 'backend_error', 'The model backend failed. Nothing on your node changed because of this error. Try again in a moment.'],
  ] as const) {
    it(`${name}: the run ends in error by code, with fixed copy and not the wire message`, async () => {
      const before = server.requests.length;
      const s = sessionFor(name, ['run-1']);
      await s.send(userText(name));
      const snap = s.getSnapshot();
      expect(matchedSince(before)).toEqual([name]);
      expect(snap.error).toBeUndefined(); // a RUN_ERROR is a run outcome, not a transport failure
      expect(snap.chat.run).toMatchObject({ status: 'error', errorCode: code, error: text });
      const wire = fixture(name).events.find((e) => e.type === 'RUN_ERROR') as { message: string } | undefined;
      expect(wire?.message).toBeTruthy();
      expect(snap.chat.run.error).not.toBe(wire?.message);
      record(name, s, before, fixture(name).events.length, `RUN_ERROR ${code}`);
    });
  }

  it('destructive-native-prompt then wait-approved: the island watches the system prompt and shows the approved result', async () => {
    const before = server.requests.length;
    const s = sessionFor('destructive-native-prompt', ['run-1', 'run-2']);
    await s.send(userText('destructive-native-prompt'));

    // The island sent {wait: true} by itself after showing the interrupt; no click was involved.
    expect(matchedSince(before)).toEqual(['destructive-native-prompt', 'destructive-native-prompt-wait-approved']);
    expect(server.requests.at(-1)?.body).toMatchObject({
      runId: 'run-2',
      resume: [{ interruptId: 'int-cf_Ui7dEstrUct1veXy', status: 'resolved', payload: { wait: true } }],
    });
    const snap = s.getSnapshot();
    expect(snap.error).toBeUndefined();
    expect(snap.chat.interrupts[0]?.interrupt.metadata['ipfs-mcp']).toMatchObject({ tool: 'repo_gc', class: 'destructive', requires_native_prompt: true });
    expect(snap.chat.interrupts[0]?.resolution).toBe('approved'); // reported by the result, not by a click
    expect(snap.chat.toolCalls).toHaveLength(1);
    expect(snap.chat.toolCalls[0]).toMatchObject({ id: 'tc-3', name: 'repo_gc', status: 'done' });
    expect(snap.chat.toolCalls[0]?.resultText).toContain('removed_count');
    expect(snap.chat.messages.at(-1)?.content).toBe('Garbage collection removed 42 blocks.');
    expect(snap.chat.run.status).toBe('finished');
    expect(snap.observing).toEqual([]);

    // Approving from the chat is refused whatever the state, and nothing is sent.
    const sent = server.requests.length;
    const id = snap.chat.interrupts[0]?.interrupt.id ?? '';
    expect(await s.answer(id, 'approve')).toEqual({ ok: false, reason: 'native_prompt' });
    expect(server.requests.length).toBe(sent);
    record(
      'destructive-native-prompt',
      s,
      before,
      fixture('destructive-native-prompt').events.length + fixture('destructive-native-prompt-wait-approved').events.length,
      'wait-approved: {wait: true} sent by the session; approve from the chat refused, nothing sent',
    );
  });

  for (const [outcome, resolution, status, code] of [
    ['destructive-native-prompt-wait-denied', 'denied', 'error', 'confirmation_declined'],
    ['destructive-native-prompt-wait-expired', 'expired', 'error', 'confirmation_expired'],
  ] as const) {
    it(`destructive-native-prompt then ${outcome.replace('destructive-native-prompt-', '')}: the card ends ${resolution} on the original call`, async () => {
      const x = await serverWithOutcome(outcome);
      const s = sessionFor('destructive-native-prompt', ['run-1', 'run-2'], x);
      await s.send(userText('destructive-native-prompt'));

      expect(x.requests.map((r) => r.matched)).toEqual(['destructive-native-prompt', outcome]);
      const snap = s.getSnapshot();
      expect(snap.error).toBeUndefined();
      expect(snap.chat.interrupts[0]?.resolution).toBe(resolution);
      expect(snap.chat.toolCalls).toHaveLength(1); // never re-emitted
      expect(snap.chat.toolCalls[0]).toMatchObject({ id: 'tc-3', status });
      expect(snap.chat.toolCalls[0]?.resultText).toContain(code);
      expect(snap.chat.run.status).toBe('finished');
      // Declined and expired are denials; nothing is left waiting, so the person can write again.
      expect(snap.chat.interrupts.every((i) => i.resolution !== undefined)).toBe(true);
      record('destructive-native-prompt', s, 0, fixture('destructive-native-prompt').events.length + fixture(outcome).events.length, `${outcome}: ${resolution}`, x);
    });
  }

  it('every request in this file went to the one configured origin and path', () => {
    for (const x of [server, ...extraServers]) {
      for (const r of x.requests) {
        expect(r.method).toBe('POST');
        expect(r.url).toBe('/agui');
        expect(r.headers.host).toBe(new URL(x.origin).host);
        expect(r.headers.authorization).toBe(`Bearer ${TOKEN}`);
      }
    }
    expect(allRequests().length).toBeGreaterThan(8);
  });

  afterAll(() => {
    const dir = resolve(import.meta.dirname, '../../evidence');
    mkdirSync(dir, { recursive: true });
    writeFileSync(
      resolve(dir, 'replay.json'),
      JSON.stringify(
        {
          source: 'mcp/contract/fixtures/agui/*.json',
          server: 'tests/helpers/fixture-server.ts, 127.0.0.1, port chosen by the OS per run',
          liveIpfsMcp: false,
          results,
        },
        null,
        2,
      ) + '\n',
    );
  });
});
