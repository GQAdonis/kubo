import { createComponentImplementation, type ReactComponentImplementation } from '@a2ui/react/v0_9';
import type { ReactNode } from 'react';
import { ApprovalCard, ApprovalCardDataSchema } from '../chat/ApprovalCard';
import { EgressBadge, EgressBadgeDataSchema } from '../chat/EgressBadge';
import { MessageStream, MessageStreamDataSchema } from '../chat/MessageStream';
import { ToolCallCard, ToolCallCardDataSchema } from '../chat/ToolCallCard';
import { ConfirmationDialog, ConfirmationDialogDataSchema } from '../components/confirmation-dialog/ConfirmationDialog';
import { renderToolResult, type ViewHandlers } from '../components/registry';
import type { ToolName } from '../contract/generated/tool-meta';
import { UiScope, type UiTheme } from '../scope/UiScope';
import { Notice } from '../ui/primitives';
import {
  ApprovalCardApi,
  ConfirmationDialogApi,
  ContentCardApi,
  DagViewerApi,
  EgressBadgeApi,
  IpnsRecordCardApi,
  MessageStreamApi,
  MfsTreeApi,
  NodeStatusPanelApi,
  PinListApi,
  ToolCallCardApi,
} from './apis';

// Each implementation is a thin adapter: A2UI resolves data-model bindings into plain props, the
// adapter validates them against the contract's Zod schema, and the same React component the MCP App
// uses draws them. Invalid data is reported in place, never drawn as something it is not.

const THEMES = new Set<UiTheme>(['dark', 'light', 'auto']);

/** A surface may carry theme.mode (dark, light, auto) from createSurface. Anything else is ignored. */
function themeOf(theme: Record<string, unknown> | undefined): UiTheme | undefined {
  const mode = theme?.['mode'];
  return typeof mode === 'string' && THEMES.has(mode as UiTheme) ? (mode as UiTheme) : undefined;
}

function Invalid({ what, issues }: { what: string; issues: readonly string[] }): ReactNode {
  return (
    <Notice tone="err" label="Invalid data">
      {what} does not match the contract schema, so nothing is drawn. {issues.join('; ')}
    </Notice>
  );
}

const issuesOf = (error: { issues: readonly { path: PropertyKey[]; message: string }[] }): string[] =>
  error.issues.slice(0, 5).map((i) => `${i.path.map(String).join('.') || '(root)'}: ${i.message}`);

function Scoped({ theme, children }: { theme: Record<string, unknown> | undefined; children: ReactNode }) {
  return <UiScope theme={themeOf(theme)}>{children}</UiScope>;
}

function view(tool: string, output: unknown, handlers: ViewHandlers): ReactNode {
  const rendered = renderToolResult(tool as ToolName, output, handlers);
  if (rendered.kind === 'view') return rendered.node;
  if (rendered.kind === 'invalid') return <Invalid what={`The ${tool} output`} issues={rendered.issues} />;
  return <Invalid what={`The ${tool} output`} issues={['no view is bound to this tool']} />;
}

const pager = (action: (() => void) | undefined): ViewHandlers => (action ? { onLoadMore: () => action() } : {});

export const ContentCardImpl = createComponentImplementation(ContentCardApi, ({ props, context }) => (
  <Scoped theme={context.theme}>{view(props.tool, props.output, pager(props.loadMoreAction))}</Scoped>
));

export const PinListImpl = createComponentImplementation(PinListApi, ({ props, context }) => (
  <Scoped theme={context.theme}>{view(props.tool, props.output, pager(props.loadMoreAction))}</Scoped>
));

export const MfsTreeImpl = createComponentImplementation(MfsTreeApi, ({ props, context }) => (
  <Scoped theme={context.theme}>{view(props.tool, props.output, pager(props.loadMoreAction))}</Scoped>
));

export const DagViewerImpl = createComponentImplementation(DagViewerApi, ({ props, context }) => (
  <Scoped theme={context.theme}>{view(props.tool, props.output, {})}</Scoped>
));

export const IpnsRecordCardImpl = createComponentImplementation(IpnsRecordCardApi, ({ props, context }) => (
  <Scoped theme={context.theme}>{view(props.tool, props.output, {})}</Scoped>
));

export const NodeStatusPanelImpl = createComponentImplementation(NodeStatusPanelApi, ({ props, context }) => (
  <Scoped theme={context.theme}>{view(props.tool, props.output, pager(props.loadMoreAction))}</Scoped>
));

export const ConfirmationDialogImpl = createComponentImplementation(ConfirmationDialogApi, ({ props, context }) => {
  const parsed = ConfirmationDialogDataSchema.safeParse({
    confirmation: props.confirmation,
    ...(props.resolution !== undefined && { resolution: props.resolution }),
  });
  return (
    <Scoped theme={context.theme}>
      {parsed.success ? (
        <ConfirmationDialog
          {...parsed.data}
          onApprove={props.approveAction ? () => props.approveAction?.() : undefined}
          onDeny={props.denyAction ? () => props.denyAction?.() : undefined}
        />
      ) : (
        <Invalid what="The confirmation" issues={issuesOf(parsed.error)} />
      )}
    </Scoped>
  );
});

export const MessageStreamImpl = createComponentImplementation(MessageStreamApi, ({ props, context }) => {
  const parsed = MessageStreamDataSchema.safeParse({ messages: props.messages });
  return (
    <Scoped theme={context.theme}>
      {parsed.success ? <MessageStream messages={parsed.data.messages} /> : <Invalid what="The messages" issues={issuesOf(parsed.error)} />}
    </Scoped>
  );
});

export const ToolCallCardImpl = createComponentImplementation(ToolCallCardApi, ({ props, context }) => {
  const parsed = ToolCallCardDataSchema.safeParse(props.call);
  return (
    <Scoped theme={context.theme}>
      {parsed.success ? (
        <ToolCallCard call={parsed.data} acknowledgedHosts={props.acknowledgedHosts ?? []} />
      ) : (
        <Invalid what="The tool call" issues={issuesOf(parsed.error)} />
      )}
    </Scoped>
  );
});

export const ApprovalCardImpl = createComponentImplementation(ApprovalCardApi, ({ props, context }) => {
  const parsed = ApprovalCardDataSchema.safeParse({
    interrupt: props.interrupt,
    ...(props.resolution !== undefined && { resolution: props.resolution }),
    ...(props.expired !== undefined && { expired: props.expired }),
  });
  return (
    <Scoped theme={context.theme}>
      {parsed.success ? (
        <ApprovalCard
          {...parsed.data}
          onApprove={props.approveAction ? () => props.approveAction?.() : undefined}
          onDeny={props.denyAction ? () => props.denyAction?.() : undefined}
        />
      ) : (
        <Invalid what="The interrupt" issues={issuesOf(parsed.error)} />
      )}
    </Scoped>
  );
});

export const EgressBadgeImpl = createComponentImplementation(EgressBadgeApi, ({ props, context }) => {
  const parsed = EgressBadgeDataSchema.safeParse({
    tool: props.tool,
    acknowledged_hosts: props.acknowledgedHosts,
    ...(props.refused !== undefined && { refused: props.refused }),
  });
  return (
    <Scoped theme={context.theme}>
      {parsed.success ? <EgressBadge {...parsed.data} /> : <Invalid what="The egress badge" issues={issuesOf(parsed.error)} />}
    </Scoped>
  );
});

/** Keyed by the contract's component name so the catalog builder can assert full coverage. */
export const IMPLEMENTATIONS: Readonly<Record<string, ReactComponentImplementation>> = {
  ContentCard: ContentCardImpl,
  PinList: PinListImpl,
  MfsTree: MfsTreeImpl,
  DagViewer: DagViewerImpl,
  IpnsRecordCard: IpnsRecordCardImpl,
  ConfirmationDialog: ConfirmationDialogImpl,
  NodeStatusPanel: NodeStatusPanelImpl,
  MessageStream: MessageStreamImpl,
  ToolCallCard: ToolCallCardImpl,
  ApprovalCard: ApprovalCardImpl,
  EgressBadge: EgressBadgeImpl,
};
