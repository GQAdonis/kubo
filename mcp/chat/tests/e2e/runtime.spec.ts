import { expect, test } from './harness';
import { ask, userTextOf } from './helpers';

// Pins which engine each project ran in. G11-1 names Electron 44 and the Chromium project must not be
// mistaken for it, so the version is asserted rather than assumed.
test('runs in the engine the project names', async ({ harness, runtime }) => {
  await harness.open({ island: 'closed' });
  const version = await harness.engineVersion();
  if (runtime === 'electron') expect(version).toMatch(/^electron 44\./);
  else expect(version).toMatch(/Chrome\/\d+/);
});

test('the Electron page is loaded from webui://localhost with web security on', async ({ harness, runtime }) => {
  test.skip(runtime !== 'electron', 'Desktop-shaped origin: Electron only');
  const page = await harness.open({ island: 'open', csp: 'strict' });
  expect(new URL(page.url()).protocol).toBe('webui:');
  expect(await page.evaluate(() => location.origin)).toBe('webui://localhost');
  // A fetch from webui:// to the loopback ipfs mcp origin works under the strict CSP, with CORS and a bearer header.
  await ask(page, userTextOf('read-pin-ls'));
  await expect(page.getByRole('table', { name: 'Pins' })).toBeVisible();
});
