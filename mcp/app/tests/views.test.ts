import { TOOL_META, type ToolName } from '@gqadonis/ipfs-mcp-ui';
import { describe, expect, it } from 'vitest';
import { contractJson } from './helpers/fixtures';
import { MCP_APP_MIME_TYPE, VIEWS, VIEW_IDS, isViewId, toolsForView, viewForTool } from '../src/views';

interface CatalogTool {
  name: string;
  ui?: { a2uiComponent?: string; appView?: string };
}

describe('view set follows the contract', () => {
  const catalog = contractJson<{ tools: CatalogTool[] } | CatalogTool[]>('catalog.json');
  const tools = Array.isArray(catalog) ? catalog : catalog.tools;

  it('has exactly the appView names the catalog declares', () => {
    const declared = [...new Set(tools.flatMap((t) => (t.ui?.appView ? [t.ui.appView] : [])))].sort();
    expect([...VIEW_IDS].sort()).toEqual(declared);
  });

  it('agrees with the generated tool metadata for every tool', () => {
    for (const t of tools) {
      const meta = TOOL_META[t.name as ToolName];
      expect(meta.appView, t.name).toBe(t.ui?.appView ?? null);
    }
  });

  it('gives every view at least one tool and every tool at most one view', () => {
    for (const id of VIEW_IDS) expect(toolsForView(id).length, id).toBeGreaterThan(0);
    for (const name of Object.keys(TOOL_META) as ToolName[]) {
      const view = viewForTool(name);
      expect(view === undefined, name).toBe(TOOL_META[name].appView === null);
    }
  });

  it('leaves the IPNS and key tools without a view', () => {
    for (const name of ['ipns_publish', 'ipns_resolve', 'key_list', 'key_gen', 'key_rename', 'key_rm'] as const) {
      expect(viewForTool(name)).toBeUndefined();
    }
  });
});

describe('resource identity', () => {
  it('serves each view at ui://ipfs-mcp/<view> with the MCP App MIME type', () => {
    expect(MCP_APP_MIME_TYPE).toBe('text/html;profile=mcp-app');
    for (const id of VIEW_IDS) {
      expect(VIEWS[id].uri).toBe(`ui://ipfs-mcp/${id}`);
      expect(VIEWS[id].file).toBe(`${id}.html`);
      expect(VIEWS[id].title).toMatch(/^IPFS /);
    }
  });

  it('recognises only its own view ids', () => {
    expect(isViewId('pins')).toBe(true);
    expect(isViewId('gallery')).toBe(false);
    expect(isViewId(undefined)).toBe(false);
  });
});
