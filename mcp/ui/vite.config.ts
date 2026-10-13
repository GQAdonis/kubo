import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import react from '@vitejs/plugin-react';
import type { Plugin } from 'vite';
import { defineConfig } from 'vitest/config';

// Keep in sync with src/styles/files.ts (a test asserts it).
const STYLE_FILES = ['tokens.css', 'base.css', 'components.css', 'views.css', 'chat.css'];

// dist/ipfs-ui.css is the same text the UiScope component injects: the source
// partials concatenated in STYLE_FILES order. One source, two delivery routes
// (inline <style> for shadow roots and iframes, a stylesheet for documents).
function emitStylesheet(): Plugin {
  return {
    name: 'ipfs-ui-emit-stylesheet',
    generateBundle() {
      const css = STYLE_FILES.map((f) => readFileSync(resolve(import.meta.dirname, 'src/styles', f), 'utf8')).join('\n');
      this.emitFile({ type: 'asset', fileName: 'ipfs-ui.css', source: css });
    },
  };
}

export default defineConfig({
  plugins: [react(), emitStylesheet()],
  build: {
    target: 'es2023',
    sourcemap: true,
    cssCodeSplit: false,
    assetsInlineLimit: 0,
    lib: {
      entry: {
        index: resolve(import.meta.dirname, 'src/index.ts'),
        a2ui: resolve(import.meta.dirname, 'src/a2ui/index.ts'),
        host: resolve(import.meta.dirname, 'src/host/index.ts'),
      },
      formats: ['es'],
    },
    rollupOptions: {
      external: [
        'react',
        'react-dom',
        'react/jsx-runtime',
        'zod',
        'zod-v3',
        'lucide-react',
        /^lucide-react\//,
        /^@a2ui\//,
      ],
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['tests/setup.ts'],
    include: ['tests/**/*.test.{ts,tsx}'],
    css: { include: /.+/ },
    restoreMocks: true,
  },
});
