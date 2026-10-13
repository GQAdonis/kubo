import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { renderToolResult } from '../../src/components/registry';
import { TOOL_META, type ToolName } from '../../src/contract/generated/tool-meta';
import { UiScope } from '../../src/scope/UiScope';
import { ALL_TOOLS, OUTPUTS } from '../fixtures/outputs';
import { blockNetwork } from '../helpers/render';

describe('offline rendering', () => {
  it('renders every bound tool fixture with networking disabled and makes no request', () => {
    const net = blockNetwork();
    try {
      for (const tool of ALL_TOOLS) {
        OUTPUTS[tool].forEach((fixture) => {
          const r = renderToolResult(tool as ToolName, fixture);
          if (TOOL_META[tool].component === null) {
            expect(r.kind, tool).toBe('unbound');
            return;
          }
          expect(r.kind, tool).toBe('view');
          if (r.kind !== 'view') return;
          const { container, unmount } = render(
            <UiScope injectStyles={false}>{r.node}</UiScope>,
          );
          // Nothing that loads a resource is ever rendered.
          expect(container.querySelectorAll('img, iframe, script, link, video, audio, object, embed, source'), tool).toHaveLength(0);
          for (const a of container.querySelectorAll('a[href]')) {
            expect(a.getAttribute('href'), tool).not.toMatch(/^(https?:)?\/\//);
          }
          unmount();
        });
      }
    } finally {
      net.restore();
    }
    expect(net.calls).toEqual([]);
  });

  it('refuses to draw output the contract does not describe, and says which fields', () => {
    const r = renderToolResult('pin_ls', { pins: [{ cid: 'bafy', type: 'sideways' }], page: { has_more: false } });
    expect(r.kind).toBe('invalid');
    if (r.kind === 'invalid') expect(r.issues.join(' ')).toContain('pins.0.type');
  });

  it('binds the routing tools to NodeStatusPanel, as the contract does', () => {
    for (const tool of ['routing_findprovs', 'routing_findpeer', 'routing_provide'] as const) {
      expect(TOOL_META[tool].component).toBe('NodeStatusPanel');
      expect(TOOL_META[tool].appView).toBe('node-status');
      const r = renderToolResult(tool, OUTPUTS[tool][0]);
      expect(r.kind, tool).toBe('view');
      if (r.kind === 'view') expect(r.component).toBe('NodeStatusPanel');
    }
  });
});
