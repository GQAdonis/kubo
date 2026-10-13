import type { KuboRPCClient } from 'kubo-rpc-client';

export type JsonSchema = Record<string, unknown>;

/** The four annotations the WebMCP draft (8 October 2026) defines. */
export interface WebMcpAnnotations {
  readOnlyHint: boolean;
  untrustedContentHint: boolean;
  consequentialHint: boolean;
  debugging: boolean;
}

/** Confirmation classes a WebMCP tool can have. `destructive` is never exported. */
export type ExportedConfirmationClass = 'none' | 'write';

export interface GeneratedTool {
  name: string;
  group: string;
  title: string;
  description: string;
  confirmation: ExportedConfirmationClass;
  annotations: WebMcpAnnotations;
  inputSchema: JsonSchema;
}

export interface ExcludedTool {
  name: string;
  reason: string;
}

/** What the confirmation modal needs to name the operation (WebMCP design: tool, CIDs/paths, effect). */
export interface ConfirmationTarget {
  /** Stable key such as `path`, `source` or `destination`; the host localizes it. */
  label: string;
  /**
   * Safe to render: every control, bidi and zero-width code point is replaced by a visible
   * `<U+XXXX>` token, and a long value is cut in the middle (see `full`).
   */
  value: string;
  /** The whole sanitized value (capped), when `value` was shortened. */
  full?: string;
  /** True when the original contained non-ASCII characters; the host should say so. */
  nonAscii?: boolean;
}

export interface ConfirmationRequest {
  tool: string;
  title: string;
  confirmation: 'write';
  /** English one-sentence effect. A host with i18n looks up its own text by `tool` and falls back to this. */
  effect: string;
  /** The CIDs, paths or names the call acts on. */
  targets: ConfirmationTarget[];
  /** Other facts the user should see, such as byte counts or flags. Values are short strings. */
  details: ConfirmationTarget[];
}

export interface ConfirmationContext {
  /** Aborted when the host cancels the call. Optional: some hosts pass none. */
  signal?: AbortSignal;
}

/**
 * Implemented by the host page. Resolve `true` only when the user approved this exact
 * request. Anything else, including a throw, is a decline and sends no RPC request. The
 * strings `queue_full` and `writes_off` are declines that say why.
 */
export type ConfirmationAnswer = boolean | 'queue_full' | 'writes_off';

export type ConfirmationHook = (request: ConfirmationRequest, context: ConfirmationContext) => ConfirmationAnswer | Promise<ConfirmationAnswer>;

export type RpcClient = KuboRPCClient;

export interface ExecContext {
  client: RpcClient;
  signal?: AbortSignal;
  /** Applied to each RPC call; a call with no host signal gets an AbortSignal.timeout of this length. */
  timeoutMs: number;
  /** Pagination cursors this tool set has issued. A cursor the tool did not issue is refused. */
  issuedCursors: Set<string>;
}

export type Executor = (input: Record<string, any>, context: ExecContext) => Promise<Record<string, unknown>>;

/** Optional second argument of `execute`; the draft passes it but Kimi does not. */
export interface ExecuteOptions {
  signal?: AbortSignal;
}

/** The object handed to `document.modelContext.registerTool`. Exactly these fields. */
export interface WebMcpTool {
  name: string;
  title: string;
  description: string;
  inputSchema: JsonSchema;
  annotations: WebMcpAnnotations;
  execute: (input: unknown, options?: ExecuteOptions) => Promise<ToolEnvelope>;
}

export type ToolStatus = 'done' | 'declined' | 'error';

/**
 * Every result is a plain JSON object. Values the agent must relay are in clearly named
 * plain fields (`summary`, `result`). Content that came from the network or from files
 * is kept apart under `data` and flagged, so a host that masks untrusted output masks
 * only that part.
 */
export type ToolEnvelope =
  | { ok: true; status: 'done'; tool: string; summary: string; result: Record<string, unknown> }
  | {
      ok: true;
      status: 'done';
      tool: string;
      summary: string;
      untrusted_content: true;
      data_notice: string;
      data: Record<string, unknown>;
    }
  | { ok: false; status: 'declined'; tool: string; code: DeclinedCode; message: string }
  | { ok: false; status: 'error'; tool: string; code: ErrorCode; message: string; detail?: string };

export type DeclinedCode =
  | 'user_declined'
  | 'cancelled'
  | 'inactive'
  | 'writes_off'
  | 'dnslink_off'
  | 'queue_full';

export type ErrorCode = 'invalid_input' | 'rpc_error' | 'cancelled' | 'confirmation_unavailable' | 'internal_error';

export interface WebMcpToolOptions {
  client: RpcClient;
  /**
   * Required for write tools. Without it no write tool is created. With it, every write
   * call asks first and sends no RPC request unless the hook resolves `true`.
   */
  confirm?: ConfirmationHook;
  /** The second switch. When false, only class `none` tools are created. Default false. */
  includeWrites?: boolean;
  /** Per RPC call timeout. Default 60000. */
  timeoutMs?: number;
  /**
   * Live checks, evaluated inside every `execute` (and again after an approval), so a host
   * that ignored unregistration still gets a declined result and no RPC request.
   * `isActive` defaults to always true; `writesAllowed` defaults to `includeWrites`;
   * `dnsLinkAllowed` defaults to never.
   */
  isActive?: () => boolean;
  writesAllowed?: () => boolean;
  dnsLinkAllowed?: () => boolean;
}
