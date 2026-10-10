// A2UI v0.9 adapter: the IPFS catalog as an @a2ui/web_core Catalog whose components are the same React
// components the MCP App renders. Import from "@gqadonis/ipfs-mcp-ui/a2ui".
export { A2UI_CATALOG_ID, A2UI_CATALOG_SHORT_NAME, A2UI_PROTOCOL_VERSION, IpfsThemeSchema, createIpfsCatalog, ipfsCatalog } from './catalog';
export { COMPONENT_APIS, CONTRACT_COMPONENT_NAMES } from './apis';
export type { IpfsComponentApi } from './apis';
export { IMPLEMENTATIONS } from './implementations';
export { validateCatalogManifest } from './validate';
export type { CatalogIssue, CatalogIssueKind } from './validate';
