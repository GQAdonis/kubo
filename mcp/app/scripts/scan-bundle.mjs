// Usage: node scripts/scan-bundle.mjs
// Fails (exit 1) when a built view or the app source can reach a remote origin, or when a document
// lacks the no-origin CSP. Run after `pnpm build`.
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { cspProblems, scan } from './lib/scan.mjs';

const root = resolve(import.meta.dirname, '..');
const dist = join(root, 'dist');
if (!existsSync(dist)) {
  console.error('dist/ is missing. Run `pnpm build` first.');
  process.exit(2);
}
const html = readdirSync(dist)
  .filter((f) => f.endsWith('.html'))
  .map((f) => join(dist, f));
if (html.length === 0) {
  console.error('dist/ has no view documents.');
  process.exit(2);
}

const findings = scan(root, { html, source: [join(root, 'src')] });
for (const f of findings) console.error(`${f.file}:${f.line}: ${f.kind}: ${f.text}`);
let problems = 0;
for (const file of html) {
  for (const p of cspProblems(readFileSync(file, 'utf8'))) {
    console.error(`${file}: csp: ${p}`);
    problems++;
  }
}
console.log(`scanned ${html.length} document(s) and src: ${findings.length} finding(s), ${problems} CSP problem(s)`);
if (findings.length > 0 || problems > 0) process.exit(1);
