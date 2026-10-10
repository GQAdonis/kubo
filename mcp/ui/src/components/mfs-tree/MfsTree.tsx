import type { ReactNode } from 'react';
import type { MfsTreeData } from '../../contract/generated/bindings';
import type {
  MfsCpOutput,
  MfsLsOutput,
  MfsMkdirOutput,
  MfsMvOutput,
  MfsRmOutput,
  MfsStatOutput,
  MfsWriteOutput,
} from '../../contract/generated/outputs';
import { TOOL_META } from '../../contract/generated/tool-meta';
import { formatInt } from '../../lib/format';
import { displayText } from '../../lib/safe-text';
import { UiScope } from '../../scope/UiScope';
import { IconChevronRight, IconFile, IconFolder } from '../../ui/icons';
import {
  Badge,
  Button,
  Bytes,
  Cid,
  DataTable,
  EmptyState,
  KeyValue,
  More,
  Panel,
  UntrustedFrame,
} from '../../ui/primitives';

export type MfsTreeProps = MfsTreeData & {
  /** Open a directory or entry. The host calls mfs_ls or mfs_stat. */
  onOpenPath?: (path: string) => void;
  onLoadMore?: (cursor: string) => void;
};

/** MFS directory listing with a path trail, file details, and receipts for MFS writes. */
export function MfsTree(props: MfsTreeProps) {
  const meta = TOOL_META[props.tool];
  let body: ReactNode;
  switch (props.tool) {
    case 'mfs_ls':
      body = <Listing o={props.output} onOpenPath={props.onOpenPath} onLoadMore={props.onLoadMore} />;
      break;
    case 'mfs_stat':
      body = <Stat o={props.output} />;
      break;
    case 'mfs_write':
      body = <Written o={props.output} />;
      break;
    case 'mfs_mkdir':
      body = <Made o={props.output} />;
      break;
    case 'mfs_cp':
      body = <Moved o={props.output} verb="Copied" />;
      break;
    case 'mfs_mv':
      body = <Moved o={props.output} verb="Moved" />;
      break;
    case 'mfs_rm':
      body = <Removed o={props.output} />;
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

/** "/a/b/c" -> [{name:"/", path:"/"}, {name:"a", path:"/a"}, ...] */
export function pathTrail(path: string): { name: string; path: string }[] {
  const parts = path.split('/').filter(Boolean);
  const trail = [{ name: '/', path: '/' }];
  let acc = '';
  for (const p of parts) {
    acc += `/${p}`;
    trail.push({ name: p, path: acc });
  }
  return trail;
}

function Trail({ path, onOpenPath }: { path: string; onOpenPath: ((path: string) => void) | undefined }) {
  const trail = pathTrail(path);
  return (
    <nav aria-label="Path">
      <ol className="iu-trail">
        {trail.map((seg, i) => {
          const last = i === trail.length - 1;
          return (
            <li key={seg.path}>
              {i > 0 ? <IconChevronRight className="iu-trail__sep" /> : null}
              {onOpenPath && !last ? (
                <Button variant="text" compact onClick={() => onOpenPath(seg.path)}>
                  {displayText(seg.name === '/' ? 'Root' : seg.name)}
                </Button>
              ) : (
                <span aria-current={last ? 'page' : undefined} className={last ? 'iu-trail__here' : undefined}>
                  {displayText(seg.name === '/' ? 'Root' : seg.name)}
                </span>
              )}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}

function Listing({
  o,
  onOpenPath,
  onLoadMore,
}: {
  o: MfsLsOutput;
  onOpenPath: ((path: string) => void) | undefined;
  onLoadMore: ((cursor: string) => void) | undefined;
}) {
  const base = o.path === '/' ? '' : o.path.replace(/\/$/, '');
  return (
    <>
      <Trail path={o.path} onOpenPath={onOpenPath} />
      <DataTable
        caption={`Entries in ${o.path}`}
        rows={o.entries}
        rowKey={(e, i) => `${e.name}:${i}`}
        empty={<EmptyState>This directory is empty.</EmptyState>}
        columns={[
          {
            key: 'name',
            header: 'Name',
            cell: (e) => (
              <span className="iu-entry">
                {e.type === 'directory' ? <IconFolder /> : <IconFile />}
                {onOpenPath ? (
                  <Button variant="text" compact onClick={() => onOpenPath(`${base}/${e.name}`)}>
                    {displayText(e.name)}
                    <span className="iu-sr-only">{e.type === 'directory' ? ', open directory' : ', show details'}</span>
                  </Button>
                ) : (
                  <span className="iu-wrap">{displayText(e.name)}</span>
                )}
              </span>
            ),
          },
          { key: 'type', header: 'Type', cell: (e) => (e.type === 'directory' ? 'Directory' : 'File') },
          { key: 'size', header: 'Size', numeric: true, cell: (e) => (e.size === undefined ? '-' : <Bytes value={e.size} />) },
          { key: 'cid', header: 'CID', cell: (e) => (e.cid ? <Cid value={e.cid} compact /> : '-') },
        ]}
      />
      <More hasMore={o.page.has_more} nextCursor={o.page.next_cursor} onLoadMore={onLoadMore} noun="entries" />
    </>
  );
}

function Stat({ o }: { o: MfsStatOutput }) {
  return (
    <KeyValue
      items={[
        { label: 'Path', value: <code className="iu-wrap">{displayText(o.path)}</code> },
        { label: 'Type', value: <Badge>{o.type === 'directory' ? 'Directory' : 'File'}</Badge> },
        { label: 'CID', value: <Cid value={o.cid} /> },
        { label: 'Size', value: <Bytes value={o.size} exact /> },
        ...(o.cumulative_size !== undefined ? [{ label: 'With children', value: <Bytes value={o.cumulative_size} exact /> }] : []),
        ...(o.blocks !== undefined ? [{ label: 'Blocks', value: <span className="iu-num">{formatInt(o.blocks)}</span> }] : []),
      ]}
    />
  );
}

function Written({ o }: { o: MfsWriteOutput }) {
  return (
    <KeyValue
      items={[
        { label: 'Wrote', value: <code className="iu-wrap">{displayText(o.path)}</code> },
        { label: 'Bytes written', value: <Bytes value={o.bytes_written} exact /> },
        { label: 'New CID', value: <Cid value={o.cid} /> },
      ]}
    />
  );
}

function Made({ o }: { o: MfsMkdirOutput }) {
  return (
    <KeyValue
      items={[
        { label: 'Created', value: <code className="iu-wrap">{displayText(o.path)}</code> },
        { label: 'CID', value: <Cid value={o.cid} /> },
      ]}
    />
  );
}

function Moved({ o, verb }: { o: MfsCpOutput | MfsMvOutput; verb: 'Copied' | 'Moved' }) {
  return (
    <KeyValue
      items={[
        { label: verb, value: <code className="iu-wrap">{displayText(o.source)}</code> },
        { label: 'To', value: <code className="iu-wrap">{displayText(o.destination)}</code> },
        ...('cid' in o ? [{ label: 'CID', value: <Cid value={o.cid} /> }] : []),
      ]}
    />
  );
}

function Removed({ o }: { o: MfsRmOutput }) {
  return (
    <KeyValue
      items={[
        { label: 'Removed', value: <code className="iu-wrap">{displayText(o.removed)}</code> },
        { label: 'Effect', value: 'The path is gone from MFS. Pinned content stays on your node.' },
      ]}
    />
  );
}
