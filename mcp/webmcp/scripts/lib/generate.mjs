// Generates src/generated/tools.ts from mcp/contract. Pure function of the contract
// files: generate() returns { relativePath: content } and never writes.
//
// What is derived, never authored here:
//  - names, titles, descriptions, groups and confirmation classes (catalog.json)
//  - input schemas (schemas/tools/*.input.json, $refs inlined)
//  - WebMCP annotations (derivations/webmcp.json rules)
//  - which tools are exposed at all (the "exposed" rule: destructive tools are not)
//
// What is decided here, because the contract cannot express it: tools that have no
// browser implementation. They are listed in the generated file with their reason.

import { readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
export const PKG_ROOT = resolve(here, '..', '..');
export const CONTRACT_ROOT = resolve(PKG_ROOT, '..', 'contract');

/** Groups that need server-side state the browser does not have. */
export const SERVER_ONLY_GROUPS = {
  index: 'The search index lives in the ipfs mcp server process; the Web UI has no embedding index.',
};

/** Single tools without a kubo-rpc-client 6.1.0 implementation or a browser meaning. */
export const NO_BROWSER_IMPLEMENTATION = {
  ipfs_node_status: 'Reports the MCP server configuration (transport, enabled groups); there is no MCP server in the page.',
  car_import: 'kubo-rpc-client 6.1.0 dag.import forwards only pin-roots, and Kubo announces CAR roots by default; the page cannot turn that off, so the tool could announce content the person did not ask to announce.',
  pin_verify: 'kubo-rpc-client 6.1.0 has no pin.verify; the page must not call the RPC API around the client.',
  dag_stat: 'kubo-rpc-client 6.1.0 has no dag.stat; the page must not call the RPC API around the client.',
};

const readJson = (rel) => JSON.parse(readFileSync(join(CONTRACT_ROOT, rel), 'utf8'));

/** Resolve a field reference from the rules grammar against a catalog tool and its group. */
function field(tool, group, ref) {
  switch (ref) {
    case 'annotations.readOnlyHint':
      return tool.annotations.readOnlyHint;
    case 'confirmation':
      return tool.confirmation;
    case 'returnsUntrustedContent':
      return tool.returnsUntrustedContent;
    case 'contentEgress':
      return tool.contentEgress;
    case 'group.diagnostic':
      return group.diagnostic;
    case 'peerSupplied':
      return Array.isArray(tool.peerSuppliedFields) && tool.peerSuppliedFields.length > 0;
    default:
      throw new Error(`webmcp.json: unknown field ${ref}`);
  }
}

/** Confirmation classes the contract defines. A tool with any other class is an error, never a default. */
const KNOWN_CLASSES = ['none', 'write', 'destructive'];

/**
 * Interpret one rule of the closed grammar:
 * copy:<field> | not-equal:<field>:<value> | in:<field>:<value>,<value>... | any:<field>,<field>...
 * An `in:` rule on `confirmation` is an allow-list: an unknown class on either side throws.
 */
export function evalRule(rule, tool, group) {
  const parts = rule.split(':');
  if (parts[0] === 'copy' && parts.length === 2) return Boolean(field(tool, group, parts[1]));
  if (parts[0] === 'not-equal' && parts.length === 3) return field(tool, group, parts[1]) !== parts[2];
  // any:<field>,<field>... is true when any listed boolean field is true.
  if (parts[0] === 'any' && parts.length === 2) {
    return parts[1].split(',').some((f) => Boolean(field(tool, group, f)));
  }
  if (parts[0] === 'in' && parts.length === 3) {
    const allowed = parts[2].split(',');
    const actual = field(tool, group, parts[1]);
    if (parts[1] === 'confirmation') {
      for (const c of [...allowed, actual]) {
        if (!KNOWN_CLASSES.includes(c)) throw new Error(`webmcp.json: unknown confirmation class ${JSON.stringify(c)} (tool ${tool.name})`);
      }
    }
    return allowed.includes(String(actual));
  }
  throw new Error(`webmcp.json: unsupported rule ${rule}`);
}

/** Inline every $ref. Refs are relative to the file that contains them. */
function inline(node, relFile) {
  if (Array.isArray(node)) return node.map((n) => inline(n, relFile));
  if (!node || typeof node !== 'object') return node;
  if (typeof node.$ref === 'string') {
    const [filePart, pointer] = node.$ref.split('#');
    const targetFile = filePart ? join(dirname(relFile), filePart) : relFile;
    let target = readJson(targetFile);
    for (const seg of (pointer ?? '').split('/').filter(Boolean)) target = target[seg];
    const { $ref: _ignored, ...siblings } = node;
    // Sibling keywords (for example a more specific description) override the target's.
    return { ...inline(target, targetFile), ...inline(siblings, relFile) };
  }
  const out = {};
  for (const [k, v] of Object.entries(node)) out[k] = inline(v, relFile);
  return out;
}

/**
 * The page replaces the two-step confirm_token with an in-page confirmation, so the
 * property is not offered to the agent.
 */
function stripConfirmToken(schema) {
  const props = { ...schema.properties };
  delete props.confirm_token;
  const out = { ...schema, properties: props };
  if (Array.isArray(schema.required)) out.required = schema.required.filter((r) => r !== 'confirm_token');
  if (out.required && out.required.length === 0) delete out.required;
  return out;
}

function inputSchemaFor(tool) {
  const raw = readJson(tool.inputSchema);
  const { $schema: _s, ...rest } = inline(raw, tool.inputSchema);
  return stripConfirmToken(rest);
}

export function buildDefinitions() {
  const catalog = readJson('catalog.json');
  const rules = readJson('derivations/webmcp.json').rules;
  const groups = new Map(catalog.groups.map((g) => [g.name, g]));
  const tools = [];
  const excluded = [];
  for (const tool of catalog.tools) {
    const group = groups.get(tool.group);
    if (!group) throw new Error(`tool ${tool.name}: unknown group ${tool.group}`);
    if (!evalRule(rules.exposed, tool, group)) {
      excluded.push({ name: tool.name, reason: 'destructive' });
      continue;
    }
    if (SERVER_ONLY_GROUPS[tool.group]) {
      excluded.push({ name: tool.name, reason: SERVER_ONLY_GROUPS[tool.group] });
      continue;
    }
    if (NO_BROWSER_IMPLEMENTATION[tool.name]) {
      excluded.push({ name: tool.name, reason: NO_BROWSER_IMPLEMENTATION[tool.name] });
      continue;
    }
    tools.push({
      name: tool.name,
      group: tool.group,
      title: tool.title,
      description: tool.description,
      confirmation: tool.confirmation,
      annotations: {
        readOnlyHint: evalRule(rules.readOnlyHint, tool, group),
        untrustedContentHint: evalRule(rules.untrustedContentHint, tool, group),
        consequentialHint: evalRule(rules.consequentialHint, tool, group),
        debugging: evalRule(rules.debugging, tool, group),
      },
      inputSchema: inputSchemaFor(tool),
    });
  }
  return { contractVersion: catalog.contractVersion, tools, excluded };
}

const HEADER =
  '// GENERATED by scripts/gen-tools.mjs from mcp/contract (catalog.json, derivations/webmcp.json, schemas/tools). Do not edit.\n' +
  '// Regenerate with `pnpm gen`; the drift test fails when this file differs from the contract.\n';

export function generate() {
  const { contractVersion, tools, excluded } = buildDefinitions();
  let out = HEADER;
  out += "import type { GeneratedTool, ExcludedTool } from '../types.js';\n\n";
  out += `export const CONTRACT_VERSION = ${JSON.stringify(contractVersion)};\n\n`;
  out += `export const TOOL_DEFINITIONS: readonly GeneratedTool[] = ${JSON.stringify(tools, null, 2)};\n\n`;
  out += `export const EXCLUDED_TOOLS: readonly ExcludedTool[] = ${JSON.stringify(excluded, null, 2)};\n`;
  return { 'src/generated/tools.ts': out };
}
