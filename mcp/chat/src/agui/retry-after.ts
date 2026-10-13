// Retry-After on a 503 (resource_exhausted). RFC 9110 allows delay-seconds or an HTTP-date. The value
// is honoured but clamped: a server must not be able to lock the composer for longer than the person
// would plausibly wait, and a missing or unreadable value falls back to a short default.

export const RETRY_AFTER_DEFAULT_SECONDS = 5;
export const RETRY_AFTER_MAX_SECONDS = 300;

const clamp = (seconds: number): number => Math.min(RETRY_AFTER_MAX_SECONDS, Math.max(1, Math.ceil(seconds)));

export function parseRetryAfter(header: string | null, nowMs: number): number {
  const value = header?.trim();
  if (!value) return RETRY_AFTER_DEFAULT_SECONDS;
  if (/^\d{1,9}$/.test(value)) return clamp(Number(value));
  // IMF-fixdate ("Wed, 09 Oct 2026 12:00:30 GMT"). Date.parse alone accepts too much, such as "1e9".
  const at = /^[A-Za-z]{3}, /.test(value) ? Date.parse(value) : Number.NaN;
  if (Number.isFinite(at)) return clamp((at - nowMs) / 1000);
  return RETRY_AFTER_DEFAULT_SECONDS;
}
