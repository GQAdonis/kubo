import {
  ConfirmationDialog,
  Notice,
  Panel,
  ToolErrorNotice,
  UntrustedFrame,
  displayText,
  renderToolResult,
  type ViewHandlers,
} from '@gqadonis/ipfs-mcp-ui';
import type { Outcome } from '../core/result';

export interface OutcomeViewProps {
  readonly outcome: Outcome;
  readonly handlers: ViewHandlers;
  /** True once the person denied this confirmation on this surface. */
  readonly denied: boolean;
}

/**
 * Draws one outcome with the library component the contract binds to it. This is the only place
 * results turn into pixels, and it draws nothing the contract does not describe: output that fails
 * its schema is reported with the reasons, not rendered.
 */
export function OutcomeView({ outcome, handlers, denied }: OutcomeViewProps) {
  switch (outcome.kind) {
    case 'result': {
      const drawn = renderToolResult(outcome.tool, outcome.output, handlers);
      if (drawn.kind === 'view') return <>{drawn.node}</>;
      if (drawn.kind === 'invalid') {
        return (
          <Notice tone="err" label="Not drawn">
            The result from <code>{outcome.tool}</code> does not match its contract, so this view does not draw it.{' '}
            {drawn.issues.map(displayText).join('; ')}
          </Notice>
        );
      }
      return (
        <Notice tone="info" label="No view">
          <code>{outcome.tool}</code> has no view. Its result is in the conversation.
        </Notice>
      );
    }
    case 'confirmation':
      return (
        <ConfirmationDialog
          confirmation={outcome.confirmation}
          {...(denied ? { resolution: 'denied' as const } : {})}
          {...(handlers.onDeny ? { onDeny: handlers.onDeny } : {})}
        />
      );
    case 'failure':
      return <ToolErrorNotice error={outcome.error} />;
    case 'unidentified':
      return (
        <Panel title="Result not drawn">
          <Notice tone="warn" label="Not drawn">
            {outcome.issues.join(' ')} The text of the result follows.
          </Notice>
          {outcome.text !== '' ? (
            <UntrustedFrame>
              <pre className="app-text">{displayText(outcome.text)}</pre>
            </UntrustedFrame>
          ) : null}
        </Panel>
      );
    case 'text':
      return outcome.untrusted ? (
        <UntrustedFrame>
          <pre className="app-text">{displayText(outcome.text)}</pre>
        </UntrustedFrame>
      ) : (
        <pre className="app-text">{displayText(outcome.text)}</pre>
      );
    case 'empty':
      return (
        <Notice tone="info" label="Empty">
          The tool returned no content.
        </Notice>
      );
  }
}
