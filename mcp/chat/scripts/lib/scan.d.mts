export interface Allowed {
  readonly pattern: RegExp;
  readonly reason: string;
}
export interface Finding {
  file: string;
  line: number;
  kind: 'url' | 'css-url' | 'network-api' | 'eval';
  text: string;
}
export const URL_ALLOWLIST: readonly Allowed[];
export const EVAL_ALLOWLIST: readonly Allowed[];
export const NETWORK_APIS: readonly RegExp[];
export const EVAL_APIS: readonly RegExp[];
export function scan(
  roots: string[],
  opts: { base: string; allow?: readonly Allowed[]; skip?: string[]; networkApis?: boolean | ((file: string) => boolean); evalApis?: boolean },
): Finding[];
