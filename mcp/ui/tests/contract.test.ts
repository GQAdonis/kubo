import { describe, expect, it } from 'vitest';
import { generate, UI_ROOT as GEN_ROOT } from '../scripts/lib/generate-contract.mjs';
import { TOOL_OUTPUT_SCHEMAS } from '../src/contract/generated/outputs';
import { TOOL_META, type ToolName } from '../src/contract/generated/tool-meta';
import { VIEW_TOOLS } from '../src/contract/generated/bindings';
import { chatElementsFor } from '../src/chat/rules';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { readContractJson, validateToolOutput } from './helpers/contract';
import { ALL_TOOLS, CID_A, OUTPUTS } from './fixtures/outputs';

describe('generated contract bindings', () => {
  it('match mcp/contract exactly (no drift)', () => {
    for (const [rel, content] of Object.entries(generate())) {
      expect(readFileSync(join(GEN_ROOT, rel), 'utf8'), rel).toBe(content);
    }
  });

  it('cover every catalog tool with an output schema and a fixture', () => {
    const catalog = readContractJson<{ tools: { name: string }[] }>('catalog.json');
    const names = catalog.tools.map((t) => t.name).sort();
    expect(Object.keys(TOOL_OUTPUT_SCHEMAS).sort()).toEqual(names);
    expect(Object.keys(TOOL_META).sort()).toEqual(names);
    expect([...ALL_TOOLS].sort()).toEqual(names);
  });

  it('bind each view component to exactly the tools the catalog names', () => {
    const catalog = readContractJson<{ tools: { name: string; ui: { a2uiComponent: string | null } }[] }>('catalog.json');
    for (const [component, tools] of Object.entries(VIEW_TOOLS)) {
      const expected = catalog.tools.filter((t) => t.ui.a2uiComponent === component).map((t) => t.name);
      expect([...(tools ?? [])], component).toEqual(expected);
    }
  });
});

describe('fixtures against the contract', () => {
  for (const tool of ALL_TOOLS) {
    OUTPUTS[tool].forEach((fixture, i) => {
      it(`${tool} fixture ${i} validates against the JSON Schema and the Zod schema`, () => {
        const json = validateToolOutput(tool, fixture);
        expect(json.errors).toBe('');
        expect(json.ok).toBe(true);
        const zod = TOOL_OUTPUT_SCHEMAS[tool].safeParse(fixture);
        expect(zod.success).toBe(true);
      });
    });
  }
});

describe('Zod and JSON Schema agree on invalid data', () => {
  const cases: [ToolName, unknown][] = [
    ['pin_ls', { pins: [{ cid: CID_A, type: 'sideways' }], page: { has_more: false } }],
    ['pin_ls', { pins: [], page: { has_more: false }, extra: 1 }],
    ['pin_ls', { pins: [], page: { has_more: 'no' } }],
    ['pin_add', { pins: ['not a cid!'] }],
    ['ipfs_cat', { path: CID_A, offset: 0, length: 1, truncated: false, encoding: 'latin1', content: 'x' }],
    ['repo_stat', { repo_size: -1, storage_max: 1, num_objects: 1 }],
    ['ipfs_node_status', { daemon_reachable: true }],
    ['stats_bw', 'not an object'],
  ];
  it.each(cases)('%s rejects the same invalid value in both validators', (tool, value) => {
    expect(validateToolOutput(tool, value).ok).toBe(false);
    expect(TOOL_OUTPUT_SCHEMAS[tool].safeParse(value).success).toBe(false);
  });
});

describe('chat rules derived from the catalog', () => {
  it('apply ToolCallCard to every tool', () => {
    for (const t of ALL_TOOLS) expect(chatElementsFor(t).toolCallCard).toBe(true);
  });
  it('apply ApprovalCard exactly to write and destructive tools', () => {
    for (const t of ALL_TOOLS) expect(chatElementsFor(t).approvalCard).toBe(TOOL_META[t].confirmation !== 'none');
  });
  it('copy egress and untrusted framing from the catalog flags', () => {
    for (const t of ALL_TOOLS) {
      expect(chatElementsFor(t).egressBadge).toBe(TOOL_META[t].contentEgress);
      expect(chatElementsFor(t).untrustedFrame).toBe(TOOL_META[t].returnsUntrustedContent);
    }
  });
});
