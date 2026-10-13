import type { Frame, Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';

// A minimal MCP Apps host (SEP-1865, spec 2026-01-26) for browser tests. It embeds a built view in a
// sandboxed iframe under the specification's default CSP and plays the host side of the protocol:
// answers ui/initialize, then sends tool input and the tool result, answers tools/call from a
// scripted table, and records ui/message and ui/update-model-context.
//
// Written against the protocol, not against the SDK, so a regression in the SDK wiring and in the
// view shows up here as a failed render, not as a passing mock.

export const MCP_APPS_DEFAULT_CSP =
  "default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; media-src 'self' data:; connect-src 'none'";

const DIST = resolve(import.meta.dirname, '..', '..', 'dist');

export function viewHtml(view: string): string {
  return readFileSync(join(DIST, `${view}.html`), 'utf8');
}

export interface HostScript {
  view: string;
  theme: 'dark' | 'light';
  toolName: string;
  input?: Record<string, unknown>;
  /** The tool result the host delivers (a CallToolResult). */
  result: unknown;
  /** Answers for tools/call, keyed by tool name. */
  toolReplies?: Record<string, unknown>;
  capabilities?: { serverTools?: boolean; message?: boolean; updateModelContext?: boolean };
}

const HOST_PAGE = `<!doctype html><meta charset="utf-8"><body style="margin:0">
<iframe id="v" sandbox="allow-scripts" style="border:0;width:100%;height:100vh"></iframe>
<script>
(function () {
  const cfg = window.__HOST__;
  const frame = document.getElementById('v');
  window.__log = { messages: [], contexts: [], toolCalls: [], sizes: [] };
  const send = (m) => frame.contentWindow.postMessage(Object.assign({ jsonrpc: '2.0' }, m), '*');
  const reply = (id, result) => send({ id, result });
  window.addEventListener('message', (e) => {
    if (e.source !== frame.contentWindow) return;
    const m = e.data || {};
    if (m.method === 'ui/initialize') {
      const caps = {};
      if (cfg.capabilities.serverTools) caps.serverTools = {};
      if (cfg.capabilities.message) caps.message = { text: {} };
      if (cfg.capabilities.updateModelContext) caps.updateModelContext = { text: {} };
      reply(m.id, {
        protocolVersion: m.params && m.params.protocolVersion || '2026-01-26',
        hostInfo: { name: 'playwright-host', version: '1.0.0' },
        hostCapabilities: caps,
        hostContext: { theme: cfg.theme, displayMode: 'inline', toolInfo: { id: 1, tool: { name: cfg.toolName, inputSchema: { type: 'object' } } } },
      });
    } else if (m.method === 'ui/notifications/initialized') {
      send({ method: 'ui/notifications/tool-input', params: { arguments: cfg.input || {} } });
      send({ method: 'ui/notifications/tool-result', params: cfg.result });
    } else if (m.method === 'tools/call') {
      window.__log.toolCalls.push(m.params);
      reply(m.id, (cfg.toolReplies || {})[m.params.name] || { content: [], isError: true });
    } else if (m.method === 'ui/message') {
      window.__log.messages.push(m.params);
      reply(m.id, {});
    } else if (m.method === 'ui/update-model-context') {
      window.__log.contexts.push(m.params);
      reply(m.id, {});
    } else if (m.method === 'ui/notifications/size-changed') {
      window.__log.sizes.push(m.params);
    } else if (m.id !== undefined && m.method === 'ping') {
      reply(m.id, {});
    }
  });
  frame.setAttribute('csp', cfg.csp);
  frame.srcdoc = cfg.html;
})();
</script>`;

/** Open a view in the scripted host and return the view's frame once it has drawn something. */
export async function openView(page: Page, script: HostScript): Promise<Frame> {
  // page.setContent rewrites the current document, so an init script (which runs on navigation) would
  // never fire; set the config on the live window instead.
  await page.evaluate((cfg) => {
    (window as unknown as { __HOST__: unknown }).__HOST__ = cfg;
  }, {
    ...script,
    capabilities: script.capabilities ?? { serverTools: true, message: true, updateModelContext: true },
    csp: MCP_APPS_DEFAULT_CSP,
    html: viewHtml(script.view),
  });
  await page.setContent(HOST_PAGE);
  const handle = await page.waitForSelector('iframe#v');
  const frame = await handle.contentFrame();
  if (!frame) throw new Error('view iframe has no content frame');
  await frame.waitForSelector('main.app');
  await frame.waitForFunction(() => document.fonts.status === 'loaded');
  return frame;
}

/** Everything the view sent the host. */
export async function hostLog(page: Page): Promise<{ messages: unknown[]; contexts: unknown[]; toolCalls: unknown[]; sizes: unknown[] }> {
  return page.evaluate(() => (window as unknown as { __log: never }).__log);
}
