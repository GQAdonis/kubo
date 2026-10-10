import { vi } from 'vitest';
import { TOOL_META_KEY, type ToolCall, type ToolResultLike } from '../../src/core/result';
import type { HostPort } from '../../src/host/port';
import type { ToolName } from '@gqadonis/ipfs-mcp-ui';

/** A tool result the way the Go server builds it: text for non-UI hosts, the structured output, the tool marker. */
export function okResult(tool: ToolName, output: unknown): ToolResultLike {
  return {
    content: [{ type: 'text', text: JSON.stringify(output) }],
    structuredContent: output,
    _meta: { [TOOL_META_KEY]: tool },
  };
}

/** The structuredContent of an isError result: the contract ToolError. */
export function errorResult(error: { code: string; message: string; retryable?: boolean; tool?: string; confirmation?: unknown }): ToolResultLike {
  return {
    isError: true,
    content: [{ type: 'text', text: error.message }],
    structuredContent: { error: { retryable: false, ...error } },
  };
}

export interface FakePort extends HostPort {
  readonly calls: ToolCall[];
  readonly messages: string[];
  readonly contexts: string[];
  readonly callTool: ReturnType<typeof vi.fn<(call: ToolCall, signal?: AbortSignal) => Promise<ToolResultLike>>>;
}

export interface FakePortOptions {
  canCallTools?: boolean;
  canMessage?: boolean;
  canUpdateContext?: boolean;
  /** Answer for each call. Defaults to an error so an unexpected call fails loudly. */
  respond?: (call: ToolCall) => ToolResultLike | Promise<ToolResultLike>;
}

export function fakePort(options: FakePortOptions = {}): FakePort {
  const calls: ToolCall[] = [];
  const messages: string[] = [];
  const contexts: string[] = [];
  const respond = options.respond ?? ((call: ToolCall) => errorResult({ code: 'internal', message: `unexpected call to ${call.tool}` }));
  const callTool = vi.fn(async (call: ToolCall) => {
    calls.push(call);
    return respond(call);
  });
  return {
    canCallTools: options.canCallTools ?? true,
    canMessage: options.canMessage ?? true,
    canUpdateContext: options.canUpdateContext ?? true,
    calls,
    messages,
    contexts,
    callTool,
    sendMessage: async (text: string) => {
      messages.push(text);
    },
    updateModelContext: async (text: string) => {
      contexts.push(text);
    },
  };
}
