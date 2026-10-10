import { describe, expect, it, vi, beforeEach } from 'vitest';
import { contrast, parseColor } from '../src/host/color';
import { mapHostStyle, safeAreaPadding } from '../src/host';
import { registerBundledFonts, resetFontRegistration } from '../src/fonts/register';
import { BUNDLED_FACES } from '../src/fonts/fonts-data';

const FULL_DARK = {
  '--color-background-primary': 'light-dark(#ffffff, #101010)',
  '--color-background-secondary': 'light-dark(#f4f4f4, #1a1a1a)',
  '--color-background-tertiary': 'light-dark(#e8e8e8, #242424)',
  '--color-text-primary': 'light-dark(#111111, #f2f2f2)',
  '--color-text-secondary': 'light-dark(#555555, #b0b0b0)',
};

describe('color parsing and contrast', () => {
  it('parses hex, rgb() and light-dark() by theme, and refuses what it cannot verify', () => {
    expect(parseColor('#fff', 'dark')).toEqual({ r: 255, g: 255, b: 255 });
    expect(parseColor('rgb(10 20 30)', 'dark')).toEqual({ r: 10, g: 20, b: 30 });
    expect(parseColor('light-dark(#ffffff, #000000)', 'dark')).toEqual({ r: 0, g: 0, b: 0 });
    expect(parseColor('light-dark(#ffffff, #000000)', 'light')).toEqual({ r: 255, g: 255, b: 255 });
    expect(parseColor('oklch(0.5 0.1 200)', 'dark')).toBeNull();
    expect(parseColor('rgb(300 0 0)', 'dark')).toBeNull();
    expect(parseColor('red', 'dark')).toBeNull();
  });

  it('computes the WCAG ratio', () => {
    expect(contrast({ r: 0, g: 0, b: 0 }, { r: 255, g: 255, b: 255 })).toBeCloseTo(21, 5);
  });
});

describe('host style variables onto the shared tokens', () => {
  it('identity policy (default): takes the theme and nothing else, whatever the host sends', () => {
    const r = mapHostStyle({ theme: 'light', variables: FULL_DARK });
    expect(r.theme).toBe('light');
    expect(r.tokenOverrides).toEqual({});
    expect(r.adopted).toEqual([]);
  });

  it('follows the system when the host sends no theme', () => {
    expect(mapHostStyle({}).theme).toBe('auto');
  });

  it('blend: adopts a complete, AA-safe neutral set and maps it onto the neutral tokens only', () => {
    const r = mapHostStyle({ theme: 'dark', variables: FULL_DARK }, { policy: 'blend' });
    expect(r.rejected).toEqual([]);
    expect(r.tokenOverrides).toEqual({
      '--ground': 'rgb(16 16 16)',
      '--surface': 'rgb(26 26 26)',
      '--raised': 'rgb(36 36 36)',
      '--text': 'rgb(242 242 242)',
      '--text-muted': 'rgb(176 176 176)',
    });
    // accent, state colors, fonts, radii and the focus ring stay the brand's
    for (const t of ['--accent', '--ok', '--err', '--font-body', '--radius-md', '--control']) {
      expect(r.tokenOverrides).not.toHaveProperty(t);
    }
  });

  it('blend: resolves light-dark() for the light theme', () => {
    const r = mapHostStyle({ theme: 'light', variables: FULL_DARK }, { policy: 'blend' });
    expect(r.tokenOverrides['--ground']).toBe('rgb(255 255 255)');
  });

  it('blend: refuses a partial set (the spec warns partial host colors clash)', () => {
    const { '--color-text-secondary': _drop, ...partial } = FULL_DARK;
    const r = mapHostStyle({ theme: 'dark', variables: partial }, { policy: 'blend' });
    expect(r.tokenOverrides).toEqual({});
    expect(r.rejected.map((x) => x.variable)).toContain('--color-text-secondary');
  });

  it('blend: refuses a set that fails AA against any surface, including the brand control fill', () => {
    const low = { ...FULL_DARK, '--color-text-secondary': 'light-dark(#555555, #404040)' };
    const r = mapHostStyle({ theme: 'dark', variables: low }, { policy: 'blend' });
    expect(r.tokenOverrides).toEqual({});
    expect(r.rejected[0]?.reason).toMatch(/below the AA minimum of 4.5:1/);
  });

  it('blend: refuses colors it cannot verify', () => {
    const r = mapHostStyle({ theme: 'dark', variables: { ...FULL_DARK, '--color-background-primary': 'oklch(0.2 0 0)' } }, { policy: 'blend' });
    expect(r.tokenOverrides).toEqual({});
    expect(r.rejected[0]?.reason).toMatch(/not a color/);
  });

  it('turns safe-area insets into padding', () => {
    expect(safeAreaPadding({ top: 1, right: 2, bottom: 3, left: 4 })).toEqual({ padding: '1px 2px 3px 4px' });
    expect(safeAreaPadding(undefined)).toEqual({});
  });
});

describe('bundled fonts', () => {
  beforeEach(() => resetFontRegistration());

  it('ships exactly the brand faces the views use, as embedded woff2', () => {
    const faces = BUNDLED_FACES.map((f) => `${f.family} ${f.weight}`);
    expect(faces).toEqual([
      'IPFS UI Syne 600', 'IPFS UI Syne 700',
      'IPFS UI Hanken Grotesk 400', 'IPFS UI Hanken Grotesk 500', 'IPFS UI Hanken Grotesk 600', 'IPFS UI Hanken Grotesk 700',
      'IPFS UI JetBrains Mono 400', 'IPFS UI JetBrains Mono 500', 'IPFS UI JetBrains Mono 600',
    ]);
    for (const f of BUNDLED_FACES) expect(f.dataUri).toMatch(/^data:font\/woff2;base64,/);
  });

  it('registers every face from bytes through FontFace, with no URL', async () => {
    const added: unknown[] = [];
    const ctor = vi.fn(function (this: Record<string, unknown>, family: string, source: unknown, desc: unknown) {
      Object.assign(this, { family, source, desc });
      this['load'] = async () => this;
    });
    vi.stubGlobal('FontFace', ctor);
    const doc = { fonts: { add: (f: unknown) => added.push(f) } } as unknown as Document;
    expect(await registerBundledFonts(doc)).toBe(BUNDLED_FACES.length);
    expect(ctor).toHaveBeenCalledTimes(BUNDLED_FACES.length);
    for (const call of ctor.mock.calls) {
      expect(call[1]).toBeInstanceOf(ArrayBuffer); // bytes, never a string URL
      expect(typeof call[1]).not.toBe('string');
    }
    expect(added).toHaveLength(BUNDLED_FACES.length);
    vi.unstubAllGlobals();
  });

  it('registers once and falls back quietly when FontFace is unavailable', async () => {
    vi.stubGlobal('FontFace', undefined);
    expect(await registerBundledFonts({ fonts: {} } as unknown as Document)).toBe(0);
    vi.unstubAllGlobals();
  });
});
