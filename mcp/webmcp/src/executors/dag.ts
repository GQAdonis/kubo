import { CID } from 'multiformats/cid';
import { concat, toBase64 } from '../bytes.js';
import { fromDagJson, toDagJson } from '../dagjson.js';
import type { Executor } from '../types.js';
import { rpc, str, toContentPath } from './util.js';

const CODECS: Record<number, string> = { 0x55: 'raw', 0x70: 'dag-pb', 0x71: 'dag-cbor', 0x129: 'dag-json', 0x0200: 'json' };
const HARD_EXPORT_LIMIT = 32 * 1024 * 1024;

export const dag_get: Executor = async (input, ctx) => {
  const resolved = await ctx.client.dag.resolve(toContentPath(input.path), rpc(ctx));
  const got = await ctx.client.dag.get(resolved.cid, {
    ...rpc(ctx),
    ...(resolved.remainderPath ? { path: resolved.remainderPath } : {}),
  });
  const node = toDagJson(got.value);
  const max: number = input.max_inline_bytes ?? 262_144;
  const text = JSON.stringify(node);
  const out: Record<string, unknown> = {
    path: input.path,
    cid: str(resolved.cid),
    codec: CODECS[resolved.cid.code] ?? `0x${resolved.cid.code.toString(16)}`,
    resource: { uri: `ipfs://${str(resolved.cid)}` },
    truncated: new TextEncoder().encode(text).length > max,
  };
  if (!out.truncated) out.node = node;
  return out;
};

export const dag_put: Executor = async (input, ctx) => {
  let parsed: unknown;
  try {
    parsed = JSON.parse(input.data);
  } catch {
    throw new RangeError('data is not valid JSON');
  }
  let node: unknown;
  try {
    node = fromDagJson(parsed);
  } catch {
    throw new RangeError('data is not valid dag-json');
  }
  const cid = await ctx.client.dag.put(node, {
    ...rpc(ctx),
    storeCodec: input.store_codec ?? 'dag-cbor',
    pin: input.pin,
  });
  return { cid: str(cid), pinned: input.pin === true };
};

export const car_export: Executor = async (input, ctx) => {
  const max: number = input.max_inline_bytes ?? 1_048_576;
  const root = CID.parse(input.root);
  const kept: Uint8Array[] = [];
  let total = 0;
  for await (const chunk of ctx.client.dag.export(root, rpc(ctx))) {
    total += chunk.length;
    if (total <= max) kept.push(chunk);
    if (total > HARD_EXPORT_LIMIT) throw new RangeError('the CAR is larger than 32 MiB');
  }
  const out: Record<string, unknown> = { root: input.root, size: total, inline: total <= max };
  out.resource = { uri: `ipfs://${input.root}` };
  if (total <= max) out.car_base64 = toBase64(concat(kept));
  return out;
};
