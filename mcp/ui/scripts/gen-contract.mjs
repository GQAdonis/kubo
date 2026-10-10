// Usage: node scripts/gen-contract.mjs [--check]
import { mkdirSync, readFileSync, writeFileSync, existsSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { UI_ROOT, generate } from './lib/generate-contract.mjs';

const check = process.argv.includes('--check');
const files = generate();
let drift = 0;
for (const [rel, content] of Object.entries(files)) {
  const abs = join(UI_ROOT, rel);
  if (check) {
    const current = existsSync(abs) ? readFileSync(abs, 'utf8') : null;
    if (current !== content) {
      console.error(`drift: ${rel}`);
      drift++;
    }
    continue;
  }
  mkdirSync(dirname(abs), { recursive: true });
  writeFileSync(abs, content);
  console.log(`wrote ${rel}`);
}
if (check && drift > 0) {
  console.error(`${drift} generated file(s) differ from mcp/contract. Run: pnpm gen:contract`);
  process.exit(1);
}
