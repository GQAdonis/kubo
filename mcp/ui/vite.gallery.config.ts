import { resolve } from 'node:path';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

// A deny-by-default Content-Security-Policy equal to the MCP Apps restrictive default (SEP-1865,
// "Host Behavior"). The gallery is served with it, so the visual, axe and keyboard specs prove the
// components work under the policy a host applies to an MCP App.
export const MCP_APPS_DEFAULT_CSP =
  "default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; media-src 'self' data:; connect-src 'none'";

export default defineConfig({
  root: resolve(import.meta.dirname, 'gallery'),
  plugins: [react()],
  build: {
    outDir: resolve(import.meta.dirname, 'gallery-dist'),
    emptyOutDir: true,
    target: 'es2023',
    assetsInlineLimit: 0,
    modulePreload: { polyfill: false },
    rollupOptions: {
      input: {
        index: resolve(import.meta.dirname, 'gallery/index.html'),
        shadow: resolve(import.meta.dirname, 'gallery/shadow.html'),
      },
    },
  },
  preview: {
    port: 4173,
    strictPort: true,
    host: '127.0.0.1',
    headers: { 'Content-Security-Policy': MCP_APPS_DEFAULT_CSP },
  },
});
