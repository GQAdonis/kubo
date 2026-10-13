// @vitest-environment node
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AguiError, createAguiClient } from '../../src/agui/client';
import { checkEndpoint } from '../../src/agui/endpoint';
import { fixture } from '../helpers/fixtures';
import { startFixtureServer, type FixtureServer } from '../helpers/fixture-server';

const TOKEN = 'test-bearer-0123456789abcdef';

let server: FixtureServer;
beforeAll(async () => {
  server = await startFixtureServer({ token: TOKEN, fragmentBytes: 5, keepAlive: true });
});
afterAll(() => server.close());

const endpoint = () => {
  const c = checkEndpoint(server.endpoint);
  if (!c.ok) throw new Error(c.reason);
  return c.url;
};

async function collect(gen: AsyncGenerator<unknown>): Promise<unknown[]> {
  const out: unknown[] = [];
  for await (const e of gen) out.push(e);
  return out;
}

describe('AG-UI client against a local fixture server', () => {
  it('posts the RunAgentInput and yields the fixture events, even when frames are split across reads', async () => {
    const f = fixture('read-pin-ls');
    const client = createAguiClient({ endpoint: endpoint(), getToken: () => TOKEN });
    const events = await collect(client.run(f.input as never));
    expect(events).toEqual(f.events);
  });

  it('sends the bearer in the Authorization header only: not in the URL, and no cookie', async () => {
    const before = server.requests.length;
    const f = fixture('read-pin-ls');
    await collect(createAguiClient({ endpoint: endpoint(), getToken: () => TOKEN }).run(f.input as never));
    const req = server.requests[before];
    expect(req).toBeDefined();
    expect(req?.url).toBe('/agui');
    expect(req?.url).not.toContain(TOKEN);
    expect(req?.headers.authorization).toBe(`Bearer ${TOKEN}`);
    expect(req?.headers.cookie).toBeUndefined();
    expect(req?.headers.referer).toBeUndefined();
    expect(req?.headers.accept).toBe('text/event-stream');
    expect(req?.headers['content-type']).toBe('application/json');
    expect(req?.matched).toBe('read-pin-ls');
  });

  it('awaits an async token provider on every request', async () => {
    let calls = 0;
    const client = createAguiClient({
      endpoint: endpoint(),
      getToken: async () => {
        calls++;
        return TOKEN;
      },
    });
    const f = fixture('read-pin-ls');
    await collect(client.run(f.input as never));
    await collect(client.run(f.input as never));
    expect(calls).toBe(2);
  });

  it('refuses to send with no token, and sends nothing', async () => {
    const before = server.requests.length;
    const client = createAguiClient({ endpoint: endpoint(), getToken: () => undefined });
    await expect(collect(client.run(fixture('read-pin-ls').input as never))).rejects.toMatchObject({ kind: 'config' });
    expect(server.requests.length).toBe(before);
  });

  it('maps 401 to unauthorized', async () => {
    const client = createAguiClient({ endpoint: endpoint(), getToken: () => 'wrong' });
    await expect(collect(client.run(fixture('read-pin-ls').input as never))).rejects.toMatchObject({ kind: 'unauthorized', status: 401 });
  });

  it('maps 404 (endpoint not mounted) to not_found, the off switch', async () => {
    const url = new URL(server.origin + '/prefix/agui');
    const client = createAguiClient({ endpoint: url, getToken: () => TOKEN });
    await expect(collect(client.run(fixture('read-pin-ls').input as never))).rejects.toMatchObject({ kind: 'not_found' });
  });

  it('maps a body that matches no recorded input to an http error', async () => {
    const client = createAguiClient({ endpoint: endpoint(), getToken: () => TOKEN });
    const bad = { ...fixture('read-pin-ls').input, runId: 'run-unknown' };
    await expect(collect(client.run(bad as never))).rejects.toMatchObject({ kind: 'http', status: 422 });
  });

  it('maps a refused connection to network', async () => {
    const dead = await startFixtureServer({ token: TOKEN });
    const url = new URL(dead.endpoint);
    await dead.close();
    const client = createAguiClient({ endpoint: url, getToken: () => TOKEN });
    await expect(collect(client.run(fixture('read-pin-ls').input as never))).rejects.toMatchObject({ kind: 'network' });
  });

  it('refuses a response that is not an event stream', async () => {
    const impl = (async () => new Response('{"ok":true}', { status: 200, headers: { 'content-type': 'application/json' } })) as typeof fetch;
    const client = createAguiClient({ endpoint: endpoint(), getToken: () => TOKEN, fetch: impl });
    await expect(collect(client.run(fixture('read-pin-ls').input as never))).rejects.toMatchObject({ kind: 'protocol' });
  });

  it('refuses an event that is not JSON or has no type', async () => {
    const respond = (text: string) =>
      (async () => new Response(text, { status: 200, headers: { 'content-type': 'text/event-stream' } })) as typeof fetch;
    const input = fixture('read-pin-ls').input as never;
    await expect(collect(createAguiClient({ endpoint: endpoint(), getToken: () => TOKEN, fetch: respond('data: nope\n\n') }).run(input))).rejects.toMatchObject({ kind: 'protocol' });
    await expect(collect(createAguiClient({ endpoint: endpoint(), getToken: () => TOKEN, fetch: respond('data: {"a":1}\n\n') }).run(input))).rejects.toMatchObject({ kind: 'protocol' });
  });

  it('does not follow a redirect to another origin', async () => {
    const seen: RequestInit[] = [];
    const impl = (async (_u: RequestInfo | URL, init?: RequestInit) => {
      seen.push(init ?? {});
      throw new TypeError('redirect mode is set to error');
    }) as typeof fetch;
    const client = createAguiClient({ endpoint: endpoint(), getToken: () => TOKEN, fetch: impl });
    await expect(collect(client.run(fixture('read-pin-ls').input as never))).rejects.toBeInstanceOf(AguiError);
    expect(seen[0]).toMatchObject({ redirect: 'error', credentials: 'omit', referrerPolicy: 'no-referrer', cache: 'no-store' });
  });

  it('aborts a run in flight', async () => {
    const slow = await startFixtureServer({ token: TOKEN, eventDelayMs: 200 });
    try {
      const ac = new AbortController();
      const client = createAguiClient({ endpoint: new URL(slow.endpoint), getToken: () => TOKEN });
      const got: unknown[] = [];
      const run = (async () => {
        for await (const e of client.run(fixture('read-pin-ls').input as never, ac.signal)) {
          got.push(e);
          ac.abort();
        }
      })();
      await expect(run).rejects.toMatchObject({ kind: 'aborted' });
      expect(got.length).toBeLessThan(fixture('read-pin-ls').events.length);
    } finally {
      await slow.close();
    }
  });
});
