// Public API of @gqadonis/ipfs-mcp-ui: pure presentational React 19 components. No host, transport
// or network imports. Hosts pass typed props and callbacks.

// Scope, styles, fonts
export { UiScope, useHostActions } from './scope/UiScope';
export type { UiHostActions, UiScopeProps, UiTheme } from './scope/UiScope';
export { STYLE_TEXT, adoptStyles } from './styles/style-text';
export { registerBundledFonts } from './fonts/register';

// Views bound to contract tool outputs
export { ContentCard } from './components/content-card/ContentCard';
export type { ContentCardProps } from './components/content-card/ContentCard';
export { PinList } from './components/pin-list/PinList';
export type { PinListProps } from './components/pin-list/PinList';
export { MfsTree, pathTrail } from './components/mfs-tree/MfsTree';
export type { MfsTreeProps } from './components/mfs-tree/MfsTree';
export { DagViewer } from './components/dag-viewer/DagViewer';
export type { DagViewerProps } from './components/dag-viewer/DagViewer';
export { IpnsRecordCard } from './components/ipns-record-card/IpnsRecordCard';
export type { IpnsRecordCardProps } from './components/ipns-record-card/IpnsRecordCard';
export { NodeStatusPanel } from './components/node-status-panel/NodeStatusPanel';
export type { NodeStatusPanelProps } from './components/node-status-panel/NodeStatusPanel';
export {
  ConfirmationDialog,
  ConfirmationDialogDataSchema,
  approvalModelFromConfirmation,
} from './components/confirmation-dialog/ConfirmationDialog';
export type { ConfirmationDialogData, ConfirmationDialogProps } from './components/confirmation-dialog/ConfirmationDialog';
export { ToolErrorNotice, isFailure } from './components/tool-error/ToolErrorNotice';
export { VIEW_REGISTRY, renderToolResult } from './components/registry';
export type { ToolResultRender, ViewEntry, ViewHandlers, ViewName } from './components/registry';

// Chat primitives (shadow-DOM safe)
export { ChatMessageItem, MessageStream, MessageStreamDataSchema } from './chat/MessageStream';
export type { ChatMessageItemProps, MessageStreamData, MessageStreamProps } from './chat/MessageStream';
export { ToolCallCard, ToolCallCardDataSchema } from './chat/ToolCallCard';
export type { ToolCallCardData, ToolCallCardProps } from './chat/ToolCallCard';
export { ApprovalCard, ApprovalCardDataSchema, approvalModelFromInterrupt, interruptExpired } from './chat/ApprovalCard';
export type { ApprovalCardData, ApprovalCardProps } from './chat/ApprovalCard';
export type { ApprovalActivation } from './approval/ApprovalPanel';
export { EgressBadge, EgressBadgeDataSchema } from './chat/EgressBadge';
export type { EgressBadgeData, EgressBadgeProps } from './chat/EgressBadge';
export { chatElementsFor, evalRule } from './chat/rules';
export type { ChatElements } from './chat/rules';
export {
  EMPTY_CHAT_STATE,
  RUN_ERROR_CODES,
  applyRunInput,
  foldAgui,
  foldAguiEvents,
  parseJson,
  isRunErrorCode,
  parseToolError,
  runErrorCopy,
  toWire,
} from './chat/agui';
export type {
  AguiEvent,
  ChatInterrupt,
  ChatMessage,
  ChatState,
  ChatToolCall,
  InterruptResolution,
  ResumeEntry,
  RunErrorCode,
  RunErrorCopy,
  RunStatus,
} from './chat/agui';

// Approval model shared by the dialog and the card
export { APPROVAL_PHASES, ApprovalModelSchema, ApprovalPhaseSchema, phaseSentence } from './approval/model';
export type { ApprovalModel, ApprovalPhase } from './approval/model';

// Contract bindings (generated from mcp/contract)
export * from './contract';

// Primitives for hosts that compose their own surfaces
export { Badge, Button, Bytes, Cid, CopyButton, DataTable, IdText, KeyValue, More, Notice, Panel, UntrustedFrame } from './ui/primitives';
export type { BadgeProps, ButtonProps, Column, Tone } from './ui/primitives';
export { displayText } from './lib/safe-text';
export { formatBytes, formatBytesExact, formatRate, formatUtc } from './lib/format';
