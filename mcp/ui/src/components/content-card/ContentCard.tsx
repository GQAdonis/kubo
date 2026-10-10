import type { ReactNode } from 'react';
import type { ContentCardData } from '../../contract/generated/bindings';
import type {
  IpfsAddOutput,
  IpfsCatOutput,
  IpfsGetOutput,
  IpfsIndexCidOutput,
  IpfsIndexSearchOutput,
  IpfsLsOutput,
  MfsReadOutput,
} from '../../contract/generated/outputs';
import { TOOL_META } from '../../contract/generated/tool-meta';
import { formatBytesExact, formatInt } from '../../lib/format';
import { PREVIEW_LIMIT, clip, displayText } from '../../lib/safe-text';
import { UiScope } from '../../scope/UiScope';
import {
  Badge,
  Bytes,
  Button,
  Cid,
  DataTable,
  EmptyState,
  KeyValue,
  More,
  Notice,
  Panel,
  UntrustedFrame,
  Warnings,
} from '../../ui/primitives';
import { IconFile, IconFolder } from '../../ui/icons';

export type ContentCardProps = ContentCardData & {
  /** Open a directory listing or entry. The host decides which tool to call. */
  onOpenPath?: (path: string) => void;
  /** Load the next page of a listing. */
  onLoadMore?: (cursor: string) => void;
};

/** File bytes, a directory listing, or a search hit for a CID or path. */
export function ContentCard(props: ContentCardProps) {
  const meta = TOOL_META[props.tool];
  let body: ReactNode;
  switch (props.tool) {
    case 'ipfs_add':
      body = <AddedView o={props.output} />;
      break;
    case 'ipfs_cat':
      body = <BytesView o={props.output} />;
      break;
    case 'mfs_read':
      body = <BytesView o={props.output} />;
      break;
    case 'ipfs_get':
      body = <GetView o={props.output} />;
      break;
    case 'ipfs_ls':
      body = <ListingView o={props.output} onOpenPath={props.onOpenPath} onLoadMore={props.onLoadMore} />;
      break;
    case 'ipfs_index_search':
      body = <SearchView o={props.output} />;
      break;
    case 'ipfs_index_cid':
      body = <IndexedView o={props.output} />;
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

function AddedView({ o }: { o: IpfsAddOutput }) {
  return (
    <>
      <KeyValue
        items={[
          { label: 'CID', value: <Cid value={o.cid} /> },
          ...(o.name ? [{ label: 'Name', value: displayText(o.name) }] : []),
          { label: 'Size', value: <Bytes value={o.size} exact /> },
          {
            label: 'Pin',
            value: <Badge tone={o.pinned ? 'ok' : 'neutral'}>{o.pinned ? 'Pinned' : 'Not pinned'}</Badge>,
          },
          {
            label: 'Providing',
            value: <Badge tone={o.provided ? 'ok' : 'neutral'}>{o.provided ? 'Announced' : 'Not announced'}</Badge>,
          },
          ...(o.resource ? [{ label: 'Resource', value: <code>{o.resource.uri}</code> }] : []),
        ]}
      />
      <Warnings items={o.warnings} />
    </>
  );
}

function BytesPreview({ content, encoding }: { content: string; encoding: 'utf8' | 'base64' }) {
  const shown = clip(encoding === 'utf8' ? displayText(content) : content, PREVIEW_LIMIT);
  return (
    <>
      {encoding === 'base64' ? (
        <Notice tone="info" label="Binary content">
          These bytes are not valid text, so they are shown as base64.
        </Notice>
      ) : null}
      <pre className="iu-pre" tabIndex={0} aria-label={encoding === 'utf8' ? 'Content text' : 'Content as base64'}>
        {shown.text}
      </pre>
      {shown.clipped ? (
        <Notice tone="warn" label="Preview clipped">
          {formatInt(shown.omitted)} more characters were returned but are not rendered.
        </Notice>
      ) : null}
    </>
  );
}

function BytesView({ o }: { o: IpfsCatOutput | MfsReadOutput }) {
  const shownEnd = o.offset + o.length;
  return (
    <>
      <KeyValue
        items={[
          { label: 'Path', value: <code className="iu-wrap">{displayText(o.path)}</code> },
          {
            label: 'Range',
            value: (
              <span className="iu-num">
                bytes {formatInt(o.offset)} to {formatInt(shownEnd)}
                {o.size !== undefined ? ` of ${formatBytesExact(o.size)}` : ''}
              </span>
            ),
          },
        ]}
      />
      {o.truncated ? (
        <Notice tone="warn" label="Truncated">
          The node returned {formatInt(o.length)} bytes
          {o.size !== undefined ? ` of ${formatInt(o.size)}` : ''}. Ask for a later offset to read the rest.
        </Notice>
      ) : null}
      {o.length === 0 ? <EmptyState>This range is empty.</EmptyState> : <BytesPreview content={o.content} encoding={o.encoding} />}
    </>
  );
}

const TYPE_LABEL: Record<string, string> = {
  file: 'File',
  directory: 'Directory',
  symlink: 'Symlink',
  unknown: 'Unknown type',
};

function GetView({ o }: { o: IpfsGetOutput }) {
  return (
    <>
      <KeyValue
        items={[
          { label: 'Path', value: <code className="iu-wrap">{displayText(o.path)}</code> },
          { label: 'CID', value: <Cid value={o.cid} /> },
          { label: 'Type', value: <Badge>{TYPE_LABEL[o.type] ?? o.type}</Badge> },
          ...(o.size !== undefined ? [{ label: 'Size', value: <Bytes value={o.size} exact /> }] : []),
          { label: 'Read separately', value: <code className="iu-wrap">{o.resource.uri}</code> },
        ]}
      />
      {o.content !== undefined && o.encoding ? (
        <>
          {o.truncated ? (
            <Notice tone="warn" label="Truncated">
              Only the start of this content is inline. Read the resource for the whole object.
            </Notice>
          ) : null}
          <BytesPreview content={o.content} encoding={o.encoding} />
        </>
      ) : (
        <p className="iu-muted">No inline content. Read the resource above to fetch the bytes.</p>
      )}
    </>
  );
}

function ListingView({
  o,
  onOpenPath,
  onLoadMore,
}: {
  o: IpfsLsOutput;
  onOpenPath: ((path: string) => void) | undefined;
  onLoadMore: ((cursor: string) => void) | undefined;
}) {
  const base = o.path.replace(/\/$/, '');
  return (
    <>
      <p className="iu-muted">
        Listing <code className="iu-wrap">{displayText(o.path)}</code>
      </p>
      <DataTable
        caption={`Entries in ${o.path}`}
        rows={o.entries}
        rowKey={(e, i) => `${e.cid}:${e.name}:${i}`}
        empty={<EmptyState>This directory has no entries.</EmptyState>}
        columns={[
          {
            key: 'name',
            header: 'Name',
            cell: (e) => (
              <span className="iu-entry">
                {e.type === 'directory' ? <IconFolder /> : <IconFile />}
                {e.type === 'directory' && onOpenPath ? (
                  <Button variant="text" compact onClick={() => onOpenPath(`${base}/${e.name}`)}>
                    {displayText(e.name)}
                  </Button>
                ) : (
                  <span className="iu-wrap">{displayText(e.name)}</span>
                )}
              </span>
            ),
          },
          { key: 'type', header: 'Type', cell: (e) => TYPE_LABEL[e.type] ?? e.type },
          { key: 'size', header: 'Size', numeric: true, cell: (e) => (e.size === undefined ? '-' : <Bytes value={e.size} />) },
          { key: 'cid', header: 'CID', cell: (e) => <Cid value={e.cid} compact /> },
        ]}
      />
      <More hasMore={o.page.has_more} nextCursor={o.page.next_cursor} onLoadMore={onLoadMore} noun="entries" />
    </>
  );
}

function SearchView({ o }: { o: IpfsIndexSearchOutput }) {
  if (o.results.length === 0) {
    return <EmptyState>No indexed chunk matches. The search used model {o.model}.</EmptyState>;
  }
  return (
    <>
      <p className="iu-muted">
        {formatInt(o.results.length)} {o.results.length === 1 ? 'match' : 'matches'} from model <code>{o.model}</code>, best first.
      </p>
      <ol className="iu-hits">
        {o.results.map((r) => (
          <li key={`${r.cid}:${r.chunk_index}`} className="iu-hit">
            <div className="iu-hit__meta">
              <Badge tone="info">Score {r.score.toFixed(3)}</Badge>
              <Cid value={r.cid} compact />
              {r.path ? <code className="iu-wrap">{displayText(r.path)}</code> : null}
              {r.mime ? <span className="iu-muted">{displayText(r.mime)}</span> : null}
              <span className="iu-muted">chunk {formatInt(r.chunk_index)}</span>
            </div>
            <p className="iu-hit__text">{displayText(r.text)}</p>
          </li>
        ))}
      </ol>
    </>
  );
}

const SKIP_REASON: Record<string, string> = {
  mime_not_allowed: 'The media type is not on the index allow list.',
  too_large: 'The content is larger than the index size limit.',
  already_indexed: 'This CID was already indexed.',
  not_a_file: 'The CID is a directory or other non-file node, so there is no text to index.',
  empty: 'The file has no content to index.',
};

function IndexedView({ o }: { o: IpfsIndexCidOutput }) {
  return (
    <>
      <KeyValue
        items={[
          { label: 'CID', value: <Cid value={o.cid} /> },
          { label: 'Chunks indexed', value: <span className="iu-num">{formatInt(o.chunks_indexed)}</span> },
          ...(o.model ? [{ label: 'Model', value: <code>{o.model}</code> }] : []),
        ]}
      />
      {o.skipped_reason ? (
        <Notice tone="info" label="Skipped">
          {SKIP_REASON[o.skipped_reason] ?? o.skipped_reason}
        </Notice>
      ) : null}
    </>
  );
}
