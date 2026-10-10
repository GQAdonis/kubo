import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { contrast, parseColor, type Rgb } from '../src/host/color';
import { STYLE_FILES } from '../src/styles/files';
import { UI_ROOT } from './helpers/contract';

const css = (f: string) => readFileSync(join(UI_ROOT, 'src', 'styles', f), 'utf8');

/** Token declarations inside the block whose selector matches `selector` (first match). */
function block(source: string, selector: string): Record<string, string> {
  const i = source.indexOf(selector);
  if (i < 0) throw new Error(`no block ${selector}`);
  const start = source.indexOf('{', i);
  const end = source.indexOf('}', start);
  const out: Record<string, string> = {};
  for (const m of source.slice(start + 1, end).matchAll(/--([\w-]+):\s*([^;]+);/g)) out[m[1] as string] = (m[2] as string).trim();
  return out;
}

const tokens = css('tokens.css');
const dark = block(tokens, "[data-ipfs-ui] {");
const light = block(tokens, "[data-ipfs-ui][data-theme='light'] {");
const autoLight = block(tokens.slice(tokens.indexOf('@media (prefers-color-scheme: light)')), "[data-ipfs-ui][data-theme='auto'] {");

const COLORS = ['ground', 'surface', 'raised', 'control', 'text', 'text-muted', 'accent', 'on-accent', 'accent-text', 'accent-tint', 'glow', 'ok', 'ok-tint', 'warn', 'warn-tint', 'err', 'err-tint', 'info', 'info-tint'];

const rgb = (t: Record<string, string>, name: string): Rgb => {
  const c = parseColor(t[name] ?? '', 'dark');
  if (!c) throw new Error(`unparseable ${name}: ${t[name]}`);
  return c;
};
const ratio = (t: Record<string, string>, fg: string, bg: string) => contrast(rgb(t, fg), rgb(t, bg));

describe('design tokens', () => {
  it('lists the stylesheet partials the build and runtime concatenate', () => {
    expect([...STYLE_FILES]).toEqual(['tokens.css', 'base.css', 'components.css', 'views.css', 'chat.css']);
    const config = readFileSync(join(UI_ROOT, 'vite.config.ts'), 'utf8');
    expect(config).toContain(`const STYLE_FILES = [${STYLE_FILES.map((f) => `'${f}'`).join(', ')}];`);
  });

  it('are the Promethity v1 tokens, hex for hex, in both themes', () => {
    const brand = join('/Users/gqadonis/Projects/promethity/branding/design-system/tokens.css');
    if (!existsSync(brand)) return; // the branding directory is outside this repository
    const src = readFileSync(brand, 'utf8');
    const brandDark = block(src, ':root, [data-theme="dark"] {');
    const brandLight = block(src, '[data-theme="light"] {');
    for (const name of COLORS) {
      expect(dark[name]?.toLowerCase(), `dark ${name}`).toBe(brandDark[name]?.toLowerCase());
      expect(light[name]?.toLowerCase(), `light ${name}`).toBe(brandLight[name]?.toLowerCase());
    }
    for (const k of ['space-1', 'space-2', 'space-3', 'space-4', 'space-5', 'space-6', 'space-7', 'radius-sm', 'radius-md', 'radius-lg', 'radius-pill']) {
      expect(dark[k], k).toBe(brandDark[k]);
    }
  });

  it('auto light mode repeats the light tokens exactly', () => {
    for (const name of COLORS) expect(autoLight[name], name).toBe(light[name]);
  });

  describe.each([
    ['dark', dark],
    ['light', light],
  ] as const)('WCAG 2.2 AA contrast in the %s theme', (_name, t) => {
    const surfaces = ['ground', 'surface', 'raised', 'control'];
    it.each(surfaces)('text and muted text hold 4.5:1 on %s', (bg) => {
      expect(ratio(t, 'text', bg)).toBeGreaterThanOrEqual(4.5);
      expect(ratio(t, 'text-muted', bg)).toBeGreaterThanOrEqual(4.5);
    });

    it('on-accent holds 4.5:1 on accent', () => {
      expect(ratio(t, 'on-accent', 'accent')).toBeGreaterThanOrEqual(4.5);
    });

    it('accent-text holds 4.5:1 where views use it (ground, surface, raised, accent-tint)', () => {
      for (const bg of ['ground', 'surface', 'raised', 'accent-tint']) expect(ratio(t, 'accent-text', bg), bg).toBeGreaterThanOrEqual(4.5);
    });

    it('each state color holds 4.5:1 on its own tint, on surface, and on raised', () => {
      for (const s of ['ok', 'warn', 'err', 'info']) {
        expect(ratio(t, s, `${s}-tint`), `${s} on tint`).toBeGreaterThanOrEqual(4.5);
        expect(ratio(t, s, 'surface'), `${s} on surface`).toBeGreaterThanOrEqual(4.5);
        expect(ratio(t, s, 'raised'), `${s} on raised`).toBeGreaterThanOrEqual(4.5);
      }
    });

    it('text and muted text hold 4.5:1 on every tint a notice or approval can sit on', () => {
      for (const tint of ['accent-tint', 'ok-tint', 'warn-tint', 'err-tint', 'info-tint']) {
        expect(ratio(t, 'text', tint), tint).toBeGreaterThanOrEqual(4.5);
        expect(ratio(t, 'text-muted', tint), `muted on ${tint}`).toBeGreaterThanOrEqual(4.5);
      }
    });

    it('the focus ring (accent) holds the brand floor of 3.7:1 on every surface, above WCAG 1.4.11 (3:1)', () => {
      for (const bg of surfaces) {
        expect(ratio(t, 'accent', bg), bg).toBeGreaterThanOrEqual(3.7);
      }
    });
  });

  it('DESIGN.md states the contrast numbers these tokens produce', () => {
    const design = readFileSync(join(UI_ROOT, '..', '..', 'DESIGN.md'), 'utf8');
    const fmt = (t: Record<string, string>, fg: string, bgs: string[]) => bgs.map((b) => ratio(t, fg, b).toFixed(1)).join(' / ');
    const four = ['ground', 'surface', 'raised', 'control'];
    const expected = [
      `| \`text\` on \`ground\` / \`surface\` / \`raised\` / \`control\` | ${fmt(dark, 'text', four)} | ${fmt(light, 'text', four)} |`,
      `| \`text-muted\` on \`ground\` / \`surface\` / \`raised\` / \`control\` | ${fmt(dark, 'text-muted', four)} | ${fmt(light, 'text-muted', four)} |`,
      `| \`on-accent\` on \`accent\` | ${ratio(dark, 'on-accent', 'accent').toFixed(1)} | ${ratio(light, 'on-accent', 'accent').toFixed(1)} |`,
      `| \`accent-text\` on \`surface\` / \`raised\` / \`accent-tint\` | ${fmt(dark, 'accent-text', ['surface', 'raised', 'accent-tint'])} | ${fmt(light, 'accent-text', ['surface', 'raised', 'accent-tint'])} |`,
      `| focus ring (\`accent\`) on \`ground\` / \`surface\` / \`raised\` / \`control\` | ${fmt(dark, 'accent', four)} | ${fmt(light, 'accent', four)} |`,
    ];
    for (const row of expected) expect(design, row).toContain(row);
    const states = (t: Record<string, string>) => ['ok', 'warn', 'err', 'info'].map((s) => ratio(t, s, `${s}-tint`).toFixed(1)).join(' / ');
    expect(design).toContain(`| state color on its own tint (ok / warn / err / info) | ${states(dark)} | ${states(light)} |`);
  });
});

describe('motion', () => {
  const all = ['tokens', 'base', 'components', 'views', 'chat'].map((f) => css(`${f}.css`)).join('\n');

  it('disables every animation and transition under prefers-reduced-motion: reduce', () => {
    const m = /@media \(prefers-reduced-motion: reduce\) \{([\s\S]*?)\n\}/.exec(all);
    expect(m).not.toBeNull();
    expect(m?.[1]).toMatch(/animation:\s*none\s*!important/);
    expect(m?.[1]).toMatch(/transition:\s*none\s*!important/);
    expect(m?.[1]).toContain('[data-ipfs-ui] *');
  });

  it('uses no keyframe animation at all, and transitions only on color, filter and transform', () => {
    expect(all).not.toMatch(/@keyframes/);
    expect(all.replace(/animation:\s*none\s*!important;/g, '')).not.toMatch(/animation\s*:/);
    for (const m of all.matchAll(/transition:\s*([^;]+);/g)) {
      const props = [...(m[1] as string).matchAll(/(^|\s)([a-z-]+)\s+var\(--iu-duration\)/g)].map((x) => x[2]);
      for (const p of props) expect(['background-color', 'color', 'filter', 'transform', 'opacity'], p).toContain(p);
    }
    for (const m of all.matchAll(/transition:\s*none/g)) expect(m[0]).toBeTruthy();
  });

  it('keeps the transition duration inside the 150-250ms range for state changes', () => {
    const ms = Number(/--iu-duration:\s*(\d+)ms/.exec(all)?.[1]);
    expect(ms).toBeGreaterThanOrEqual(150);
    expect(ms).toBeLessThanOrEqual(250);
  });
});

describe('design rules', () => {
  const all = ['tokens', 'base', 'components', 'views', 'chat'].map((f) => css(`${f}.css`)).join('\n').replace(/\/\*[\s\S]*?\*\//g, '');

  it('draws no borders, shadows or gradients (depth is fills)', () => {
    // `border: 0` only removes the user-agent border; any other border declaration draws one.
    for (const m of all.matchAll(/(^|[\s;{])(border(?:-(?:top|right|bottom|left|inline|block)[\w-]*)?|outline-style)\s*:\s*([^;]+);/g)) {
      expect(m[3]?.trim(), `${m[2]}: ${m[3]}`).toMatch(/^(0|none)$/);
    }
    expect(all).not.toMatch(/box-shadow\s*:/);
    expect(all).not.toMatch(/text-shadow\s*:/);
    expect(all).not.toMatch(/gradient\(/);
  });

  it('uses outline only for the focus ring, and only the accent', () => {
    const outlines = [...all.matchAll(/outline[\w-]*:\s*([^;]+);/g)].map((m) => m[1]);
    expect(outlines.length).toBeGreaterThan(0);
    for (const o of outlines) expect(o).toMatch(/var\(--iu-focus(-offset)?\)/);
    expect(all).toContain('--iu-focus: 2px solid var(--accent);');
    expect(all).toContain('--iu-focus-offset: 2px;');
  });

  it('loads no remote or relative resource from CSS', () => {
    expect(all).not.toMatch(/url\(/);
    expect(all).not.toMatch(/@import/);
    expect(all).not.toMatch(/@font-face/);
  });
});
