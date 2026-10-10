import type { ReactNode } from 'react';
import type { IpnsRecordCardData } from '../../contract/generated/bindings';
import type {
  IpnsPublishOutput,
  IpnsResolveOutput,
  KeyGenOutput,
  KeyListOutput,
  KeyRenameOutput,
  KeyRmOutput,
} from '../../contract/generated/outputs';
import { TOOL_META } from '../../contract/generated/tool-meta';
import { plural } from '../../lib/format';
import { displayText } from '../../lib/safe-text';
import { UiScope } from '../../scope/UiScope';
import { IconKey } from '../../ui/icons';
import { Badge, DataTable, EmptyState, IdText, KeyValue, Panel, UntrustedFrame } from '../../ui/primitives';

export type IpnsRecordCardProps = IpnsRecordCardData;

const KEY_NOTE = 'Only the key name and public ID are shown. Private key material never leaves the node.';

/** An IPNS name and what it resolves to, and the keystore keys that can publish names. */
export function IpnsRecordCard(props: IpnsRecordCardProps) {
  const meta = TOOL_META[props.tool];
  let body: ReactNode;
  switch (props.tool) {
    case 'ipns_resolve':
      body = <Resolved o={props.output} />;
      break;
    case 'ipns_publish':
      body = <Published o={props.output} />;
      break;
    case 'key_list':
      body = <Keys o={props.output} />;
      break;
    case 'key_gen':
      body = <Generated o={props.output} />;
      break;
    case 'key_rename':
      body = <Renamed o={props.output} />;
      break;
    case 'key_rm':
      body = <KeysRemoved o={props.output} />;
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

function Resolved({ o }: { o: IpnsResolveOutput }) {
  return (
    <>
      <KeyValue
        items={[
          { label: 'Name', value: <IdText value={o.name} kind="IPNS name" /> },
          { label: 'Resolves to', value: <IdText value={o.path} kind="path" /> },
        ]}
      />
      <p className="iu-muted">A name can point somewhere else at any time. Check the target before you rely on it.</p>
    </>
  );
}

function Published({ o }: { o: IpnsPublishOutput }) {
  return (
    <KeyValue
      items={[
        { label: 'Name', value: <IdText value={o.name} kind="IPNS name" /> },
        { label: 'Now points to', value: <IdText value={o.value} kind="path" /> },
        { label: 'Record', value: <Badge tone="ok">Published</Badge> },
      ]}
    />
  );
}

function Keys({ o }: { o: KeyListOutput }) {
  return (
    <>
      <DataTable
        caption="Keystore keys"
        rows={o.keys}
        rowKey={(k) => k.name}
        empty={<EmptyState>The keystore has no keys.</EmptyState>}
        columns={[
          {
            key: 'name',
            header: 'Name',
            cell: (k) => (
              <span className="iu-entry">
                <IconKey />
                {displayText(k.name)}
              </span>
            ),
          },
          { key: 'id', header: 'Public ID', cell: (k) => <IdText value={k.id} kind="key ID" compact /> },
        ]}
      />
      <p className="iu-muted">
        {plural(o.keys.length, 'key')}. {KEY_NOTE}
      </p>
    </>
  );
}

function Generated({ o }: { o: KeyGenOutput }) {
  return (
    <>
      <KeyValue
        items={[
          { label: 'Key name', value: displayText(o.name) },
          { label: 'Public ID', value: <IdText value={o.id} kind="key ID" /> },
        ]}
      />
      <p className="iu-muted">{KEY_NOTE}</p>
    </>
  );
}

function Renamed({ o }: { o: KeyRenameOutput }) {
  return (
    <KeyValue
      items={[
        { label: 'Was', value: displayText(o.old_name) },
        { label: 'Now', value: displayText(o.new_name) },
        { label: 'Public ID', value: <IdText value={o.id} kind="key ID" /> },
      ]}
    />
  );
}

function KeysRemoved({ o }: { o: KeyRmOutput }) {
  return (
    <>
      <DataTable
        caption="Removed keys"
        rows={o.removed}
        rowKey={(k) => k.name}
        columns={[
          { key: 'name', header: 'Name', cell: (k) => displayText(k.name) },
          { key: 'id', header: 'Public ID', cell: (k) => <IdText value={k.id} kind="key ID" compact /> },
        ]}
      />
      <p className="iu-muted">IPNS names published with a removed key can no longer be updated.</p>
    </>
  );
}
