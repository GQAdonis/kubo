import type { ToolError } from '../../contract/generated/common';
import { displayText } from '../../lib/safe-text';
import { UiScope } from '../../scope/UiScope';
import { Badge } from '../../ui/primitives';

export type ToolErrorNoticeProps = { error: ToolError };

// Confirmation states are shown by the approval panel, not as failures.
const NOT_FAILURES = new Set(['confirmation_required', 'confirmation_pending']);

export function isFailure(error: ToolError): boolean {
  return !NOT_FAILURES.has(error.error.code);
}

/** A tool failure: the server's message, the error code, and whether a retry can help. */
export function ToolErrorNotice({ error }: ToolErrorNoticeProps) {
  const e = error.error;
  return (
    <UiScope>
      <div className="iu-notice iu-notice--err" role="group" aria-label="Tool error">
        <p>
          <strong className="iu-notice__label">Failed.</strong> {displayText(e.message)}
        </p>
        <p className="iu-tool">
          <Badge tone="err">{e.code}</Badge>
          {e.tool ? <> {e.tool}</> : null} {e.retryable ? 'The same call may succeed later.' : 'Retrying the same call will not help.'}
        </p>
      </div>
    </UiScope>
  );
}
