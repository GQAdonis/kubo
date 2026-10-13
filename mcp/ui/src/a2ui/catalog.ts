import { Catalog } from '@a2ui/web_core/v0_9';
import type { ReactComponentImplementation } from '@a2ui/react/v0_9';
import { z } from 'zod-v3';
import { CONTRACT_COMPONENT_NAMES } from './apis';
import { A2UI_CATALOG_ALIASES, A2UI_CATALOG_ID, A2UI_CATALOG_SHORT_NAME } from '../contract/generated/tool-meta';
import { IMPLEMENTATIONS } from './implementations';

/**
 * The published catalog ID. A2UI treats catalogId as an identifier used to match client and server,
 * not a resolvable resource (spec: "Catalog Identification & Compatibility"). It is a URI to avoid
 * collisions, and the same string is the JSON Schema $id of catalog/ipfs-mcp.catalog.json. Both it
 * and the short alias are generated from the contract (derivations/a2ui-components.json). Nothing in
 * this package fetches it; the short alias is for docs and logs and is never sent as a catalogId.
 */
export { A2UI_CATALOG_ID, A2UI_CATALOG_ALIASES, A2UI_CATALOG_SHORT_NAME };

export const A2UI_PROTOCOL_VERSION = 'v0.9';

/** createSurface.theme for this catalog. Everything else about the look is the shared token set. */
export const IpfsThemeSchema = z.object({
  mode: z.enum(['dark', 'light', 'auto']).optional().describe('dark (default), light, or follow the system setting.'),
});

const INSTRUCTIONS = [
  'IPFS components for an agent that operates the person\'s own Kubo node. Each view component draws the',
  'structured output of one MCP tool: set `tool` to the tool name and bind `output` to the data model',
  'path that holds the tool\'s structuredContent. Output that does not match the tool\'s schema is not drawn.',
  'Write and destructive tools always pass a confirmation first: use ApprovalCard or ConfirmationDialog to',
  'show it, and wire approveAction only when this surface can really approve. Never imply an action ran',
  'before it did.',
].join(' ');

function implementationsInContractOrder(): ReactComponentImplementation[] {
  return CONTRACT_COMPONENT_NAMES.map((name) => {
    const impl = IMPLEMENTATIONS[name];
    if (!impl) throw new Error(`mcp/ui: contract component ${name} has no A2UI implementation`);
    return impl;
  });
}

/** The A2UI v0.9 catalog of IPFS components, ready for a MessageProcessor. */
export function createIpfsCatalog(): Catalog<ReactComponentImplementation> {
  return new Catalog<ReactComponentImplementation>(
    A2UI_CATALOG_ID,
    A2UI_PROTOCOL_VERSION,
    implementationsInContractOrder(),
    [],
    IpfsThemeSchema,
    INSTRUCTIONS,
  );
}

export const ipfsCatalog: Catalog<ReactComponentImplementation> = createIpfsCatalog();
