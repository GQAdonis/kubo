// Usage: node scripts/scan-external-urls.mjs [path ...]
// With no arguments scans src, catalog and (when present) dist. Exits 1 on any finding.
import { existsSync } from 'node:fs';
import { resolve } from 'node:path';
import { scan } from './lib/scan.mjs';

const root = resolve(import.meta.dirname, '..');
const args = process.argv.slice(2);
const targets = (args.length > 0 ? args : ['src', 'catalog', 'dist']).map((p) => resolve(root, p)).filter((p) => existsSync(p));

const findings = scan(targets, { base: root });
for (const f of findings) console.error(`${f.file}:${f.line}: ${f.kind}: ${f.text}`);
console.log(`scanned ${targets.length} path(s): ${findings.length} finding(s)`);
if (findings.length > 0) process.exit(1);
