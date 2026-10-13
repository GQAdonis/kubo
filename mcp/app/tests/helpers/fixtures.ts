// The realistic structured outputs for every contract tool come from the component library's own
// fixtures: mcp/ui proves each one validates against the contract JSON Schema and the generated Zod
// schema, so the app tests start from contract-valid data. Read-only import; nothing here edits mcp/ui.
export { ADDR_A, CID_A, CID_B, CID_C, KEY_ID, OUTPUTS, PEER_A, PEER_B } from '../../../ui/tests/fixtures/outputs';

import { readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';

const CONTRACT_ROOT = resolve(import.meta.dirname, '..', '..', '..', 'contract');

export function contractJson<T = unknown>(rel: string): T {
  return JSON.parse(readFileSync(join(CONTRACT_ROOT, rel), 'utf8')) as T;
}

/** The three ConfirmationRequired examples from mcp/contract. */
export const CONFIRMATIONS = {
  humanWithToken: contractJson<Record<string, unknown>>('confirmation/examples/ConfirmationRequired/human-mcp-with-token.json'),
  a2a: contractJson<Record<string, unknown>>('confirmation/examples/ConfirmationRequired/a2a-no-token.json'),
  nativePrompt: contractJson<Record<string, unknown>>('confirmation/examples/ConfirmationRequired/ui-native-prompt.json'),
} as const;
