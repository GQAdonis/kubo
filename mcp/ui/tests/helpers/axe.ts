import axe, { type AxeResults, type Result } from 'axe-core';

// WCAG 2.2 AA plus axe best practices. Rules that need layout (color-contrast, target-size) cannot run
// in jsdom: contrast is proven by tests/tokens.test.ts and in a real browser by tests/visual/*.spec.ts.
// `region`, `landmark-*` and `page-has-heading-one` judge whole pages; these are components.
const PAGE_LEVEL = ['region', 'landmark-one-main', 'page-has-heading-one', 'landmark-no-duplicate-main', 'landmark-unique'];
const NEEDS_LAYOUT = ['color-contrast', 'color-contrast-enhanced', 'target-size', 'scrollable-region-focusable'];

export async function axeViolations(root: Element, opts: { layoutRules?: boolean } = {}): Promise<Result[]> {
  const disabled = [...PAGE_LEVEL, ...(opts.layoutRules ? [] : NEEDS_LAYOUT)];
  const results: AxeResults = await axe.run(root, {
    runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'] },
    rules: Object.fromEntries(disabled.map((id) => [id, { enabled: false }])),
  });
  return results.violations;
}

export function describeViolations(v: readonly Result[]): string {
  return v.map((x) => `${x.id} (${x.impact}): ${x.help}\n${x.nodes.slice(0, 3).map((n) => `  ${n.html}`).join('\n')}`).join('\n');
}
