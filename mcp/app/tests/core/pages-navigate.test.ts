import { TOOL_META, type ToolName } from '@gqadonis/ipfs-mcp-ui';
import { describe, expect, it } from 'vitest';
import { canOpenPaths, openPathCall } from '../../src/core/navigate';
import { canPage, mergePage, nextPageCall } from '../../src/core/pages';
import { isReadOnlyTool } from '../../src/core/result';
import { CID_A, CID_B, CID_C, contractJson, OUTPUTS } from '../helpers/fixtures';

interface InputSchema {
  properties?: Record<string, unknown>;
  required?: string[];
  additionalProperties?: boolean;
}

/** Arguments a view builds must satisfy the contract input schema of the tool it calls. */
function expectValidArgs(tool: ToolName, args: Record<string, unknown>): void {
  const schema = contractJson<InputSchema>(`schemas/tools/${tool}.input.json`);
  for (const key of Object.keys(args)) expect(Object.keys(schema.properties ?? {}), `${tool}.${key}`).toContain(key);
  for (const key of schema.required ?? []) expect(Object.keys(args), `${tool} requires ${key}`).toContain(key);
}

describe('pagination', () => {
  it('pages only paginated, read-only tools', () => {
    for (const tool of ['ipfs_ls', 'pin_ls', 'pin_verify', 'mfs_ls', 'swarm_peers'] as const) expect(canPage(tool), tool).toBe(true);
    // ipfs_index_reconcile is paginated but a write: a view never pages it.
    expect(TOOL_META.ipfs_index_reconcile.paginated).toBe(true);
    expect(canPage('ipfs_index_reconcile')).toBe(false);
    expect(canPage('pin_add')).toBe(false);
  });

  it('repeats the call with the cursor and keeps the other arguments', () => {
    const next = nextPageCall({ tool: 'pin_ls', args: { type: 'recursive', limit: 50 } }, 'cur_2');
    expect(next).toEqual({ tool: 'pin_ls', args: { type: 'recursive', limit: 50, cursor: 'cur_2' } });
    expectValidArgs(next.tool, next.args);
  });

  it('appends the next pin page and takes its page info', () => {
    const first = OUTPUTS.pin_ls[0] as { pins: unknown[] };
    const second = { pins: [{ cid: CID_C, type: 'direct' }], page: { has_more: false } };
    const merged = mergePage('pin_ls', first, second) as { pins: unknown[]; page: { has_more: boolean } };
    expect(merged.pins).toHaveLength(first.pins.length + 1);
    expect(merged.page).toEqual({ has_more: false });
  });

  it('refuses a page for another path', () => {
    const first = { path: '/docs', entries: [{ name: 'a', type: 'file', cid: CID_A }], page: { has_more: true, next_cursor: 'c' } };
    const other = { path: '/elsewhere', entries: [{ name: 'b', type: 'file', cid: CID_B }], page: { has_more: false } };
    expect(mergePage('mfs_ls', first, other)).toBeUndefined();
  });

  it('refuses a page that does not match the contract', () => {
    const first = OUTPUTS.pin_ls[0];
    expect(mergePage('pin_ls', first, { pins: [{ cid: 'not a cid', type: 'direct' }], page: { has_more: false } })).toBeUndefined();
    expect(mergePage('pin_ls', first, { nothing: true })).toBeUndefined();
    expect(mergePage('pin_add', first, first)).toBeUndefined();
  });

  it('does not mutate what is already on screen', () => {
    const first = structuredClone(OUTPUTS.pin_ls[0]) as { pins: unknown[] };
    const before = first.pins.length;
    mergePage('pin_ls', first, { pins: [{ cid: CID_C, type: 'direct' }], page: { has_more: false } });
    expect(first.pins).toHaveLength(before);
  });
});

describe('opening paths', () => {
  const mfs = OUTPUTS.mfs_ls[0];

  it('opens a directory from ipfs_ls as another ipfs_ls', () => {
    const call = openPathCall('ipfs_ls', OUTPUTS.ipfs_ls[0], `${CID_A}/images`);
    expect(call).toEqual({ tool: 'ipfs_ls', args: { path: `${CID_A}/images` } });
    expectValidArgs(call!.tool, call!.args as Record<string, unknown>);
  });

  it('opens an mfs directory entry with mfs_ls and a file entry with mfs_stat', () => {
    const dir = openPathCall('mfs_ls', mfs, '/docs/notes/archive');
    const file = openPathCall('mfs_ls', mfs, '/docs/notes/todo.md');
    expect(dir).toEqual({ tool: 'mfs_ls', args: { path: '/docs/notes/archive' } });
    expect(file).toEqual({ tool: 'mfs_stat', args: { path: '/docs/notes/todo.md' } });
    expectValidArgs(dir!.tool, dir!.args as Record<string, unknown>);
    expectValidArgs(file!.tool, file!.args as Record<string, unknown>);
  });

  it('opens a path-trail ancestor with mfs_ls', () => {
    expect(openPathCall('mfs_ls', mfs, '/docs')).toEqual({ tool: 'mfs_ls', args: { path: '/docs' } });
    expect(openPathCall('mfs_ls', mfs, '/')).toEqual({ tool: 'mfs_ls', args: { path: '/' } });
  });

  it('offers navigation for the listing tools only', () => {
    expect(canOpenPaths('ipfs_ls')).toBe(true);
    expect(canOpenPaths('mfs_ls')).toBe(true);
    expect(canOpenPaths('pin_ls')).toBe(false);
    expect(openPathCall('pin_ls', OUTPUTS.pin_ls[0], '/x')).toBeUndefined();
  });

  it('only ever builds read-only calls', () => {
    const calls = [
      openPathCall('ipfs_ls', OUTPUTS.ipfs_ls[0], 'x'),
      openPathCall('mfs_ls', mfs, '/docs/notes/archive'),
      openPathCall('mfs_ls', mfs, '/docs/notes/todo.md'),
      nextPageCall({ tool: 'pin_ls', args: {} }, 'c'),
    ];
    for (const c of calls) expect(isReadOnlyTool(c!.tool)).toBe(true);
  });
});
