// @vitest-environment node
import { describe, expect, it, vi } from 'vitest';
import { OriginRefusedError, SUGGESTED_AGUI_ENDPOINT, checkEndpoint, originGuardedFetch } from '../../src/agui/endpoint';

describe('checkEndpoint', () => {
  it.each([
    ['https://node.example.net/agui'],
    ['http://127.0.0.1:41233/agui'],
    ['http://localhost:9/agui'],
    ['http://[::1]:7/agui'],
    ['https://node.example.net/prefix/agui'],
    [SUGGESTED_AGUI_ENDPOINT],
  ])('accepts %s', (url) => {
    expect(checkEndpoint(url).ok).toBe(true);
  });

  it.each([
    [undefined, /No endpoint/],
    ['', /No endpoint/],
    ['not a url', /absolute URL/],
    ['/agui', /absolute URL/],
    ['ftp://127.0.0.1/agui', /https, or http on a loopback/],
    ['http://node.example.net/agui', /clear text/],
    ['https://user:pw@node.example.net/agui', /credentials/],
    ['https://node.example.net/agui?token=abc', /query or fragment/],
    ['https://node.example.net/agui#x', /query or fragment/],
    ['https://node.example.net/mcp', /path must end in \/agui/],
  ])('refuses %s', (url, reason) => {
    const check = checkEndpoint(url);
    expect(check.ok).toBe(false);
    if (!check.ok) expect(check.reason).toMatch(reason);
  });

  it('exposes the origin it will talk to', () => {
    const c = checkEndpoint('http://127.0.0.1:41233/agui');
    expect(c.ok && c.origin).toBe('http://127.0.0.1:41233');
  });
});

describe('originGuardedFetch', () => {
  it('passes a request to the configured origin through', async () => {
    const impl = vi.fn(async () => new Response('ok'));
    const guarded = originGuardedFetch('http://127.0.0.1:41233', impl as unknown as typeof fetch);
    await guarded('http://127.0.0.1:41233/agui', { method: 'POST' });
    expect(impl).toHaveBeenCalledOnce();
  });

  it.each(['http://127.0.0.1:41234/agui', 'https://elsewhere.example.net/agui', 'http://localhost:41233/agui', '/agui'])(
    'throws before sending to %s',
    (target) => {
      const impl = vi.fn(async () => new Response('ok'));
      const guarded = originGuardedFetch('http://127.0.0.1:41233', impl as unknown as typeof fetch);
      expect(() => guarded(target)).toThrow(OriginRefusedError);
      expect(impl).not.toHaveBeenCalled();
    },
  );
});
