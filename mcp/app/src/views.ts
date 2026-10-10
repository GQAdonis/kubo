import { TOOL_META, type ToolName } from '@gqadonis/ipfs-mcp-ui';
import { VIEWS, isViewId, type ViewId, type ViewSpec } from './view-specs';

export * from './view-specs';

/** The tools whose results this view draws, in catalog order. */
export function toolsForView(id: ViewId): readonly ToolName[] {
  return (Object.keys(TOOL_META) as ToolName[]).filter((t) => TOOL_META[t].appView === id);
}

/** The view a tool links to, or undefined when the contract binds it to none. */
export function viewForTool(tool: ToolName): ViewSpec | undefined {
  const id = TOOL_META[tool].appView ?? undefined;
  return isViewId(id) ? VIEWS[id] : undefined;
}
