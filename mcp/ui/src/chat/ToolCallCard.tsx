import type { ReactNode } from 'react';
import { TOOL_META, type ToolName } from '../contract/generated/tool-meta';
import { JsonTree } from '../components/dag-viewer/JsonTree';
import { renderToolResult, type ViewHandlers } from '../components/registry';
import { ConfirmationDialog } from '../components/confirmation-dialog/ConfirmationDialog';
import { ToolErrorNotice, isFailure } from '../components/tool-error/ToolErrorNotice';
import { displayText } from '../lib/safe-text';
import { UiScope } from '../scope/UiScope';
import { Badge, Notice, type Tone } from '../ui/primitives';
import { ChatToolCallWireSchema, parseJson, parseToolError, type ChatToolCallWire } from './agui';
import { EgressBadge } from './EgressBadge';
import { chatElementsFor } from './rules';

export const ToolCallCardDataSchema = ChatToolCallWireSchema;
export type ToolCallCardData = ChatToolCallWire;

export type ToolCallCardProps = {
  call: ToolCallCardData;
  /** Hosts acknowledged for content egress, shown on tools that carry node content. */
  acknowledgedHosts?: readonly string[];
  /** Handlers passed to the embedded view and to a confirmation shown in place of a result. */
  handlers?: ViewHandlers;
};

const STATUS: Record<ToolCallCardData['status'], { word: string; tone: Tone }> = {
  running: { word: 'Running', tone: 'info' },
  interrupted: { word: 'Waiting for approval', tone: 'warn' },
  done: { word: 'Done', tone: 'ok' },
  error: { word: 'Failed', tone: 'err' },
};

const CLASS_WORD = { none: 'Read only', write: 'Write', destructive: 'Destructive' } as const;
const CLASS_TONE = { none: 'neutral', write: 'neutral', destructive: 'err' } as const;

const isToolName = (name: string): name is ToolName => name in TOOL_META;

/**
 * One tool call: what was asked, whether it ran, and what came back. When the contract binds the tool
 * to a view and the result validates against that tool's output schema, the view is embedded; any
 * other result is shown as data, never as something it is not. The card is not itself a panel, so a
 * view's panel is never nested inside another card.
 */
export function ToolCallCard({ call, acknowledgedHosts = [], handlers = {} }: ToolCallCardProps) {
  const known = isToolName(call.name);
  const meta = known ? TOOL_META[call.name as ToolName] : undefined;
  const args = parseJson(call.arguments);
  const error = call.result !== undefined ? parseToolError(call.result) : null;
  const status = STATUS[call.status];

  return (
    <UiScope>
      <article className="iu-call" aria-label={`Tool call ${call.name}`}>
        <header className="iu-call__head">
          <h4 className="iu-call__title">{meta?.title ?? displayText(call.name)}</h4>
          <code className="iu-tool">{displayText(call.name)}</code>
          <Badge tone={meta ? CLASS_TONE[meta.confirmation] : 'neutral'}>
            {meta ? CLASS_WORD[meta.confirmation] : 'Unknown tool'}
          </Badge>
          <Badge tone={status.tone}>{status.word}</Badge>
        </header>
        <details className="iu-call__args">
          <summary>Arguments</summary>
          {args !== undefined && args !== null && typeof args === 'object' ? (
            <JsonTree value={args} label={`Arguments of ${call.name}`} />
          ) : (
            <pre className="iu-pre" tabIndex={0} aria-label="Arguments text">
              {displayText(call.arguments || '(none)')}
            </pre>
          )}
        </details>
        {call.status === 'interrupted' && call.result === undefined ? (
          <p className="iu-muted">Nothing has run. This call waits for a person to approve it.</p>
        ) : null}
        {call.status === 'running' && call.result === undefined ? <p className="iu-muted">Running on your node.</p> : null}
        {call.result !== undefined ? (
          <Result
            call={call}
            toolName={known ? (call.name as ToolName) : undefined}
            error={error}
            acknowledgedHosts={acknowledgedHosts}
            handlers={handlers}
          />
        ) : null}
        {meta && call.status !== 'error' && chatElementsFor(meta.name).egressBadge ? (
          <EgressBadge tool={meta.name} acknowledged_hosts={[...acknowledgedHosts]} />
        ) : null}
      </article>
    </UiScope>
  );
}

function Result({
  call,
  toolName,
  error,
  acknowledgedHosts,
  handlers,
}: {
  call: ToolCallCardData;
  toolName: ToolName | undefined;
  error: ReturnType<typeof parseToolError>;
  acknowledgedHosts: readonly string[];
  handlers: ViewHandlers;
}): ReactNode {
  if (error) {
    const confirmation = error.error.confirmation;
    return (
      <>
        {confirmation ? (
          <ConfirmationDialog confirmation={confirmation} onApprove={handlers.onApprove} onDeny={handlers.onDeny} />
        ) : null}
        {isFailure(error) ? <ToolErrorNotice error={error} /> : null}
        {error.error.code === 'egress_not_acknowledged' && toolName ? (
          <EgressBadge tool={toolName} acknowledged_hosts={[...acknowledgedHosts]} refused />
        ) : null}
      </>
    );
  }
  const output = parseJson(call.result ?? '');
  if (output === undefined) {
    return (
      <Notice tone="warn" label="Unreadable result">
        The result is not valid JSON, so it is shown as text.
        <pre className="iu-pre" tabIndex={0} aria-label="Result text">
          {displayText(call.result ?? '')}
        </pre>
      </Notice>
    );
  }
  if (toolName) {
    const rendered = renderToolResult(toolName, output, handlers);
    if (rendered.kind === 'view') return rendered.node;
    if (rendered.kind === 'invalid') {
      return (
        <>
          <Notice tone="warn" label="Result does not match the contract">
            {toolName} returned data its output schema does not describe, so no view is drawn. {rendered.issues.join('; ')}
          </Notice>
          <JsonTree value={output} label={`Raw result of ${call.name}`} />
        </>
      );
    }
  }
  return <JsonTree value={output} label={`Result of ${call.name}`} />;
}
