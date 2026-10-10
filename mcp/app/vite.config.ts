import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

// Vitest configuration. The production bundles are built by scripts/build.mjs, one single-file HTML
// per view, with the same React plugin. @gqadonis/ipfs-mcp-ui is a linked package whose build output
// imports react and zod; dedupe keeps one copy of each in the bundle and in the test run.
export default defineConfig({
  plugins: [react()],
  resolve: { dedupe: ['react', 'react-dom', 'zod', 'lucide-react'] },
  test: {
    environment: 'jsdom',
    setupFiles: ['tests/setup.ts'],
    include: ['tests/**/*.test.{ts,tsx}'],
    css: { include: /.+/ },
    restoreMocks: true,
    server: { deps: { inline: [/@gqadonis\/ipfs-mcp-ui/, /@modelcontextprotocol/] } },
  },
});
