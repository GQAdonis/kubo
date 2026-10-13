import { decodeContent, sha256Hex } from './bytes.js';
import type { ConfirmationRequest, ConfirmationTarget, GeneratedTool } from './types.js';

// Everything an agent supplies and the person is asked to judge goes through `sanitize`
// first. Without it a path could carry a right-to-left override that reorders what the
// person reads, a zero-width character that makes two different names look equal, or a
// control character that hides text. Each such code point is shown as a visible <U+XXXX>.

const SHORT = 120;
const FULL_CAP = 20_000;
const PREVIEW_SHORT = 240;

// C0 and C1 controls, soft hyphen, Arabic letter mark, Mongolian vowel separator, zero-width
// and directional marks and overrides, line and paragraph separators, word joiner and
// invisible operators, BOM, interlinear annotation marks, the tag block and lone surrogates.
const INVISIBLE = new RegExp(
  '[\\u0000-\\u001f\\u007f-\\u009f\\u00ad\\u061c\\u180e\\u200b-\\u200f\\u2028-\\u202e\\u2060-\\u206f\\ufeff\\ufff9-\\ufffb]|[\\u{e0000}-\\u{e007f}]|[\\ud800-\\udfff]',
  'gu',
);

/** Replace invisible and direction-changing code points with visible tokens. */
export function sanitize(text: string): string {
  return text.replace(INVISIBLE, (ch: string) => `<U+${ch.codePointAt(0)!.toString(16).toUpperCase().padStart(4, '0')}>`);
}

/** Cut a long value in the middle so both ends stay visible. */
export function middleTruncate(text: string, max: number): string {
  if (text.length <= max) return text;
  const head = Math.ceil((max - 1) / 2);
  const tail = Math.floor((max - 1) / 2);
  return `${text.slice(0, head)}…${text.slice(text.length - tail)}`;
}

/** Short, single-line text for the person: sanitized, then cut in the middle. */
export function preview(text: string, max = PREVIEW_SHORT): string {
  return middleTruncate(sanitize(text), max);
}

/** One labelled value for the dialog. `full` carries the whole sanitized value when it was cut. */
export function item(label: string, raw: unknown, max = SHORT): ConfirmationTarget {
  const text = String(raw);
  const clean = sanitize(text);
  const out: ConfirmationTarget = { label, value: middleTruncate(clean, max) };
  if (clean.length > max) out.full = clean.length > FULL_CAP ? `${clean.slice(0, FULL_CAP)}…` : clean;
  // eslint-disable-next-line no-control-regex
  if (/[^\u0000-\u007f]/.test(text)) out.nonAscii = true;
  return out;
}

const yesNo = (v: unknown): string => (v === true ? 'yes' : 'no');

interface Described {
  effect: string;
  targets: ConfirmationTarget[];
  details: ConfirmationTarget[];
}

type Input = Record<string, any>;

/** Byte length and SHA-256 of what will be stored, so the person can compare it with what they expect. */
function fingerprint(bytes: Uint8Array | undefined): ConfirmationTarget[] {
  if (!bytes) return [];
  return [item('bytes', bytes.length), item('sha256', sha256Hex(bytes), 80)];
}

function bytesOfContent(i: Input): Uint8Array | undefined {
  try {
    return decodeContent(String(i.content), i.encoding === 'base64' ? 'base64' : 'utf8');
  } catch {
    return undefined;
  }
}

function contentPreview(i: Input, field: string): ConfirmationTarget {
  return i.encoding === 'base64' ? item('content_preview', '(base64 data)') : item('content_preview', i[field], PREVIEW_SHORT);
}

const describers: Record<string, (i: Input) => Described> = {
  ipfs_add: (i) => ({
    effect: "Add the content below to this node's storage.",
    targets: i.filename ? [item('name', i.filename)] : [],
    details: [
      ...fingerprint(bytesOfContent(i)),
      item('pin', yesNo(i.pin)),
      item('provide', yesNo(i.provide)),
      ...(i.cid_version !== undefined ? [item('recipe_cid_version', i.cid_version)] : []),
      ...(i.raw_leaves !== undefined ? [item('recipe_raw_leaves', yesNo(i.raw_leaves))] : []),
      ...(i.chunker !== undefined ? [item('recipe_chunker', i.chunker)] : []),
      ...(i.hash !== undefined ? [item('recipe_hash', i.hash)] : []),
      contentPreview(i, 'content'),
    ],
  }),
  pin_add: (i) => ({
    effect: 'Pin this content on the node so it is kept.',
    targets: [item('path', i.path)],
    details: [item('recursive', yesNo(i.recursive ?? true)), ...(i.name ? [item('name', i.name)] : [])],
  }),
  mfs_write: (i) => ({
    effect: 'Write this content to a file in the Files area, replacing the file if it exists.',
    targets: [item('path', i.path)],
    details: [...fingerprint(bytesOfContent(i)), item('parents', yesNo(i.parents)), contentPreview(i, 'content')],
  }),
  mfs_mkdir: (i) => ({
    effect: 'Create a folder in the Files area.',
    targets: [item('path', i.path)],
    details: [item('parents', yesNo(i.parents))],
  }),
  mfs_cp: (i) => ({
    effect: 'Copy a file or folder in the Files area.',
    targets: [item('source', i.source), item('destination', i.destination)],
    details: [item('parents', yesNo(i.parents))],
  }),
  mfs_mv: (i) => ({
    effect: 'Move a file or folder in the Files area.',
    targets: [item('source', i.source), item('destination', i.destination)],
    details: [],
  }),
  ipns_publish: (i) => ({
    effect: 'Publish this path under an IPNS name, signed with the named key.',
    targets: [item('path', i.path), item('key', i.key ?? 'self')],
    details: [
      item('replaces', 'yes'),
      ...((i.key ?? 'self') === 'self' ? [item('identity_key', 'yes')] : []),
      item('lifetime', i.lifetime ?? '48h'),
      ...(i.ttl ? [item('ttl', i.ttl)] : []),
    ],
  }),
  key_gen: (i) => ({
    effect: "Create a new key in this node's keystore.",
    targets: [item('name', i.name)],
    details: [item('key_type', i.type ?? 'ed25519'), ...(i.size !== undefined ? [item('key_size', i.size)] : [])],
  }),
  key_rename: (i) => ({
    effect: 'Rename a key in the keystore.',
    targets: [item('old_name', i.old_name), item('new_name', i.new_name)],
    details: [],
  }),
  dag_put: (i) => ({
    effect: 'Store this node in the DAG.',
    targets: [],
    details: [
      ...fingerprint(new TextEncoder().encode(String(i.data))),
      item('store_codec', i.store_codec ?? 'dag-cbor'),
      item('pin', yesNo(i.pin)),
      item('content_preview', i.data, PREVIEW_SHORT),
    ],
  }),
  routing_provide: (i) => ({
    effect: 'Announce to the network that this node provides this CID.',
    targets: [item('cid', i.cid)],
    details: [item('recursive', yesNo(i.recursive))],
  }),
};

/** Tools that have a describer. A write tool without one is not exported (a test enforces it). */
export const DESCRIBED_TOOLS: readonly string[] = Object.keys(describers);

export function describeWrite(tool: GeneratedTool, input: Record<string, unknown>): ConfirmationRequest {
  const d = describers[tool.name]?.(input) ?? {
    effect: `Run ${tool.title}.`,
    targets: [],
    details: [item('input', JSON.stringify(input), PREVIEW_SHORT)],
  };
  return { tool: tool.name, title: tool.title, confirmation: 'write', ...d };
}
