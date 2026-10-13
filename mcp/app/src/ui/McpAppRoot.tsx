import type { App, McpUiHostContext } from '@modelcontextprotocol/ext-apps';
import { useApp } from '@modelcontextprotocol/ext-apps/react';
import { useMemo, useState } from 'react';
import { createMcpPort } from '../host/mcp-port';
import { STATIC_PORT } from '../host/port';
import type { ToolResultLike } from '../core/result';
import type { ViewId } from '../views';
import { ViewShell, type HostView } from './ViewShell';

const APP_VERSION = '0.1.0';

interface Received {
  readonly input: Readonly<Record<string, unknown>> | undefined;
  readonly result: ToolResultLike | undefined;
  readonly resultKey: number;
}

function hostViewOf(ctx: McpUiHostContext | undefined): HostView {
  return {
    theme: ctx?.theme,
    variables: ctx?.styles?.variables,
    safeAreaInsets: ctx?.safeAreaInsets,
    toolInfoName: ctx?.toolInfo?.tool.name,
  };
}

/**
 * Connects one view to its host: ui/initialize, tool input and result, host context changes and
 * teardown, through the MCP Apps SDK. Handlers are registered before the connection is made, so the
 * result that follows initialization is never missed.
 */
export function McpAppRoot({ view }: { readonly view: ViewId }) {
  const [received, setReceived] = useState<Received>({ input: undefined, result: undefined, resultKey: 0 });
  const [ctx, setCtx] = useState<McpUiHostContext | undefined>(undefined);
  const [failure, setFailure] = useState<string | undefined>(undefined);

  const { app, isConnected, error } = useApp({
    appInfo: { name: `ipfs-mcp-${view}`, version: APP_VERSION },
    capabilities: {},
    onAppCreated: (created: App) => {
      created.ontoolinput = (params) => {
        setReceived((r) => ({ ...r, input: params.arguments }));
      };
      created.ontoolresult = (params) => {
        setReceived((r) => ({ ...r, result: params, resultKey: r.resultKey + 1 }));
      };
      created.onhostcontextchanged = (next) => {
        setCtx((prev) => ({ ...prev, ...next }));
      };
      created.onteardown = async () => ({});
      created.onerror = (e) => {
        setFailure(e instanceof Error ? e.message : 'The host connection reported an error.');
      };
    },
  });

  const port = useMemo(() => (app && isConnected ? createMcpPort(app) : STATIC_PORT), [app, isConnected]);
  // The context from initialize, then merged changes.
  const hostContext = ctx ?? (isConnected ? app?.getHostContext() : undefined);
  const host = useMemo(() => hostViewOf(hostContext), [hostContext]);

  return (
    <ViewShell
      view={view}
      port={port}
      host={host}
      input={received.input}
      result={received.result}
      resultKey={received.resultKey}
      error={error?.message ?? failure}
    />
  );
}
