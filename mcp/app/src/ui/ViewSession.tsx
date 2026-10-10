import { Button, Notice, type ViewHandlers } from '@gqadonis/ipfs-mcp-ui';
import { useEffect, useReducer, useRef } from 'react';
import { canOpenPaths, openPathCall } from '../core/navigate';
import { canPage, mergePage, nextPageCall } from '../core/pages';
import { classifyResult, isReadOnlyTool, type Outcome, type ToolCall, type ToolResultLike } from '../core/result';
import { initialSession, sessionReducer, topFrame, type Busy, type Frame, type Status } from '../core/session';
import type { HostPort } from '../host/port';
import type { ViewId } from '../views';
import { OutcomeView } from './OutcomeView';

export interface ViewSessionProps {
  readonly view: ViewId;
  readonly port: HostPort;
  /** The arguments of the call that produced the result: what Refresh repeats. */
  readonly input: Readonly<Record<string, unknown>> | undefined;
  readonly result: ToolResultLike;
  /** hostContext.toolInfo.tool.name, when the host supplies it. */
  readonly toolInfoName: string | undefined;
}

function firstFrame({ view, input, result, toolInfoName }: Omit<ViewSessionProps, 'port'>): Frame {
  const outcome = classifyResult(result, { view, toolInfoName });
  const call: ToolCall | undefined =
    outcome.kind === 'result' && isReadOnlyTool(outcome.tool) ? { tool: outcome.tool, args: input ?? {} } : undefined;
  return { outcome, call };
}

const BUSY_TEXT: Readonly<Record<Exclude<Busy, 'idle'>, string>> = {
  'loading-more': 'Loading the next page.',
  opening: 'Opening.',
  refreshing: 'Refreshing.',
};

function messageOf(error: unknown): string {
  return error instanceof Error && error.message !== '' ? error.message : 'The host did not answer.';
}

/**
 * The interactive part of a view: navigation history, pagination, refresh and the two requests a
 * view can make of the agent. One session per tool result (the shell keys it), so a new result from
 * the host starts a clean history.
 *
 * Every call goes through the HostPort and is read-only by construction: exec refuses any tool whose
 * confirmation class is not none.
 */
export function ViewSession(props: ViewSessionProps) {
  const { view, port } = props;
  const [state, dispatch] = useReducer(sessionReducer, props, (p) => initialSession(firstFrame(p)));
  const inflight = useRef<AbortController | null>(null);
  useEffect(() => () => inflight.current?.abort(), []);

  const top = topFrame(state);

  /** Run one read-only call. Returns the classified outcome, or undefined when it was refused or superseded. */
  async function exec(call: ToolCall, busy: Busy): Promise<Outcome | undefined> {
    if (!isReadOnlyTool(call.tool)) {
      dispatch({ type: 'status', status: { tone: 'err', label: 'Refused', text: `This view does not call ${call.tool}; it changes data.` } });
      return undefined;
    }
    inflight.current?.abort();
    const ctl = new AbortController();
    inflight.current = ctl;
    dispatch({ type: 'busy', busy });
    try {
      const res = await port.callTool(call, ctl.signal);
      if (ctl.signal.aborted) return undefined;
      return classifyResult(res, { view, toolInfoName: call.tool });
    } catch (error: unknown) {
      if (!ctl.signal.aborted) dispatch({ type: 'status', status: { tone: 'err', label: 'Not completed', text: messageOf(error) } });
      return undefined;
    }
  }

  function failed(outcome: Outcome): Status {
    if (outcome.kind === 'failure') return { tone: 'err', label: 'Failed', text: outcome.error.error.message };
    if (outcome.kind === 'confirmation') return { tone: 'err', label: 'Not run', text: 'This read needs a confirmation it should not need.' };
    return { tone: 'err', label: 'Not drawn', text: 'The server answered with content this view cannot show.' };
  }

  async function loadMore(cursor: string): Promise<void> {
    if (top.outcome.kind !== 'result' || !top.call || !canPage(top.outcome.tool)) return;
    const outcome = await exec(nextPageCall(top.call, cursor), 'loading-more');
    if (!outcome) return;
    if (outcome.kind !== 'result' || outcome.tool !== top.outcome.tool) {
      dispatch({ type: 'status', status: failed(outcome) });
      return;
    }
    const merged = mergePage(top.outcome.tool, top.outcome.output, outcome.output);
    if (merged === undefined) {
      dispatch({ type: 'status', status: { tone: 'err', label: 'Not added', text: 'The next page did not match the contract, so nothing was added.' } });
      return;
    }
    dispatch({ type: 'replace', frame: { outcome: { kind: 'result', tool: top.outcome.tool, output: merged }, call: top.call } });
  }

  async function openPath(path: string): Promise<void> {
    if (top.outcome.kind !== 'result') return;
    const call = openPathCall(top.outcome.tool, top.outcome.output, path);
    if (!call) return;
    const outcome = await exec(call, 'opening');
    if (outcome) dispatch({ type: 'push', frame: { outcome, call } });
  }

  async function refresh(): Promise<void> {
    if (!top.call) return;
    const outcome = await exec(top.call, 'refreshing');
    if (outcome) dispatch({ type: 'replace', frame: { outcome, call: top.call } });
  }

  async function requestUnpin(cid: string): Promise<void> {
    try {
      await port.sendMessage(`Please remove the pin for ${cid} from my node. I will approve it when asked.`);
      dispatch({ type: 'status', status: { tone: 'info', label: 'Requested', text: `Asked your agent to remove the pin for ${cid}. A person still approves it first.` } });
    } catch (error: unknown) {
      dispatch({ type: 'status', status: { tone: 'err', label: 'Not sent', text: messageOf(error) } });
    }
  }

  async function deny(): Promise<void> {
    if (top.outcome.kind !== 'confirmation') return;
    const { confirmation_id: id, summary } = top.outcome.confirmation;
    dispatch({ type: 'deny', confirmationId: id });
    try {
      await port.updateModelContext(`The person denied confirmation ${id} (${summary.tool}) in the IPFS view. Do not repeat that call.`);
    } catch (error: unknown) {
      dispatch({ type: 'status', status: { tone: 'err', label: 'Agent not told', text: `Denied here, but the agent was not told: ${messageOf(error)}` } });
    }
  }

  const tool = top.outcome.kind === 'result' ? top.outcome.tool : undefined;
  const handlers: ViewHandlers = {
    ...(port.canCallTools && tool && canOpenPaths(tool) ? { onOpenPath: (p: string) => void openPath(p) } : {}),
    ...(port.canCallTools && tool && canPage(tool) && top.call ? { onLoadMore: (c: string) => void loadMore(c) } : {}),
    ...(port.canMessage && tool === 'pin_ls' ? { onRequestUnpin: (c: string) => void requestUnpin(c) } : {}),
    ...(port.canUpdateContext && top.outcome.kind === 'confirmation' ? { onDeny: () => void deny() } : {}),
  };

  const confirmationId = top.outcome.kind === 'confirmation' ? top.outcome.confirmation.confirmation_id : undefined;
  const canBack = state.frames.length > 1;
  const canRefresh = port.canCallTools && top.call !== undefined;
  const busyText = state.busy === 'idle' ? '' : BUSY_TEXT[state.busy];

  return (
    <div className="app-body" aria-busy={state.busy !== 'idle'}>
      <OutcomeView outcome={top.outcome} handlers={handlers} denied={confirmationId !== undefined && state.denied.includes(confirmationId)} />
      {canBack || canRefresh ? (
        <div className="app-bar" role="group" aria-label="View actions">
          {canBack ? (
            <Button variant="secondary" disabled={state.busy !== 'idle'} onClick={() => dispatch({ type: 'back' })}>
              Back to previous
            </Button>
          ) : null}
          {canRefresh ? (
            <Button variant="secondary" disabled={state.busy !== 'idle'} onClick={() => void refresh()}>
              Refresh
            </Button>
          ) : null}
        </div>
      ) : null}
      <div className="app-status" role="status" aria-live="polite" aria-atomic="true">
        {busyText !== '' ? <p className="iu-muted">{busyText}</p> : null}
        {state.status ? (
          <Notice tone={state.status.tone} label={state.status.label}>
            {state.status.text}
          </Notice>
        ) : null}
      </div>
    </div>
  );
}
