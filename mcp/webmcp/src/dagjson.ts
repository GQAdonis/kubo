import { CID } from 'multiformats/cid';
import { fromBase64, toBase64 } from './bytes.js';

const FORBIDDEN_KEYS = new Set(['__proto__', 'constructor', 'prototype']);

function setOwn(obj: Record<string, unknown>, key: string, value: unknown): void {
  // defineProperty never runs the __proto__ setter, so a hostile key stays a plain data property.
  Object.defineProperty(obj, key, { value, enumerable: true, writable: true, configurable: true });
}

/** Decoded IPLD value to dag-json shape: CID -> {"/": cid}, bytes -> {"/": {"bytes": b64}}. */
export function toDagJson(value: unknown): unknown {
  if (value === null || typeof value !== 'object') {
    if (typeof value === 'bigint') return Number(value);
    return value;
  }
  const cid = CID.asCID(value);
  if (cid) return { '/': cid.toString() };
  if (value instanceof Uint8Array) return { '/': { bytes: toBase64(value).replace(/=+$/, '') } };
  if (Array.isArray(value)) return value.map(toDagJson);
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(value)) setOwn(out, k, toDagJson(v));
  return out;
}

/**
 * The inverse, for dag_put. Property names that could reach Object.prototype are refused,
 * so what the person approved is exactly what is stored.
 */
export function fromDagJson(value: unknown): unknown {
  if (value === null || typeof value !== 'object') return value;
  if (Array.isArray(value)) return value.map(fromDagJson);
  const obj = value as Record<string, unknown>;
  const keys = Object.keys(obj);
  for (const k of keys) {
    if (FORBIDDEN_KEYS.has(k)) throw new RangeError(`property name ${k} is not allowed`);
  }
  if (keys.length === 1 && keys[0] === '/') {
    const inner = obj['/'];
    if (typeof inner === 'string') return CID.parse(inner);
    if (inner && typeof inner === 'object') {
      const bytes = (inner as Record<string, unknown>).bytes;
      if (typeof bytes === 'string') return fromBase64(bytes);
    }
    throw new RangeError('invalid dag-json link or bytes');
  }
  const out: Record<string, unknown> = {};
  for (const k of keys) setOwn(out, k, fromDagJson(obj[k]));
  return out;
}
