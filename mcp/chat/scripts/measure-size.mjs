// Task 1.4: record the bundle size. Measures dist/ (without source maps) as raw, gzip -9 and brotli, and
// separates what a host page pays when the island is loaded but closed (the entry) from what it pays on
// first open (the lazy chunks). Writes evidence/bundle-size.json and prints a table.
import { readFileSync, readdirSync, statSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { brotliCompressSync, constants, gzipSync } from 'node:zlib';

const root = resolve(import.meta.dirname, '..');
const dist = resolve(root, 'dist');

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) yield* walk(p);
    else yield p;
  }
}

const files = [...walk(dist)]
  .filter((p) => /\.(js|css)$/.test(p))
  .map((p) => {
    const buf = readFileSync(p);
    return {
      file: relative(root, p),
      raw: buf.length,
      gzip: gzipSync(buf, { level: 9 }).length,
      brotli: brotliCompressSync(buf, { params: { [constants.BROTLI_PARAM_QUALITY]: 11 } }).length,
    };
  })
  .sort((a, b) => b.raw - a.raw);

// What a page pays while the island is loaded but closed: the entry and whatever it imports statically.
// (Dynamic import() targets are the lazy chunks.)
const staticImports = (file, seen = new Set()) => {
  if (seen.has(file)) return seen;
  seen.add(file);
  const text = readFileSync(resolve(root, file), 'utf8');
  for (const m of text.matchAll(/^\s*import\s[^;]*?from\s*["']\.\/([^"']+)["']/gm)) staticImports(relative(root, resolve(dirname(resolve(root, file)), m[1])), seen);
  return seen;
};
const closedSet = staticImports('dist/ipfs-agent-chat.js');
const entry = files.filter((f) => closedSet.has(f.file));
const lazy = files.filter((f) => !closedSet.has(f.file));
const sum = (list, k) => list.reduce((n, f) => n + f[k], 0);
const totals = (list) => ({ raw: sum(list, 'raw'), gzip: sum(list, 'gzip'), brotli: sum(list, 'brotli') });

const result = {
  measured: 'dist/ (JavaScript and CSS only; source maps excluded)',
  closed: { description: 'paid when the host page loads the module and never opens the sidebar', ...totals(entry) },
  firstOpen: { description: 'paid once, on first open: React 19, the A2UI renderer and catalog, the library, zod, the bundled brand fonts', ...totals(lazy) },
  total: totals(files),
  files,
  budgets: { note: 'web/performance.md: app page JS under 300 kB gzip. The lazy total includes the brand fonts as base64 woff2.' },
};

const out = resolve(root, 'evidence/bundle-size.json');
mkdirSync(dirname(out), { recursive: true });
writeFileSync(out, JSON.stringify(result, null, 2) + '\n');

const kb = (n) => (n / 1024).toFixed(1).padStart(8) + ' kB';
console.log('file'.padEnd(46), 'raw'.padStart(11), 'gzip'.padStart(11), 'brotli'.padStart(11));
for (const f of files) console.log(f.file.padEnd(46), kb(f.raw), kb(f.gzip), kb(f.brotli));
console.log('closed (entry)'.padEnd(46), kb(result.closed.raw), kb(result.closed.gzip), kb(result.closed.brotli));
console.log('first open (lazy)'.padEnd(46), kb(result.firstOpen.raw), kb(result.firstOpen.gzip), kb(result.firstOpen.brotli));
console.log('total'.padEnd(46), kb(result.total.raw), kb(result.total.gzip), kb(result.total.brotli));
