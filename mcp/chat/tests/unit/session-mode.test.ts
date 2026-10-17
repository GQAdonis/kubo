// @vitest-environment node
import { describe, expect, it, vi } from 'vitest';
import { AguiError, SESSION_EXPIRED_MESSAGE, createAguiClient } from '../../src/agui/client';
import { checkEndpoint } from '../../src/agui/endpoint';
import { fixture, sseText } from '../helpers/fixtures';

// Session mode (console-session-auth 3.3): the cookie session is the credential. The request goes
// same-origin with credentials, carries no Authorization header, and never asks the host for a token.

const check = checkEndpoint('https://console.example.test/agui');
if (!check.ok) throw new Error(check.reason);
const endpoint = check.url;

async function drain(gen: AsyncGenerator<unknown>): Promise<unknown[]> {
  const out: unknown[] = [];
  for await (const e of gen) out.push(e);
  return out;
}

describe('AG-UI client in session mode', () => {
  it('sends same-origin credentials, no Authorization header, and never calls getToken', async () => {
    const f = fixture('read-pin-ls');
    const getToken = vi.fn(() => 'must-not-be-used');
    const seen: RequestInit[] = [];
    const fetchImpl = vi.fn(async (_url: RequestInfo | URL, init: RequestInit = {}) => {
      seen.push(init);
      return new Response(sseText(f.events), { status: 200, headers: { 'content-type': 'text/event-stream' } });
    });
    const client = createAguiClient({ endpoint, getToken, session: true, fetch: fetchImpl as never });
    await drain(client.run(f.input as never));
    expect(seen[0]!.credentials).toBe('same-origin');
    expect(new Headers(seen[0]!.headers).has('authorization')).toBe(false);
    expect(getToken).not.toHaveBeenCalled();
  });

  it('turns a 401 into the sign-in-again error', async () => {
    const f = fixture('read-pin-ls');
    const fetchImpl = vi.fn(async () => new Response('', { status: 401 }));
    const client = createAguiClient({ endpoint, getToken: () => undefined, session: true, fetch: fetchImpl as never });
    const err = await drain(client.run(f.input as never)).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(AguiError);
    expect((err as AguiError).kind).toBe('unauthorized');
    expect((err as AguiError).message).toBe(SESSION_EXPIRED_MESSAGE);
  });

  it('keeps the bearer path unchanged when session is not set', async () => {
    const f = fixture('read-pin-ls');
    const seen: RequestInit[] = [];
    const fetchImpl = vi.fn(async (_url: RequestInfo | URL, init: RequestInit = {}) => {
      seen.push(init);
      return new Response(sseText(f.events), { status: 200, headers: { 'content-type': 'text/event-stream' } });
    });
    await drain(createAguiClient({ endpoint, getToken: () => 'tok', fetch: fetchImpl as never }).run(f.input as never));
    expect(seen[0]!.credentials).toBe('omit');
    expect(new Headers(seen[0]!.headers).get('authorization')).toBe('Bearer tok');
  });
});
