import { COMPONENT_META } from '../contract/generated/tool-meta';
import { VIEW_TOOLS } from '../contract/generated/bindings';

// Structural validation of a generated A2UI v0.9 catalog document against the contract and the
// implementations. It is pure and has no I/O: pass the manifest and the implementation names.

export type CatalogIssueKind =
  | 'id-mismatch'
  | 'not-a-schema-document'
  | 'unknown-component'
  | 'missing-component'
  | 'missing-implementation'
  | 'orphan-implementation'
  | 'malformed-component'
  | 'any-component-mismatch'
  | 'dangling-ref'
  | 'external-ref'
  | 'tool-binding-mismatch'
  | 'child-reference-untyped';

export interface CatalogIssue {
  readonly kind: CatalogIssueKind;
  readonly component?: string;
  readonly message: string;
}

type Json = null | boolean | number | string | Json[] | { [k: string]: Json };
type Obj = { [k: string]: Json };

const isObj = (v: unknown): v is Obj => typeof v === 'object' && v !== null && !Array.isArray(v);

/** Resolve a local JSON Pointer ("#/$defs/X") inside the document. */
function resolvePointer(doc: Obj, ref: string): Json | undefined {
  if (!ref.startsWith('#')) return undefined;
  const parts = ref
    .slice(1)
    .split('/')
    .filter((p, i) => !(i === 0 && p === ''))
    .map((p) => p.replace(/~1/g, '/').replace(/~0/g, '~'));
  let cur: Json = doc;
  for (const p of parts) {
    if (Array.isArray(cur)) cur = cur[Number(p)] ?? null;
    else if (isObj(cur) && p in cur) cur = cur[p] as Json;
    else return undefined;
  }
  return cur;
}

function collectRefs(node: Json, out: string[] = []): string[] {
  if (Array.isArray(node)) node.forEach((n) => collectRefs(n, out));
  else if (isObj(node)) {
    for (const [k, v] of Object.entries(node)) {
      if (k === '$ref' && typeof v === 'string') out.push(v);
      else collectRefs(v, out);
    }
  }
  return out;
}

export interface ValidateInput {
  readonly manifest: unknown;
  /** Names that have a React implementation. */
  readonly implemented: readonly string[];
  /** Override the contract (tests only). */
  readonly contract?: {
    readonly components: readonly { readonly name: string; readonly kind: string }[];
    readonly viewTools: Readonly<Record<string, readonly string[]>>;
  };
}

const CHILD_PROPS = new Set(['child', 'children', 'componentId', 'childId']);

export function validateCatalogManifest(input: ValidateInput): CatalogIssue[] {
  const issues: CatalogIssue[] = [];
  const add = (kind: CatalogIssueKind, message: string, component?: string) =>
    issues.push({ kind, message, ...(component !== undefined && { component }) });

  const m = input.manifest;
  if (!isObj(m)) {
    add('not-a-schema-document', 'The manifest is not a JSON object.');
    return issues;
  }
  const contractComponents = (input.contract?.components ?? COMPONENT_META).map((c) => c.name as string);
  const viewTools = (input.contract?.viewTools ?? VIEW_TOOLS) as Readonly<Record<string, readonly string[] | undefined>>;

  // Identity (A2UI-P: both $id and catalogId, set to the same URI).
  if (typeof m['catalogId'] !== 'string' || m['catalogId'] === '') add('id-mismatch', 'catalogId is missing.');
  if (m['$id'] !== m['catalogId']) add('id-mismatch', `$id (${String(m['$id'])}) must equal catalogId (${String(m['catalogId'])}).`);
  if (m['$schema'] !== 'https://json-schema.org/draft/2020-12/schema') add('not-a-schema-document', '$schema must be JSON Schema 2020-12.');

  const components = m['components'];
  if (!isObj(components)) {
    add('not-a-schema-document', 'components is missing or not an object.');
    return issues;
  }
  const names = Object.keys(components);

  // Every entry resolves to a contract component and an implementation, and nothing is left out.
  for (const n of names) {
    if (!contractComponents.includes(n)) add('unknown-component', `Catalog entry ${n} is not a contract component.`, n);
    if (!input.implemented.includes(n)) add('missing-implementation', `Catalog entry ${n} has no React implementation.`, n);
  }
  for (const n of contractComponents) {
    if (!names.includes(n)) add('missing-component', `Contract component ${n} has no catalog entry.`, n);
  }
  for (const n of input.implemented) {
    if (!names.includes(n)) add('orphan-implementation', `Implementation ${n} has no catalog entry.`, n);
  }

  // Each entry is a well-formed A2UI component schema.
  for (const [n, def] of Object.entries(components)) {
    if (!isObj(def)) {
      add('malformed-component', 'Entry is not an object.', n);
      continue;
    }
    const props = def['properties'];
    const required = def['required'];
    if (!isObj(props) || !isObj(props['id']) || !isObj(props['component'])) {
      add('malformed-component', 'Entry must declare id and component properties.', n);
      continue;
    }
    if ((props['component'] as Obj)['const'] !== n) add('malformed-component', `component const must be ${n}.`, n);
    if (!Array.isArray(required) || !required.includes('id') || !required.includes('component')) {
      add('malformed-component', 'id and component must be required.', n);
    }
    for (const p of Object.keys(props)) {
      if (CHILD_PROPS.has(p)) {
        add('child-reference-untyped', `Property ${p} looks like a child reference; it must use ComponentId or ChildList.`, n);
      }
    }
    // View entries must carry the contract's tool binding.
    const meta = def['x-ipfs-mcp'];
    const expected = viewTools[n];
    if (expected) {
      const tools = isObj(meta) && Array.isArray(meta['tools']) ? meta['tools'] : [];
      if (JSON.stringify([...tools].sort()) !== JSON.stringify([...expected].sort())) {
        add('tool-binding-mismatch', `Tools ${JSON.stringify(tools)} differ from the contract's ${JSON.stringify(expected)}.`, n);
      }
    }
  }

  // anyComponent lists exactly the entries.
  const defs = m['$defs'];
  const any = isObj(defs) && isObj(defs['anyComponent']) ? (defs['anyComponent'] as Obj)['oneOf'] : undefined;
  if (!Array.isArray(any)) {
    add('any-component-mismatch', '$defs.anyComponent.oneOf is missing.');
  } else {
    const listed = any.map((x) => (isObj(x) ? String(x['$ref']) : '')).map((r) => r.replace('#/components/', ''));
    for (const n of names) if (!listed.includes(n)) add('any-component-mismatch', `anyComponent does not list ${n}.`, n);
    for (const l of listed) if (!names.includes(l)) add('any-component-mismatch', `anyComponent lists unknown entry ${l}.`, l);
  }

  // Every $ref is local and resolves.
  for (const ref of new Set(collectRefs(m))) {
    if (!ref.startsWith('#')) {
      add('external-ref', `$ref ${ref} points outside the document; a catalog must be self-contained.`);
    } else if (resolvePointer(m, ref) === undefined) {
      add('dangling-ref', `$ref ${ref} does not resolve.`);
    }
  }
  return issues;
}
