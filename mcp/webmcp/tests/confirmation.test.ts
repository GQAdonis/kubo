import { describe, expect, it, vi } from 'vitest';
import { TOOL_DEFINITIONS } from '../src/generated/tools.js';
import { createWebMcpTools } from '../src/index.js';
import { recorder } from './helpers/mock-client.js';

// A valid input for every write tool, so a decline is tested on the real code path.
const VALID_WRITE_INPUT: Record<string, Record<string, unknown>> = {
  ipfs_add: { content: 'hello', pin: true, provide: false },
  pin_add: { path: 'bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi' },
  mfs_write: { path: '/a.txt', content: 'x' },
  mfs_mkdir: { path: '/d' },
  mfs_cp: { source: '/a', destination: '/b' },
  mfs_mv: { source: '/a', destination: '/b' },
  ipns_publish: { path: '/ipfs/bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi' },
  key_gen: { name: 'k1' },
  key_rename: { old_name: 'k1', new_name: 'k2' },
  dag_put: { data: '{"a":1}', pin: false },
  routing_provide: { cid: 'bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi' },
};

const writeNames = TOOL_DEFINITIONS.filter((t) => t.confirmation === 'write').map((t) => t.name);

describe('confirmation hook', () => {
  it('has a valid test input for every exported write tool', () => {
    expect(Object.keys(VALID_WRITE_INPUT).sort()).toEqual([...writeNames].sort());
  });

  it.each(writeNames)('%s: no RPC call when the hook declines', async (name) => {
    const { client, calls } = recorder();
    const confirm = vi.fn().mockResolvedValue(false);
    const tool = createWebMcpTools({ client, confirm, includeWrites: true }).find((t) => t.name === name)!;
    const result = await tool.execute(VALID_WRITE_INPUT[name]);
    expect(confirm).toHaveBeenCalledTimes(1);
    expect(confirm.mock.calls[0]![0]).toMatchObject({ tool: name, confirmation: 'write' });
    expect(calls).toEqual([]);
    expect(result).toMatchObject({ ok: false, status: 'declined', code: 'user_declined' });
  });

  it.each(writeNames)('%s: a hook that throws counts as a decline', async (name) => {
    const { client, calls } = recorder();
    const confirm = vi.fn().mockRejectedValue(new Error('modal crashed'));
    const tool = createWebMcpTools({ client, confirm, includeWrites: true }).find((t) => t.name === name)!;
    const result = await tool.execute(VALID_WRITE_INPUT[name]);
    expect(calls).toEqual([]);
    expect(result).toMatchObject({ ok: false, status: 'declined' });
  });

  it('treats anything other than true as a decline', async () => {
    const { client, calls } = recorder();
    for (const answer of [undefined, null, 'yes', 1, {}]) {
      const confirm = vi.fn().mockResolvedValue(answer);
      const tool = createWebMcpTools({ client, confirm, includeWrites: true }).find((t) => t.name === 'mfs_mkdir')!;
      const result = await tool.execute(VALID_WRITE_INPUT.mfs_mkdir);
      expect(result).toMatchObject({ ok: false, status: 'declined' });
    }
    expect(calls).toEqual([]);
  });

  it('creates no write tool without a hook, or without the second switch', () => {
    const { client } = recorder();
    const names = (opts: object) => createWebMcpTools({ client, ...opts }).map((t) => t.name);
    const none = TOOL_DEFINITIONS.filter((t) => t.confirmation === 'none').map((t) => t.name);
    expect(names({})).toEqual(none);
    expect(names({ includeWrites: true })).toEqual(none);
    expect(names({ confirm: () => true })).toEqual(none);
    expect(names({ confirm: () => true, includeWrites: true }).length).toBe(TOOL_DEFINITIONS.length);
  });

  it('runs the RPC call after approval, with the validated input', async () => {
    const { client, calls } = recorder({
      'files.mkdir': async () => undefined,
      'files.stat': async () => ({ cid: 'bafyfake', size: 0, cumulativeSize: 0, type: 'directory', blocks: 0 }),
    });
    const confirm = vi.fn().mockResolvedValue(true);
    const tool = createWebMcpTools({ client, confirm, includeWrites: true }).find((t) => t.name === 'mfs_mkdir')!;
    const result = await tool.execute({ path: '/d', parents: true });
    expect(result).toMatchObject({ ok: true, status: 'done', result: { path: '/d', cid: 'bafyfake' } });
    expect(calls.map((c) => c.path)).toEqual(['files.mkdir', 'files.stat']);
  });

  it('passes the host signal to the hook when there is one, and nothing when there is none', async () => {
    const { client } = recorder();
    const seen: unknown[] = [];
    const confirm = vi.fn(async (_r, ctx) => { seen.push(ctx); return false; });
    const tool = createWebMcpTools({ client, confirm, includeWrites: true }).find((t) => t.name === 'mfs_mkdir')!;
    await tool.execute({ path: '/d' });
    const ac = new AbortController();
    await tool.execute({ path: '/d' }, { signal: ac.signal });
    expect(seen[0]).toEqual({});
    expect(seen[1]).toEqual({ signal: ac.signal });
  });

  it('sends no request when the call is cancelled while the user is deciding', async () => {
    const { client, calls } = recorder();
    const ac = new AbortController();
    const confirm = vi.fn(async () => { ac.abort(); return true; });
    const tool = createWebMcpTools({ client, confirm, includeWrites: true }).find((t) => t.name === 'mfs_mkdir')!;
    const result = await tool.execute({ path: '/d' }, { signal: ac.signal });
    expect(calls).toEqual([]);
    expect(result).toMatchObject({ ok: false, status: 'declined', code: 'cancelled' });
  });

  it('never invokes the hook for read tools', async () => {
    const { client } = recorder({ 'key.list': async () => [{ name: 'self', id: 'k51abc' }] });
    const confirm = vi.fn().mockResolvedValue(false);
    const tool = createWebMcpTools({ client, confirm, includeWrites: true }).find((t) => t.name === 'key_list')!;
    const result = await tool.execute({});
    expect(confirm).not.toHaveBeenCalled();
    expect(result).toMatchObject({ ok: true, result: { keys: [{ name: 'self', id: 'k51abc' }] } });
  });

  it('invalid input is rejected before the hook and before any RPC call', async () => {
    const { client, calls } = recorder();
    const confirm = vi.fn().mockResolvedValue(true);
    const tool = createWebMcpTools({ client, confirm, includeWrites: true }).find((t) => t.name === 'mfs_write')!;
    const result = await tool.execute({ path: '/ipfs/bafy', content: 'x' });
    expect(confirm).not.toHaveBeenCalled();
    expect(calls).toEqual([]);
    expect(result).toMatchObject({ ok: false, status: 'error', code: 'invalid_input' });
  });
});
