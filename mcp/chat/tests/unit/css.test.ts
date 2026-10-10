// @vitest-environment node
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { contrast, parseColor } from '@gqadonis/ipfs-mcp-ui/host';
import { describe, expect, it } from 'vitest';

// The island's stylesheet is Tailwind 4 compiled for a shadow root (scripts/build-css.mjs). It must
// not carry what would leak outside the root, and the design's colors must hold their contrast.

const css = readFileSync(resolve(import.meta.dirname, '../../src/generated/island.css'), 'utf8');
const uiCss = readFileSync(resolve(import.meta.dirname, '../../../ui/dist/ipfs-ui.css'), 'utf8');

describe('generated island stylesheet', () => {
  it.each([
    ['@property registrations (a shadow tree cannot register them)', /@property\b/],
    ['@font-face (ignored in a shadow root; fonts come from FontFace)', /@font-face/],
    ['@import or url() (a fetch)', /@import|\burl\(/],
    ['a :root selector (matches nothing in a shadow tree)', /:root\b/],
    ['html or body selectors (restyle the host page)', /(^|[},\s])(html|body)\b[^{;]*\{/m],
    ['Tailwind preflight (the page reset)', /::file-selector-button|button, \[type='button'\]|-webkit-tap-highlight-color/],
    ['a box-shadow declaration (DESIGN.md: no shadows)', /box-shadow\s*:\s*(?!none)/],
    ['a gradient (DESIGN.md: none)', /gradient\(/],
  ])('has no %s', (_label, pattern) => {
    expect(css).not.toMatch(pattern);
  });

  it('gives Tailwind variables their initial values without @property', () => {
    expect(css).toMatch(/\*, ::before, ::after, ::backdrop\s*\{[^}]*--tw-shadow:/);
  });

  it('anchors theme variables on :host', () => {
    expect(css).toMatch(/:host\s*\{/);
  });

  it('slides the panel with a transform and hides it with visibility, on library tokens', () => {
    expect(css).toMatch(/\.ac-sidebar\s*\{[^}]*transform:\s*translateX\(100%\)/);
    expect(css).toMatch(/\.ac-sidebar\[data-state=['"]open['"]\]\s*\{[^}]*transform:\s*translateX\(0\)/);
    expect(css).toMatch(/\.ac-sidebar\s*\{[^}]*visibility:\s*hidden/);
    expect(css).toMatch(/var\(--iu-duration\)/);
  });

  it('removes motion for people who ask for none', () => {
    expect(css).toMatch(/@media \(prefers-reduced-motion: reduce\)[\s\S]*transition:\s*none/);
  });

  it('uses only transform, opacity and visibility in transitions', () => {
    const props = [...css.matchAll(/transition:\s*([^;]+);/g)].flatMap((m) => (m[1] ?? '').split(',').map((s) => s.trim().split(/\s+/)[0]));
    expect(props.filter((p) => p && p !== 'none')).toSatisfy((list: string[]) => list.every((p) => ['transform', 'visibility', 'opacity'].includes(p)));
  });
});

// ---- token contrast for the pairs this surface uses, recomputed from the shipped library CSS

function tokens(block: string): Record<string, string> {
  return Object.fromEntries([...block.matchAll(/--([a-z-]+):\s*(#[0-9a-fA-F]{6})\s*;/g)].map((m) => [m[1] as string, m[2] as string]));
}
const dark = tokens(/\[data-ipfs-ui\]\s*\{[^}]*--ground:[^}]*\}/.exec(uiCss)?.[0] ?? '');
const light = tokens(/\[data-ipfs-ui\]\[data-theme='light'\]\s*\{[^}]*\}/.exec(uiCss)?.[0] ?? '');

const PAIRS: { fg: string; bg: string; min: number; use: string }[] = [
  { fg: 'text', bg: 'surface', min: 4.5, use: 'panel text' },
  { fg: 'text-muted', bg: 'surface', min: 4.5, use: 'secondary text on the panel' },
  { fg: 'text', bg: 'raised', min: 4.5, use: 'user message text' },
  { fg: 'text-muted', bg: 'raised', min: 4.5, use: 'composer hint on the footer' },
  { fg: 'text', bg: 'control', min: 4.5, use: 'typed text in the message box' },
  { fg: 'text-muted', bg: 'control', min: 4.5, use: 'placeholder' },
  { fg: 'on-accent', bg: 'accent', min: 4.5, use: 'Send message label' },
  { fg: 'accent', bg: 'surface', min: 3.7, use: 'focus ring on the panel' },
  { fg: 'accent', bg: 'raised', min: 3.7, use: 'focus ring on the footer' },
  { fg: 'accent', bg: 'control', min: 3.7, use: 'focus ring on the message box' },
  { fg: 'accent-text', bg: 'surface', min: 4.5, use: 'Arguments disclosure link' },
];

describe.each([
  ['dark', dark],
  ['light', { ...dark, ...light }],
] as const)('%s token contrast on the chat surface', (theme, t) => {
  it('reads the tokens', () => {
    expect(t['surface']).toBeDefined();
    expect(Object.keys(t).length).toBeGreaterThan(15);
  });
  it.each(PAIRS)('$fg on $bg holds $min:1 ($use)', ({ fg, bg, min }) => {
    const a = parseColor(t[fg] ?? '', theme);
    const b = parseColor(t[bg] ?? '', theme);
    expect(a && b).toBeTruthy();
    expect(contrast(a as never, b as never)).toBeGreaterThanOrEqual(min);
  });
});
