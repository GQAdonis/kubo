import { z } from 'zod';

// Imported first by mount.tsx, so it runs before any schema in the island or in @gqadonis/ipfs-mcp-ui
// is used. With jitless set, zod never probes for `new Function` to compile faster parsers. A page
// whose CSP lacks unsafe-eval would report that probe as a violation even though zod catches the error;
// the island needs the interpreted path only. G11-4: no unsafe-eval.
z.config({ jitless: true });
