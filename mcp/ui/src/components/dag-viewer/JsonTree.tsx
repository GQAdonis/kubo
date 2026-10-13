import { displayText } from '../../lib/safe-text';
import { Cid } from '../../ui/primitives';

// An IPLD node rendered as nested disclosures. <details>/<summary> gives keyboard operation
// (Enter and Space) and screen reader expansion state for free, so there is no custom tree widget.
// All values are rendered as text. Links ({"/": "<cid>"}) and bytes ({"/": {"bytes": "..."}}) are
// recognised because they are how dag-json encodes IPLD links and byte strings.

const MAX_DEPTH = 12;
const OPEN_DEPTH = 1;
const PAGE = 100;

type Json = null | boolean | number | string | Json[] | { [k: string]: Json };

export interface JsonTreeProps {
  value: unknown;
  /** Accessible name for the whole tree. */
  label: string;
}

export function JsonTree({ value, label }: JsonTreeProps) {
  return (
    <div className="iu-json" role="group" aria-label={label}>
      <Node name={null} value={value as Json} depth={0} />
    </div>
  );
}

function linkOf(v: Json): string | null {
  if (v && typeof v === 'object' && !Array.isArray(v)) {
    const keys = Object.keys(v);
    if (keys.length === 1 && keys[0] === '/' && typeof v['/'] === 'string') return v['/'];
  }
  return null;
}

function bytesOf(v: Json): string | null {
  if (v && typeof v === 'object' && !Array.isArray(v)) {
    const keys = Object.keys(v);
    const inner = v['/'];
    if (keys.length === 1 && keys[0] === '/' && inner && typeof inner === 'object' && !Array.isArray(inner)) {
      const b = (inner as { [k: string]: Json })['bytes'];
      if (typeof b === 'string') return b;
    }
  }
  return null;
}

function Key({ name }: { name: string | null }) {
  return name === null ? null : <span className="iu-json__key">{displayText(name)}: </span>;
}

function Node({ name, value, depth }: { name: string | null; value: Json; depth: number }) {
  const link = linkOf(value);
  if (link !== null) {
    return (
      <div className="iu-json__row">
        <Key name={name} />
        <span className="iu-muted">link </span>
        <Cid value={link} compact />
      </div>
    );
  }
  const bytes = bytesOf(value);
  if (bytes !== null) {
    return (
      <div className="iu-json__row">
        <Key name={name} />
        <span className="iu-muted">bytes, {bytes.length} base64 characters</span>
      </div>
    );
  }
  if (value === null || typeof value !== 'object') {
    return (
      <div className="iu-json__row">
        <Key name={name} />
        <Scalar value={value} />
      </div>
    );
  }
  if (depth >= MAX_DEPTH) {
    return (
      <div className="iu-json__row">
        <Key name={name} />
        <span className="iu-muted">nested deeper than {MAX_DEPTH} levels, not rendered</span>
      </div>
    );
  }
  const isArray = Array.isArray(value);
  const entries: [string, Json][] = isArray ? value.map((v, i) => [String(i), v]) : Object.entries(value);
  const shown = entries.slice(0, PAGE);
  const summary = isArray ? `array, ${entries.length} ${entries.length === 1 ? 'item' : 'items'}` : `object, ${entries.length} ${entries.length === 1 ? 'key' : 'keys'}`;
  return (
    <details className="iu-json__group" open={depth < OPEN_DEPTH}>
      <summary>
        <Key name={name} />
        <span className="iu-muted">{summary}</span>
      </summary>
      <div className="iu-json__children">
        {shown.map(([k, v]) => (
          <Node key={k} name={isArray ? `[${k}]` : k} value={v} depth={depth + 1} />
        ))}
        {entries.length > PAGE ? (
          <p className="iu-muted">{entries.length - PAGE} more entries are not rendered.</p>
        ) : null}
      </div>
    </details>
  );
}

function Scalar({ value }: { value: null | boolean | number | string }) {
  if (value === null) return <span className="iu-json__null">null</span>;
  if (typeof value === 'string') return <span className="iu-json__str">{JSON.stringify(displayText(value))}</span>;
  return <span className={typeof value === 'number' ? 'iu-json__num' : 'iu-json__bool'}>{String(value)}</span>;
}
