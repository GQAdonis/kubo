import type { ToolCall, ToolResultLike } from '../core/result';

/**
 * Everything a view may ask of its host, as a port. The views import nothing from the MCP Apps SDK
 * except in ./mcp-port.ts, so the whole interaction surface is this interface: tests, the gallery
 * harness and the real host adapter all implement it.
 *
 * Views call server tools through callTool and nothing else. There is no fetch, no gateway URL and no
 * RPC address anywhere in the bundle; the sandbox CSP has connect-src 'none' and the build scan
 * proves the code agrees.
 */
export interface HostPort {
  /** The host proxies tools/call to the server (host capability serverTools). Without it the view is static. */
  readonly canCallTools: boolean;
  /** The host accepts ui/message (it can pass a request to the agent as a user message). */
  readonly canMessage: boolean;
  /** The host accepts ui/update-model-context (state the model sees on its next turn, no turn started). */
  readonly canUpdateContext: boolean;
  callTool(call: ToolCall, signal?: AbortSignal): Promise<ToolResultLike>;
  sendMessage(text: string): Promise<void>;
  updateModelContext(text: string): Promise<void>;
}

/** A port for a host that offers nothing: the view renders the result it was given and nothing more. */
export const STATIC_PORT: HostPort = {
  canCallTools: false,
  canMessage: false,
  canUpdateContext: false,
  callTool: () => Promise.reject(new Error('This host does not let views call server tools.')),
  sendMessage: () => Promise.reject(new Error('This host does not accept messages from views.')),
  updateModelContext: () => Promise.reject(new Error('This host does not accept model context from views.')),
};
