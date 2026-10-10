import { TOOL_META, TOOL_OUTPUT_SCHEMAS, type ToolName } from '@gqadonis/ipfs-mcp-ui';
import { isReadOnlyTool, type ToolCall } from './result';

/** The array each paginated read tool returns its rows in. */
const LIST_KEY: Readonly<Partial<Record<ToolName, string>>> = {
  ipfs_ls: 'entries',
  pin_ls: 'pins',
  pin_verify: 'results',
  mfs_ls: 'entries',
  swarm_peers: 'peers',
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/** Whether a view may load further pages of this tool: paginated in the contract and read-only. */
export function canPage(tool: ToolName): boolean {
  return TOOL_META[tool].paginated && isReadOnlyTool(tool) && tool in LIST_KEY;
}

/** The same call for the next page: the original arguments with the cursor swapped. */
export function nextPageCall(call: ToolCall, cursor: string): ToolCall {
  return { tool: call.tool, args: { ...call.args, cursor } };
}

/**
 * Append a loaded page to what is already on screen. The next page is validated against the contract
 * schema first; a page that does not validate, or that belongs to another path, is not merged.
 * Returns the merged output, or undefined when the page cannot be merged.
 */
export function mergePage(tool: ToolName, prev: unknown, next: unknown): unknown {
  const key = LIST_KEY[tool];
  if (!key || !isRecord(prev) || !TOOL_OUTPUT_SCHEMAS[tool].safeParse(next).success || !isRecord(next)) return undefined;
  const a = prev[key];
  const b = next[key];
  if (!Array.isArray(a) || !Array.isArray(b)) return undefined;
  if ('path' in prev && prev['path'] !== next['path']) return undefined;
  return { ...prev, [key]: [...a, ...b], page: next['page'] };
}
