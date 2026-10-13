import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';

const here = dirname(fileURLToPath(import.meta.url));
export const UI_ROOT = resolve(here, '..', '..');
export const CONTRACT_ROOT = resolve(UI_ROOT, '..', 'contract');

export const readContractJson = <T = unknown>(rel: string): T =>
  JSON.parse(readFileSync(join(CONTRACT_ROOT, rel), 'utf8')) as T;

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p, out);
    else if (name.endsWith('.json')) out.push(p);
  }
  return out;
}

let cached: Ajv2020 | undefined;

/**
 * An Ajv instance holding every contract schema under a file:/// URI mirroring its path, so the
 * schemas' relative $refs resolve exactly as they do in the Go validator. The meta-schema and every
 * schema are local; nothing is fetched.
 */
export function contractAjv(): Ajv2020 {
  if (cached) return cached;
  const ajv = new Ajv2020({ strict: false, allErrors: true });
  addFormats(ajv);
  for (const file of [...walk(join(CONTRACT_ROOT, 'schemas')), join(CONTRACT_ROOT, 'confirmation', 'confirmation.schema.json'), join(CONTRACT_ROOT, 'fixtures', 'agui.schema.json')]) {
    const rel = relative(CONTRACT_ROOT, file).split('\\').join('/');
    if (rel.startsWith('schemas/examples') || rel.includes('/examples/')) continue;
    ajv.addSchema(JSON.parse(readFileSync(file, 'utf8')) as object, `file:///${rel}`);
  }
  cached = ajv;
  return ajv;
}

export function validateToolOutput(tool: string, value: unknown): { ok: boolean; errors: string } {
  const validate = contractAjv().getSchema(`file:///schemas/tools/${tool}.output.json`);
  if (!validate) throw new Error(`no output schema for ${tool}`);
  const ok = validate(value) as boolean;
  return { ok, errors: ok ? '' : contractAjv().errorsText(validate.errors) };
}

export function validateAgui(value: unknown): { ok: boolean; errors: string } {
  const validate = contractAjv().getSchema('file:///fixtures/agui.schema.json');
  if (!validate) throw new Error('agui schema not loaded');
  const ok = validate(value) as boolean;
  return { ok, errors: ok ? '' : contractAjv().errorsText(validate.errors) };
}

export function readAguiFixtures(): { name: string; fixture: AguiFixtureFile }[] {
  const dir = join(CONTRACT_ROOT, 'fixtures', 'agui');
  return readdirSync(dir)
    .filter((n) => n.endsWith('.json'))
    .sort()
    .map((n) => ({ name: n.replace(/\.json$/, ''), fixture: JSON.parse(readFileSync(join(dir, n), 'utf8')) as AguiFixtureFile }));
}

export interface AguiFixtureFile {
  description: string;
  provenance: string;
  principalKind: string;
  input: { threadId: string; runId: string; messages: unknown[]; resume?: unknown[] };
  events: { type: string; [k: string]: unknown }[];
}
