import { expect, test } from './harness';
import { ask, userTextOf } from './helpers';

// Falsifier G11-4 (docs/mcp/0005), at run time: under a deny-by-default CSP with no 'unsafe-eval' and no
// 'unsafe-inline', the island works, loads nothing remote, makes requests only to the harness origin (its own
// files) and the one configured ipfs mcp origin, raises no securitypolicyviolation, and leaves the bearer in
// no storage, URL or attribute. In Electron 44 the page is webui://localhost, as in Desktop.

test('works under a strict CSP and makes only the requests it should', async ({ harness }) => {
  const page = await harness.open({ island: 'closed', csp: 'strict' });
  const violations: string[] = [];
  const requests: { url: string; method: string }[] = [];
  const consoleErrors: string[] = [];
  await page.exposeFunction('__reportViolation', (v: string) => violations.push(v));
  await page.evaluate(() => {
    document.addEventListener('securitypolicyviolation', (e) => (window as unknown as { __reportViolation: (v: string) => void }).__reportViolation(`${e.violatedDirective} ${e.blockedURI}`));
  });
  page.on('request', (r) => requests.push({ url: r.url(), method: r.method() }));
  page.on('console', (m) => m.type() === 'error' && consoleErrors.push(m.text()));

  await page.getByRole('button', { name: 'Ask the agent' }).click();
  await expect(page.locator('ipfs-agent-chat aside[data-state="open"]')).toBeVisible();
  // A read turn, a write that stops at an approval, and an approved resume.
  await ask(page, userTextOf('read-pin-ls'));
  await expect(page.getByRole('table', { name: 'Pins' })).toBeVisible();
  await ask(page, userTextOf('write-interrupt'));
  await expect(page.getByText('Awaiting a person')).toBeVisible();
  await page.getByRole('button', { name: 'Approve change' }).click();
  await expect(page.getByText('Pinned.')).toBeVisible();

  expect(violations).toEqual([]);
  expect(consoleErrors.filter((e) => /Content Security Policy|unsafe-eval|Refused to/i.test(e))).toEqual([]);

  // Every request: the harness origin (page files, chunks) or the one ipfs mcp origin. Nothing else.
  const pageOrigin = new URL(page.url()).origin;
  const origins = new Set(requests.map((r) => new URL(r.url).origin));
  expect([...origins].filter((o) => o !== pageOrigin && o !== harness.env.harnessOrigin && o !== harness.env.fixtureOrigin)).toEqual([]);

  // To ipfs mcp: POST /agui only (the CORS preflight aside), with the bearer in the header, never in the URL.
  const toMcp = requests.filter((r) => r.url.startsWith(harness.env.fixtureOrigin));
  expect(toMcp.length).toBeGreaterThan(0);
  for (const r of toMcp) {
    expect(new URL(r.url).pathname).toBe('/agui');
    expect(['POST', 'OPTIONS']).toContain(r.method);
    expect(r.url).not.toContain(harness.env.token);
  }
  for (const r of await harness.fixtureRequests()) {
    expect(r.url).toBe('/agui');
    expect(r.headers['authorization']).toBe(`Bearer ${harness.env.token}`);
    expect(r.headers['cookie']).toBeUndefined();
  }
});

test('leaves the bearer in no storage, attribute, URL or markup', async ({ harness }) => {
  const page = await harness.open({ island: 'open', csp: 'strict' });
  await ask(page, userTextOf('read-pin-ls'));
  await expect(page.getByRole('table', { name: 'Pins' })).toBeVisible();

  const probe = await page.evaluate(async (token) => {
    const dbs = (await indexedDB.databases?.()) ?? [];
    const host = document.querySelector('ipfs-agent-chat') as HTMLElement & { token?: unknown };
    return {
      local: localStorage.length,
      session: sessionStorage.length,
      cookie: document.cookie,
      databases: dbs.length,
      caches: (await caches?.keys?.())?.length ?? 0,
      inDocument: document.documentElement.outerHTML.includes(token),
      inShadow: (host.shadowRoot?.innerHTML ?? '').includes(token),
      inUrl: location.href.includes(token),
      hostAttributes: host.getAttributeNames().join(','),
      tokenReadBack: host.token,
      tokenInJson: JSON.stringify(host).includes(token),
    };
  }, harness.env.token);
  expect(probe).toEqual({
    local: 0,
    session: 0,
    cookie: '',
    databases: 0,
    caches: 0,
    inDocument: false,
    inShadow: false,
    inUrl: false,
    hostAttributes: expect.not.stringContaining('token'),
    tokenReadBack: undefined,
    tokenInJson: false,
  });
});

test('refuses to use a token set as an attribute, and removes it', async ({ harness }) => {
  const page = await harness.open({ island: 'closed' });
  const result = await page.evaluate(() => {
    const el = document.createElement('ipfs-agent-chat');
    el.setAttribute('token', 'attribute-token-0123456789');
    document.body.append(el);
    return { stillThere: el.hasAttribute('token'), tokenSet: (el as unknown as { tokenSet: boolean }).tokenSet };
  });
  expect(result).toEqual({ stillThere: false, tokenSet: false });
});
