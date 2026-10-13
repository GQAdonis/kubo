declare module '*/scripts/csp.mjs' {
  export const CSP: string;
}

declare module '*/scripts/lib/scan.mjs' {
  export interface Finding {
    file: string;
    line: number;
    kind: string;
    text: string;
  }
  export const URL_ALLOWLIST: readonly { pattern: RegExp; reason: string }[];
  export const FORBIDDEN_CALLS: readonly { re: RegExp; what: string }[];
  export function scan(base: string, roots: { html: string[]; source: string[] }): Finding[];
  export function cspOf(html: string): string | undefined;
  export function cspProblems(html: string): string[];
}
