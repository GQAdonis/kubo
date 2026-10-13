// Locale is fixed so output is identical across hosts and in tests; the product copy is English.
const INT = new Intl.NumberFormat('en-US');
const DEC = new Intl.NumberFormat('en-US', { maximumFractionDigits: 2, minimumFractionDigits: 0 });

const UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'] as const;

export function formatInt(n: number): string {
  return INT.format(n);
}

/** Binary units, as Kubo reports sizes: 1536 -> "1.5 KiB". */
export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return '0 B';
  let value = n;
  let unit = 0;
  while (value >= 1024 && unit < UNITS.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${unit === 0 ? INT.format(value) : DEC.format(value)} ${UNITS[unit]}`;
}

/** "1.5 KiB (1,536 bytes)": the exact byte count is always available. */
export function formatBytesExact(n: number): string {
  if (n < 1024) return `${INT.format(n)} ${n === 1 ? 'byte' : 'bytes'}`;
  return `${formatBytes(n)} (${INT.format(n)} bytes)`;
}

export function formatRate(bytesPerSecond: number): string {
  return `${formatBytes(bytesPerSecond)}/s`;
}

/** 2026-10-09T12:02:00Z -> "2026-10-09 12:02:00 UTC". Invalid input is returned unchanged. */
export function formatUtc(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return `${d.toISOString().slice(0, 19).replace('T', ' ')} UTC`;
}

/** Visual shortening for dense tables. The full value must stay available to assistive tech and copy. */
export function middleEllipsis(value: string, head = 12, tail = 8): string {
  if (value.length <= head + tail + 1) return value;
  return `${value.slice(0, head)}…${value.slice(-tail)}`;
}

export function plural(n: number, one: string, many = `${one}s`): string {
  return `${INT.format(n)} ${n === 1 ? one : many}`;
}
