import { existsSync, mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, describe, expect, it } from 'vitest';
import { scan } from '../scripts/lib/scan.mjs';
import { UI_ROOT } from './helpers/contract';

const tmp = mkdtempSync(join(tmpdir(), 'ipfs-ui-scan-'));
afterAll(() => rmSync(tmp, { recursive: true, force: true }));

const plant = (name: string, body: string) => {
  mkdirSync(tmp, { recursive: true });
  const p = join(tmp, name);
  writeFileSync(p, body);
  return p;
};

describe('no-remote-assets scan', () => {
  it('finds no external URL, CSS url() or network API in the source and the catalog', () => {
    const findings = scan([join(UI_ROOT, 'src'), join(UI_ROOT, 'catalog')], { base: UI_ROOT });
    expect(findings).toEqual([]);
  });

  it('finds none in the build output either, when it has been built', () => {
    const dist = join(UI_ROOT, 'dist');
    if (!existsSync(dist)) return; // pnpm check builds first; a bare `pnpm test` may not have
    expect(scan([dist], { base: UI_ROOT })).toEqual([]);
  });

  it('fails on a planted CDN URL, remote font, protocol-relative URL and @import', () => {
    expect(scan([plant('a.js', 'const u = "https://cdn.example.com/lib.js";')], { base: tmp }).map((f) => f.kind)).toEqual(['url']);
    expect(scan([plant('b.css', '@import url("https://fonts.googleapis.com/css2?family=Syne");')], { base: tmp }).map((f) => f.kind)).toContain('css-url');
    expect(scan([plant('c.html', '<script src="//cdn.example.com/x.js"></script>')], { base: tmp })).toHaveLength(1);
    expect(scan([plant('d.css', '@font-face{src:url("fonts/x.woff2")}')], { base: tmp }).map((f) => f.kind)).toEqual(['css-url']);
  });

  it('fails on network APIs in code', () => {
    for (const code of ['await fetch("/api")', 'new WebSocket(u)', 'new XMLHttpRequest()', 'navigator.sendBeacon(u)', 'new Image()', 'navigator.serviceWorker.register(s)']) {
      expect(scan([plant('e.ts', code)], { base: tmp }).map((f) => f.kind), code).toEqual(['network-api']);
    }
  });

  it('allows only the identifiers it lists, and lets data: URIs through', () => {
    expect(scan([plant('f.json', '{"$schema":"https://json-schema.org/draft/2020-12/schema"}')], { base: tmp })).toEqual([]);
    expect(scan([plant('g.js', 'const f = "data:font/woff2;base64,AAAA"; const ns = "http://www.w3.org/2000/svg";')], { base: tmp })).toEqual([]);
    expect(scan([plant('h.json', '{"$schema":"https://json-schema.org/draft/2019-09/schema"}')], { base: tmp })).toHaveLength(1);
  });
});
