// Usage: node scripts/scan-external-urls.mjs [--verbose]
// G11-4: the island must load no remote resource and need no unsafe-eval.
//   src    URLs, and network APIs outside the one module allowed to call them (src/agui/client.ts)
//   dist   URLs not on the allowlist, CSS url()/@import, and eval-like code generation
// Network-API words in dist are reported separately, not failed: a bundled React and zod contain the
// words (EventSource in a comment, XMLHttpRequest in a feature test); what counts is a URL or a call.
import { existsSync } from 'node:fs';
import { resolve } from 'node:path';
import { EVAL_ALLOWLIST, URL_ALLOWLIST, scan } from './lib/scan.mjs';

const root = resolve(import.meta.dirname, '..');
const verbose = process.argv.includes('--verbose');

const srcFindings = scan([resolve(root, 'src')], { base: root, networkApis: (rel) => rel !== 'src/agui/client.ts' });
const distDir = resolve(root, 'dist');
const distFindings = existsSync(distDir) ? scan([distDir], { base: root, networkApis: false }) : [];

for (const f of [...srcFindings, ...distFindings]) console.error(`${f.file}:${f.line}: ${f.kind}: ${f.text}`);
console.log(`scanned src: ${srcFindings.length} finding(s); dist: ${existsSync(distDir) ? `${distFindings.length} finding(s)` : 'not built'}`);
if (verbose) {
  for (const a of URL_ALLOWLIST) console.log(`allowed identifier: ${a.pattern} (${a.reason})`);
  for (const a of EVAL_ALLOWLIST) console.log(`allowed code: ${a.pattern} (${a.reason})`);
}
if (srcFindings.length + distFindings.length > 0) process.exit(1);
