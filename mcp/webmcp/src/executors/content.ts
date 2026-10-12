import { collectLimited, decodeContent, encodeContent } from '../bytes.js';
import type { Executor } from '../types.js';
import { paginate, rpc, str, toContentPath, toNumber } from './util.js';

const CAP = 1_048_576;
const typeOf = (t: unknown): 'file' | 'directory' | 'symlink' | 'unknown' =>
  t === 'file' ? 'file' : t === 'dir' || t === 'directory' ? 'directory' : t === 'symlink' ? 'symlink' : 'unknown';

export const ipfs_node_info: Executor = async (_input, ctx) => {
  const [id, version] = await Promise.all([ctx.client.id(rpc(ctx)), ctx.client.version(rpc(ctx))]);
  const out: Record<string, unknown> = {
    peer_id: str(id.id),
    agent_version: id.agentVersion,
    version: version.version,
    addresses: id.addresses.map((a) => str(a)),
  };
  if (version.commit) out.commit = version.commit;
  if (version.repo) out.repo_version = version.repo;
  return out;
};

export const ipfs_add: Executor = async (input, ctx) => {
  const bytes = decodeContent(input.content, input.encoding ?? 'utf8');
  if (bytes.length > CAP) throw new RangeError(`content is larger than ${CAP} bytes`);
  const filename: string | undefined = input.filename;
  const options: Record<string, unknown> = { ...rpc(ctx), pin: input.pin };
  if (filename) options.wrapWithDirectory = true;
  if (input.cid_version !== undefined) options.cidVersion = input.cid_version;
  if (input.raw_leaves !== undefined) options.rawLeaves = input.raw_leaves;
  if (input.chunker !== undefined) options.chunker = input.chunker;
  if (input.hash !== undefined) options.hashAlg = input.hash;
  // Kubo announces the root of an added file by default. Without an explicit provide:true the
  // page must not announce anything the person did not ask to announce.
  if (input.provide !== true) options['fast-provide-root'] = false;
  const added = await ctx.client.add(filename ? { path: filename, content: bytes } : bytes, options);
  const cid = str(added.cid);
  let provided = false;
  const warnings: string[] = [];
  if (input.provide) {
    try {
      for await (const _event of ctx.client.routing.provide(added.cid, rpc(ctx))) {
        // The stream ends when the announcement is done.
      }
      provided = true;
    } catch (e) {
      warnings.push(`The content was added, but announcing it failed: ${e instanceof Error ? e.message.slice(0, 200) : 'unknown error'}`);
    }
  }
  const out: Record<string, unknown> = { cid, size: toNumber(added.size), pinned: input.pin === true, provided };
  if (filename) out.name = filename;
  out.resource = { uri: `ipfs://${cid}` };
  if (warnings.length > 0) out.warnings = warnings;
  return out;
};

export const ipfs_cat: Executor = async (input, ctx) => {
  const path = toContentPath(input.path);
  const offset: number = input.offset ?? 0;
  const length: number = input.length ?? CAP;
  const stat = await ctx.client.files.stat(path, rpc(ctx)).catch(() => undefined);
  const { bytes, more } = await collectLimited(ctx.client.cat(path, { ...rpc(ctx), offset, length: length + 1 }), length);
  const enc = encodeContent(bytes, input.encoding ?? 'utf8');
  const out: Record<string, unknown> = {
    path: input.path,
    offset,
    length: bytes.length,
    truncated: more,
    encoding: enc.encoding,
    content: enc.content,
  };
  if (stat) out.size = toNumber(stat.size);
  return out;
};

export const ipfs_get: Executor = async (input, ctx) => {
  const path = toContentPath(input.path);
  const stat = await ctx.client.files.stat(path, rpc(ctx));
  const type = typeOf(stat.type);
  const cid = str(stat.cid);
  const out: Record<string, unknown> = {
    path: input.path,
    cid,
    type,
    size: toNumber(stat.size),
    resource: { uri: `ipfs://${cid}` },
  };
  const max: number = input.max_inline_bytes ?? 65_536;
  if (type === 'file') {
    if (stat.size <= max) {
      const { bytes } = await collectLimited(ctx.client.cat(path, rpc(ctx)), max);
      const enc = encodeContent(bytes, 'utf8');
      out.encoding = enc.encoding;
      out.content = enc.content;
      out.truncated = false;
    } else {
      out.truncated = true;
    }
  }
  return out;
};

export const ipfs_ls: Executor = async (input, ctx) => {
  const path = toContentPath(input.path);
  const { items, page } = await paginate(ctx.client.ls(path, rpc(ctx)), ctx, input.cursor, input.limit);
  return {
    path: input.path,
    entries: items.map((e) => ({
      name: e.name,
      cid: str(e.cid),
      type: typeOf(e.type),
      size: toNumber(e.size),
    })),
    page,
  };
};
