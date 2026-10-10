import type { Outcome, ToolCall } from './result';

/** One screen: what was shown and the read-only call that can show it again. */
export interface Frame {
  readonly outcome: Outcome;
  /** Present when the screen came from a read-only call, so Refresh can repeat it. */
  readonly call: ToolCall | undefined;
}

export type Busy = 'idle' | 'loading-more' | 'opening' | 'refreshing';

/** What the person was last told about an action, in a live region. */
export interface Status {
  readonly tone: 'info' | 'err';
  readonly label: string;
  readonly text: string;
}

export interface SessionState {
  /** Navigation history, oldest first. Never empty. */
  readonly frames: readonly Frame[];
  readonly busy: Busy;
  readonly status: Status | undefined;
  /** Confirmation ids the person denied on this surface. */
  readonly denied: readonly string[];
}

export type SessionAction =
  | { readonly type: 'busy'; readonly busy: Busy }
  | { readonly type: 'push'; readonly frame: Frame }
  | { readonly type: 'replace'; readonly frame: Frame }
  | { readonly type: 'back' }
  | { readonly type: 'status'; readonly status: Status | undefined }
  | { readonly type: 'deny'; readonly confirmationId: string };

export function initialSession(frame: Frame): SessionState {
  return { frames: [frame], busy: 'idle', status: undefined, denied: [] };
}

export function topFrame(state: SessionState): Frame {
  return state.frames[state.frames.length - 1] as Frame; // never empty: initialSession seeds one and back keeps one
}

export function sessionReducer(state: SessionState, action: SessionAction): SessionState {
  switch (action.type) {
    case 'busy':
      return { ...state, busy: action.busy, status: action.busy === 'idle' ? state.status : undefined };
    case 'push':
      return { ...state, frames: [...state.frames, action.frame], busy: 'idle', status: undefined };
    case 'replace':
      return { ...state, frames: [...state.frames.slice(0, -1), action.frame], busy: 'idle', status: undefined };
    case 'back':
      return state.frames.length > 1
        ? { ...state, frames: state.frames.slice(0, -1), status: undefined }
        : state;
    case 'status':
      return { ...state, busy: 'idle', status: action.status };
    case 'deny':
      return state.denied.includes(action.confirmationId)
        ? state
        : { ...state, denied: [...state.denied, action.confirmationId] };
  }
}
