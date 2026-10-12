import type { ExecContext } from '../types.js';

/**
 * Options every RPC call carries: the host's abort signal when it sent one, otherwise a
 * signal that fires after the timeout (Kimi passes none), and the client's own timeout.
 */
export function rpc(ctx: ExecContext): { signal?: AbortSignal; timeout: number } {
  if (ctx.signal) return { signal: ctx.signal, timeout: ctx.timeoutMs };
  if (typeof AbortSignal !== 'undefined' && typeof AbortSignal.timeout === 'function') {
    return { signal: AbortSignal.timeout(ctx.timeoutMs), timeout: ctx.timeoutMs };
  }
  return { timeout: ctx.timeoutMs };
}

/** A bare CID or CID/sub/path becomes /ipfs/...; /ipfs, /ipns and /ipld paths pass through. */
export function toContentPath(path: string): string {
  return path.startsWith('/') ? path : `/ipfs/${path}`;
}

/** Largest safe integer for sizes that arrive as bigint. */
export function toNumber(v: unknown): number {
  if (typeof v === 'bigint') return v > BigInt(Number.MAX_SAFE_INTEGER) ? Number.MAX_SAFE_INTEGER : Number(v);
  return typeof v === 'number' && Number.isFinite(v) ? v : 0;
}

export interface Page<T> {
  items: T[];
  page: { has_more: boolean; next_cursor?: string };
}

const CURSOR = /^o([0-9]{1,9})$/;

/** A cursor is accepted only if this tool set issued it, so an agent cannot jump to an offset it was not given. */
export function parseCursor(cursor: unknown, issued: Set<string>): number {
  if (cursor === undefined) return 0;
  const m = typeof cursor === 'string' && issued.has(cursor) ? CURSOR.exec(cursor) : null;
  if (!m) throw new RangeError('cursor is not a value returned by this tool');
  return Number(m[1]);
}

/**
 * Offset pagination over an async source. The cursor is opaque to the agent. The source
 * is read only as far as one page plus one item, so a long listing is not pulled whole.
 */
export async function paginate<T>(
  source: AsyncIterable<T> | Iterable<T>,
  ctx: ExecContext,
  cursor: unknown,
  limit: unknown,
): Promise<Page<T>> {
  const start = parseCursor(cursor, ctx.issuedCursors);
  const size = typeof limit === 'number' ? limit : 100;
  const items: T[] = [];
  let index = 0;
  let more = false;
  for await (const item of source as AsyncIterable<T>) {
    if (index++ < start) continue;
    if (items.length === size) {
      more = true;
      break;
    }
    items.push(item);
  }
  return { items, page: nextPage(ctx, more, start + size) };
}

function nextPage(ctx: ExecContext, more: boolean, next: number): Page<never>['page'] {
  if (!more) return { has_more: false };
  const cursor = `o${next}`;
  ctx.issuedCursors.add(cursor);
  return { has_more: true, next_cursor: cursor };
}

export function paginateArray<T>(all: T[], ctx: ExecContext, cursor: unknown, limit: unknown): Page<T> {
  const start = parseCursor(cursor, ctx.issuedCursors);
  const size = typeof limit === 'number' ? limit : 100;
  const items = all.slice(start, start + size);
  return { items, page: nextPage(ctx, start + size < all.length, start + size) };
}

export function str(v: unknown): string {
  return typeof v === 'string' ? v : String(v);
}
