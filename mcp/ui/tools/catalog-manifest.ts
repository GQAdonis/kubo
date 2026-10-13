// Build-time only (Node): produces the published A2UI v0.9 catalog document. It starts from the
// document @a2ui/web_core generates from the component APIs (so the manifest can never list a
// component the code does not define), then binds the contract's output schemas to each view.
// Loaded by scripts/gen-catalog.mjs through Vite's SSR loader and by the tests.
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { dirname, join, posix, resolve } from 'node:path';
import { createIpfsCatalog, A2UI_CATALOG_SHORT_NAME } from '../src/a2ui/catalog';
import { VIEW_TOOLS } from '../src/contract/generated/bindings';
import { CONTRACT_VERSION } from '../src/contract/generated/tool-meta';

const UI_ROOT = resolve(import.meta.dirname, '..');
const CONTRACT_ROOT = resolve(UI_ROOT, '..', 'contract');

type Json = null | boolean | number | string | Json[] | { [k: string]: Json };
type Obj = { [k: string]: Json };

const readContract = (rel: string): Obj => JSON.parse(readFileSync(join(CONTRACT_ROOT, rel), 'utf8')) as Obj;
const isObj = (v: unknown): v is Obj => typeof v === 'object' && v !== null && !Array.isArray(v);

const NAMESPACE: Record<string, string> = {
  'schemas/common.json': 'ipfs',
  'confirmation/confirmation.schema.json': 'ipfs_confirmation',
  'fixtures/agui.schema.json': 'ipfs_agui',
};

/** Rewrites contract schemas into catalog-local ones: every cross-file $ref becomes #/$defs/<ns>_<Name>. */
class Bundler {
  readonly defs: Record<string, Json> = {};

  /** Bundle a whole file (a tool output schema) and return it without $schema/title noise. */
  schemaOf(file: string): Obj {
    const root = readContract(file);
    return this.rewrite(root, file) as Obj;
  }

  /** Bundle one named $def of a file and register it; returns a local $ref to it. */
  defRef(file: string, name: string): Obj {
    const key = `${NAMESPACE[file] ?? 'ipfs'}_${name}`;
    if (!(key in this.defs)) {
      this.defs[key] = {}; // placeholder breaks cycles
      const src = (readContract(file)['$defs'] as Obj)[name];
      if (!src) throw new Error(`${file} has no $defs/${name}`);
      this.defs[key] = this.rewrite(src, file);
    }
    return { $ref: `#/$defs/${key}` };
  }

  private rewrite(node: Json, file: string): Json {
    if (Array.isArray(node)) return node.map((n) => this.rewrite(n, file));
    if (!isObj(node)) return node;
    const out: Obj = {};
    for (const [k, v] of Object.entries(node)) {
      if (k === '$schema' || k === '$defs') continue;
      if (k === '$ref' && typeof v === 'string') {
        const [filePart, pointer] = v.split('#') as [string, string | undefined];
        const target = filePart === '' ? file : posix.normalize(posix.join(dirname(file), filePart));
        const m = /^\/\$defs\/(\w+)$/.exec(pointer ?? '');
        if (!m) throw new Error(`unsupported $ref ${v} in ${file}`);
        Object.assign(out, this.defRef(target, m[1] as string));
        continue;
      }
      out[k] = this.rewrite(v, file);
    }
    return out;
  }
}

const dataBindingRef = { $ref: '#/$defs/DataBinding' };

function oneOfLiteralOrBinding(literal: Json, description: string, combinator: 'oneOf' | 'anyOf' = 'oneOf'): Obj {
  return { description, [combinator]: [literal, dataBindingRef] };
}

export function buildCatalogManifest(): Obj {
  const catalog = createIpfsCatalog();
  const doc = JSON.parse(JSON.stringify(catalog.catalogSchema)) as Obj;
  const components = doc['components'] as Obj;
  const bundler = new Bundler();

  doc['$id'] = catalog.id;
  const catalogSource = readFileSync(join(CONTRACT_ROOT, 'derivations/a2ui-components.json'), 'utf8');
  const digest = createHash('sha256')
    .update(catalogSource)
    .update(readFileSync(join(CONTRACT_ROOT, 'catalog.json'), 'utf8'))
    .digest('hex')
    .slice(0, 16);

  const toolsSchemaKey = (tool: string) => `schemas/tools/${tool}.output.json`;

  // View components: `output` is the bound tool's contract output, a literal or a data binding.
  for (const [name, tools] of Object.entries(VIEW_TOOLS)) {
    const entry = components[name] as Obj | undefined;
    if (!entry || !tools) continue;
    const props = entry['properties'] as Obj;
    props['output'] = oneOfLiteralOrBinding(
      { type: 'object', description: 'The structured output of the bound tool; the schema is selected by `tool`.' },
      'The tool output as a literal, or a data model path that holds it. When literal, it must match the contract output schema of `tool` (see allOf).',
      'anyOf', // a data binding is also an object; the contract schemas in allOf do the real checking
    );
    entry['allOf'] = tools.map((tool) => {
      const outputSchema = bundler.schemaOf(toolsSchemaKey(tool));
      return {
        if: { properties: { tool: { const: tool } }, required: ['tool'] },
        then: { properties: { output: { oneOf: [outputSchema, dataBindingRef] } } },
      } as Obj;
    });
    entry['x-ipfs-mcp'] = { kind: 'view', tools: [...tools] };
  }

  // Components bound to contract schemas that are not tool outputs.
  const bind = (name: string, prop: string, literal: Json, description: string) => {
    const entry = components[name] as Obj;
    (entry['properties'] as Obj)[prop] = oneOfLiteralOrBinding(literal, description);
  };
  bind('ConfirmationDialog', 'confirmation', bundler.defRef('confirmation/confirmation.schema.json', 'ConfirmationRequired'), 'A ConfirmationRequired payload (ToolError.error.confirmation), literal or data model path.');
  bind('ApprovalCard', 'interrupt', bundler.defRef('fixtures/agui.schema.json', 'Interrupt'), 'The AG-UI interrupt from RUN_FINISHED, literal or data model path.');
  bind('MessageStream', 'messages', { type: 'array', items: bundler.defRef('fixtures/agui.schema.json', 'Message') }, 'AG-UI messages, literal or data model path.');
  const sealed = (name: string, kind: string, extra: Obj = {}) => {
    (components[name] as Obj)['x-ipfs-mcp'] = { kind, ...extra };
  };
  sealed('ConfirmationDialog', 'view', { schema: 'confirmation/confirmation.schema.json#/$defs/ConfirmationRequired' });
  sealed('MessageStream', 'chat');
  sealed('ToolCallCard', 'chat');
  sealed('ApprovalCard', 'chat', { schema: 'fixtures/agui.schema.json#/$defs/Interrupt' });
  sealed('EgressBadge', 'chat');

  // The shared FunctionCall definition refers to "catalog.json#/$defs/anyFunction", the A2UI
  // convention for "this catalog". Our document is named ipfs-mcp.catalog.json, so make the reference
  // local, and declare that the catalog defines no functions (nothing validates as anyFunction).
  const defs = doc['$defs'] as Obj;
  selfReference(defs);
  selfReference(components);
  if (!('anyFunction' in defs)) defs['anyFunction'] = { not: {}, description: 'This catalog defines no functions.' };

  // Contract $defs the bindings pulled in. Sorted so the file is stable.
  for (const k of Object.keys(bundler.defs).sort()) defs[k] = bundler.defs[k] as Json;

  const ordered: Obj = {
    $schema: doc['$schema'] as Json,
    $id: doc['$id'] as Json,
    catalogId: doc['catalogId'] as Json,
    title: 'IPFS MCP A2UI catalog',
    description:
      'IPFS components for an agent that operates the person\'s own Kubo node. Generated from mcp/ui; do not edit. Regenerate with `pnpm gen:catalog`.',
    'x-ipfs-mcp': {
      shortName: A2UI_CATALOG_SHORT_NAME,
      protocolVersion: 'v0.9',
      contractVersion: CONTRACT_VERSION,
      contractDigest: digest,
    },
    ...(doc['instructions'] !== undefined && { instructions: doc['instructions'] }),
    components: sortKeys(components),
    ...(doc['functions'] !== undefined && { functions: doc['functions'] }),
    $defs: sortKeys(defs),
  };
  return ordered;
}

function selfReference(node: Json): void {
  if (Array.isArray(node)) node.forEach(selfReference);
  else if (isObj(node)) {
    for (const [k, v] of Object.entries(node)) {
      if (k === '$ref' && typeof v === 'string' && v.startsWith('catalog.json#/')) node[k] = v.slice('catalog.json'.length);
      else selfReference(v);
    }
  }
}

function sortKeys(o: Obj): Obj {
  return Object.fromEntries(Object.keys(o).sort().map((k) => [k, o[k] as Json]));
}

export function manifestText(): string {
  return `${JSON.stringify(buildCatalogManifest(), null, 2)}\n`;
}
