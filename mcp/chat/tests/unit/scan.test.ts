// @vitest-environment node
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { EVAL_ALLOWLIST, URL_ALLOWLIST, scan } from '../../scripts/lib/scan.mjs';

// Falsifier G11-4: the island loads no remote resource and needs no unsafe-eval. This is the static half;
// the Playwright CSP spec proves the same at run time with real requests and securitypolicyviolation events.

const root = resolve(import.meta.dirname, '../..');
const src = resolve(root, 'src');
const dist = resolve(root, 'dist');

describe('external URL and code-generation scan', () => {
  it('finds no URL, import or network call in src, outside the one client module', () => {
    const findings = scan([src], { base: root, networkApis: (rel) => rel !== 'src/agui/client.ts' });
    expect(findings).toEqual([]);
  });

  it('keeps fetch in exactly one module', () => {
    const files = readdirSync(src, { recursive: true, withFileTypes: true })
      .filter((d) => d.isFile() && /\.tsx?$/.test(d.name))
      .map((d) => resolve(d.parentPath, d.name))
      .filter((f) => /\bfetch\s*\(/.test(readFileSync(f, 'utf8').replace(/\/\/.*$/gm, '').replace(/\/\*[\s\S]*?\*\//g, '')));
    expect(files.map((f) => f.slice(root.length + 1))).toEqual(['src/agui/client.ts']);
  });

  it.runIf(existsSync(dist))('finds no URL, css url() or eval in the built bundle beyond the justified identifiers', () => {
    const findings = scan([dist], { base: root, networkApis: false });
    expect(findings).toEqual([]);
  });

  it('allows only identifiers and message text, each with a reason', () => {
    for (const a of [...URL_ALLOWLIST, ...EVAL_ALLOWLIST]) expect(a.reason.length).toBeGreaterThan(20);
    // None of the allowed URL shapes can be a script, stylesheet or font location.
    for (const a of URL_ALLOWLIST) expect(a.pattern.source).not.toMatch(/\.(js|css|woff2?)\b/);
  });

  it('detects what it is meant to detect', () => {
    // A file with one line of each kind the scanner must flag.
    const probe = scan([resolve(import.meta.dirname, '../helpers/scan-probe.txt.js')], { base: root });
    expect(probe.map((f) => f.kind).sort()).toEqual(['eval', 'network-api', 'url']);
  });
});
