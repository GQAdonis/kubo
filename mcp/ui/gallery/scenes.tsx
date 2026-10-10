import type { ReactNode } from 'react';
import { ApprovalCard } from '../src/chat/ApprovalCard';
import { EgressBadge } from '../src/chat/EgressBadge';
import { MessageStream } from '../src/chat/MessageStream';
import { ToolCallCard } from '../src/chat/ToolCallCard';
import { applyRunInput, EMPTY_CHAT_STATE, foldAguiEvents, toWire, type ChatState } from '../src/chat/agui';
import { ConfirmationDialog } from '../src/components/confirmation-dialog/ConfirmationDialog';
import { renderToolResult, type ViewHandlers } from '../src/components/registry';
import { ToolErrorNotice } from '../src/components/tool-error/ToolErrorNotice';
import type { ToolName } from '../src/contract/generated/tool-meta';
import { OUTPUTS } from '../tests/fixtures/outputs';
import destructiveNative from '../../contract/fixtures/agui/destructive-native-prompt.json';
import egressRefused from '../../contract/fixtures/agui/egress-refused.json';
import readPinLs from '../../contract/fixtures/agui/read-pin-ls.json';
import writeInterrupt from '../../contract/fixtures/agui/write-interrupt.json';
import writeResume from '../../contract/fixtures/agui/write-resume-approved.json';
import confirmationNative from '../../contract/confirmation/examples/ConfirmationRequired/ui-native-prompt.json';
import confirmationHuman from '../../contract/confirmation/examples/ConfirmationRequired/human-mcp-with-token.json';

// The scenes the visual-regression, axe and keyboard specs drive. Each is a real component fed real
// contract fixtures. The gallery is a test harness; it is not shipped.

const noop = () => undefined;
const handlers: ViewHandlers = { onOpenPath: noop, onLoadMore: noop, onRequestUnpin: noop, onApprove: noop, onDeny: noop };

function view(tool: ToolName, i = 0): ReactNode {
  const r = renderToolResult(tool, OUTPUTS[tool][i], handlers);
  if (r.kind !== 'view') throw new Error(`scene: ${tool} did not render`);
  return r.node;
}

function Stack({ children }: { children: ReactNode }) {
  return <div className="gallery-stack">{children}</div>;
}

const fold = (fixture: { events: unknown[]; input: unknown }, prior: ChatState = EMPTY_CHAT_STATE) =>
  foldAguiEvents(fixture.events as never, applyRunInput(prior, fixture.input as never));

function transcript(state: ChatState, opts: { approve: boolean }): ReactNode {
  return (
    <Stack>
      <MessageStream messages={state.messages} />
      {state.toolCalls.map((c) => (
        <ToolCallCard key={c.id} call={toWire(c)} acknowledgedHosts={['model.example.net']} handlers={handlers} />
      ))}
      {state.interrupts.map((i) => (
        <ApprovalCard
          key={i.interrupt.id}
          interrupt={i.interrupt}
          {...(i.resolution && { resolution: i.resolution })}
          {...(opts.approve && { onApprove: noop, onDeny: noop })}
        />
      ))}
    </Stack>
  );
}

const confirmation = (extra: object, props: object = { onApprove: noop, onDeny: noop }) => (
  <ConfirmationDialog confirmation={{ ...(confirmationHuman as object) } as never} {...extra} {...props} />
);

export interface Scene {
  readonly id: string;
  readonly title: string;
  readonly render: () => ReactNode;
}

export const SCENES: readonly Scene[] = [
  { id: 'pins', title: 'Pin list', render: () => <Stack>{view('pin_ls')}{view('pin_ls', 1)}{view('pin_verify')}</Stack> },
  { id: 'pins-writes', title: 'Pin results', render: () => <Stack>{view('pin_add')}{view('pin_update')}{view('pin_rm')}</Stack> },
  { id: 'content-text', title: 'Content text', render: () => <Stack>{view('ipfs_cat')}{view('ipfs_cat', 1)}{view('ipfs_add')}</Stack> },
  { id: 'content-listing', title: 'Content listing and search', render: () => <Stack>{view('ipfs_ls')}{view('ipfs_index_search')}{view('ipfs_get')}</Stack> },
  { id: 'mfs', title: 'MFS', render: () => <Stack>{view('mfs_ls')}{view('mfs_stat')}{view('mfs_mv')}</Stack> },
  { id: 'dag', title: 'DAG', render: () => <Stack>{view('dag_get')}{view('dag_stat')}{view('car_import')}</Stack> },
  { id: 'ipns', title: 'IPNS and keys', render: () => <Stack>{view('ipns_resolve')}{view('key_list')}{view('key_gen')}</Stack> },
  { id: 'node-status', title: 'Node status', render: () => <Stack>{view('ipfs_node_info')}{view('ipfs_node_status')}{view('ipfs_node_status', 1)}</Stack> },
  { id: 'node-routing', title: 'Routing: providers, peer, provide', render: () => <Stack>{view('routing_findprovs')}{view('routing_findprovs', 1)}{view('routing_findpeer')}{view('routing_provide')}{view('routing_provide', 1)}</Stack> },
  { id: 'node-repo', title: 'Node repo and peers', render: () => <Stack>{view('repo_stat')}{view('stats_bw')}{view('swarm_peers')}{view('repo_gc')}</Stack> },
  {
    id: 'confirmation-pending',
    title: 'Confirmation, pending and answerable',
    render: () => <Stack>{confirmation({})}</Stack>,
  },
  {
    id: 'confirmation-states',
    title: 'Confirmation, every other state',
    render: () => (
      <Stack>
        <ConfirmationDialog confirmation={confirmationNative as never} onApprove={noop} onDeny={noop} />
        {confirmation({}, {})}
        {confirmation({ resolution: 'approved' })}
        <ConfirmationDialog confirmation={{ ...(confirmationHuman as object), state: 'denied' } as never} />
        <ConfirmationDialog confirmation={{ ...(confirmationHuman as object), state: 'expired' } as never} />
      </Stack>
    ),
  },
  { id: 'chat-read', title: 'Chat: read call', render: () => transcript(fold(readPinLs), { approve: false }) },
  { id: 'chat-write-pending', title: 'Chat: write waits for approval', render: () => transcript(fold(writeInterrupt), { approve: true }) },
  { id: 'chat-write-approved', title: 'Chat: write approved and run', render: () => transcript(fold(writeResume, fold(writeInterrupt)), { approve: true }) },
  { id: 'chat-native', title: 'Chat: destructive call waits for the system prompt', render: () => transcript(fold(destructiveNative), { approve: true }) },
  {
    id: 'chat-egress',
    title: 'Chat: egress refused',
    render: () => (
      <Stack>
        {transcript(fold(egressRefused), { approve: false })}
        <EgressBadge tool="ipfs_cat" acknowledged_hosts={[]} />
        <ToolErrorNotice error={{ error: { code: 'daemon_unreachable', message: 'The daemon is not running. Start it, then ask again.', retryable: true, tool: 'pin_ls' } }} />
      </Stack>
    ),
  },
];

export const SCENE_IDS = SCENES.map((s) => s.id);
