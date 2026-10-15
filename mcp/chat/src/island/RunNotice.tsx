import { Button, Notice, runErrorCopy, type ChatState, type Tone } from '@gqadonis/ipfs-mcp-ui';
import type { SessionError } from './session';

// The one place a failed run or a refused request is put into words. Everything shown is fixed copy
// chosen by a code: a RUN_ERROR code (the stream began and then failed), or the kind of an HTTP error
// (the request was refused before any stream). Nothing the server wrote, a RUN_ERROR message or an
// error body, is displayed.

export type NoticeTone = Exclude<Tone, 'neutral' | 'accent'>;

export interface NoticeModel {
  readonly tone: NoticeTone;
  readonly label: string;
  readonly text: string;
  /** The status badge word while this notice is showing. */
  readonly status: string;
  /** The refused message can be sent again. */
  readonly canRetry: boolean;
  /** Epoch ms before which a retry is disabled. */
  readonly retryNotBefore?: number | undefined;
}

export function noticeFor(error: SessionError | undefined, run: ChatState['run']): NoticeModel | undefined {
  if (error) {
    switch (error.kind) {
      case 'session_busy':
        return { tone: 'warn', label: 'Busy', text: error.message, status: 'Busy', canRetry: error.retryText !== undefined };
      case 'resource_exhausted':
        return {
          tone: 'warn',
          label: 'Busy',
          text: error.message,
          status: 'Busy',
          canRetry: error.retryText !== undefined,
          retryNotBefore: error.retryNotBefore,
        };
      case 'backend_not_configured':
        // Drawn as the panel's empty state (BackendEmpty), not as a failure notice.
        return undefined;
      case 'failed_precondition':
        return { tone: 'warn', label: 'Waiting', text: error.message, status: 'Waiting for approval', canRetry: false };
      default:
        return { tone: 'err', label: 'Chat failed', text: error.message, status: 'Failed', canRetry: false };
    }
  }
  if (run.status === 'error') {
    const copy = runErrorCopy(run.errorCode);
    const tone: NoticeTone = run.errorCode === 'backend_error' || run.errorCode === undefined ? 'err' : 'warn';
    return { tone, label: copy.label, text: copy.body, status: tone === 'err' ? 'Failed' : copy.label, canRetry: false };
  }
  return undefined;
}

export interface RunNoticeProps {
  readonly model: NoticeModel;
  /** The host clock, epoch ms, advanced when a retry hold ends. */
  readonly nowMs: number;
  readonly onRetry: () => void;
}

export function RunNotice({ model, nowMs, onRetry }: RunNoticeProps) {
  const held = model.retryNotBefore !== undefined && nowMs < model.retryNotBefore;
  return (
    <div role={model.tone === 'err' ? 'alert' : 'status'} className="flex flex-col items-start gap-s2 px-s3 pb-s2">
      <Notice tone={model.tone} label={model.label}>
        {model.text}
      </Notice>
      {model.canRetry ? (
        <Button variant="secondary" onClick={onRetry} disabled={held}>
          Send message again
        </Button>
      ) : null}
    </div>
  );
}
