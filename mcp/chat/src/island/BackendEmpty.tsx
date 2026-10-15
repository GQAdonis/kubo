import { Button } from '@gqadonis/ipfs-mcp-ui';

// The empty state for a node that answered 503 backend_not_configured: the chat is mounted but no model
// backend is set. The fix lives in the host's Settings, so the panel names it and links to it; it does
// not retry by itself and it never asks for a model here (the island talks only to /agui).

/** The Web UI route that holds the backend settings. A hash link: following it loads nothing and sends nothing. */
export const SETTINGS_HREF = '#/settings/agent';

export interface BackendEmptyProps {
  /** The message the node refused, if it was taken back, so it can be sent again once a backend exists. */
  readonly retryText: string | undefined;
  readonly onRetry: () => void;
  /** Closes the sidebar so the page behind it is visible after the link is followed. */
  readonly onNavigate: () => void;
}

export function BackendEmpty({ retryText, onRetry, onNavigate }: BackendEmptyProps) {
  return (
    <div className="flex flex-col gap-s2 px-s3 py-s3" data-testid="backend-empty">
      <h4 className="font-ui-heading text-[17px] leading-[1.3] font-semibold">Choose a backend to start</h4>
      <p role="status" className="max-w-[66ch] text-muted-foreground">
        The agent needs a backend before it can answer. Choose one in Settings.
        {retryText !== undefined ? ' Your message was not sent. It is kept below until you send it again.' : ''}
      </p>
      <div className="flex flex-wrap items-center gap-s2">
        <a className="iu-btn iu-btn--primary" href={SETTINGS_HREF} onClick={onNavigate}>
          Choose a backend
        </a>
        {retryText !== undefined ? (
          <Button variant="secondary" onClick={onRetry}>
            Send message again
          </Button>
        ) : null}
      </div>
      {retryText !== undefined ? (
        <p className="max-w-[66ch] break-words text-muted-foreground" data-testid="backend-empty-unsent">
          <strong className="font-semibold text-foreground">Not sent:</strong> {retryText}
        </p>
      ) : null}
    </div>
  );
}
