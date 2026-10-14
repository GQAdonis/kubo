import { memo, useEffect, useRef, useState } from 'react';
import { A2uiSurface, type ReactComponentImplementation } from '@a2ui/react/v0_9';
import { MessageProcessor } from '@a2ui/web_core/v0_9';
import type { A2uiStreamMessage, ChatToolCall, UiTheme } from '@gqadonis/ipfs-mcp-ui';
import { toWire } from '@gqadonis/ipfs-mcp-ui';
import { A2UI_CATALOG_ID, ipfsCatalog } from '@gqadonis/ipfs-mcp-ui/a2ui';

// A tool call is drawn by the shared A2UI catalog, not by a private renderer. The messages that build
// the surface come from one of two sources for the same surfaceId (`call-<toolCallId>`):
//
//   - Server (mcp-agui-a2ui-registry): the run stream carries CUSTOM `a2ui` events with the surface
//     the server built from the tool result. It is the same ToolCallCard bound to /call, but its
//     EgressBadge names the principal's acknowledged hosts, which only the server knows.
//   - Local fallback: the island speaks A2UI v0.9 to a local MessageProcessor itself — createSurface
//     with the catalog id, one ToolCallCard bound to the data model, and an updateDataModel each time
//     the call changes (arguments streaming in, then the result). This draws every call a server
//     emitted no surface for: pending and interrupted calls always, and every call when `agent.a2ui`
//     is off or the server is older.
//
// Either way the catalog validates the result against the tool's contract schema before it draws a
// view, so a result the contract does not describe is shown as data, never as a view.

const VERSION = 'v0.9';

const createMessages = (surfaceId: string, theme: UiTheme, call: ChatToolCall): A2uiStreamMessage[] => [
  { version: VERSION, createSurface: { surfaceId, catalogId: A2UI_CATALOG_ID, theme: { mode: theme } } },
  { version: VERSION, updateComponents: { surfaceId, components: [{ id: 'root', component: 'ToolCallCard', call: { path: '/call' } }] } },
  { version: VERSION, updateDataModel: { surfaceId, path: '/call', value: toWire(call) } },
];

// The server's createSurface carries no theme; the island owns theming, so it adds its own mode.
const withTheme = (messages: readonly A2uiStreamMessage[], theme: UiTheme): A2uiStreamMessage[] =>
  messages.map((m) => {
    const cs = m['createSurface'];
    if (typeof cs !== 'object' || cs === null || Array.isArray(cs)) return m;
    return { ...m, createSurface: { ...(cs as Record<string, unknown>), theme: { mode: theme } } };
  });

const noAction = (): void => undefined;

export interface ToolCallSurfaceProps {
  readonly call: ChatToolCall;
  readonly theme: UiTheme;
  /**
   * The A2UI messages the server streamed for this call's surface, in arrival order. When present
   * they draw the card; when absent the island builds the same surface locally.
   */
  readonly serverMessages?: readonly A2uiStreamMessage[];
}

export const ToolCallSurface = memo(function ToolCallSurface({ call, theme, serverMessages }: ToolCallSurfaceProps) {
  const surfaceId = `call-${call.id}`;
  const server = serverMessages !== undefined && serverMessages.length > 0;
  const [processor] = useState(() => {
    const p = new MessageProcessor<ReactComponentImplementation>([ipfsCatalog], noAction);
    p.processMessages((server ? withTheme(serverMessages, theme) : createMessages(surfaceId, theme, call)) as never);
    return p;
  });
  // How many of the server's messages the processor has applied. They stream in one CUSTOM event at a
  // time, so each change applies only the new tail.
  const applied = useRef(server ? serverMessages.length : 0);

  // Keep the surface in step with its source. This synchronizes an external store (the A2UI model)
  // with props, which is what an effect is for.
  useEffect(() => {
    if (!server) {
      // Local fallback: follow the call as its arguments stream in and its result lands.
      processor.processMessages([{ version: VERSION, updateDataModel: { surfaceId, path: '/call', value: toWire(call) } }] as never);
      return;
    }
    const fresh = serverMessages.slice(applied.current);
    if (fresh.length === 0) return;
    // The processor rejects a second createSurface for one id, so a surface that began locally hands
    // over by deleting the local one first. The server surface then replaces it whole.
    const batch: A2uiStreamMessage[] =
      applied.current === 0 ? [{ version: VERSION, deleteSurface: { surfaceId } }, ...withTheme(fresh, theme)] : withTheme(fresh, theme);
    processor.processMessages(batch as never);
    applied.current = serverMessages.length;
  }, [processor, surfaceId, call, theme, server, serverMessages]);

  const surface = processor.model.surfacesMap.get(surfaceId);
  return surface ? <A2uiSurface surface={surface} /> : null;
});
