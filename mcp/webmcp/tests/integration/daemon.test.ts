import { create } from 'kubo-rpc-client';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { createWebMcpTools } from '../../src/index.js';
import type { ConfirmationRequest, ToolEnvelope } from '../../src/index.js';
import { outputValidator } from './conformance.js';
import { startDaemon, type Daemon } from './daemon.js';

// Runs every exported tool against a real daemon in a temporary repo and checks each
// result against the contract's output schema.

let daemon: Daemon;
const validate = outputValidator();
const requests: ConfirmationRequest[] = [];
const confirm = vi.fn(async (req: ConfirmationRequest) => {
  requests.push(req);
  return true;
});

function toolSet(hook = confirm) {
  const client = create({ url: daemon.url });
  return createWebMcpTools({ client, confirm: hook, includeWrites: true });
}

// One tool set for the run: pagination cursors are only valid for the set that issued them.
let shared: ReturnType<typeof toolSet> | undefined;

async function call(name: string, input: unknown, tools = (shared ??= toolSet())): Promise<Record<string, any>> {
  const tool = tools.find((t) => t.name === name);
  if (!tool) throw new Error(`no tool ${name}`);
  const env = (await tool.execute(input)) as ToolEnvelope;
  if (!env.ok) throw new Error(`${name} failed: ${JSON.stringify(env)}`);
  const payload = 'data' in env ? env.data : env.result;
  expect(validate(name, payload), `${name} output must match the contract schema`).toEqual([]);
  return payload as Record<string, any>;
}

beforeAll(async () => {
  daemon = await startDaemon();
});

afterAll(async () => {
  await daemon?.stop();
});

describe('against a real ipfs daemon', () => {
  let fileCid = '';
  let dirCid = '';

  it('uses a non-default API port and a temporary repo', () => {
    const port = Number(new URL(daemon.url).port);
    expect([4000, 4001, 5001, 8080]).not.toContain(port);
    expect(daemon.repo).toContain('webmcp-ipfs-');
  });

  it('ipfs_node_info', async () => {
    const out = await call('ipfs_node_info', {});
    expect(out.peer_id).toMatch(/^12D3KooW/);
    expect(out.addresses.length).toBeGreaterThan(0);
  });

  it('ipfs_add, ipfs_cat, ipfs_get', async () => {
    const added = await call('ipfs_add', { content: 'hello webmcp', pin: false, provide: false, cid_version: 1 });
    fileCid = added.cid;
    expect(added).toMatchObject({ size: 12, pinned: false, provided: false });
    const cat = await call('ipfs_cat', { path: fileCid });
    expect(cat).toMatchObject({ content: 'hello webmcp', encoding: 'utf8', truncated: false, length: 12 });
    const part = await call('ipfs_cat', { path: fileCid, offset: 6, length: 3 });
    expect(part).toMatchObject({ content: 'web', truncated: true, offset: 6 });
    const got = await call('ipfs_get', { path: `/ipfs/${fileCid}` });
    expect(got).toMatchObject({ cid: fileCid, type: 'file', content: 'hello webmcp' });
    const big = await call('ipfs_get', { path: fileCid, max_inline_bytes: 2 });
    expect(big.truncated).toBe(true);
    expect(big.content).toBeUndefined();
  });

  it('ipfs_add with a filename wraps a directory that ipfs_ls lists', async () => {
    const added = await call('ipfs_add', { content: 'in a dir', filename: 'note.txt', pin: false, provide: false, cid_version: 1 });
    dirCid = added.cid;
    expect(added.name).toBe('note.txt');
    const ls = await call('ipfs_ls', { path: dirCid });
    expect(ls.entries.map((e: any) => e.name)).toEqual(['note.txt']);
    expect(ls.page).toEqual({ has_more: false });
  });

  it('ipfs_add reports binary content as base64', async () => {
    const added = await call('ipfs_add', { content: '/wD+', encoding: 'base64', pin: false, provide: false, cid_version: 1 });
    const cat = await call('ipfs_cat', { path: added.cid });
    expect(cat.encoding).toBe('base64');
    expect(cat.content).toBe('/wD+');
  });

  it('pin_add then pin_ls', async () => {
    const pinned = await call('pin_add', { path: fileCid, name: 'webmcp-test' });
    expect(pinned.pins).toEqual([fileCid]);
    const ls = await call('pin_ls', { type: 'recursive' });
    expect(ls.pins).toContainEqual(expect.objectContaining({ cid: fileCid, type: 'recursive' }));
    const one = await call('pin_ls', { path: fileCid });
    expect(one.pins.length).toBe(1);
  });

  it('MFS: mkdir, write, read, stat, ls, cp, mv', async () => {
    await call('mfs_mkdir', { path: '/webmcp', parents: true });
    const w = await call('mfs_write', { path: '/webmcp/a.txt', content: 'abc', parents: true });
    expect(w.bytes_written).toBe(3);
    const r = await call('mfs_read', { path: '/webmcp/a.txt' });
    expect(r).toMatchObject({ content: 'abc', truncated: false, size: 3 });
    const st = await call('mfs_stat', { path: '/webmcp/a.txt' });
    expect(st).toMatchObject({ type: 'file', size: 3 });
    const ls = await call('mfs_ls', { path: '/webmcp' });
    expect(ls.entries.map((e: any) => e.name)).toEqual(['a.txt']);
    await call('mfs_cp', { source: `/ipfs/${fileCid}`, destination: '/webmcp/copy.txt' });
    await call('mfs_mv', { source: '/webmcp/a.txt', destination: '/webmcp/b.txt' });
    const after = await call('mfs_ls', { path: '/webmcp' });
    expect(after.entries.map((e: any) => e.name).sort()).toEqual(['b.txt', 'copy.txt']);
    const page = await call('mfs_ls', { path: '/webmcp', limit: 1 });
    expect(page.page).toMatchObject({ has_more: true });
    const next = await call('mfs_ls', { path: '/webmcp', limit: 1, cursor: page.page.next_cursor });
    expect(next.entries.length).toBe(1);
  });

  it('keys: gen, list, rename', async () => {
    const gen = await call('key_gen', { name: 'webmcp-key' });
    const list = await call('key_list', {});
    expect(list.keys.map((k: any) => k.name)).toContain('webmcp-key');
    const renamed = await call('key_rename', { old_name: 'webmcp-key', new_name: 'webmcp-key2' });
    expect(renamed.id).toBe(gen.id);
  });

  it('dag_put, dag_get and car_export', async () => {
    const put = await call('dag_put', { data: JSON.stringify({ hello: 'dag', link: { '/': fileCid } }), pin: false });
    const got = await call('dag_get', { path: put.cid });
    expect(got.node).toEqual({ hello: 'dag', link: { '/': fileCid } });
    expect(got.codec).toBe('dag-cbor');
    const exp = await call('car_export', { root: put.cid });
    expect(exp.inline).toBe(true);
    // CARv1 header: varint length, then dag-cbor {roots, version}. The root CID bytes are inside it.
    const car = Buffer.from(exp.car_base64, 'base64');
    expect(car.length).toBe(exp.size);
    expect(car[0]).toBeGreaterThan(0);
  });

  it('ipns_publish and ipns_resolve', async () => {
    const pub = await call('ipns_publish', { path: `/ipfs/${fileCid}`, key: 'self', lifetime: '1h' });
    expect(pub.value).toBe(`/ipfs/${fileCid}`);
    const res = await call('ipns_resolve', { name: pub.name });
    expect(res.path).toBe(`/ipfs/${fileCid}`);
  });

  it('routing_provide, swarm_peers, stats_bw, repo_stat', async () => {
    // A test node has no peers, so Kubo refuses to announce; the tool must say so instead of throwing.
    const prov = await toolSet().find((t) => t.name === 'routing_provide')!.execute({ cid: fileCid });
    expect(prov).toMatchObject({ ok: false, status: 'error', code: 'rpc_error' });
    const peers = await call('swarm_peers', {});
    expect(Array.isArray(peers.peers)).toBe(true);
    const bw = await call('stats_bw', {});
    expect(bw.total_in).toBeGreaterThanOrEqual(0);
    const repo = await call('repo_stat', {});
    expect(repo.num_objects).toBeGreaterThan(0);
  });

  it('routing_findprovs and routing_findpeer finish with a result or a clear error', async () => {
    const tools = toolSet();
    const provs = await tools.find((t) => t.name === 'routing_findprovs')!.execute({ cid: fileCid, num_providers: 1 });
    expect(['done', 'error']).toContain(provs.status);
    const peer = await tools.find((t) => t.name === 'routing_findpeer')!.execute({ peer_id: '12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo' });
    expect(['done', 'error']).toContain(peer.status);
  });

  it('a declined write changes nothing on the node', async () => {
    const decline = vi.fn(async () => false);
    const tools = toolSet(decline);
    const env = await tools.find((t) => t.name === 'mfs_mkdir')!.execute({ path: '/declined' });
    expect(env).toMatchObject({ ok: false, status: 'declined' });
    expect(decline).toHaveBeenCalledTimes(1);
    const client = create({ url: daemon.url });
    await expect(client.files.stat('/declined')).rejects.toThrow();
    const add = await tools.find((t) => t.name === 'ipfs_add')!.execute({ content: 'never added xyz', pin: true, provide: false });
    expect(add).toMatchObject({ ok: false, status: 'declined' });
  });

  it('asked for confirmation on every write above and on no read', () => {
    const names = new Set(requests.map((r) => r.tool));
    for (const n of ['ipfs_add', 'pin_add', 'mfs_mkdir', 'mfs_write', 'mfs_cp', 'mfs_mv', 'key_gen', 'key_rename', 'dag_put', 'ipns_publish', 'routing_provide']) {
      expect(names.has(n), `${n} should have been confirmed`).toBe(true);
    }
    for (const r of requests) expect(r.confirmation).toBe('write');
  });

  it('DNSLink names are declined before any request, key names are not', async () => {
    const tools = createWebMcpTools({ client: create({ url: daemon.url }) });
    const run = (name: string, input: unknown) => tools.find((t) => t.name === name)!.execute(input);
    expect(await run('ipfs_cat', { path: '/ipns/example.com/a' })).toMatchObject({ ok: false, status: 'declined', code: 'dnslink_off' });
    expect(await run('ipns_resolve', { name: 'example.com' })).toMatchObject({ ok: false, status: 'declined', code: 'dnslink_off' });
    expect(await run('mfs_stat', { path: '/a/../ipns/example.com' })).toMatchObject({ ok: false, code: 'invalid_input' });
    const self = (await call('ipfs_node_info', {})).peer_id as string;
    // A libp2p-key name passes the toggle; this node has not published under it, so the node answers.
    const res = await run('ipns_resolve', { name: self });
    expect(res.status === 'done' || (res.status === 'error' && res.code === 'rpc_error')).toBe(true);
  });

  it('an unreachable node gives an error result, not an exception', async () => {
    const dead = create({ url: 'http://127.0.0.1:9' });
    const tools = createWebMcpTools({ client: dead, timeoutMs: 3000 });
    const env = await tools.find((t) => t.name === 'ipfs_node_info')!.execute({});
    expect(env).toMatchObject({ ok: false, status: 'error', code: 'rpc_error' });
  });
});
