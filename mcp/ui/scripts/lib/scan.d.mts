export interface Finding {
  file: string;
  line: number;
  kind: 'url' | 'css-url' | 'network-api';
  text: string;
}
export interface AllowEntry {
  pattern: RegExp;
  reason: string;
}
export const URL_ALLOWLIST: AllowEntry[];
export const NETWORK_APIS: RegExp[];
export function scan(roots: string[], opts: { base: string; allow?: AllowEntry[]; skip?: string[]; checkApis?: boolean }): Finding[];
