import { expect, test } from './harness';
import { snapshotHost } from './helpers';

// Falsifier G11-1 (docs/mcp/0005), first half: "Web UI pages are unchanged with the island loaded but
// closed". The host page is a Tachyons page with hostile global rules (harness/host.css). The same page is
// opened without the island and with it loaded and closed; the two must be pixel-identical and
// structurally identical, in both color schemes at three widths, in Chromium and in Electron 44.

const WIDTHS = [320, 768, 1024] as const;
const SCHEMES = ['light', 'dark'] as const;

for (const width of WIDTHS) {
  for (const scheme of SCHEMES) {
    test(`a loaded, closed island leaves the host page identical at ${width}px, ${scheme}`, async ({ harness }) => {
      const baseline = await harness.open({ island: 'none', width, scheme });
      const before = await snapshotHost(baseline);
      // One baseline per width and scheme; the closed page is compared against the very same image.
      await expect(baseline).toHaveScreenshot(`host-${width}-${scheme}.png`, { fullPage: true, maxDiffPixels: 0 });

      const closed = await harness.open({ island: 'closed', width, scheme });
      await expect(closed.locator('ipfs-agent-chat')).toHaveCount(1);
      const after = await snapshotHost(closed);

      expect(after.head).toBe(before.head);
      expect(after.body).toBe(before.body);
      expect(after.sheets).toBe(before.sheets);
      expect(after.adopted).toBe(before.adopted);
      expect(after.fontFaces).toBe(before.fontFaces);
      expect(after.computed).toEqual(before.computed);
      expect(after.rects).toEqual(before.rects);
      // The stand-in Web UI page scrolls sideways at 320px on its own; the island must add nothing to that.
      expect(after.scrollWidth).toBeLessThanOrEqual(Math.max(before.scrollWidth, after.clientWidth));
      await expect(closed).toHaveScreenshot(`host-${width}-${scheme}.png`, { fullPage: true, maxDiffPixels: 0 });
    });
  }
}

test('a closed island loads its small entry and nothing else', async ({ harness }) => {
  // The harness server's own log: a page listener sees nothing in Electron, which opens a fresh window per open().
  await harness.open({ island: 'closed' });
  const paths = (await harness.harnessRequests()).filter((p) => p.startsWith('/island/'));
  // The entry plus the tiny endpoint-validation chunk it imports statically. The mount chunk (React and
  // the UI) and the embedded fonts load only when the sidebar first opens.
  expect(paths[0]).toBe('/island/ipfs-agent-chat.js');
  expect(paths.slice(1)).toEqual([expect.stringMatching(/^\/island\/chunks\/endpoint-[\w-]+\.js$/)]);
  expect(await harness.fixtureRequests()).toEqual([]); // the agent endpoint is never contacted before the sidebar opens
});

test('opening and closing the sidebar changes nothing on the host page but the faces the island registers', async ({ harness }) => {
  const page = await harness.open({ island: 'closed', scheme: 'dark' });
  const before = await snapshotHost(page);

  await page.getByRole('button', { name: 'Ask the agent' }).click();
  await expect(page.locator('ipfs-agent-chat aside[data-state="open"]')).toBeVisible();
  const open = await snapshotHost(page);
  // The sidebar is a fixed overlay inside a shadow root: the host reflows nothing and gains no node, style or rule.
  expect(open.rects).toEqual(before.rects);
  expect(open.computed).toEqual(before.computed);
  // The host's own launcher toggles aria-expanded; that is the host reacting, not the island writing.
  const body = (s: string) => s.replace(/aria-expanded="(true|false)"/g, 'aria-expanded=""');
  expect(body(open.body)).toBe(body(before.body));
  expect(open.head).toBe(before.head);
  expect(open.sheets).toBe(before.sheets);
  expect(open.adopted).toBe(before.adopted);
  // The one document-level effect: the brand faces are registered with document.fonts from embedded bytes.
  const families = await page.evaluate(() => [...document.fonts].map((f) => f.family.replaceAll('"', '')));
  expect(new Set(families)).toEqual(new Set(['IPFS UI Syne', 'IPFS UI Hanken Grotesk', 'IPFS UI JetBrains Mono']));

  await page.keyboard.press('Escape');
  await expect(page.locator('ipfs-agent-chat aside[data-state="closed"]')).toBeAttached();
  const closed = await snapshotHost(page);
  expect(closed.rects).toEqual(before.rects);
  expect(closed.computed).toEqual(before.computed);
  expect(closed.head).toBe(before.head);
  await expect(page.locator('#chat-launcher')).toBeFocused(); // focus returns to the opener
});

test('hostile global rules do not reach the sidebar', async ({ harness }) => {
  const page = await harness.open({ island: 'open', scheme: 'dark' });
  const heading = page.getByRole('heading', { name: 'Agent chat' });
  const send = page.getByRole('button', { name: 'Send message' });
  const box = page.getByRole('textbox', { name: 'Message to the agent' });
  const css = async (l: typeof heading, props: string[]) => l.evaluate((el, p) => Object.fromEntries(p.map((k) => [k, getComputedStyle(el)[k as keyof CSSStyleDeclaration]])), props);

  // host.css sets: h* uppercase, wide tracking, magenta; button/input/textarea lime with a red 3px border and 18px padding.
  expect(await css(heading, ['textTransform', 'letterSpacing', 'color'])).toMatchObject({ textTransform: 'none', letterSpacing: 'normal', color: 'rgb(245, 242, 238)' });
  expect(await css(send, ['borderTopWidth', 'borderRadius', 'backgroundColor'])).toMatchObject({ borderTopWidth: '0px', borderRadius: '6px' });
  expect(await css(box, ['borderTopWidth', 'borderRadius', 'backgroundColor', 'paddingTop'])).toMatchObject({ borderTopWidth: '0px', borderRadius: '6px', backgroundColor: 'rgb(45, 44, 42)' });
});
