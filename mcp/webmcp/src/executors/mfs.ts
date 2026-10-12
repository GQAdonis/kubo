import { collectLimited, decodeContent, encodeContent } from '../bytes.js';
import type { Executor, ExecContext } from '../types.js';
import { paginate, rpc, str, toNumber } from './util.js';

const CAP = 1_048_576;

async function cidOf(ctx: ExecContext, path: string): Promise<string> {
  const stat = await ctx.client.files.stat(path, rpc(ctx));
  return str(stat.cid);
}

export const mfs_ls: Executor = async (input, ctx) => {
  const path: string = input.path ?? '/';
  const { items, page } = await paginate(ctx.client.files.ls(path, rpc(ctx)), ctx, input.cursor, input.limit);
  return {
    path,
    entries: items.map((e) => ({
      name: e.name,
      type: e.type === 'directory' ? 'directory' : 'file',
      size: toNumber(e.size),
      cid: str(e.cid),
    })),
    page,
  };
};

export const mfs_stat: Executor = async (input, ctx) => {
  const s = await ctx.client.files.stat(input.path, rpc(ctx));
  return {
    path: input.path,
    cid: str(s.cid),
    type: s.type === 'directory' ? 'directory' : 'file',
    size: toNumber(s.size),
    cumulative_size: toNumber(s.cumulativeSize),
    blocks: toNumber(s.blocks),
  };
};

export const mfs_read: Executor = async (input, ctx) => {
  const offset: number = input.offset ?? 0;
  const length: number = input.length ?? CAP;
  const stat = await ctx.client.files.stat(input.path, rpc(ctx));
  const { bytes, more } = await collectLimited(
    ctx.client.files.read(input.path, { ...rpc(ctx), offset, length: length + 1 }),
    length,
  );
  const enc = encodeContent(bytes, input.encoding ?? 'utf8');
  return {
    path: input.path,
    offset,
    length: bytes.length,
    size: toNumber(stat.size),
    truncated: more,
    encoding: enc.encoding,
    content: enc.content,
  };
};

export const mfs_write: Executor = async (input, ctx) => {
  const bytes = decodeContent(input.content, input.encoding ?? 'utf8');
  if (bytes.length > CAP) throw new RangeError(`content is larger than ${CAP} bytes`);
  await ctx.client.files.write(input.path, bytes, {
    ...rpc(ctx),
    create: true,
    truncate: true,
    parents: input.parents ?? false,
    flush: true,
  });
  return { path: input.path, cid: await cidOf(ctx, input.path), bytes_written: bytes.length };
};

export const mfs_mkdir: Executor = async (input, ctx) => {
  await ctx.client.files.mkdir(input.path, { ...rpc(ctx), parents: input.parents ?? false, flush: true });
  return { path: input.path, cid: await cidOf(ctx, input.path) };
};

export const mfs_cp: Executor = async (input, ctx) => {
  await ctx.client.files.cp(input.source, input.destination, { ...rpc(ctx), parents: input.parents ?? false, flush: true });
  return { source: input.source, destination: input.destination, cid: await cidOf(ctx, input.destination) };
};

export const mfs_mv: Executor = async (input, ctx) => {
  await ctx.client.files.mv(input.source, input.destination, { ...rpc(ctx), flush: true });
  return { source: input.source, destination: input.destination };
};
