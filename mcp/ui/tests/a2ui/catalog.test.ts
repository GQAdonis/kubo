import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { A2UI_CATALOG_ID, createIpfsCatalog } from '../../src/a2ui/catalog';
import { IMPLEMENTATIONS } from '../../src/a2ui/implementations';
import { validateCatalogManifest, type CatalogIssueKind } from '../../src/a2ui/validate';
import { manifestText } from '../../tools/catalog-manifest';
import { CID_A, OUTPUTS } from '../fixtures/outputs';
import { UI_ROOT } from '../helpers/contract';

const onDisk = () => JSON.parse(readFileSync(join(UI_ROOT, 'catalog', 'ipfs-mcp.catalog.json'), 'utf8')) as Record<string, unknown>;
const implemented = Object.keys(IMPLEMENTATIONS);
const clone = <T,>(v: T): T => JSON.parse(JSON.stringify(v)) as T;
const kinds = (m: unknown, impl: readonly string[] = implemented): CatalogIssueKind[] =>
  validateCatalogManifest({ manifest: m, implemented: impl }).map((i) => i.kind);

let generated: string;
beforeAll(() => {
  // zod-to-json-schema reports the shared recursive FunctionCall type while building; expected, see gen-catalog.mjs
  vi.spyOn(console, 'warn').mockImplementation(() => undefined);
  generated = manifestText();
});

describe('A2UI catalog manifest generation', () => {
  it('matches the committed catalog/ipfs-mcp.catalog.json byte for byte', () => {
    expect(readFileSync(join(UI_ROOT, 'catalog', 'ipfs-mcp.catalog.json'), 'utf8')).toBe(generated);
  });

  it('publishes a catalog ID equal to $id and to the Catalog the code exports', () => {
    const m = onDisk();
    expect(m['catalogId']).toBe(A2UI_CATALOG_ID);
    expect(m['$id']).toBe(A2UI_CATALOG_ID);
    expect(createIpfsCatalog().id).toBe(A2UI_CATALOG_ID);
    expect(createIpfsCatalog().protocolVersion).toBe('v0.9');
  });

  it('lists exactly the contract components', () => {
    expect(Object.keys(onDisk()['components'] as object).sort()).toEqual(
      ['ApprovalCard', 'ConfirmationDialog', 'ContentCard', 'DagViewer', 'EgressBadge', 'IpnsRecordCard', 'MessageStream', 'MfsTree', 'NodeStatusPanel', 'PinList', 'ToolCallCard'],
    );
  });
});

describe('A2UI catalog validation', () => {
  it('passes for the real manifest', () => {
    expect(validateCatalogManifest({ manifest: onDisk(), implemented })).toEqual([]);
  });

  it('fails on a dangling entry: a catalog component with no implementation', () => {
    const m = clone(onDisk());
    const components = m['components'] as Record<string, unknown>;
    components['Ghost'] = { type: 'object', properties: { id: { $ref: '#/$defs/ComponentId' }, component: { const: 'Ghost' } }, required: ['id', 'component'] };
    const k = kinds(m);
    expect(k).toContain('unknown-component');
    expect(k).toContain('missing-implementation');
    expect(k).toContain('any-component-mismatch'); // anyComponent does not list it
  });

  it('fails on a dangling entry: an implementation the catalog does not list', () => {
    expect(kinds(onDisk(), [...implemented, 'Ghost'])).toContain('orphan-implementation');
  });

  it('fails when a catalog entry loses its implementation', () => {
    const k = kinds(onDisk(), implemented.filter((n) => n !== 'PinList'));
    expect(k).toContain('missing-implementation');
  });

  it('fails when a contract component is missing from the catalog', () => {
    const m = clone(onDisk());
    delete (m['components'] as Record<string, unknown>)['EgressBadge'];
    expect(kinds(m)).toContain('missing-component');
  });

  it('fails on a dangling $ref and on an external $ref', () => {
    const m = clone(onDisk());
    ((m['components'] as Record<string, { properties: Record<string, unknown> }>)['PinList'] as { properties: Record<string, unknown> }).properties['extra'] = { $ref: '#/$defs/Nope' };
    expect(kinds(m)).toContain('dangling-ref');
    const n = clone(onDisk());
    ((n['components'] as Record<string, { properties: Record<string, unknown> }>)['PinList'] as { properties: Record<string, unknown> }).properties['extra'] = { $ref: 'https://example.invalid/x.json#/y' };
    expect(kinds(n)).toContain('external-ref');
  });

  it('fails when anyComponent points at a missing entry', () => {
    const m = clone(onDisk());
    const any = (m['$defs'] as { anyComponent: { oneOf: unknown[] } }).anyComponent.oneOf;
    any.push({ $ref: '#/components/Ghost' });
    const k = kinds(m);
    expect(k).toContain('dangling-ref');
    expect(k).toContain('any-component-mismatch');
  });

  it('fails when a view loses its tool binding or $id differs from catalogId', () => {
    const m = clone(onDisk());
    ((m['components'] as Record<string, Record<string, unknown>>)['PinList'] as Record<string, unknown>)['x-ipfs-mcp'] = { kind: 'view', tools: ['pin_ls'] };
    m['$id'] = 'https://example.invalid/other';
    const k = kinds(m);
    expect(k).toContain('tool-binding-mismatch');
    expect(k).toContain('id-mismatch');
  });

  it('flags untyped child references', () => {
    const m = clone(onDisk());
    ((m['components'] as Record<string, { properties: Record<string, unknown> }>)['MfsTree'] as { properties: Record<string, unknown> }).properties['children'] = { type: 'array' };
    expect(kinds(m)).toContain('child-reference-untyped');
  });
});

describe('manifest against the official A2UI v0.9 envelope schemas', () => {
  const schemaDir = join(UI_ROOT, 'node_modules', '@a2ui', 'web_core', 'src', 'v0_9', 'schemas');
  const read = (f: string) => JSON.parse(readFileSync(join(schemaDir, f), 'utf8')) as Record<string, unknown>;
  let validate: ReturnType<Ajv2020['compile']>;

  beforeAll(() => {
    const ajv = new Ajv2020({ strict: false, allErrors: true });
    addFormats(ajv);
    ajv.addSchema(read('common_types.json'));
    // The envelope refs "catalog.json#/$defs/anyComponent": map that name to our catalog (A2UI-P, "Swappable Catalogs").
    const ours = clone(onDisk());
    ours['$id'] = 'https://a2ui.org/specification/v0_9/catalog.json';
    ajv.addSchema(ours);
    ajv.addSchema(read('server_to_client.json'));
    validate = ajv.getSchema('https://a2ui.org/specification/v0_9/server_to_client.json') as typeof validate;
  });

  const update = (components: unknown[]) => ({ version: 'v0.9', updateComponents: { surfaceId: 's1', components } });

  it('accepts createSurface for the published catalog ID', () => {
    expect(validate({ version: 'v0.9', createSurface: { surfaceId: 's1', catalogId: A2UI_CATALOG_ID, theme: { mode: 'light' } } })).toBe(true);
  });

  it('accepts a view with a literal contract output and with a data-model path', () => {
    expect(validate(update([{ id: 'root', component: 'PinList', tool: 'pin_ls', output: OUTPUTS.pin_ls[0] }]))).toBe(true);
    expect(validate(update([{ id: 'root', component: 'PinList', tool: 'pin_ls', output: { path: '/pins' } }]))).toBe(true);
  });

  it('accepts a literal output for every bound tool', () => {
    const bound: [string, string][] = [['ContentCard', 'ipfs_cat'], ['MfsTree', 'mfs_ls'], ['DagViewer', 'dag_stat'], ['IpnsRecordCard', 'key_list'], ['NodeStatusPanel', 'repo_stat']];
    for (const [component, tool] of bound) {
      const out = (OUTPUTS as Record<string, unknown[]>)[tool]?.[0];
      expect(validate(update([{ id: 'root', component, tool, output: out }])), `${component}/${tool}`).toBe(true);
    }
  });

  it('rejects a literal output that does not match the tool schema', () => {
    expect(validate(update([{ id: 'root', component: 'PinList', tool: 'pin_ls', output: { pins: [{ cid: CID_A, type: 'sideways' }], page: { has_more: false } } }]))).toBe(false);
  });

  it('rejects a tool the view is not bound to, an unknown component, and extra properties', () => {
    expect(validate(update([{ id: 'root', component: 'PinList', tool: 'ipfs_cat', output: { path: '/x' } }]))).toBe(false);
    expect(validate(update([{ id: 'root', component: 'Ghost' }]))).toBe(false);
    expect(validate(update([{ id: 'root', component: 'PinList', tool: 'pin_ls', output: { path: '/p' }, surprise: 1 }]))).toBe(false);
  });

  it('accepts chat components bound to the AG-UI fixtures shapes', () => {
    expect(
      validate(update([
        { id: 'root', component: 'EgressBadge', tool: 'ipfs_cat', acknowledgedHosts: ['model.example.net'] },
        { id: 'm', component: 'MessageStream', messages: [{ id: 'm1', role: 'user', content: 'hi' }] },
        { id: 'c', component: 'ToolCallCard', call: { id: 't1', name: 'pin_ls', arguments: '{}', status: 'running' } },
      ])),
    ).toBe(true);
  });
});
