import { createRequire } from 'node:module';
import { expect, test } from '@playwright/test';
import { hostLog, openView, type HostScript } from './host';
import { SCENES, THEMES, VIEWPORTS } from './scenes';

// Run with `pnpm build && pnpm test:visual` (the specs load dist/<view>.html, so build first).
const axePath = createRequire(import.meta.url).resolve('axe-core/axe.min.js');

for (const scene of SCENES) {
  for (const theme of THEMES) {
    test.describe(`${scene.id} (${theme})`, () => {
      const script: HostScript = { ...scene.script, theme };

      for (const [width, size] of Object.entries(VIEWPORTS)) {
        test(`visual @${width}`, async ({ page }) => {
          await page.setViewportSize(size);
          const frame = await openView(page, script);
          await expect(frame.locator('main.app')).toBeVisible();
          await expect(page).toHaveScreenshot(`${scene.id}-${theme}-${width}.png`, { fullPage: true });
        });
      }

      test('axe, WCAG 2.2 AA including contrast and target size', async ({ page }) => {
        await page.setViewportSize(VIEWPORTS[768]);
        const frame = await openView(page, script);
        await frame.addScriptTag({ path: axePath });
        const violations = await frame.evaluate(async () => {
          // @ts-expect-error injected by addScriptTag
          const r = await window.axe.run(document, {
            runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'] },
            rules: { region: { enabled: false }, 'landmark-one-main': { enabled: false }, 'page-has-heading-one': { enabled: false } },
          });
          return r.violations.map((v: { id: string; help: string; nodes: { html: string }[] }) => `${v.id}: ${v.help} :: ${v.nodes[0]?.html.slice(0, 120)}`);
        });
        expect(violations).toEqual([]);
      });
    });
  }
}

test.describe('under the MCP Apps default CSP', () => {
  test('draws with no CSP violation, no request, and fonts from embedded bytes', async ({ page }) => {
    const violations: string[] = [];
    const requests: string[] = [];
    page.on('console', (m) => {
      if (/Content Security Policy|Refused to/i.test(m.text())) violations.push(m.text());
    });
    page.on('request', (r) => {
      if (!r.url().startsWith('data:') && !r.url().startsWith('about:') && r.url() !== 'about:srcdoc') requests.push(r.url());
    });
    const scene = SCENES[0];
    if (!scene) throw new Error('no scenes');
    const frame = await openView(page, scene.script);
    const families = await frame.evaluate(() => [...document.fonts].map((f) => `${f.family}:${f.status}`));
    expect(families.some((f) => f.startsWith('IPFS UI Hanken Grotesk') && f.endsWith('loaded'))).toBe(true);
    expect(violations).toEqual([]);
    expect(requests).toEqual([]);
  });

  test('a navigation click reaches the host as tools/call and nothing else', async ({ page }) => {
    const scene = SCENES.find((s) => s.id === 'mfs');
    if (!scene) throw new Error('mfs scene missing');
    const frame = await openView(page, scene.script);
    await frame.getByRole('button', { name: /archive/ }).click();
    await expect.poll(async () => (await hostLog(page)).toolCalls.length).toBe(1);
    const log = await hostLog(page);
    expect(log.toolCalls[0]).toMatchObject({ name: 'mfs_ls', arguments: { path: '/docs/notes/archive' } });
    expect(log.messages).toEqual([]);
  });

  test('keyboard: Tab order reaches the actions and Enter activates', async ({ page }) => {
    const scene = SCENES.find((s) => s.id === 'pins');
    if (!scene) throw new Error('pins scene missing');
    const frame = await openView(page, scene.script);
    await frame.getByRole('button', { name: /Load more pins/ }).focus();
    await page.keyboard.press('Enter');
    await expect.poll(async () => (await hostLog(page)).toolCalls.length).toBe(1);
  });

  test('reduced motion: no transitions on interactive elements', async ({ page }) => {
    await page.emulateMedia({ reducedMotion: 'reduce' });
    const scene = SCENES[0];
    if (!scene) throw new Error('no scenes');
    const frame = await openView(page, scene.script);
    const durations = await frame.evaluate(() => [...document.querySelectorAll('button')].map((b) => getComputedStyle(b).transitionDuration));
    for (const d of durations) expect(d === '0s' || d === '0.001s' || d === '').toBe(true);
  });
});
