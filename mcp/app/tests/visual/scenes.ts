import { CID_B, CID_C, CONFIRMATIONS, OUTPUTS } from '../helpers/fixtures';
import type { HostScript } from './host';

// The scenes the visual and axe specs play: one per view plus the states that carry trust (a
// confirmation waiting on the system prompt, a failure). Results are built the way the Go server
// builds them: text for non-UI hosts, structuredContent, and the tool marker.
export const THEMES = ['dark', 'light'] as const;

// The breakpoints the project's web rules name: 320, 768, 1024, 1440.
export const VIEWPORTS = {
  320: { width: 320, height: 800 },
  768: { width: 768, height: 900 },
  1024: { width: 1024, height: 900 },
  1440: { width: 1440, height: 900 },
} as const;

function ok(tool: string, output: unknown): unknown {
  return { content: [{ type: 'text', text: JSON.stringify(output) }], structuredContent: output, _meta: { 'ai.ipfs/tool': tool } };
}

function err(error: Record<string, unknown>): unknown {
  return { isError: true, content: [{ type: 'text', text: String(error['message']) }], structuredContent: { error: { retryable: false, ...error } } };
}

export interface Scene {
  readonly id: string;
  readonly script: HostScript;
}

const archive = { path: '/docs/notes/archive', entries: [{ name: 'old.md', type: 'file', size: 9, cid: CID_B }], page: { has_more: false } };
const page2 = { pins: [{ cid: CID_C, type: 'direct', name: 'second-page' }], page: { has_more: false } };

export const SCENES: readonly Scene[] = [
  { id: 'pins', script: { view: 'pins', theme: 'dark', toolName: 'pin_ls', input: { type: 'all' }, result: ok('pin_ls', OUTPUTS.pin_ls[0]), toolReplies: { pin_ls: ok('pin_ls', page2) } } },
  { id: 'content-listing', script: { view: 'content', theme: 'dark', toolName: 'ipfs_ls', result: ok('ipfs_ls', OUTPUTS.ipfs_ls[0]) } },
  { id: 'content-file', script: { view: 'content', theme: 'dark', toolName: 'ipfs_cat', result: ok('ipfs_cat', OUTPUTS.ipfs_cat[0]) } },
  { id: 'mfs', script: { view: 'mfs', theme: 'dark', toolName: 'mfs_ls', result: ok('mfs_ls', OUTPUTS.mfs_ls[0]), toolReplies: { mfs_ls: ok('mfs_ls', archive) } } },
  { id: 'dag', script: { view: 'dag', theme: 'dark', toolName: 'dag_get', result: ok('dag_get', OUTPUTS.dag_get[0]) } },
  { id: 'node-status', script: { view: 'node-status', theme: 'dark', toolName: 'ipfs_node_status', result: ok('ipfs_node_status', OUTPUTS.ipfs_node_status[0]) } },
  { id: 'node-peers', script: { view: 'node-status', theme: 'dark', toolName: 'swarm_peers', result: ok('swarm_peers', OUTPUTS.swarm_peers[0]) } },
  {
    id: 'confirmation-pending',
    script: { view: 'pins', theme: 'dark', toolName: 'pin_add', result: err({ code: 'confirmation_required', message: 'Confirm first.', tool: 'pin_add', confirmation: CONFIRMATIONS.humanWithToken }) },
  },
  {
    id: 'confirmation-native-prompt',
    script: { view: 'node-status', theme: 'dark', toolName: 'repo_gc', result: err({ code: 'confirmation_required', message: 'Confirm first.', tool: 'repo_gc', confirmation: CONFIRMATIONS.nativePrompt }) },
  },
  { id: 'failure', script: { view: 'node-status', theme: 'dark', toolName: 'ipfs_node_status', result: err({ code: 'daemon_unreachable', message: 'The daemon is not running.', tool: 'ipfs_node_status', retryable: true }) } },
];
