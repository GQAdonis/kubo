import type { Executor } from '../types.js';
import { rpc, str, toContentPath } from './util.js';

export const ipns_publish: Executor = async (input, ctx) => {
  const options: Record<string, unknown> = { ...rpc(ctx), key: input.key ?? 'self', lifetime: input.lifetime ?? '48h' };
  if (input.ttl !== undefined) options.ttl = input.ttl;
  const res = await ctx.client.name.publish(toContentPath(input.path), options);
  return { name: res.name, value: res.value };
};

export const ipns_resolve: Executor = async (input, ctx) => {
  const name: string = input.name;
  let last: string | undefined;
  for await (const p of ctx.client.name.resolve(name, {
    ...rpc(ctx),
    recursive: input.recursive ?? true,
    nocache: input.nocache ?? false,
  })) {
    last = p;
  }
  if (last === undefined) throw new Error('the name did not resolve to a path');
  return { name, path: last };
};

export const key_list: Executor = async (_input, ctx) => {
  const keys = await ctx.client.key.list(rpc(ctx));
  return { keys: keys.map((k) => ({ name: k.name, id: str(k.id) })) };
};

export const key_gen: Executor = async (input, ctx) => {
  const options: Record<string, unknown> = { ...rpc(ctx), type: input.type ?? 'ed25519' };
  if (input.size !== undefined) options.size = input.size;
  const key = await ctx.client.key.gen(input.name, options);
  return { name: key.name, id: str(key.id) };
};

export const key_rename: Executor = async (input, ctx) => {
  const res = await ctx.client.key.rename(input.old_name, input.new_name, rpc(ctx));
  return { old_name: input.old_name, new_name: input.new_name, id: str(res.id) };
};
