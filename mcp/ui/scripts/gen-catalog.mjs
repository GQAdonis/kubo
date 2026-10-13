// Usage: node scripts/gen-catalog.mjs [--check]
// Writes (or checks) catalog/ipfs-mcp.catalog.json, the published A2UI v0.9 catalog document.
// The manifest is built from the TypeScript sources, so they are loaded through Vite's SSR loader.
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { createServer } from 'vite';

const root = resolve(import.meta.dirname, '..');
const target = resolve(root, 'catalog', 'ipfs-mcp.catalog.json');
const check = process.argv.includes('--check');

// zod-to-json-schema warns "Recursive reference detected ... Defaulting to any" while it walks the
// shared, self-referential FunctionCall type before @a2ui/web_core swaps it for a $ref. The warning
// is expected and the output is correct (the validator asserts every $ref resolves), so it is
// counted instead of printed.
let recursive = 0;
const warn = console.warn;
console.warn = (...args) => {
  if (typeof args[0] === 'string' && args[0].startsWith('Recursive reference detected')) recursive++;
  else warn(...args);
};

const server = await createServer({
  root,
  logLevel: 'error',
  appType: 'custom',
  server: { middlewareMode: true, hmr: false, watch: null },
  optimizeDeps: { noDiscovery: true },
});
try {
  const mod = await server.ssrLoadModule('/tools/catalog-manifest.ts');
  const text = mod.manifestText();
  if (check) {
    const current = existsSync(target) ? readFileSync(target, 'utf8') : null;
    if (current !== text) {
      console.error('catalog/ipfs-mcp.catalog.json differs from the generated manifest. Run: pnpm gen:catalog');
      process.exitCode = 1;
    }
  } else {
    mkdirSync(dirname(target), { recursive: true });
    writeFileSync(target, text);
    console.log(`wrote ${target}`);
  }
} finally {
  await server.close();
}
