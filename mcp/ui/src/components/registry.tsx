import type { ReactNode } from 'react';
import type { z, ZodType } from 'zod';
import {
  ContentCardDataSchema,
  DagViewerDataSchema,
  IpnsRecordCardDataSchema,
  MfsTreeDataSchema,
  NodeStatusPanelDataSchema,
  PinListDataSchema,
} from '../contract/generated/bindings';
import { TOOL_META, type ComponentName, type ToolName } from '../contract/generated/tool-meta';
import { ConfirmationDialog, ConfirmationDialogDataSchema } from './confirmation-dialog/ConfirmationDialog';
import { ContentCard } from './content-card/ContentCard';
import { DagViewer } from './dag-viewer/DagViewer';
import { IpnsRecordCard } from './ipns-record-card/IpnsRecordCard';
import { MfsTree } from './mfs-tree/MfsTree';
import { NodeStatusPanel } from './node-status-panel/NodeStatusPanel';
import { PinList } from './pin-list/PinList';

/** Host callbacks any view may use. A view ignores the ones it has no use for. */
export interface ViewHandlers {
  onOpenPath?: (path: string) => void;
  onLoadMore?: (cursor: string) => void;
  onRequestUnpin?: (cid: string) => void;
  onApprove?: () => void;
  onDeny?: () => void;
}

export interface ViewEntry<S extends ZodType = ZodType> {
  readonly name: ComponentName;
  /** The one Zod schema for this component's data props, derived from the contract. */
  readonly schema: S;
  readonly render: (data: z.infer<S>, handlers: ViewHandlers) => ReactNode;
}

function entry<S extends ZodType>(name: ComponentName, schema: S, render: ViewEntry<S>['render']): ViewEntry<S> {
  return { name, schema, render };
}

export const VIEW_REGISTRY = {
  ContentCard: entry('ContentCard', ContentCardDataSchema, (d, h) => (
    <ContentCard {...d} onOpenPath={h.onOpenPath} onLoadMore={h.onLoadMore} />
  )),
  PinList: entry('PinList', PinListDataSchema, (d, h) => (
    <PinList {...d} onLoadMore={h.onLoadMore} onRequestUnpin={h.onRequestUnpin} />
  )),
  MfsTree: entry('MfsTree', MfsTreeDataSchema, (d, h) => (
    <MfsTree {...d} onOpenPath={h.onOpenPath} onLoadMore={h.onLoadMore} />
  )),
  DagViewer: entry('DagViewer', DagViewerDataSchema, (d) => <DagViewer {...d} />),
  IpnsRecordCard: entry('IpnsRecordCard', IpnsRecordCardDataSchema, (d) => <IpnsRecordCard {...d} />),
  NodeStatusPanel: entry('NodeStatusPanel', NodeStatusPanelDataSchema, (d, h) => (
    <NodeStatusPanel {...d} onLoadMore={h.onLoadMore} />
  )),
  ConfirmationDialog: entry('ConfirmationDialog', ConfirmationDialogDataSchema, (d, h) => (
    <ConfirmationDialog {...d} onApprove={h.onApprove} onDeny={h.onDeny} />
  )),
} as const;

export type ViewName = keyof typeof VIEW_REGISTRY;

export type ToolResultRender =
  | { readonly kind: 'view'; readonly component: ViewName; readonly node: ReactNode }
  | { readonly kind: 'invalid'; readonly component: ViewName; readonly issues: readonly string[] }
  | { readonly kind: 'unbound' };

/**
 * Render a tool's structured output with the view the contract binds to that tool. Output that does
 * not validate against the bound schema is reported, not rendered: the library never draws a result
 * the contract does not describe.
 */
export function renderToolResult(tool: ToolName, output: unknown, handlers: ViewHandlers = {}): ToolResultRender {
  const component = TOOL_META[tool].component;
  if (!component || !(component in VIEW_REGISTRY)) return { kind: 'unbound' };
  const view = VIEW_REGISTRY[component as ViewName] as ViewEntry;
  const parsed = view.schema.safeParse({ tool, output });
  if (!parsed.success) {
    return {
      kind: 'invalid',
      component: component as ViewName,
      issues: parsed.error.issues.slice(0, 5).map((i) => `${i.path.join('.') || '(root)'}: ${i.message}`),
    };
  }
  return { kind: 'view', component: component as ViewName, node: view.render(parsed.data, handlers) };
}
