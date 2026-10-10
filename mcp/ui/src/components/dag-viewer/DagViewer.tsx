import type { ReactNode } from 'react';
import type { DagViewerData } from '../../contract/generated/bindings';
import type {
  CarExportOutput,
  CarImportOutput,
  DagGetOutput,
  DagPutOutput,
  DagStatOutput,
} from '../../contract/generated/outputs';
import { TOOL_META } from '../../contract/generated/tool-meta';
import { formatInt, plural } from '../../lib/format';
import { UiScope } from '../../scope/UiScope';
import { Badge, Bytes, Cid, DataTable, EmptyState, KeyValue, Notice, Panel, UntrustedFrame } from '../../ui/primitives';
import { JsonTree } from './JsonTree';

export type DagViewerProps = DagViewerData;

/** An IPLD node, DAG statistics, and CAR import and export results. */
export function DagViewer(props: DagViewerProps) {
  const meta = TOOL_META[props.tool];
  let body: ReactNode;
  switch (props.tool) {
    case 'dag_get':
      body = <Node o={props.output} />;
      break;
    case 'dag_stat':
      body = <Stat o={props.output} />;
      break;
    case 'dag_put':
      body = <Put o={props.output} />;
      break;
    case 'car_import':
      body = <Imported o={props.output} />;
      break;
    case 'car_export':
      body = <Exported o={props.output} />;
      break;
  }
  return (
    <UiScope>
      <Panel title={meta.title} tool={props.tool}>
        {meta.returnsUntrustedContent ? <UntrustedFrame>{body}</UntrustedFrame> : body}
      </Panel>
    </UiScope>
  );
}

function Node({ o }: { o: DagGetOutput }) {
  return (
    <>
      <KeyValue
        items={[
          { label: 'Path', value: <code className="iu-wrap">{o.path}</code> },
          { label: 'CID', value: <Cid value={o.cid} /> },
          ...(o.codec ? [{ label: 'Codec', value: <code>{o.codec}</code> }] : []),
          ...(o.resource ? [{ label: 'Read separately', value: <code className="iu-wrap">{o.resource.uri}</code> }] : []),
        ]}
      />
      {o.truncated ? (
        <Notice tone="warn" label="Truncated">
          This node is larger than the inline limit. Read the resource for the whole node.
        </Notice>
      ) : null}
      {o.node !== undefined ? (
        <JsonTree value={o.node} label="IPLD node" />
      ) : (
        <p className="iu-muted">No inline node. Read the resource above for the encoded block.</p>
      )}
    </>
  );
}

function Stat({ o }: { o: DagStatOutput }) {
  return (
    <>
      <KeyValue
        items={[
          { label: 'Total size', value: <Bytes value={o.total_size} exact /> },
          { label: 'Unique blocks', value: <span className="iu-num">{formatInt(o.unique_blocks)}</span> },
          ...(o.shared_size !== undefined ? [{ label: 'Shared size', value: <Bytes value={o.shared_size} exact /> }] : []),
          ...(o.ratio !== undefined ? [{ label: 'Dedup ratio', value: <span className="iu-num">{o.ratio.toFixed(2)}</span> }] : []),
        ]}
      />
      <DataTable
        caption="DAGs measured"
        rows={o.dags}
        rowKey={(d, i) => `${d.cid}:${i}`}
        empty={<EmptyState>No DAG was measured.</EmptyState>}
        columns={[
          { key: 'cid', header: 'Root CID', cell: (d) => <Cid value={d.cid} compact /> },
          { key: 'size', header: 'Size', numeric: true, cell: (d) => <Bytes value={d.size} /> },
          { key: 'blocks', header: 'Blocks', numeric: true, cell: (d) => formatInt(d.num_blocks) },
        ]}
      />
    </>
  );
}

function Put({ o }: { o: DagPutOutput }) {
  return (
    <KeyValue
      items={[
        { label: 'Stored as', value: <Cid value={o.cid} /> },
        { label: 'Pin', value: <Badge tone={o.pinned ? 'ok' : 'neutral'}>{o.pinned ? 'Pinned' : 'Not pinned'}</Badge> },
      ]}
    />
  );
}

function Imported({ o }: { o: CarImportOutput }) {
  const failed = o.roots.filter((r) => r.pin_error);
  return (
    <>
      <p>
        {plural(o.roots.length, 'root')} imported{o.block_count !== undefined ? `, ${plural(o.block_count, 'block')} read` : ''}.
      </p>
      <DataTable
        caption="Imported roots"
        rows={o.roots}
        rowKey={(r, i) => `${r.cid}:${i}`}
        columns={[
          { key: 'cid', header: 'Root CID', cell: (r) => <Cid value={r.cid} compact /> },
          {
            key: 'pinned',
            header: 'Pin',
            cell: (r) => <Badge tone={r.pinned ? 'ok' : r.pin_error ? 'err' : 'neutral'}>{r.pinned ? 'Pinned' : r.pin_error ? 'Pin failed' : 'Not pinned'}</Badge>,
          },
          { key: 'error', header: 'Detail', cell: (r) => r.pin_error ?? <span className="iu-muted">None</span> },
        ]}
      />
      {failed.length > 0 ? (
        <Notice tone="err" label="Pin failed">
          {plural(failed.length, 'root')} imported but not pinned. Garbage collection can remove unpinned blocks.
        </Notice>
      ) : null}
    </>
  );
}

function Exported({ o }: { o: CarExportOutput }) {
  return (
    <KeyValue
      items={[
        { label: 'Root', value: <Cid value={o.root} /> },
        ...(o.size !== undefined ? [{ label: 'Archive size', value: <Bytes value={o.size} exact /> }] : []),
        {
          label: 'Delivery',
          value: o.inline ? (
            <Badge tone="info">Inline base64, {o.car_base64 ? formatInt(o.car_base64.length) : 0} characters</Badge>
          ) : (
            <Badge>Read as a resource</Badge>
          ),
        },
        ...(o.resource ? [{ label: 'Resource', value: <code className="iu-wrap">{o.resource.uri}</code> }] : []),
      ]}
    />
  );
}
