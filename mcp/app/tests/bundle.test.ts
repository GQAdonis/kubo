import { createHash } from 'node:crypto';
import { existsSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { CSP } from '../scripts/csp.mjs';
import { FORBIDDEN_CALLS, URL_ALLOWLIST, cspProblems, scan } from '../scripts/lib/scan.mjs';
import { MCP_APP_MIME_TYPE, VIEWS, VIEW_IDS } from '../src/view-specs';

const root = resolve(import.meta.dirname, '..');
const dist = join(root, 'dist');
const read = (f: string): string => readFileSync(join(dist, f), 'utf8');

describe('built bundles (run `pnpm build` first)', () => {
  it('has exactly one self-contained document per view plus the manifest', () => {
    expect(existsSync(dist)).toBe(true);
    expect(readdirSync(dist).sort()).toEqual([...VIEW_IDS.map((v) => VIEWS[v].file), 'manifest.json'].sort());
  });

  it.each(VIEW_IDS)('%s is a complete HTML document with its view id and the no-origin CSP', (id) => {
    const html = read(VIEWS[id].file);
    expect(html.startsWith('<!doctype html>')).toBe(true);
    expect(html).toContain(`data-view="${id}"`);
    expect(html).toContain(`<title>${VIEWS[id].title}</title>`);
    expect(cspProblems(html)).toEqual([]);
  });

  it('allows no origin, no network and no eval in the CSP', () => {
    expect(CSP).toContain("default-src 'none'");
    expect(CSP).toContain("connect-src 'none'");
    expect(CSP).not.toMatch(/https?:|wss?:|\*|font-src|unsafe-eval/);
  });

  it.each(VIEW_IDS)('%s references no external script, stylesheet, image or frame', (id) => {
    const html = read(VIEWS[id].file);
    expect(html).not.toMatch(/<script\b[^>]*\bsrc=/i);
    expect(html).not.toMatch(/<link\b[^>]*\bhref=/i);
    expect(html).not.toMatch(/<iframe\b/i);
  });

  it('finds no external URL, network call or eval in the documents or the source', () => {
    const html = VIEW_IDS.map((v) => join(dist, VIEWS[v].file));
    expect(scan(root, { html, source: [join(root, 'src')] })).toEqual([]);
  });

  it('embeds the brand fonts as bytes (no font URL) and registers them through FontFace', () => {
    const html = read('pins.html');
    expect(html).toContain('FontFace');
    expect(html).toMatch(/data:font\/woff2;base64,/);
  });

  it('lists every URL allowlist entry with a reason, and keeps the list short', () => {
    expect(URL_ALLOWLIST.length).toBeLessThanOrEqual(8);
    for (const a of URL_ALLOWLIST) expect(a.reason.length).toBeGreaterThan(10);
    expect(FORBIDDEN_CALLS.map((c) => c.what)).toContain('fetch()');
  });

  it('catches a deliberate violation (the scanner is not vacuous)', () => {
    const dir = mkdtempSync(join(tmpdir(), 'scan-'));
    try {
      const doc = join(dir, 'bad.html');
      writeFileSync(doc, '<!doctype html><script type="module">fetch("https://evil.example.com/x"); new Function("1")</script><link href="//cdn.example.net/a.css">');
      const kinds = scan(dir, { html: [doc], source: [] }).map((f) => f.kind);
      expect(kinds).toEqual(expect.arrayContaining(['url', 'forbidden-call', 'subresource']));
      expect(cspProblems('<meta http-equiv="Content-Security-Policy" content="default-src *">')).not.toEqual([]);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});

describe('manifest', () => {
  const manifest = JSON.parse(read('manifest.json')) as {
    schema: number;
    mimeType: string;
    csp: string;
    resourceMeta: { ui: { prefersBorder: boolean } };
    views: { id: string; uri: string; file: string; bytes: number; sha256: string; tools: { name: string; confirmation: string }[] }[];
  };

  it('describes each view with its URI, MIME type, size and digest', () => {
    expect(manifest.mimeType).toBe(MCP_APP_MIME_TYPE);
    expect(manifest.csp).toBe(CSP);
    expect(manifest.resourceMeta).toEqual({ ui: { prefersBorder: false } });
    expect(manifest.views.map((v) => v.id)).toEqual([...VIEW_IDS]);
    for (const v of manifest.views) {
      const bytes = readFileSync(join(dist, v.file));
      expect(v.uri).toBe(`ui://ipfs-mcp/${v.id}`);
      expect(v.bytes).toBe(bytes.length);
      expect(v.sha256).toBe(createHash('sha256').update(bytes).digest('hex'));
      expect(v.tools.length).toBeGreaterThan(0);
    }
  });

  it('links every tool that has an appView and no tool that has none', () => {
    const linked = manifest.views.flatMap((v) => v.tools.map((t) => t.name));
    expect(new Set(linked).size).toBe(linked.length);
    for (const n of ['ipns_publish', 'key_list', 'key_rm']) expect(linked).not.toContain(n);
    expect(linked).toContain('pin_ls');
    expect(linked).toContain('repo_gc');
  });
});
