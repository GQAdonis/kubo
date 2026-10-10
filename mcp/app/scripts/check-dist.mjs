// Usage: node scripts/check-dist.mjs
//
// The rebuild-and-diff check. Rebuilds every view from source into a scratch directory and compares it
// with the committed mcp/app/dist, file by file, byte for byte. Exits 1 when a file differs, is
// missing or is extra: the committed bundle the Go server embeds no longer matches its source.
//
// Kubo's `make build` stays free of a JS toolchain because dist/ is committed. This script is what
// keeps that safe: run it in CI after `pnpm install --frozen-lockfile` and `pnpm build:ui`.
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdtempSync, readFileSync, readdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

const root = resolve(import.meta.dirname, '..');
const committed = join(root, 'dist');
const scratch = mkdtempSync(join(tmpdir(), 'ipfs-mcp-app-check-'));

function digests(dir) {
  const out = new Map();
  if (!existsSync(dir)) return out;
  for (const f of readdirSync(dir)) {
    if (f.endsWith('.html') || f === 'manifest.json') out.set(f, createHash('sha256').update(readFileSync(join(dir, f))).digest('hex'));
  }
  return out;
}

try {
  const built = spawnSync(process.execPath, [join(root, 'scripts/build.mjs'), '--out', scratch], { cwd: root, encoding: 'utf8' });
  if (built.status !== 0) {
    console.error(built.stdout, built.stderr);
    console.error('check-dist: the rebuild failed, so the committed bundle cannot be verified.');
    process.exit(2);
  }
  const want = digests(scratch);
  const have = digests(committed);
  const problems = [];
  for (const [f, h] of want) {
    if (!have.has(f)) problems.push(`missing from dist: ${f}`);
    else if (have.get(f) !== h) problems.push(`differs from a fresh build: ${f}`);
  }
  for (const f of have.keys()) if (!want.has(f)) problems.push(`not produced by the build: ${f}`);

  if (problems.length > 0) {
    for (const p of problems) console.error(`check-dist: ${p}`);
    console.error('check-dist: dist/ is stale. Run `pnpm build` in mcp/app and commit the result.');
    process.exit(1);
  }
  console.log(`check-dist: dist/ matches a fresh build (${want.size} files).`);
} finally {
  rmSync(scratch, { recursive: true, force: true });
}
