import { registerBundledFonts } from '@gqadonis/ipfs-mcp-ui';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { z } from 'zod';
import './app.css';
import { McpAppRoot } from './ui/McpAppRoot';
import { isViewId } from './views';

// The sandbox CSP has no 'unsafe-eval'. Zod's JIT object parser uses new Function(), so switch it
// off before the first parse. The SDK's App does the same; doing it here also covers the library's
// own contract validation, whichever runs first.
z.config({ jitless: true });

const view = document.documentElement.dataset['view'];
const mount = document.getElementById('root');

if (!isViewId(view) || !mount) {
  throw new Error(`MCP App document is missing a valid data-view (got "${String(view)}").`);
}

// Brand faces from embedded bytes; no font-src and no request. Fallback stacks carry the layout meanwhile.
void registerBundledFonts();

createRoot(mount).render(
  <StrictMode>
    <McpAppRoot view={view} />
  </StrictMode>,
);
