import { memo, useEffect, useState } from 'react';
import { A2uiSurface, type ReactComponentImplementation } from '@a2ui/react/v0_9';
import { MessageProcessor } from '@a2ui/web_core/v0_9';
import type { ChatToolCall, UiTheme } from '@gqadonis/ipfs-mcp-ui';
import { toWire } from '@gqadonis/ipfs-mcp-ui';
import { A2UI_CATALOG_ID, ipfsCatalog } from '@gqadonis/ipfs-mcp-ui/a2ui';

// A tool call is drawn by the shared A2UI catalog, not by a private renderer. For each call the island
// speaks A2UI v0.9 to a local MessageProcessor: createSurface with the catalog id, one ToolCallCard
// component bound to the data model, and an updateDataModel each time the call changes (arguments
// streaming in, then the result). The catalog validates the result against the tool's contract schema
// before it draws a view, so a result the contract does not describe is shown as data, never as a view.

type Message = Record<string, unknown>;

const VERSION = 'v0.9';

const createMessages = (surfaceId: string, theme: UiTheme, call: ChatToolCall): Message[] => [
  { version: VERSION, createSurface: { surfaceId, catalogId: A2UI_CATALOG_ID, theme: { mode: theme } } },
  { version: VERSION, updateComponents: { surfaceId, components: [{ id: 'root', component: 'ToolCallCard', call: { path: '/call' } }] } },
  { version: VERSION, updateDataModel: { surfaceId, path: '/call', value: toWire(call) } },
];

const noAction = (): void => undefined;

export interface ToolCallSurfaceProps {
  readonly call: ChatToolCall;
  readonly theme: UiTheme;
}

export const ToolCallSurface = memo(function ToolCallSurface({ call, theme }: ToolCallSurfaceProps) {
  const surfaceId = `call-${call.id}`;
  const [processor] = useState(() => {
    const p = new MessageProcessor<ReactComponentImplementation>([ipfsCatalog], noAction);
    p.processMessages(createMessages(surfaceId, theme, call) as never);
    return p;
  });

  // Keep the surface's data model in step with the call. This synchronizes an external store (the A2UI
  // model) with props, which is what an effect is for.
  useEffect(() => {
    processor.processMessages([{ version: VERSION, updateDataModel: { surfaceId, path: '/call', value: toWire(call) } }] as never);
  }, [processor, surfaceId, call]);

  const surface = processor.model.surfacesMap.get(surfaceId);
  return surface ? <A2uiSurface surface={surface} /> : null;
});
