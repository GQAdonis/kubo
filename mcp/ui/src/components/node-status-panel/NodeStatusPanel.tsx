import type { ReactNode } from 'react';
import type { NodeStatusPanelData } from '../../contract/generated/bindings';
import type {
  IpfsIndexReconcileOutput,
  IpfsIndexStatusOutput,
  IpfsNodeInfoOutput,
  IpfsNodeStatusOutput,
  RepoGcOutput,
  RepoStatOutput,
  RoutingFindpeerOutput,
  RoutingFindprovsOutput,
  RoutingProvideOutput,
  StatsBwOutput,
  SwarmPeersOutput,
} from '../../contract/generated/outputs';
import { TOOL_META } from '../../contract/generated/tool-meta';
import { formatBytes, formatInt, formatRate, plural } from '../../lib/format';
import { displayText } from '../../lib/safe-text';
import { UiScope } from '../../scope/UiScope';
import { IconAlert, IconOk } from '../../ui/icons';
import { Badge, Bytes, Cid, DataTable, EmptyState, IdText, KeyValue, More, Notice, Panel, Warnings } from '../../ui/primitives';
import { Meter } from './Meter';

export type NodeStatusPanelProps = NodeStatusPanelData & {
  onLoadMore?: (cursor: string) => void;
};

/** Node identity, server status, peers, bandwidth, repo and index status. */
export function NodeStatusPanel(props: NodeStatusPanelProps) {
  const meta = TOOL_META[props.tool];
  let body: ReactNode;
  switch (props.tool) {
    case 'ipfs_node_info':
      body = <Info o={props.output} />;
      break;
    case 'ipfs_node_status':
      body = <Status o={props.output} />;
      break;
    case 'routing_findprovs':
      body = <Providers o={props.output} />;
      break;
    case 'routing_findpeer':
      body = <FoundPeer o={props.output} />;
      break;
    case 'routing_provide':
      body = <Provide o={props.output} />;
      break;
    case 'swarm_peers':
      body = <Peers o={props.output} onLoadMore={props.onLoadMore} />;
      break;
    case 'stats_bw':
      body = <Bandwidth o={props.output} />;
      break;
    case 'repo_stat':
      body = <Repo o={props.output} />;
      break;
    case 'repo_gc':
      body = <Gc o={props.output} />;
      break;
    case 'ipfs_index_status':
      body = <IndexStatus o={props.output} />;
      break;
    case 'ipfs_index_reconcile':
      body = <Reconcile o={props.output} />;
      break;
  }
  return (
    <UiScope>
      <Panel title={meta.title} tool={props.tool}>
        {body}
      </Panel>
    </UiScope>
  );
}

function Info({ o }: { o: IpfsNodeInfoOutput }) {
  return (
    <>
      <KeyValue
        items={[
          { label: 'Peer ID', value: <IdText value={o.peer_id} kind="peer ID" /> },
          { label: 'Version', value: <code>{o.version}</code> },
          ...(o.agent_version ? [{ label: 'Agent', value: <code className="iu-wrap">{o.agent_version}</code> }] : []),
          ...(o.commit ? [{ label: 'Commit', value: <code>{o.commit}</code> }] : []),
          ...(o.repo_version ? [{ label: 'Repo version', value: <code>{o.repo_version}</code> }] : []),
        ]}
      />
      <h4 className="iu-subtitle">Addresses</h4>
      {o.addresses.length === 0 ? (
        <EmptyState>The node reports no addresses.</EmptyState>
      ) : (
        <ul className="iu-list">
          {o.addresses.map((a) => (
            <li key={a}>
              <IdText value={a} kind="address" />
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

const UNAVAILABLE: Record<string, string> = {
  missing_rpc_command: 'The daemon lacks an RPC command this tool needs.',
  daemon_too_old: 'The daemon is older than this tool requires.',
  group_disabled: 'The tool group is not enabled.',
  destructive_disabled: 'Destructive tools are off by default. Name the destructive selector to offer it; every call still asks first.',
  read_only: 'This server runs read-only.',
  not_configured: 'It needs configuration that is not set.',
  principal_not_granted: 'Your principal is not granted this tool.',
};

function Status({ o }: { o: IpfsNodeStatusOutput }) {
  return (
    <>
      <div className="iu-badges">
        <Badge tone={o.daemon_reachable ? 'ok' : 'err'} icon={o.daemon_reachable ? <IconOk /> : <IconAlert />}>
          {o.daemon_reachable ? 'Daemon reachable' : 'Daemon unreachable'}
        </Badge>
        <Badge tone={o.version_supported ? 'ok' : 'warn'}>
          {o.version_supported ? 'Version supported' : 'Version not supported'}
        </Badge>
        <Badge tone={o.read_only ? 'info' : 'neutral'}>{o.read_only ? 'Read-only' : 'Writes enabled'}</Badge>
        <Badge>Transport {o.transport}</Badge>
      </div>
      <KeyValue
        items={[
          ...(o.daemon_version ? [{ label: 'Daemon version', value: <code>{o.daemon_version}</code> }] : []),
          ...(o.min_kubo_version ? [{ label: 'Minimum daemon', value: <code>{o.min_kubo_version}</code> }] : []),
          { label: 'Contract version', value: <code>{o.contract_version}</code> },
          {
            label: 'Enabled groups',
            value:
              o.enabled_groups.length === 0 ? (
                'None'
              ) : (
                <ul className="iu-badges" aria-label="Enabled tool groups">
                  {o.enabled_groups.map((g) => (
                    <li key={g}>
                      <Badge>{g}</Badge>
                    </li>
                  ))}
                </ul>
              ),
          },
        ]}
      />
      {o.unavailable_tools.length > 0 ? (
        <>
          <h4 className="iu-subtitle">{plural(o.unavailable_tools.length, 'tool')} unavailable</h4>
          <DataTable
            caption="Unavailable tools"
            rows={o.unavailable_tools}
            rowKey={(t) => t.name}
            columns={[
              { key: 'name', header: 'Tool', cell: (t) => <code>{t.name}</code> },
              { key: 'reason', header: 'Why', cell: (t) => UNAVAILABLE[t.reason] ?? t.reason },
              {
                key: 'rpc',
                header: 'Missing RPC',
                cell: (t) => (t.missing_rpc?.length ? <code className="iu-wrap">{t.missing_rpc.join(', ')}</code> : '-'),
              },
            ]}
          />
        </>
      ) : null}
    </>
  );
}

function Peers({ o, onLoadMore }: { o: SwarmPeersOutput; onLoadMore: ((cursor: string) => void) | undefined }) {
  return (
    <>
      <p className="iu-muted">
        {plural(o.peers.length, 'peer')} connected{o.page.has_more ? ' on this page; more exist' : ''}.
      </p>
      <DataTable
        caption="Connected peers"
        rows={o.peers}
        rowKey={(p, i) => `${p.id}:${i}`}
        empty={<EmptyState>No peers are connected. The node cannot fetch content it does not hold.</EmptyState>}
        columns={[
          { key: 'id', header: 'Peer ID', cell: (p) => <IdText value={p.id} kind="peer ID" compact /> },
          { key: 'addr', header: 'Address', cell: (p) => <code className="iu-wrap">{displayText(p.addr)}</code> },
          { key: 'dir', header: 'Direction', cell: (p) => p.direction ?? '-' },
          { key: 'lat', header: 'Latency', numeric: true, cell: (p) => p.latency ?? '-' },
        ]}
      />
      <More hasMore={o.page.has_more} nextCursor={o.page.next_cursor} onLoadMore={onLoadMore} noun="peers" />
    </>
  );
}

function Providers({ o }: { o: RoutingFindprovsOutput }) {
  return (
    <>
      <p className="iu-muted">
        {plural(o.providers.length, 'provider')} found for <Cid value={o.cid} compact />.
      </p>
      <DataTable
        caption="Providers"
        rows={o.providers}
        rowKey={(p, i) => `${p.id}:${i}`}
        empty={<EmptyState>No provider was found for this CID. It may be held by no reachable peer.</EmptyState>}
        columns={[
          { key: 'id', header: 'Peer ID', cell: (p) => <IdText value={p.id} kind="peer ID" compact /> },
          { key: 'addrs', header: 'Addresses', cell: (p) => <AddrList addrs={p.addrs} /> },
        ]}
      />
    </>
  );
}

function FoundPeer({ o }: { o: RoutingFindpeerOutput }) {
  return (
    <>
      <KeyValue items={[{ label: 'Peer ID', value: <IdText value={o.id} kind="peer ID" /> }]} />
      <h4 className="iu-subtitle">Addresses</h4>
      {o.addrs.length === 0 ? <EmptyState>The peer was found but advertises no addresses.</EmptyState> : <AddrList addrs={o.addrs} />}
    </>
  );
}

function AddrList({ addrs }: { addrs: readonly string[] }) {
  if (addrs.length === 0) return <span className="iu-muted">None advertised</span>;
  return (
    <ul className="iu-list">
      {addrs.map((a) => (
        <li key={a}>
          <code className="iu-wrap">{displayText(a)}</code>
        </li>
      ))}
    </ul>
  );
}

function Provide({ o }: { o: RoutingProvideOutput }) {
  return (
    <>
      <div className="iu-badges">
        <Badge tone={o.announced ? 'ok' : 'warn'} icon={o.announced ? <IconOk /> : <IconAlert />}>
          {o.announced ? 'Announced' : 'Not announced'}
        </Badge>
      </div>
      <KeyValue items={[{ label: 'CID', value: <Cid value={o.cid} /> }]} />
      {o.providing_disabled ? (
        <Notice tone="warn" label="Providing is off">
          This node has providing switched off, so it does not announce content to the network.
        </Notice>
      ) : null}
    </>
  );
}

function Bandwidth({ o }: { o: StatsBwOutput }) {
  return (
    <KeyValue
      items={[
        { label: 'Received in total', value: <Bytes value={o.total_in} exact /> },
        { label: 'Sent in total', value: <Bytes value={o.total_out} exact /> },
        { label: 'Receiving now', value: <span className="iu-num">{formatRate(o.rate_in)}</span> },
        { label: 'Sending now', value: <span className="iu-num">{formatRate(o.rate_out)}</span> },
      ]}
    />
  );
}

function Repo({ o }: { o: RepoStatOutput }) {
  const limited = o.storage_max > 0;
  return (
    <>
      {limited ? (
        <Meter
          label="Repo usage"
          value={o.repo_size}
          max={o.storage_max}
          text={`${formatBytes(o.repo_size)} of ${formatBytes(o.storage_max)}`}
        />
      ) : null}
      <KeyValue
        items={[
          { label: 'Repo size', value: <Bytes value={o.repo_size} exact /> },
          { label: 'Storage limit', value: limited ? <Bytes value={o.storage_max} exact /> : 'No limit set' },
          { label: 'Objects', value: <span className="iu-num">{formatInt(o.num_objects)}</span> },
          ...(o.version ? [{ label: 'Repo version', value: <code>{o.version}</code> }] : []),
        ]}
      />
    </>
  );
}

function Gc({ o }: { o: RepoGcOutput }) {
  return (
    <>
      <p>
        <Badge tone={o.errors?.length ? 'warn' : 'ok'}>Garbage collection ran</Badge> It removed{' '}
        {plural(o.removed_count, 'block')}.
      </p>
      {o.removed_sample?.length ? (
        <>
          <h4 className="iu-subtitle">Sample of removed blocks</h4>
          <ul className="iu-list">
            {o.removed_sample.map((c) => (
              <li key={c}>
                <Cid value={c} compact />
              </li>
            ))}
          </ul>
        </>
      ) : null}
      {o.errors?.map((e) => (
        <Notice key={e} tone="err" label="Error">
          {displayText(e)}
        </Notice>
      ))}
      <Warnings items={o.warnings} />
    </>
  );
}

const MODE: Record<string, string> = {
  disabled: 'Index disabled',
  embedded: 'Embedded index',
  remote: 'Remote index',
};

function IndexStatus({ o }: { o: IpfsIndexStatusOutput }) {
  return (
    <>
      <div className="iu-badges">
        <Badge tone={o.enabled ? 'ok' : 'neutral'}>{o.enabled ? 'Indexing on' : 'Indexing off'}</Badge>
        <Badge>{MODE[o.mode] ?? o.mode}</Badge>
        {o.store_reachable !== undefined ? (
          <Badge tone={o.store_reachable ? 'ok' : 'err'}>{o.store_reachable ? 'Store reachable' : 'Store unreachable'}</Badge>
        ) : null}
        <Badge>{o.auto ? 'Automatic' : 'On request'}</Badge>
      </div>
      <KeyValue
        items={[
          ...(o.model ? [{ label: 'Model', value: <code>{o.model}</code> }] : []),
          ...(o.dimensions !== undefined ? [{ label: 'Dimensions', value: <span className="iu-num">{formatInt(o.dimensions)}</span> }] : []),
          ...(o.indexed_cids !== undefined ? [{ label: 'Indexed CIDs', value: <span className="iu-num">{formatInt(o.indexed_cids)}</span> }] : []),
          ...(o.chunk_count !== undefined ? [{ label: 'Chunks', value: <span className="iu-num">{formatInt(o.chunk_count)}</span> }] : []),
          ...(o.cid_indexed !== undefined
            ? [{ label: 'Asked CID', value: <Badge tone={o.cid_indexed ? 'ok' : 'neutral'}>{o.cid_indexed ? 'Indexed' : 'Not indexed'}</Badge> }]
            : []),
        ]}
      />
    </>
  );
}

function Reconcile({ o }: { o: IpfsIndexReconcileOutput }) {
  return (
    <>
      <p>
        {o.dry_run ? <Badge tone="info">Dry run</Badge> : <Badge tone="warn">Applied</Badge>} Checked {plural(o.checked, 'CID')}:{' '}
        {formatInt(o.kept)} kept, {formatInt(o.deleted.length)} {o.dry_run ? 'would be deleted' : 'deleted'}.
      </p>
      {o.deleted.length > 0 ? (
        <ul className="iu-list">
          {o.deleted.map((c) => (
            <li key={c}>
              <Cid value={c} compact />
            </li>
          ))}
        </ul>
      ) : null}
      {o.page.has_more ? <p className="iu-muted">More entries remain to check. Run it again to continue.</p> : null}
      <Warnings items={o.warnings} />
    </>
  );
}
