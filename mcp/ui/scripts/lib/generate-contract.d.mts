export const UI_ROOT: string;
export const CONTRACT_ROOT: string;
/** Pure: returns { relativePath: fileContent } for src/contract/generated. */
export function generate(): Record<string, string>;
