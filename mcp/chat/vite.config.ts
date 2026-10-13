import { resolve } from 'node:path';
import react from '@vitejs/plugin-react';
import type { Plugin } from 'vite';
import { defineConfig } from 'vitest/config';

// @gqadonis/ipfs-mcp-ui is linked from ../ui, and its build externalizes its dependencies. Left alone,
// its imports of zod, zod-v3 and @a2ui/* would resolve from ../ui/node_modules: a second copy of each,
// and a second @a2ui/web_core means a Catalog class that this package's MessageProcessor does not
// recognize. Resolve exactly these names, for importers under ../ui, from this package instead.
//
// This is deliberately not `resolve.dedupe`: @a2ui/web_core itself imports `zod` and needs the Zod 3
// that pnpm gives it (versions.toml: "@a2ui/web_core 0.12.0 runs on Zod 3"). Deduping `zod` would hand
// it Zod 4, where `ZodEffects` does not exist.
const LINKED_UI = `${resolve(import.meta.dirname, '../ui')}/`;
const SHARED = /^(zod|zod\/.*|zod-v3|zod-v3\/.*|@a2ui\/react(\/.*)?|@a2ui\/web_core(\/.*)?)$/;

function resolveSharedFromThisPackage(): Plugin {
  return {
    name: 'resolve-linked-ui-deps-from-chat',
    enforce: 'pre',
    async resolveId(source, importer, options) {
      if (!importer?.startsWith(LINKED_UI) || !SHARED.test(source)) return null;
      return this.resolve(source, resolve(import.meta.dirname, 'package.json'), { ...options, skipSelf: true });
    },
  };
}

// One bundle for the host page to import() once: dist/ipfs-agent-chat.js is the small element, and the
// React island is a separate chunk the element loads on first open. Everything the island needs is
// bundled (the Web UI cannot supply React 19), so the build externalizes nothing.
//
// Build modes
//   production (default)  shadow root closed.
//   harness               shadow root open, for Playwright, which cannot pierce a closed root. Same code.
//   test (vitest)         shadow root open.
// @a2ui/react's v0_9 entry runs this at import time: it appends a <style id="a2ui-date-time-input-webkit-styles">
// to document.head. That changes the host page's DOM the moment the island loads, and a page CSP without
// style-src 'unsafe-inline' reports it as a violation. It only styles the A2UI DateTimeInput, which the
// IPFS catalog does not contain. Remove the block at build time. If a newer @a2ui/react changes the code
// so the pattern no longer matches, the build fails here rather than shipping a host-page side effect.
// There is no supported way to avoid it: 0.12.0 has no option, environment switch or separate entry that
// leaves it out (the block runs unconditionally at module scope of the v0_9 entry). The
// draft of the upstream report is in EVIDENCE.md, "Upstream report draft (not filed)". Drop this plugin
// when a release makes the injection opt-in.
function stripA2uiGlobalStyle(): Plugin {
  const block = /if \(typeof document !== "undefined"\) \{\s*const styleId = "a2ui-date-time-input-webkit-styles";[\s\S]*?document\.head\.appendChild\(style\);\s*\}\s*\}/;
  return {
    name: 'strip-a2ui-global-style',
    enforce: 'pre',
    transform(code, id) {
      if (!id.includes('/@a2ui/react/') || !code.includes('a2ui-date-time-input-webkit-styles')) return null;
      if (!block.test(code)) this.error(`strip-a2ui-global-style: the document.head style block in ${id} changed shape; update the pattern.`);
      return { code: code.replace(block, '/* removed by strip-a2ui-global-style: no document.head side effects */'), map: null };
    },
  };
}

export default defineConfig(({ mode }) => ({
  plugins: [resolveSharedFromThisPackage(), stripA2uiGlobalStyle(), react()],
  define: {
    // Library mode does not replace this; React would reach for `process` in the browser.
    'process.env.NODE_ENV': JSON.stringify(mode === 'production' || mode === 'harness' ? 'production' : 'development'),
    __AC_OPEN_SHADOW__: JSON.stringify(mode !== 'production'),
  },
  resolve: {
    // React must be one copy across the library and this package, or hooks fail across the boundary.
    dedupe: ['react', 'react-dom', 'lucide-react'],
  },
  build: {
    target: 'es2023',
    sourcemap: true,
    cssCodeSplit: false,
    assetsInlineLimit: 0,
    outDir: mode === 'harness' ? 'dist-harness' : 'dist',
    emptyOutDir: true,
    lib: {
      entry: resolve(import.meta.dirname, 'src/index.ts'),
      formats: ['es'],
      fileName: () => 'ipfs-agent-chat.js',
    },
    rollupOptions: {
      output: {
        chunkFileNames: 'chunks/[name]-[hash].js',
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['tests/setup.ts'],
    include: ['tests/unit/**/*.test.{ts,tsx}'],
    restoreMocks: true,
    testTimeout: 30_000,
    // Run @a2ui/react through the same transforms as the build, so the tests see the bundle's behavior
    // (notably: no <style> appended to document.head).
    server: { deps: { inline: [/@a2ui\//] } },
  },
}));
