// Small, dependency-free color math for WCAG 2.2 contrast. Only the forms hosts actually send are
// parsed (#rgb, #rrggbb, rgb(), light-dark() of those); anything else is "unknown", and callers must
// treat unknown as a reason not to adopt a host value.

export interface Rgb {
  readonly r: number;
  readonly g: number;
  readonly b: number;
}

export function parseColor(input: string, theme: 'light' | 'dark'): Rgb | null {
  const value = input.trim().toLowerCase();
  const ld = /^light-dark\(\s*(.+?)\s*,\s*(.+?)\s*\)$/.exec(value);
  if (ld) return parseColor((theme === 'light' ? ld[1] : ld[2]) as string, theme);

  const hex = /^#([0-9a-f]{3}|[0-9a-f]{6})$/.exec(value);
  if (hex) {
    const h = hex[1] as string;
    const full = h.length === 3 ? [...h].map((c) => c + c).join('') : h;
    const n = Number.parseInt(full, 16);
    return { r: (n >> 16) & 255, g: (n >> 8) & 255, b: n & 255 };
  }

  const fn = /^rgb\(\s*(\d{1,3})\s*[ ,]\s*(\d{1,3})\s*[ ,]\s*(\d{1,3})\s*\)$/.exec(value);
  if (fn) {
    const [r, g, b] = [fn[1], fn[2], fn[3]].map(Number) as [number, number, number];
    if (r > 255 || g > 255 || b > 255) return null;
    return { r, g, b };
  }
  return null;
}

const lin = (c: number) => {
  const s = c / 255;
  return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
};

export function luminance({ r, g, b }: Rgb): number {
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
}

/** WCAG 2.x contrast ratio, 1 to 21. */
export function contrast(a: Rgb, b: Rgb): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x) as [number, number];
  return (hi + 0.05) / (lo + 0.05);
}

export function parseHexStrict(hex: string): Rgb {
  const c = parseColor(hex, 'dark');
  if (!c) throw new Error(`not a color: ${hex}`);
  return c;
}
