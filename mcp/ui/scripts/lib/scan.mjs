// Scans source and build output for anything that could reach the network: external URLs, and the
// browser APIs that fetch. MCP App views run under a deny-by-default CSP and call server tools; nothing
// in this package may load a resource. Used by scripts/scan-external-urls.mjs and tests/scan.test.ts.
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { extname, join, relative } from 'node:path';

/**
 * URLs that are identifiers, not fetches. Each entry says why. Keep this list short: a new entry is
 * a claim that the string is never requested.
 */
export const URL_ALLOWLIST = [
  { pattern: /^https:\/\/json-schema\.org\/draft\/2020-12\/schema$/, reason: 'JSON Schema dialect identifier ($schema), never fetched' },
  { pattern: /^https:\/\/github\.com\/GQAdonis\/kubo\/blob\/master\/mcp\/ui\/catalog\/ipfs-mcp\.catalog\.json$/, reason: 'the published A2UI catalogId and $id; an identifier, never fetched (A2UI spec: not a resolvable resource)' },
  { pattern: /^http:\/\/www\.w3\.org\/(2000\/svg|1999\/xhtml|1999\/xlink|1998\/Math\/MathML|XML\/1998\/namespace|2000\/xmlns\/)$/, reason: 'XML namespace identifiers used by SVG and DOM APIs' },
  { pattern: /^https:\/\/a2ui\.org\/specification\/v0_9\/common_types\.json$/, reason: 'A2UI schema $id used in $ref, resolved by the validator against its bundled copy' },
];

/** Browser APIs that perform or schedule a network request. */
export const NETWORK_APIS = [
  /\bfetch\s*\(/,
  /\bXMLHttpRequest\b/,
  /\bWebSocket\b/,
  /\bEventSource\b/,
  /\bsendBeacon\b/,
  /\bnew\s+Image\s*\(/,
  /\bimportScripts\s*\(/,
  /\bserviceWorker\b/,
  /\bnew\s+Worker\s*\(/,
];

const URL_RE = /(?:https?:)?\/\/[A-Za-z0-9][A-Za-z0-9.-]*\.[A-Za-z]{2,}[^\s"'`)<>\\]*/g;
const CSS_URL_RE = /url\(\s*(['"]?)(?!data:)([^'")]+)\1\s*\)|@import\s/g;
const TEXT_EXT = new Set(['.js', '.mjs', '.cjs', '.ts', '.tsx', '.css', '.json', '.html']);

/**
 * @typedef {{file: string, line: number, kind: 'url'|'css-url'|'network-api', text: string}} Finding
 */

function* walk(dir, skip) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (skip.some((s) => p.includes(s))) continue;
    const st = statSync(p);
    if (st.isDirectory()) yield* walk(p, skip);
    else if (TEXT_EXT.has(extname(name)) && !name.endsWith('.d.ts') && !name.endsWith('.map')) yield p;
  }
}

/**
 * @param {string[]} roots files or directories
 * @param {{base: string, allow?: typeof URL_ALLOWLIST, skip?: string[], checkApis?: boolean}} opts
 * @returns {Finding[]}
 */
export function scan(roots, opts) {
  const allow = opts.allow ?? URL_ALLOWLIST;
  const skip = opts.skip ?? ['node_modules', '/tests/', '/scripts/', '/tools/'];
  const findings = [];
  for (const root of roots) {
    const st = statSync(root);
    const files = st.isDirectory() ? [...walk(root, skip)] : [root];
    for (const file of files) {
      const text = readFileSync(file, 'utf8');
      const rel = relative(opts.base, file);
      const lines = text.split('\n');
      lines.forEach((line, i) => {
        for (const m of line.matchAll(URL_RE)) {
          // `//` also matches the second slash of a comment marker followed by a path; require a dotted host
          const url = m[0].replace(/[.,;:]+$/, '');
          if (allow.some((a) => a.pattern.test(url))) continue;
          findings.push({ file: rel, line: i + 1, kind: 'url', text: url });
        }
        if (extname(file) === '.css') {
          for (const m of line.matchAll(CSS_URL_RE)) findings.push({ file: rel, line: i + 1, kind: 'css-url', text: m[0] });
        }
        if (opts.checkApis !== false && /\.(m?js|cjs|tsx?)$/.test(file)) {
          const code = line.replace(/\/\/.*$/, '');
          for (const re of NETWORK_APIS) if (re.test(code)) findings.push({ file: rel, line: i + 1, kind: 'network-api', text: line.trim().slice(0, 120) });
        }
      });
    }
  }
  return findings;
}
