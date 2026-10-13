/**
 * The MCP App views. The set and the names come from the contract (`ui.appView` in
 * mcp/contract/catalog.json, surfaced as TOOL_META[tool].appView); tests/views.test.ts fails when
 * this list and the contract disagree. A tool with no appView (the IPNS and key tools) has no view
 * and stays a text and structured-content tool.
 */
export const VIEW_IDS = ['content', 'pins', 'mfs', 'dag', 'node-status'] as const;
export type ViewId = (typeof VIEW_IDS)[number];

export interface ViewSpec {
  readonly id: ViewId;
  /** The ui:// resource URI the server registers and the tools link to. */
  readonly uri: string;
  /** Resource name and document title. Sentence case. */
  readonly title: string;
  /** One sentence for resources/list. */
  readonly description: string;
  /** File name in mcp/app/dist. */
  readonly file: string;
}

/** The ui:// authority every view shares. */
export const UI_AUTHORITY = 'ipfs-mcp';

/** `text/html;profile=mcp-app`, the MIME type of an MCP App resource (SEP-1865). */
export const MCP_APP_MIME_TYPE = 'text/html;profile=mcp-app';

const SPECS: Record<ViewId, Pick<ViewSpec, 'title' | 'description'>> = {
  content: {
    title: 'IPFS content',
    description: 'File bytes, directory listings, search hits and add receipts for content on your node.',
  },
  pins: {
    title: 'IPFS pins',
    description: 'What your node keeps: the pin list, verification results and pin changes.',
  },
  mfs: {
    title: 'IPFS files',
    description: 'The mutable file system on your node: listings, file details and change receipts.',
  },
  dag: {
    title: 'IPFS DAG',
    description: 'DAG nodes, sizes and CAR transfers as a tree you can follow.',
  },
  'node-status': {
    title: 'IPFS node status',
    description: 'Node identity, server status, peers, bandwidth, repo usage, providers and index status.',
  },
};

export const VIEWS: Readonly<Record<ViewId, ViewSpec>> = Object.fromEntries(
  VIEW_IDS.map((id) => [id, { id, uri: `ui://${UI_AUTHORITY}/${id}`, file: `${id}.html`, ...SPECS[id] }]),
) as Record<ViewId, ViewSpec>;

export function isViewId(value: string | undefined): value is ViewId {
  return value !== undefined && (VIEW_IDS as readonly string[]).includes(value);
}
