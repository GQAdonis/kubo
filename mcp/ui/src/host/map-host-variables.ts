import type { UiTheme } from '../scope/UiScope';
import { contrast, parseColor, parseHexStrict, type Rgb } from './color';

// Maps MCP Apps host style variables (McpUiStyleVariableKey, SEP-1865 "Theming") onto the shared
// design tokens. One token set themes both adapters (MCP App views and the A2UI catalog); this
// module is the only place host values touch it. It is pure: it takes the host context, returns
// token overrides, and imports nothing from the MCP Apps SDK.
//
// Policy "identity" (default): only the theme mode is taken from the host. The brand neutrals,
// accent, state colors, fonts and radii stay Promethity, as PRODUCT.md applies the identity in full.
// Policy "blend": the host's neutral surfaces and text colors replace the brand neutrals, but only
// when the host supplies the complete set, every value parses, and every text pairing still meets
// WCAG AA 4.5:1. Anything less keeps the brand neutrals, because the spec itself warns that partial
// host colors clash.

export type HostThemeName = 'light' | 'dark';
export type HostPolicy = 'identity' | 'blend';

export interface HostStyleInput {
  readonly theme?: HostThemeName | undefined;
  readonly variables?: Readonly<Record<string, string | undefined>> | undefined;
}

/** Host variable -> design token, for the neutrals blend may adopt. */
export const BLENDABLE: Readonly<Record<string, string>> = {
  '--color-background-primary': '--ground',
  '--color-background-secondary': '--surface',
  '--color-background-tertiary': '--raised',
  '--color-text-primary': '--text',
  '--color-text-secondary': '--text-muted',
};

/** Host variables that are never adopted, and the reason. Listed so the policy is inspectable. */
export const LOCKED: Readonly<Record<string, string>> = {
  '--font-sans': 'brand font stack',
  '--font-mono': 'brand font stack',
  '--border-radius-sm': 'brand radius scale',
  '--border-radius-md': 'brand radius scale',
  '--border-radius-lg': 'brand radius scale',
  '--color-text-danger': 'brand state colors are verified on their tints',
  '--color-text-success': 'brand state colors are verified on their tints',
  '--color-text-warning': 'brand state colors are verified on their tints',
  '--color-text-info': 'brand state colors are verified on their tints',
  '--color-ring-primary': 'focus ring is the brand accent, 2px, offset 2px',
  '--shadow-sm': 'no shadows: depth is fills',
  '--shadow-md': 'no shadows: depth is fills',
  '--shadow-lg': 'no shadows: depth is fills',
};

export interface HostThemeResult {
  readonly theme: UiTheme;
  /** CSS custom properties to put on the UiScope style. Empty under the identity policy. */
  readonly tokenOverrides: Readonly<Record<string, string>>;
  readonly adopted: readonly string[];
  readonly rejected: readonly { readonly variable: string; readonly reason: string }[];
}

const BRAND_CONTROL = {
  dark: '#2d2c2a',
  light: '#e1dbd2',
} as const;

export function mapHostStyle(input: HostStyleInput, options: { policy?: HostPolicy } = {}): HostThemeResult {
  const policy = options.policy ?? 'identity';
  const theme: UiTheme = input.theme ?? 'auto';
  if (policy === 'identity') return { theme, tokenOverrides: {}, adopted: [], rejected: [] };

  const mode: HostThemeName = input.theme ?? 'dark';
  const vars = input.variables ?? {};
  const rejected: { variable: string; reason: string }[] = [];
  const colors = new Map<string, { token: string; rgb: Rgb; raw: string }>();

  for (const [hostVar, token] of Object.entries(BLENDABLE)) {
    const raw = vars[hostVar];
    if (raw === undefined || raw.trim() === '') {
      rejected.push({ variable: hostVar, reason: 'not supplied; the host must supply the complete neutral set' });
      continue;
    }
    const rgb = parseColor(raw, mode);
    if (!rgb) {
      rejected.push({ variable: hostVar, reason: 'not a color this library can verify' });
      continue;
    }
    colors.set(hostVar, { token, rgb, raw });
  }
  if (colors.size !== Object.keys(BLENDABLE).length) {
    return { theme, tokenOverrides: {}, adopted: [], rejected };
  }

  const surfaces = ['--color-background-primary', '--color-background-secondary', '--color-background-tertiary'];
  const control = parseHexStrict(BRAND_CONTROL[mode]);
  const texts = ['--color-text-primary', '--color-text-secondary'];
  for (const t of texts) {
    const fg = (colors.get(t) as { rgb: Rgb }).rgb;
    const bgs: [string, Rgb][] = [...surfaces.map((s): [string, Rgb] => [s, (colors.get(s) as { rgb: Rgb }).rgb]), ['brand control', control]];
    for (const [name, bg] of bgs) {
      const ratio = contrast(fg, bg);
      if (ratio < 4.5) {
        rejected.push({ variable: t, reason: `${ratio.toFixed(2)}:1 on ${name}, below the AA minimum of 4.5:1` });
      }
    }
  }
  if (rejected.length > 0) return { theme, tokenOverrides: {}, adopted: [], rejected };

  const tokenOverrides: Record<string, string> = {};
  for (const [, { token, rgb }] of colors) {
    tokenOverrides[token] = `rgb(${rgb.r} ${rgb.g} ${rgb.b})`;
  }
  return { theme, tokenOverrides, adopted: [...colors.keys()], rejected: [] };
}

/** Safe-area insets (CSS pixels) as padding for the view root. */
export function safeAreaPadding(insets: { top: number; right: number; bottom: number; left: number } | undefined): Record<string, string> {
  if (!insets) return {};
  return { padding: `${insets.top}px ${insets.right}px ${insets.bottom}px ${insets.left}px` };
}
