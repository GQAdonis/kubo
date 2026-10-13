import { CHAT_RULES, TOOL_META, type ToolMeta, type ToolName } from '../contract/generated/tool-meta';

/**
 * Evaluates the contract's chat rules (derivations/a2ui-components.json). Rule forms, as in the Go
 * evaluator: `always`, `copy:<field>`, `not-equal:<field>:<value>`. The generator rejects any other form.
 */
export function evalRule(rule: string, meta: ToolMeta): boolean {
  if (rule === 'always') return true;
  const parts = rule.split(':');
  const kind = parts[0];
  const field = parts[1] as keyof ToolMeta | undefined;
  if (kind === 'copy' && field) return Boolean(meta[field]);
  if (kind === 'not-equal' && field) return String(meta[field]) !== parts[2];
  throw new Error(`unsupported chat rule: ${rule}`);
}

export interface ChatElements {
  readonly toolCallCard: boolean;
  readonly approvalCard: boolean;
  readonly egressBadge: boolean;
  readonly untrustedFrame: boolean;
}

/** Which chat elements wrap a call to this tool, derived from the catalog and never authored per tool. */
export function chatElementsFor(tool: ToolName): ChatElements {
  const meta = TOOL_META[tool];
  return {
    toolCallCard: evalRule(CHAT_RULES.ToolCallCard, meta),
    approvalCard: evalRule(CHAT_RULES.ApprovalCard, meta),
    egressBadge: evalRule(CHAT_RULES.EgressBadge, meta),
    untrustedFrame: evalRule(CHAT_RULES.untrustedFrame, meta),
  };
}
