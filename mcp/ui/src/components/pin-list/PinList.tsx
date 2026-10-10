import type { ReactNode } from 'react';
import type { PinListData } from '../../contract/generated/bindings';
import type {
  PinAddOutput,
  PinLsOutput,
  PinRmOutput,
  PinUpdateOutput,
  PinVerifyOutput,
} from '../../contract/generated/outputs';
import { TOOL_META } from '../../contract/generated/tool-meta';
import { formatInt, plural } from '../../lib/format';
import { displayText } from '../../lib/safe-text';
import { UiScope } from '../../scope/UiScope';
import { IconDenied, IconOk } from '../../ui/icons';
import { Badge, Button, Cid, DataTable, EmptyState, KeyValue, More, Notice, Panel, Warnings } from '../../ui/primitives';

export type PinListProps = PinListData & {
  /** Load the next page. */
  onLoadMore?: (cursor: string) => void;
  /**
   * Ask for a pin to be removed. This only asks: the host calls pin_rm, the server raises the
   * confirmation, and nothing is unpinned until a person approves. Omit it and no button renders.
   */
  onRequestUnpin?: (cid: string) => void;
};

/** Pins with type, name and verification state; paginated. */
export function PinList(props: PinListProps) {
  const meta = TOOL_META[props.tool];
  let body: ReactNode;
  switch (props.tool) {
    case 'pin_ls':
      body = <PinTable o={props.output} onLoadMore={props.onLoadMore} onRequestUnpin={props.onRequestUnpin} />;
      break;
    case 'pin_add':
      body = <Added o={props.output} />;
      break;
    case 'pin_rm':
      body = <Removed o={props.output} />;
      break;
    case 'pin_update':
      body = <Updated o={props.output} />;
      break;
    case 'pin_verify':
      body = <Verified o={props.output} onLoadMore={props.onLoadMore} />;
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

const PIN_TYPE: Record<string, string> = { recursive: 'Recursive', direct: 'Direct', indirect: 'Indirect' };

function PinTable({
  o,
  onLoadMore,
  onRequestUnpin,
}: {
  o: PinLsOutput;
  onLoadMore: ((cursor: string) => void) | undefined;
  onRequestUnpin: ((cid: string) => void) | undefined;
}) {
  return (
    <>
      <p className="iu-muted">
        {plural(o.pins.length, 'pin')} on this page{o.page.has_more ? '; more exist' : ''}.
      </p>
      <DataTable
        caption="Pins"
        rows={o.pins}
        rowKey={(p, i) => `${p.cid}:${i}`}
        empty={<EmptyState>No pins match. Pin content with an agent and it appears here.</EmptyState>}
        columns={[
          { key: 'cid', header: 'CID', cell: (p) => <Cid value={p.cid} compact /> },
          { key: 'type', header: 'Type', cell: (p) => <Badge>{PIN_TYPE[p.type] ?? p.type}</Badge> },
          { key: 'name', header: 'Name', cell: (p) => (p.name ? displayText(p.name) : <span className="iu-muted">No name</span>) },
          ...(onRequestUnpin
            ? [
                {
                  key: 'action',
                  header: 'Action',
                  cell: (p: PinLsOutput['pins'][number]) => (
                    <Button variant="text" compact onClick={() => onRequestUnpin(p.cid)}>
                      Request unpin{' '}
                      <span className="iu-sr-only">{p.cid}</span>
                    </Button>
                  ),
                },
              ]
            : []),
        ]}
      />
      {onRequestUnpin && o.pins.length > 0 ? (
        <p className="iu-muted">Request unpin asks the agent to remove a pin. A person still approves it first.</p>
      ) : null}
      <More hasMore={o.page.has_more} nextCursor={o.page.next_cursor} onLoadMore={onLoadMore} noun="pins" />
    </>
  );
}

function CidList({ cids }: { cids: readonly string[] }) {
  return (
    <ul className="iu-list">
      {cids.map((cid) => (
        <li key={cid}>
          <Cid value={cid} />
        </li>
      ))}
    </ul>
  );
}

function Added({ o }: { o: PinAddOutput }) {
  return (
    <>
      <p>
        <Badge tone="ok" icon={<IconOk />}>
          Pinned
        </Badge>{' '}
        {plural(o.pins.length, 'CID')} now pinned on your node{o.name ? ` as ${displayText(o.name)}` : ''}.
      </p>
      <CidList cids={o.pins} />
    </>
  );
}

function Removed({ o }: { o: PinRmOutput }) {
  return (
    <>
      <p>
        <Badge tone="warn">Unpinned</Badge> {plural(o.unpinned.length, 'CID')}. Unpinned content becomes eligible for garbage
        collection.
      </p>
      <CidList cids={o.unpinned} />
      <Warnings items={o.warnings} />
    </>
  );
}

function Updated({ o }: { o: PinUpdateOutput }) {
  return (
    <>
      <KeyValue
        items={[
          { label: 'From', value: <Cid value={o.from} /> },
          { label: 'To', value: <Cid value={o.to} /> },
          {
            label: 'Old pin',
            value: <Badge tone={o.unpinned_old ? 'warn' : 'neutral'}>{o.unpinned_old ? 'Unpinned' : 'Kept'}</Badge>,
          },
        ]}
      />
      <Warnings items={o.warnings} />
    </>
  );
}

function Verified({ o, onLoadMore }: { o: PinVerifyOutput; onLoadMore: ((cursor: string) => void) | undefined }) {
  const bad = o.results.filter((r) => !r.ok).length;
  return (
    <>
      <p>
        {bad === 0 ? (
          <Badge tone="ok" icon={<IconOk />}>
            All verified
          </Badge>
        ) : (
          <Badge tone="err" icon={<IconDenied />}>
            {formatInt(bad)} damaged
          </Badge>
        )}{' '}
        {plural(o.results.length, 'pin')} checked on this page.
      </p>
      <DataTable
        caption="Pin verification"
        rows={o.results}
        rowKey={(r, i) => `${r.cid}:${i}`}
        columns={[
          { key: 'cid', header: 'CID', cell: (r) => <Cid value={r.cid} compact /> },
          {
            key: 'state',
            header: 'State',
            cell: (r) =>
              r.ok ? (
                <Badge tone="ok">Verified</Badge>
              ) : (
                <Badge tone="err">Damaged</Badge>
              ),
          },
          {
            key: 'detail',
            header: 'Detail',
            cell: (r) =>
              r.bad_nodes && r.bad_nodes.length > 0 ? (
                <details>
                  <summary>{plural(r.bad_nodes.length, 'bad block')}</summary>
                  <ul className="iu-list">
                    {r.bad_nodes.map((n) => (
                      <li key={n.cid}>
                        <Cid value={n.cid} compact /> <span className="iu-muted">{displayText(n.error)}</span>
                      </li>
                    ))}
                  </ul>
                </details>
              ) : (
                <span className="iu-muted">None</span>
              ),
          },
        ]}
      />
      {bad > 0 ? (
        <Notice tone="err" label="Damaged">
          Damaged blocks are missing or fail their hash check. The content may be unreadable until it is fetched again.
        </Notice>
      ) : null}
      <More hasMore={o.page.has_more} nextCursor={o.page.next_cursor} onLoadMore={onLoadMore} noun="results" />
    </>
  );
}
