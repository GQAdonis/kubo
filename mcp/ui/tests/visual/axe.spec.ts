import { createRequire } from 'node:module';
import { expect, test } from '@playwright/test';
import { SCENE_IDS, THEMES, VIEWPORTS, openScene } from './helpers';

// axe-core in a real browser, with the rules jsdom cannot run: color-contrast, target-size (WCAG 2.2
// 2.5.8), scrollable-region-focusable. WCAG 2.2 AA plus best practices, every scene, both themes, at
// 320 and 1024 px. The gallery is served with the MCP Apps default CSP, so this also proves the
// components need no inline-script, remote font or connection.
const require = createRequire(import.meta.url);
const axePath = require.resolve('axe-core/axe.min.js');

for (const theme of THEMES) {
  for (const width of [320, 1024] as const) {
    test(`axe: all scenes, ${theme}, ${width}px`, async ({ page }) => {
      await page.setViewportSize(VIEWPORTS[width]);
      const failures: string[] = [];
      for (const scene of SCENE_IDS) {
        await openScene(page, scene, theme);
        await page.addScriptTag({ path: axePath });
        const violations = await page.evaluate(async () => {
          // @ts-expect-error injected by addScriptTag
          const results = await window.axe.run(document, {
            runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'] },
          });
          return results.violations.map((v: { id: string; impact: string; help: string; nodes: { html: string; failureSummary?: string }[] }) => ({
            id: v.id,
            impact: v.impact,
            help: v.help,
            nodes: v.nodes.slice(0, 3).map((n) => `${n.html.slice(0, 140)} :: ${n.failureSummary?.split('\n').slice(0, 3).join(' ')}`),
          }));
        });
        for (const v of violations) failures.push(`[${scene}] ${v.id} (${v.impact}): ${v.help}\n  ${v.nodes.join('\n  ')}`);
      }
      expect(failures, failures.join('\n')).toEqual([]);
    });
  }
}

test('target size: every interactive control is at least 24 by 24 CSS pixels (WCAG 2.2 2.5.8)', async ({ page }) => {
  await page.setViewportSize(VIEWPORTS[320]);
  const small: string[] = [];
  for (const scene of SCENE_IDS) {
    await openScene(page, scene, 'dark');
    const found = await page.evaluate(() => {
      const out: string[] = [];
      for (const el of document.querySelectorAll('button, summary, [role="button"], a[href], input, select')) {
        const r = el.getBoundingClientRect();
        if (r.width === 0 && r.height === 0) continue;
        if (r.width < 24 || r.height < 24) out.push(`${el.tagName.toLowerCase()} "${(el.textContent ?? '').trim().slice(0, 30)}" ${Math.round(r.width)}x${Math.round(r.height)}`);
      }
      return out;
    });
    for (const f of found) small.push(`[${scene}] ${f}`);
  }
  expect(small).toEqual([]);
});
