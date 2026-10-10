// Usage: node scripts/build.mjs [--out <dir>]
//
// Builds one single-file HTML document per MCP App view into <dir> (default ./dist), plus
// manifest.json. Each document inlines the script, the stylesheet and the brand fonts: the Go server
// embeds the files as-is and serves them as ui://ipfs-mcp/<view> resources.
//
// The output is a pure function of the sources and the lockfile: no timestamps, no absolute paths.
// scripts/check-dist.mjs relies on that to prove the committed dist matches its source.
import react from '@vitejs/plugin-react';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { gzipSync } from 'node:zlib';
import { build } from 'vite';
import { viteSingleFile } from 'vite-plugin-singlefile';
import { MCP_APP_MIME_TYPE, VIEWS, VIEW_IDS } from '../src/view-specs.ts';
import { CSP } from './csp.mjs';

const root = resolve(import.meta.dirname, '..');
const argv = process.argv.slice(2);
const outIdx = argv.indexOf('--out');
const outDir = resolve(root, outIdx >= 0 ? (argv[outIdx + 1] ?? '') : 'dist');
if (outIdx >= 0 && !argv[outIdx + 1]) {
  console.error('--out needs a directory');
  process.exit(2);
}

const uiDist = resolve(root, '../ui/dist/index.js');
if (!existsSync(uiDist)) {
  console.error('mcp/ui/dist is missing. Build the component library first: pnpm build:ui');
  process.exit(2);
}
const { TOOL_META } = await import('@gqadonis/ipfs-mcp-ui');

/** Fills the document template for one view. */
function viewHtml(view) {
  const spec = VIEWS[view];
  return {
    name: 'ipfs-mcp-app-view-html',
    transformIndexHtml: {
      order: 'pre',
      handler: (html) =>
        html
          .replaceAll('%VIEW_ID%', spec.id)
          .replaceAll('%VIEW_TITLE%', spec.title)
          .replaceAll('%CSP%', CSP),
    },
  };
}

async function buildView(view, scratch) {
  const dir = join(scratch, view);
  await build({
    configFile: false,
    root: join(root, 'src'),
    logLevel: 'warn',
    plugins: [react(), viewHtml(view), viteSingleFile()],
    resolve: { dedupe: ['react', 'react-dom', 'zod', 'lucide-react'] },
    define: { 'process.env.NODE_ENV': JSON.stringify('production') },
    build: {
      outDir: dir,
      emptyOutDir: true,
      target: 'es2023',
      minify: true,
      sourcemap: false,
      cssCodeSplit: false,
      reportCompressedSize: false,
      // The preload polyfill calls fetch(); a single-file document has nothing to preload.
      modulePreload: false,
      rollupOptions: { input: join(root, 'src/view.html') },
    },
  });
  return readFileSync(join(dir, 'view.html'));
}

const scratch = mkdtempSync(join(tmpdir(), 'ipfs-mcp-app-'));
try {
  mkdirSync(outDir, { recursive: true });
  for (const f of readdirSync(outDir)) {
    if (f.endsWith('.html') || f === 'manifest.json') rmSync(join(outDir, f));
  }

  const views = [];
  for (const id of VIEW_IDS) {
    const bytes = await buildView(id, scratch);
    const spec = VIEWS[id];
    writeFileSync(join(outDir, spec.file), bytes);
    const tools = Object.values(TOOL_META)
      .filter((t) => t.appView === id)
      .map((t) => ({ name: t.name, confirmation: t.confirmation, component: t.component }));
    views.push({
      id,
      uri: spec.uri,
      name: spec.title,
      description: spec.description,
      file: spec.file,
      bytes: bytes.length,
      sha256: createHash('sha256').update(bytes).digest('hex'),
      tools,
    });
    console.log(`${spec.file.padEnd(18)} ${String(bytes.length).padStart(9)} bytes  ${String(gzipSync(bytes).length).padStart(8)} gzip`);
  }

  const manifest = {
    schema: 1,
    mimeType: MCP_APP_MIME_TYPE,
    csp: CSP,
    resourceMeta: { ui: { prefersBorder: false } },
    views,
  };
  writeFileSync(join(outDir, 'manifest.json'), `${JSON.stringify(manifest, null, 2)}\n`);
  console.log(`wrote ${views.length} views and manifest.json to ${outDir}`);
} finally {
  rmSync(scratch, { recursive: true, force: true });
}
