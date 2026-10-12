import { readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import Ajv2020 from 'ajv/dist/2020.js';
import { describe, expect, it } from 'vitest';
// @ts-expect-error plain ESM script without types
import { CONTRACT_ROOT, PKG_ROOT, evalRule, generate } from '../scripts/lib/generate.mjs';
import { DESCRIBED_TOOLS } from '../src/confirm.js';
import { EXECUTORS } from '../src/executors/index.js';
import { EXCLUDED_TOOLS, TOOL_DEFINITIONS } from '../src/generated/tools.js';

const catalog = JSON.parse(readFileSync(join(CONTRACT_ROOT, 'catalog.json'), 'utf8'));
const rules = JSON.parse(readFileSync(join(CONTRACT_ROOT, 'derivations/webmcp.json'), 'utf8')).rules;

describe('generated from mcp/contract', () => {
  it('matches the checked-in file (drift test)', () => {
    const files = generate() as Record<string, string>;
    for (const [rel, content] of Object.entries(files)) {
      expect(readFileSync(join(PKG_ROOT, rel), 'utf8'), `${rel} differs from the contract; run pnpm gen`).toBe(content);
    }
  });

  it('accounts for every contract tool exactly once', () => {
    const exported = new Set(TOOL_DEFINITIONS.map((t) => t.name));
    const excluded = new Set(EXCLUDED_TOOLS.map((t) => t.name));
    for (const t of catalog.tools) {
      expect(exported.has(t.name) !== excluded.has(t.name), `${t.name} must be exported xor excluded`).toBe(true);
    }
    expect(exported.size + excluded.size).toBe(catalog.tools.length);
  });

  it('exports no destructive tool and names each destructive tool as excluded', () => {
    expect(TOOL_DEFINITIONS.filter((t) => (t.confirmation as string) === 'destructive')).toEqual([]);
    const destructive = catalog.tools.filter((t: any) => t.confirmation === 'destructive').map((t: any) => t.name);
    expect(destructive.length).toBeGreaterThan(0);
    for (const name of destructive) {
      expect(EXCLUDED_TOOLS).toContainEqual({ name, reason: 'destructive' });
    }
  });

  it('copies names, titles, descriptions and confirmation classes from the catalog', () => {
    for (const def of TOOL_DEFINITIONS) {
      const src = catalog.tools.find((t: any) => t.name === def.name);
      expect(def.title).toBe(src.title);
      expect(def.description).toBe(src.description);
      expect(def.confirmation).toBe(src.confirmation);
      expect(def.group).toBe(src.group);
    }
  });

  it('derives annotations with the contract rules and emits only WebMCP annotation names', () => {
    const groups = new Map<string, any>(catalog.groups.map((g: any) => [g.name, g]));
    for (const def of TOOL_DEFINITIONS) {
      const src = catalog.tools.find((t: any) => t.name === def.name);
      const g = groups.get(src.group);
      expect(Object.keys(def.annotations).sort()).toEqual(['consequentialHint', 'debugging', 'readOnlyHint', 'untrustedContentHint']);
      expect(def.annotations.readOnlyHint).toBe(evalRule(rules.readOnlyHint, src, g));
      expect(def.annotations.consequentialHint).toBe(src.confirmation !== 'none');
      expect(def.annotations.untrustedContentHint).toBe(evalRule(rules.untrustedContentHint, src, g));
      expect(def.annotations.debugging).toBe(g.diagnostic);
    }
  });

  it('gives every exported tool an executor, and every write tool a confirmation description', () => {
    for (const def of TOOL_DEFINITIONS) {
      expect(EXECUTORS[def.name], `${def.name} needs an executor`).toBeTypeOf('function');
      if (def.confirmation === 'write') expect(DESCRIBED_TOOLS, `${def.name} needs a describer`).toContain(def.name);
    }
    expect(Object.keys(EXECUTORS).sort()).toEqual(TOOL_DEFINITIONS.map((t) => t.name).sort());
  });

  it('has self-contained, valid input schemas without confirm_token', () => {
    const ajv = new Ajv2020({ strict: false, allErrors: true });
    for (const def of TOOL_DEFINITIONS) {
      expect(JSON.stringify(def.inputSchema)).not.toContain('$ref');
      expect(JSON.stringify(def.inputSchema)).not.toContain('confirm_token');
      expect(() => ajv.compile(def.inputSchema), def.name).not.toThrow();
      expect(def.inputSchema.type).toBe('object');
    }
  });

  it('uses only schema keywords the package validator understands', () => {
    const known = new Set([
      'type', 'enum', 'pattern', 'minLength', 'maxLength', 'minimum', 'maximum', 'minItems', 'maxItems', 'items',
      'properties', 'required', 'additionalProperties', 'default', 'description', 'title', 'contentEncoding',
    ]);
    const walk = (node: unknown, where: string) => {
      if (Array.isArray(node)) return node.forEach((n, i) => walk(n, `${where}[${i}]`));
      if (!node || typeof node !== 'object') return;
      for (const [k, v] of Object.entries(node)) {
        if (k === 'properties') {
          for (const [pk, pv] of Object.entries(v as object)) walk(pv, `${where}.${pk}`);
          continue;
        }
        expect(known.has(k), `${where}: keyword ${k} is not validated`).toBe(true);
        if (k === 'items') walk(v, `${where}.items`);
      }
    };
    for (const def of TOOL_DEFINITIONS) walk(def.inputSchema, def.name);
  });

  it('resolves the contract root next to the package', () => {
    expect(CONTRACT_ROOT).toBe(resolve(PKG_ROOT, '..', 'contract'));
  });

  it('has no React dependency', () => {
    const pkg = JSON.parse(readFileSync(join(PKG_ROOT, 'package.json'), 'utf8'));
    const all = Object.keys({ ...pkg.dependencies, ...pkg.peerDependencies, ...pkg.devDependencies });
    expect(all.filter((n) => /^react(-|$)/.test(n))).toEqual([]);
  });
});
