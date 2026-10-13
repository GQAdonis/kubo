// Scans source and build output for anything that could reach the network or run generated code.
// Falsifier G11-4 (docs/mcp/0005): the island bundle must load no remote resource and need no
// `unsafe-eval`. Used by scripts/scan-external-urls.mjs and tests/unit/scan.test.ts.
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { extname, join, relative } from 'node:path';

/**
 * URL-shaped strings that are identifiers or message text, not fetches. Each entry says why. A new entry
 * is a claim that the string is never requested; keep the list short and exact.
 */
export const URL_ALLOWLIST = [
  { pattern: /^http:\/\/www\.w3\.org\/(2000\/svg|1999\/xhtml|1999\/xlink|1998\/Math\/MathML|XML\/1998\/namespace|2000\/xmlns\/?)$/, reason: 'XML namespace identifiers used by SVG and DOM APIs (react-dom, lucide-react)' },
  { pattern: /^https:\/\/react\.dev\/errors\/?$/, reason: 'prefix of the minified-error message text react-dom builds; it is shown in an Error message, never requested' },
  { pattern: /^https:\/\/react\.dev\/link\/[a-z-]+$/, reason: 'documentation link text inside react-dom warning messages, never requested' },
  { pattern: /^https?:\/\/json-schema\.org\/draft[-/][0-9-]+\/schema#?$/, reason: 'JSON Schema dialect identifiers ($schema) in zod and a2ui; never fetched' },
  { pattern: /^https:\/\/github\.com\/GQAdonis\/kubo\/blob\/master\/mcp\/ui\/catalog\/ipfs-mcp\.catalog\.json$/, reason: 'the published A2UI catalogId; an identifier, never fetched (A2UI spec: not a resolvable resource)' },
  { pattern: /^https:\/\/a2ui\.org\/specification\/v0_[89]\/.*\.json$/, reason: 'A2UI schema and catalog $id values in @a2ui/web_core; identifiers, resolved against the bundled copy' },
  { pattern: /^https:\/\/github\.com\/(colinhacks\/zod|ipfs\/ipfs-webui)\b.*$/, reason: 'documentation text in error messages' },
  { pattern: /^https:\/\/(zod\.dev|v4\.zod\.dev)\/.*$/, reason: 'documentation text in zod error messages' },
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

/** Code generation: what a CSP without unsafe-eval forbids. */
export const EVAL_APIS = [/\beval\s*\(/, /\bnew\s+Function\b/, /\bFunction\s*\(\s*['"`]/, /\bsetTimeout\s*\(\s*['"`]/, /\bsetInterval\s*\(\s*['"`]/];

// A scheme-qualified URL, or a protocol-relative one that starts after a quote, paren, equals or space
// (so `//i.test(` at the end of a minified regex literal is not mistaken for a host).
/** Code that mentions code generation but is never reached on this island's path. Exact, with the proof. */
export const EVAL_ALLOWLIST = [
  {
    pattern: /return Function\(""\),\s*!0/,
    reason: 'zod 4 allowsEval probe. Not reached: src/island/zod-config.ts sets jitless before any parse, and tests/unit/csp-eval.test.ts counts Function constructor calls during a full chat turn (zero).',
  },
];

const URL_RE = /(?:https?:\/\/|(?<=["'`(=\s])\/\/)[A-Za-z0-9][A-Za-z0-9.-]*\.[A-Za-z]{2,}[^\s"'`()<>\\]*/g;
const CSS_URL_RE = /url\(\s*(['"]?)(?!data:)([^'")]+)\1\s*\)|@import\s/g;
const TEXT_EXT = new Set(['.js', '.mjs', '.cjs', '.ts', '.tsx', '.css', '.json', '.html']);

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
 * @typedef {{file: string, line: number, kind: 'url'|'css-url'|'network-api'|'eval', text: string}} Finding
 * @param {string[]} roots files or directories
 * @param {{base: string, allow?: typeof URL_ALLOWLIST, skip?: string[], networkApis?: boolean | ((file: string) => boolean), evalApis?: boolean}} opts
 * @returns {Finding[]}
 */
export function scan(roots, opts) {
  const allow = opts.allow ?? URL_ALLOWLIST;
  // src/styles/island.css is Tailwind's input: its @import lines are resolved at build time. The compiled
  // result (src/generated) and dist are what ship, and both are scanned.
  const skip = opts.skip ?? ['node_modules', 'src/styles/'];
  const findings = [];
  for (const root of roots) {
    const st = statSync(root);
    const files = st.isDirectory() ? [...walk(root, skip)] : [root];
    for (const file of files) {
      const text = readFileSync(file, 'utf8');
      const rel = relative(opts.base, file);
      const isCode = /\.(m?js|cjs|tsx?)$/.test(file);
      const checkNet = typeof opts.networkApis === 'function' ? opts.networkApis(rel) : opts.networkApis !== false;
      text.split('\n').forEach((line, i) => {
        for (const m of line.matchAll(URL_RE)) {
          const url = m[0].replace(/[.,;:]+$/, '');
          if (allow.some((a) => a.pattern.test(url))) continue;
          findings.push({ file: rel, line: i + 1, kind: 'url', text: url });
        }
        if (extname(file) === '.css') {
          for (const m of line.matchAll(CSS_URL_RE)) findings.push({ file: rel, line: i + 1, kind: 'css-url', text: m[0] });
        }
        if (isCode) {
          const code = line.replace(/^\s*\/\/.*$/, '').replace(/^\s*\*.*$/, '');
          const at = (kind) => findings.push({ file: rel, line: i + 1, kind, text: line.trim().slice(0, 120) });
          if (checkNet && NETWORK_APIS.some((re) => re.test(code))) at('network-api');
          if (opts.evalApis !== false && !EVAL_ALLOWLIST.some((a) => a.pattern.test(code)) && EVAL_APIS.some((re) => re.test(code))) at('eval');
        }
      });
    }
  }
  return findings;
}
