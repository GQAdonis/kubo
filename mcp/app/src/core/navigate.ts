import type { ToolName } from '@gqadonis/ipfs-mcp-ui';
import type { ToolCall } from './result';

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function lastSegment(path: string): string {
  const parts = path.split('/').filter(Boolean);
  return parts[parts.length - 1] ?? '';
}

function dirname(path: string): string {
  const parts = path.split('/').filter(Boolean);
  parts.pop();
  return parts.length === 0 ? '/' : `/${parts.join('/')}`;
}

/**
 * The read-only call that follows a path the person chose in the current view, or undefined when the
 * view offers no navigation for this tool. The component only knows the path; the tool is decided here
 * from what is on screen.
 *
 * - ipfs_ls: directories open as another ipfs_ls.
 * - mfs_ls: a directory entry or a path-trail segment opens as mfs_ls; a file entry opens its details
 *   with mfs_stat.
 */
export function openPathCall(tool: ToolName, output: unknown, path: string): ToolCall | undefined {
  if (tool === 'ipfs_ls') return { tool: 'ipfs_ls', args: { path } };
  if (tool !== 'mfs_ls' || !isRecord(output)) return undefined;

  const entries = Array.isArray(output['entries']) ? output['entries'] : [];
  const here = typeof output['path'] === 'string' ? output['path'] : '/';
  const entry = entries.find(
    (e): e is Record<string, unknown> =>
      isRecord(e) && e['name'] === lastSegment(path) && dirname(path) === (here === '' ? '/' : here.replace(/(.)\/$/, '$1')),
  );
  if (entry && entry['type'] !== 'directory') return { tool: 'mfs_stat', args: { path } };
  return { tool: 'mfs_ls', args: { path } };
}

/** Tools whose result can offer navigation. */
export function canOpenPaths(tool: ToolName): boolean {
  return tool === 'ipfs_ls' || tool === 'mfs_ls';
}
