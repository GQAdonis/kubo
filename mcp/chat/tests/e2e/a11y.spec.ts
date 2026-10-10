import { expect, test } from './harness';
import { AXE_SOURCE, ask, userTextOf } from './helpers';

// WCAG 2.2 AA in a real engine, including the rules jsdom cannot run: color-contrast and target-size.
// axe-core is injected into the page and run against the island element; it walks open shadow roots.

type AxeResult = { violations: { id: string; impact: string; help: string; nodes: { html: string; failureSummary?: string }[] }[] };

const TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'];
const STATES = [
  { name: 'empty', fixture: undefined, settle: /Reads run at once/ },
  { name: 'result', fixture: 'read-pin-ls', settle: /You have one recursive pin/ },
  { name: 'pending approval', fixture: 'write-interrupt', settle: /Awaiting a person/ },
  { name: 'native-prompt approval', fixture: 'destructive-native-prompt', settle: /Waiting for system prompt/ },
  { name: 'refused egress', fixture: 'egress-refused', settle: /I cannot read that file/ },
] as const;

for (const scheme of ['dark', 'light'] as const) {
  for (const state of STATES) {
    test(`no axe violations: ${state.name}, ${scheme}`, async ({ harness }) => {
      const page = await harness.open({ island: 'open', scheme, width: 1024, height: 900 });
      if (state.fixture) {
        await ask(page, userTextOf(state.fixture));
        // A native-prompt state is a run held open on {wait: true}; every other state has finished.
          if (state.fixture !== 'destructive-native-prompt') await expect(page.getByRole('button', { name: 'Stop run' })).toHaveCount(0);
      }
      await expect(page.getByText(state.settle).first()).toBeVisible();
      await page.evaluate(() => document.fonts.ready);
      await page.evaluate(AXE_SOURCE());

      const results = (await page.evaluate(
        (tags) =>
          (window as unknown as { axe: { run: (ctx: unknown, opts: unknown) => Promise<unknown> } }).axe.run('ipfs-agent-chat', {
            runOnly: { type: 'tag', values: tags },
            // The island is a component of a page: page-level rules judge the stand-in host, not it.
            rules: Object.fromEntries(['region', 'landmark-one-main', 'page-has-heading-one'].map((id) => [id, { enabled: false }])),
          }),
        TAGS,
      )) as AxeResult;

      const report = results.violations.map((v) => `${v.id} (${v.impact}): ${v.help}\n${v.nodes.slice(0, 3).map((n) => `  ${n.html}\n  ${n.failureSummary ?? ''}`).join('\n')}`).join('\n');
      expect(results.violations, report).toEqual([]);
    });
  }
}

test('target size: every control is at least 24 CSS px, and the primary controls 44', async ({ harness }) => {
  const page = await harness.open({ island: 'open', width: 1024, height: 900 });
  await ask(page, userTextOf('write-interrupt'));
  await expect(page.getByText('Awaiting a person')).toBeVisible();
  const sizes = await page.evaluate(() =>
    [...(document.querySelector('ipfs-agent-chat')?.shadowRoot?.querySelectorAll('button, textarea, summary, [tabindex="0"]') ?? [])].map((el) => {
      const r = el.getBoundingClientRect();
      return { name: (el.getAttribute('aria-label') ?? el.textContent ?? el.tagName).trim().slice(0, 30), w: Math.round(r.width), h: Math.round(r.height), tag: el.tagName };
    }),
  );
  expect(sizes.length).toBeGreaterThan(4);
  for (const s of sizes) {
    expect(s.h, s.name).toBeGreaterThanOrEqual(24);
    if (s.tag === 'BUTTON' || s.tag === 'TEXTAREA') expect(s.h, `${s.name} (DESIGN.md: buttons at least 44px)`).toBeGreaterThanOrEqual(44);
  }
});
