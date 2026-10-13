import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
// @ts-expect-error plain ESM script without types
import { CONTRACT_ROOT } from '../../scripts/lib/generate.mjs';

const BASE = 'https://contract.test/';

/** Validates tool results against the contract's own output schemas. */
export function outputValidator() {
  const ajv = new Ajv2020({ strict: false, allErrors: true });
  (addFormats as unknown as (a: Ajv2020) => void)(ajv);
  const load = (rel: string) => {
    const schema = JSON.parse(readFileSync(join(CONTRACT_ROOT, rel), 'utf8'));
    schema.$id = BASE + rel;
    return schema;
  };
  ajv.addSchema(load('schemas/common.json'));
  ajv.addSchema(load('confirmation/confirmation.schema.json'));
  return (tool: string, value: unknown): string[] => {
    const id = `${BASE}schemas/tools/${tool}.output.json`;
    let validate = ajv.getSchema(id);
    if (!validate) {
      ajv.addSchema(load(`schemas/tools/${tool}.output.json`));
      validate = ajv.getSchema(id)!;
    }
    return validate(value) ? [] : (validate.errors ?? []).map((e) => `${e.instancePath} ${e.message}`);
  };
}
