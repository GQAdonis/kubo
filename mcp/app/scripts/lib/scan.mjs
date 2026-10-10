// Proves the views can reach no remote origin. Scans the built documents and the source for anything
// that names a remote origin, loads a sub-resource, performs a network request or needs unsafe-eval.
// Used by scripts/scan-bundle.mjs and tests/bundle.test.ts.
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { extname, join, relative } from 'node:path';
import { CSP } from '../csp.mjs';

/**
 * URLs that are identifiers or message text, not fetches. Each entry says why. A new entry is a claim
 * that the string is never requested; keep the list short.
 */
export const URL_ALLOWLIST = [
  { pattern: /^http:\/\/www\.w3\.org\/(2000\/svg|1999\/xhtml|1999\/xlink|1998\/Math\/MathML|XML\/1998\/namespace|2000\/xmlns\/?)$/, reason: 'XML namespace identifiers used by SVG and DOM APIs' },
  { pattern: /^https:\/\/react\.dev\/errors\/?/, reason: 'text inside React production error messages, shown to the developer, never requested' },
  { pattern: /^https?:\/\/json-schema\.org\/(draft\/20(19-09|20-12)|draft-0[47])\/schema#?$/, reason: 'JSON Schema dialect identifiers ($schema) used by the SDK validators, never fetched' },
  { pattern: /^https:\/\/github\.com\/cfworker$/, reason: 'placeholder base URL the SDK JSON Schema validator uses to resolve relative $ref ids, never requested' },
  { pattern: /^https:\/\/github\.com\/GQAdonis\/kubo\/blob\/master\/mcp\/ui\/catalog\/ipfs-mcp\.catalog\.json$/, reason: 'the published A2UI catalogId, an identifier (A2UI spec: not a resolvable resource)' },
  { pattern: /^https:\/\/a2ui\.org\/specification\/v0_9\/common_types\.json$/, reason: 'A2UI schema $id resolved against a bundled copy' },
];

/** Constructs that perform or schedule a request, or compile code at run time. Call syntax only. */
export const FORBIDDEN_CALLS = [
  { re: /\bfetch\s*\(/, what: 'fetch()' },
  { re: /new\s+XMLHttpRequest\b/, what: 'XMLHttpRequest' },
  { re: /new\s+WebSocket\s*\(/, what: 'WebSocket' },
  { re: /new\s+EventSource\s*\(/, what: 'EventSource' },
  { re: /\bsendBeacon\s*\(/, what: 'sendBeacon()' },
  { re: /new\s+(Shared)?Worker\s*\(/, what: 'Worker' },
  { re: /\bimportScripts\s*\(/, what: 'importScripts()' },
  { re: /serviceWorker\s*\.\s*register\b/, what: 'service worker' },
  { re: /new\s+Function\s*\(/, what: 'new Function() (needs unsafe-eval)' },
  { re: /(?<![.\w$])eval\s*\(/, what: 'eval() (needs unsafe-eval)' },
];

const SCHEME_URL = /https?:\/\/[A-Za-z0-9][A-Za-z0-9.-]*\.[A-Za-z]{2,}[^\s"'`)<>\\]*/g;
const REL_URL_ATTR = /\b(?:src|href|action|poster|srcset|data)\s*=\s*["']\s*\/\/[^"']+/gi;
const SUBRESOURCE = [
  { re: /<script\b[^>]*\bsrc\s*=/i, what: '<script src>' },
  { re: /<link\b[^>]*\bhref\s*=/i, what: '<link href>' },
  { re: /<img\b[^>]*\bsrc\s*=\s*["'](?!data:)/i, what: '<img src> that is not data:' },
  { re: /<iframe\b/i, what: '<iframe>' },
  { re: /<base\b/i, what: '<base>' },
  { re: /<form\b[^>]*\baction\s*=/i, what: '<form action>' },
];
/** Checked inside <style> blocks and .css files only; in script text `url(` is a method name. */
const CSS_RESOURCE = [
  { re: /@import\b/, what: 'CSS @import' },
  { re: /url\(\s*['"]?(?!data:|#)[^'")\s]/i, what: 'CSS url() that is not data:' },
];
const STYLE_BLOCK = /<style\b[^>]*>([\s\S]*?)<\/style>/gi;

const TEXT_EXT = new Set(['.html', '.js', '.mjs', '.ts', '.tsx', '.css', '.json']);

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules') continue;
    const p = join(dir, name);
    if (statSync(p).isDirectory()) yield* walk(p);
    else if (TEXT_EXT.has(extname(name))) yield p;
  }
}

/** @typedef {{file: string, line: number, kind: string, text: string}} Finding */

/**
 * @param {string} base repository-relative base for reporting
 * @param {{html: string[], source: string[]}} roots built documents (files) and source directories
 * @returns {Finding[]}
 */
export function scan(base, roots) {
  const findings = [];
  const add = (file, line, kind, text) => findings.push({ file: relative(base, file), line, kind, text: text.slice(0, 140) });

  const check = (file, text, { documents }) => {
    const lines = text.split('\n');
    lines.forEach((line, i) => {
      for (const m of line.matchAll(SCHEME_URL)) {
        const url = m[0].replace(/[.,;:]+$/, '');
        if (!URL_ALLOWLIST.some((a) => a.pattern.test(url))) add(file, i + 1, 'url', url);
      }
      for (const m of line.matchAll(REL_URL_ATTR)) add(file, i + 1, 'protocol-relative-url', m[0]);
      for (const { re, what } of FORBIDDEN_CALLS) if (re.test(line.replace(/^\s*\/\/.*$/, ''))) add(file, i + 1, 'forbidden-call', what);
    });
    if (documents) {
      for (const { re, what } of SUBRESOURCE) if (re.test(text)) add(file, 1, 'subresource', what);
    }
    const css = extname(file) === '.css' ? [text] : documents ? [...text.matchAll(STYLE_BLOCK)].map((m) => m[1] ?? '') : [];
    for (const block of css) for (const { re, what } of CSS_RESOURCE) if (re.test(block)) add(file, 1, 'subresource', what);
  };

  for (const file of roots.html) check(file, readFileSync(file, 'utf8'), { documents: true });
  for (const dir of roots.source) for (const file of walk(dir)) check(file, readFileSync(file, 'utf8'), { documents: false });
  return findings;
}

/** The CSP meta tag of a built document, or undefined. */
export function cspOf(html) {
  const m = html.match(/<meta\s+http-equiv="Content-Security-Policy"\s+content="([^"]*)"/i);
  return m?.[1];
}

/** Problems with a document's CSP: absent, different from the build constant, or naming an origin. */
export function cspProblems(html) {
  const csp = cspOf(html);
  if (csp === undefined) return ['no Content-Security-Policy meta tag'];
  const problems = [];
  if (csp !== CSP) problems.push('CSP differs from scripts/csp.mjs');
  if (/https?:|wss?:|\/\/|\*/.test(csp)) problems.push('CSP names an origin or a wildcard');
  if (!/default-src 'none'/.test(csp)) problems.push("CSP does not start from default-src 'none'");
  if (!/connect-src 'none'/.test(csp)) problems.push("CSP does not set connect-src 'none'");
  if (/font-src/.test(csp)) problems.push('CSP has a font-src; fonts are registered from embedded bytes');
  if (/unsafe-eval/.test(csp)) problems.push('CSP allows unsafe-eval');
  return problems;
}
