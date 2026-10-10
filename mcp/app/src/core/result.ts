import {
  TOOL_META,
  TOOL_OUTPUT_SCHEMAS,
  ToolErrorSchema,
  type ConfirmationRequired,
  type ToolError,
  type ToolName,
} from '@gqadonis/ipfs-mcp-ui';
import { toolsForView, type ViewId } from '../views';

/**
 * The part of a CallToolResult the views read. Structural on purpose: the SDK's CallToolResult is
 * assignable to it, and the core stays testable without the SDK.
 */
export interface ToolResultLike {
  readonly isError?: boolean | undefined;
  readonly structuredContent?: unknown;
  readonly content?: readonly { readonly type: string; readonly text?: string | undefined }[] | undefined;
  readonly _meta?: Readonly<Record<string, unknown>> | undefined;
}

/** What a tool result means for the screen. Exhaustive: every result lands in exactly one of these. */
export type Outcome =
  /** Structured output of a known tool. The bound component validates it against the contract schema. */
  | { readonly kind: 'result'; readonly tool: ToolName; readonly output: unknown }
  /** A write or destructive call that is waiting for a person. Nothing has run. */
  | { readonly kind: 'confirmation'; readonly confirmation: ConfirmationRequired }
  /** A tool error that is not a confirmation. */
  | { readonly kind: 'failure'; readonly error: ToolError }
  /** Structured output that no tool of this view explains. Shown as data, never drawn as a view. */
  | { readonly kind: 'unidentified'; readonly issues: readonly string[]; readonly text: string }
  /** No structured content: the text blocks, which are all a host without MCP Apps would show. */
  | { readonly kind: 'text'; readonly text: string; readonly untrusted: boolean }
  | { readonly kind: 'empty' };

/** `_meta` key the server may set to name the producing tool (see INTEGRATION.md). */
export const TOOL_META_KEY = 'ai.ipfs/tool';
/** `_meta` key the server sets on results carrying bytes or names from the network. */
export const UNTRUSTED_META_KEY = 'ai.ipfs/untrusted-content';

const CONFIRMATION_CODES: ReadonlySet<string> = new Set(['confirmation_required', 'confirmation_pending']);
const MAX_TEXT = 64_000;

export function isToolName(value: unknown): value is ToolName {
  return typeof value === 'string' && Object.hasOwn(TOOL_META, value);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

export function textOf(result: ToolResultLike): string {
  const parts = (result.content ?? []).flatMap((c) => (c.type === 'text' && typeof c.text === 'string' ? [c.text] : []));
  const text = parts.join('\n');
  return text.length > MAX_TEXT ? `${text.slice(0, MAX_TEXT)}\n...` : text;
}

export interface ToolHints {
  /** hostContext.toolInfo.tool.name: the tool call that instantiated this view. */
  readonly toolInfoName?: string | undefined;
  /** The view this document is. Narrows inference when neither the result nor the host names the tool. */
  readonly view: ViewId;
}

/**
 * Which tool produced a result. Order of trust: the server's `_meta` marker, the host's toolInfo, then
 * inference from the structured content against the output schemas of the tools linked to this view.
 * Inference picks the first tool in catalog order whose schema accepts the content; the schemas of one
 * view are distinct enough that this is unambiguous for every current tool, and a test pins that.
 */
export function identifyTool(result: ToolResultLike, hints: ToolHints): ToolName | undefined {
  const marked = result._meta?.[TOOL_META_KEY];
  if (isToolName(marked)) return marked;
  if (isToolName(hints.toolInfoName)) return hints.toolInfoName;
  const sc = result.structuredContent;
  if (!isRecord(sc)) return undefined;
  return toolsForView(hints.view).find((t) => TOOL_OUTPUT_SCHEMAS[t].safeParse(sc).success);
}

function syntheticError(message: string): ToolError {
  return { error: { code: 'internal', message: message || 'The tool reported an error without details.', retryable: false } };
}

/** Classify a tool result. Pure. */
export function classifyResult(result: ToolResultLike, hints: ToolHints): Outcome {
  const sc = result.structuredContent;
  const text = textOf(result);

  if (isRecord(sc) && 'error' in sc) {
    const parsed = ToolErrorSchema.safeParse(sc);
    if (!parsed.success) return { kind: 'failure', error: syntheticError(text) };
    const e = parsed.data.error;
    if (CONFIRMATION_CODES.has(e.code) && e.confirmation) return { kind: 'confirmation', confirmation: e.confirmation };
    return { kind: 'failure', error: parsed.data };
  }
  if (result.isError === true) return { kind: 'failure', error: syntheticError(text) };

  if (isRecord(sc)) {
    const tool = identifyTool(result, hints);
    if (tool) return { kind: 'result', tool, output: sc };
    return { kind: 'unidentified', issues: ['No tool linked to this view accepts this structured content.'], text };
  }

  if (text !== '') return { kind: 'text', text, untrusted: result._meta?.[UNTRUSTED_META_KEY] === true };
  return { kind: 'empty' };
}

/** A call a view may make: a tool name and its complete arguments. */
export interface ToolCall {
  readonly tool: ToolName;
  readonly args: Readonly<Record<string, unknown>>;
}

/**
 * Views are read-only. They may call only tools whose confirmation class is none, so a click in a
 * view can never start a write or destructive call. Writes happen through the agent, which is
 * confirmed server-side (ADR-0003).
 */
export function isReadOnlyTool(tool: ToolName): boolean {
  return TOOL_META[tool].confirmation === 'none';
}
