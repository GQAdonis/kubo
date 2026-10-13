// Re-exports of the generated contract bindings. Everything here is derived from mcp/contract by
// scripts/gen-contract.mjs; edit the contract, not these files.
export * from './generated/bindings';
export * from './generated/tool-meta';
export { TOOL_OUTPUT_SCHEMAS } from './generated/outputs';
export type { ToolOutputMap } from './generated/outputs';
export {
  ConfirmationRequiredSchema,
  OutcomeSchema,
  PendingConfirmationSchema,
  SummarySchema,
  TargetSchema,
} from './generated/confirmation';
export type { ConfirmationClass, ConfirmationRequired, Outcome, PendingConfirmation, Resolver, Summary, Target } from './generated/confirmation';
export { ToolErrorSchema } from './generated/common';
export type { ToolError } from './generated/common';
export { AguiInterruptSchema, AguiMessageSchema } from './generated/agui';
export type { AguiInterrupt, AguiMessage } from './generated/agui';
