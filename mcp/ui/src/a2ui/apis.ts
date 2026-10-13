// A2UI v0.9 component APIs: name plus a Zod (v3, the version @a2ui/web_core builds on) schema of
// the component's properties. The catalog manifest is generated from these, never hand-written.
//
// Data crosses the A2UI boundary as a data-model path: `output: { "path": "/pins" }` binds the
// component to the tool's structured output. The same object may be given as a literal. The React
// component then validates it against the contract's Zod schema before drawing anything.
import { CommonSchemas } from '@a2ui/web_core/v0_9';
import { z } from 'zod-v3';
import { COMPONENT_META, type ComponentName } from '../contract/generated/tool-meta';
import { VIEW_TOOLS } from '../contract/generated/bindings';

const { DataBinding, DynamicBoolean, DynamicString, DynamicStringList, Action } = CommonSchemas;

/**
 * Re-describe a shared A2UI type while keeping its `REF:` marker. @a2ui/web_core turns the marker into
 * a `$ref` to the shared definition when it writes the catalog document, and the binder reads it to
 * classify the property (Dynamic*, Action). A plain `.describe()` would replace the marker and inline
 * the whole definition (and its recursive FunctionCall) into every property.
 */
const like = <T extends z.ZodTypeAny>(schema: T, defName: string, description: string): T =>
  schema.describe(`REF:#/$defs/${defName}|${description}`);

const dataObject = (describes: string) =>
  z.union([z.record(z.string(), z.unknown()).describe(describes), DataBinding]).describe(describes);

const dataArray = (describes: string) =>
  z.union([z.array(z.record(z.string(), z.unknown())).describe(describes), DataBinding]).describe(describes);

const nonEmpty = <T extends string>(xs: readonly T[]): [T, ...T[]] => {
  const [first, ...rest] = xs;
  if (first === undefined) throw new Error('A2UI view component has no bound tools');
  return [first, ...rest];
};

const viewProps = (component: ComponentName) => ({
  tool: z.enum(nonEmpty(VIEW_TOOLS[component] ?? [])).describe('The contract tool whose structured output this view draws.'),
  output: dataObject('The structured output of the tool, as the contract output schema defines it. Usually a data model path.'),
});

const pagedAction = like(Action, 'Action', 'Dispatched when the person asks for the next page. Bind the cursor in the action context.').optional();

export interface IpfsComponentApi {
  readonly name: ComponentName;
  readonly schema: z.ZodObject<z.ZodRawShape>;
}

const api = (name: ComponentName, shape: z.ZodRawShape): IpfsComponentApi => ({
  name,
  schema: z.object({ ...shape, accessibility: CommonSchemas.AccessibilityAttributes.optional() }),
});

export const ContentCardApi = api('ContentCard', { ...viewProps('ContentCard'), loadMoreAction: pagedAction });
export const PinListApi = api('PinList', { ...viewProps('PinList'), loadMoreAction: pagedAction });
export const MfsTreeApi = api('MfsTree', { ...viewProps('MfsTree'), loadMoreAction: pagedAction });
export const DagViewerApi = api('DagViewer', viewProps('DagViewer'));
export const IpnsRecordCardApi = api('IpnsRecordCard', viewProps('IpnsRecordCard'));
export const NodeStatusPanelApi = api('NodeStatusPanel', { ...viewProps('NodeStatusPanel'), loadMoreAction: pagedAction });

export const ConfirmationDialogApi = api('ConfirmationDialog', {
  confirmation: dataObject('A ConfirmationRequired payload (ToolError.error.confirmation). When its requires_native_prompt is true the system prompt decides: no approve control is drawn.'),
  resolution: like(DynamicString, 'DynamicString', 'approved or denied, once decided on this surface.').optional(),
  approveAction: like(Action, 'Action', 'Dispatched on approve. Omit it when approval happens elsewhere: no approve control is drawn.').optional(),
  denyAction: like(Action, 'Action', 'Dispatched on deny.').optional(),
});

export const MessageStreamApi = api('MessageStream', {
  messages: dataArray('AG-UI messages: id, role (user, assistant, system), content.'),
});

export const ToolCallCardApi = api('ToolCallCard', {
  call: dataObject('One tool call: id, name, arguments (JSON text), optional result (JSON text), status.'),
  acknowledgedHosts: like(DynamicStringList, 'DynamicStringList', 'Hosts acknowledged for content egress.').optional(),
});

export const ApprovalCardApi = api('ApprovalCard', {
  interrupt: dataObject('The AG-UI interrupt from RUN_FINISHED, with metadata.ipfs-mcp.'),
  resolution: like(DynamicString, 'DynamicString', 'approved or denied, once the person answered in this chat.').optional(),
  expired: like(DynamicBoolean, 'DynamicBoolean', 'True when the interrupt expiry has passed. Expiry is denial.').optional(),
  approveAction: like(Action, 'Action', 'Dispatched on approve. Not drawn when the system prompt decides.').optional(),
  denyAction: like(Action, 'Action', 'Dispatched on deny.').optional(),
});

export const EgressBadgeApi = api('EgressBadge', {
  tool: like(DynamicString, 'DynamicString', 'The tool whose result this marks.'),
  acknowledgedHosts: like(DynamicStringList, 'DynamicStringList', 'Hosts in egress_ack.'),
  refused: like(DynamicBoolean, 'DynamicBoolean', 'True when the content was refused.').optional(),
});

export const COMPONENT_APIS: readonly IpfsComponentApi[] = [
  ContentCardApi,
  PinListApi,
  MfsTreeApi,
  DagViewerApi,
  IpnsRecordCardApi,
  ConfirmationDialogApi,
  NodeStatusPanelApi,
  MessageStreamApi,
  ToolCallCardApi,
  ApprovalCardApi,
  EgressBadgeApi,
];

/** The contract lists these components; the A2UI adapter must implement exactly them. */
export const CONTRACT_COMPONENT_NAMES: readonly ComponentName[] = COMPONENT_META.map((c) => c.name);
