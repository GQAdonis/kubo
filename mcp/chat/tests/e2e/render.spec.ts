import { expect, test } from './harness';
import { RGB, ask, userTextOf } from './helpers';

// Falsifier G11-1, second half: the island "renders styled" in Chromium and in Electron 44, in both
// themes, at the widths the Web UI is used at. Styled means: the Promethity tokens apply, the brand faces
// load, the panel sits where a right-side sidebar sits, nothing overflows, and the host's hostile global
// rules do not reach it. Screenshots carry the state coverage; baselines are created on the first run of
// the gate (`pnpm test:visual:update`) and reviewed there.

const WIDTHS = [320, 768, 1024, 1440] as const;

for (const scheme of ['dark', 'light'] as const) {
  for (const width of WIDTHS) {
    test(`renders styled and in place at ${width}px, ${scheme}`, async ({ harness }) => {
      const page = await harness.open({ island: 'open', scheme, width, height: 800 });
      const aside = page.locator('ipfs-agent-chat aside');
      const tokens = RGB[scheme];

      const box = await aside.boundingBox();
      expect(box).not.toBeNull();
      expect(Math.round((box?.x ?? 0) + (box?.width ?? 0))).toBe(width); // flush with the right edge
      expect(box?.width ?? 0).toBeLessThanOrEqual(Math.min(416, width));
      expect(Math.round(box?.height ?? 0)).toBe(800);

      const style = await aside.evaluate((el) => {
        const cs = getComputedStyle(el);
        return { position: cs.position, background: cs.backgroundColor, color: cs.color, visibility: cs.visibility, transform: cs.transform };
      });
      expect(style).toMatchObject({ position: 'fixed', background: tokens.surface, color: tokens.text, visibility: 'visible' });
      expect(['none', 'matrix(1, 0, 0, 1, 0, 0)']).toContain(style.transform);

      // The footer is a `raised` well, the message box a `control` fill, Send the one accent fill.
      expect(await page.locator('ipfs-agent-chat footer').evaluate((el) => getComputedStyle(el).backgroundColor)).toBe(tokens.raised);
      const field = page.getByRole('textbox', { name: 'Message to the agent' });
      expect(await field.evaluate((el) => getComputedStyle(el).backgroundColor)).toBe(tokens.control);
      expect((await field.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
      expect(await page.getByRole('button', { name: 'Send message' }).evaluate((el) => getComputedStyle(el).backgroundColor)).toBe(tokens.accent);

      // The page does not scroll sideways because of the island.
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

      // Brand faces, from embedded bytes.
      await page.evaluate(() => document.fonts.ready);
      const loaded = await page.evaluate(() => [...document.fonts].filter((f) => f.status === 'loaded').map((f) => f.family.replaceAll('"', '')));
      expect(loaded).toEqual(expect.arrayContaining(['IPFS UI Syne', 'IPFS UI Hanken Grotesk']));
      expect(await page.getByRole('heading', { name: 'Agent chat' }).evaluate((el) => getComputedStyle(el).fontFamily)).toMatch(/^"?IPFS UI Syne/);
      expect(await page.getByText('Ask about your node').first().evaluate((el) => getComputedStyle(el).fontFamily)).toMatch(/^"?IPFS UI Syne/);
      expect(await page.getByText(/Reads run at once/).evaluate((el) => getComputedStyle(el).fontFamily)).toMatch(/^"?IPFS UI Hanken Grotesk/);
    });
  }
}

test('slides in with a transform and stops moving for people who ask for no motion', async ({ harness, runtime }) => {
  const page = await harness.open({ island: 'closed', scheme: 'dark' });
  const aside = page.locator('ipfs-agent-chat aside');
  // Closed: off the right edge, hidden, inert.
  await page.getByRole('button', { name: 'Ask the agent' }).click();
  await expect(aside).toHaveAttribute('data-state', 'open');
  expect(await aside.evaluate((el) => getComputedStyle(el).transitionProperty)).toContain('transform');
  expect(await aside.evaluate((el) => getComputedStyle(el).transitionDuration)).toContain('0.18s');

  if (runtime === 'chromium') {
    await page.emulateMedia({ reducedMotion: 'reduce' });
    expect(await aside.evaluate((el) => getComputedStyle(el).transitionDuration.split(',').every((d) => d.trim() === '0s'))).toBe(true);
  }
});

test('a closed sidebar is unreachable: hidden, inert, and out of the tab order', async ({ harness }) => {
  const page = await harness.open({ island: 'closed' });
  const aside = page.locator('ipfs-agent-chat aside');
  await expect(aside).toHaveAttribute('data-state', 'closed');
  await expect(aside).toHaveAttribute('inert', '');
  expect(await aside.evaluate((el) => getComputedStyle(el).visibility)).toBe('hidden');
  await page.locator('#chat-launcher').focus();
  for (let i = 0; i < 12; i++) {
    await page.keyboard.press('Tab');
    expect(await page.evaluate(() => document.activeElement?.tagName)).not.toBe('IPFS-AGENT-CHAT');
  }
});

// ---- state coverage, as screenshots. Each state is reached through the real controls.

const STATES = [
  { name: 'empty', fixture: undefined, settle: /Reads run at once/ },
  { name: 'result', fixture: 'read-pin-ls', settle: /You have one recursive pin/ },
  { name: 'pending-approval', fixture: 'write-interrupt', settle: /Awaiting a person/ },
  { name: 'native-prompt', fixture: 'destructive-native-prompt', settle: /Waiting for system prompt/ },
  { name: 'egress-refused', fixture: 'egress-refused', settle: /I cannot read that file/ },
] as const;

for (const scheme of ['dark', 'light'] as const) {
  for (const width of [320, 1024] as const) {
    for (const state of STATES) {
      test(`state ${state.name}, ${scheme}, ${width}px`, async ({ harness }) => {
        const page = await harness.open({ island: 'open', scheme, width, height: 900 });
        if (state.fixture) {
          await ask(page, userTextOf(state.fixture));
          // A native-prompt state is a run held open on {wait: true}; every other state has finished.
          if (state.fixture !== 'destructive-native-prompt') await expect(page.getByRole('button', { name: 'Stop run' })).toHaveCount(0);
        }
        await expect(page.getByText(state.settle).first()).toBeVisible();
        // The sidebar alone: the host page behind it is covered by isolation.spec.ts.
        await expect(page.locator('ipfs-agent-chat aside')).toHaveScreenshot(`${state.name}-${scheme}-${width}.png`);
      });
    }
  }
}
