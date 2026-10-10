import type { App } from '@modelcontextprotocol/ext-apps';
import type { HostPort } from './port';
import type { ToolCall } from '../core/result';

/** Per-call ceiling. A host that never answers must not leave a view loading forever. */
export const CALL_TIMEOUT_MS = 30_000;

/**
 * The one adapter between the views and the MCP Apps SDK: tools/call through the host's proxy
 * (App.callServerTool), ui/message and ui/update-model-context. Capabilities are read from what the
 * host declared at ui/initialize, so a host that offers less gets a view that does less, not an error.
 */
export function createMcpPort(app: App): HostPort {
  const caps = app.getHostCapabilities();
  const canCallTools = caps?.serverTools !== undefined;
  const canMessage = caps?.message?.text !== undefined;
  const canUpdateContext = caps?.updateModelContext?.text !== undefined;

  return {
    canCallTools,
    canMessage,
    canUpdateContext,
    callTool: (call: ToolCall, signal?: AbortSignal) =>
      app.callServerTool(
        { name: call.tool, arguments: { ...call.args } },
        { timeout: CALL_TIMEOUT_MS, ...(signal ? { signal } : {}) },
      ),
    sendMessage: async (text: string) => {
      const res = await app.sendMessage({ role: 'user', content: [{ type: 'text', text }] });
      if (res.isError) throw new Error('The host did not accept the message.');
    },
    updateModelContext: async (text: string) => {
      await app.updateModelContext({ content: [{ type: 'text', text }] });
    },
  };
}
