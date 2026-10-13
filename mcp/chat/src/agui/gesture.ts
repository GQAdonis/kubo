// Interrupt answers need a user gesture (ADR-0005; falsifier G11-3). The signal is Event.isTrusted:
// the browser sets it only for events caused by a real pointer or key, and a script cannot forge it.
// element.click() and dispatchEvent(new MouseEvent('click')) make events with isTrusted false.
//
// @gqadonis/ipfs-mcp-ui's ApprovalCard hands its click to onApprove and onDeny, but the island does not
// rely on that argument: a script that obtained the handler could pass any object. So the island
// listens in the capture phase on the card, which runs before the button's own handler, and records
// whether the activation that is about to call the handler was trusted and which decision control it
// hit. The handler then consumes that record exactly once. A call with no fresh trusted record is
// refused: that covers a scripted click, a script that obtains the handler and calls it directly, and a
// trusted click on some other part of the card followed by a scripted call.

export type Decision = 'approve' | 'deny';

export interface GestureRecord {
  readonly decision: Decision | 'other';
  readonly trusted: boolean;
  readonly at: number;
}

/** A recorded gesture is good for this long: one event-loop turn plus slack for slow input handling. */
export const GESTURE_WINDOW_MS = 400;

/** Classify the control an activation hit, from the ApprovalPanel markup. Primary is Approve, secondary is Deny. */
export function decisionOf(target: EventTarget | null): Decision | 'other' {
  if (!(target instanceof Element)) return 'other';
  const button = target.closest('.iu-approval__actions button');
  if (!button) return 'other';
  return button.classList.contains('iu-btn--primary') ? 'approve' : 'deny';
}

export interface GestureGate {
  /** Call from onClickCapture on the card. */
  record(event: { readonly isTrusted: boolean; readonly target: EventTarget | null }): void;
  /** Call from the decision handler. True once per trusted activation of that control. */
  consume(decision: Decision): boolean;
}

export function createGestureGate(now: () => number = () => performance.now()): GestureGate {
  let last: GestureRecord | undefined;
  return {
    record(event) {
      last = { decision: decisionOf(event.target), trusted: event.isTrusted === true, at: now() };
    },
    consume(decision) {
      const g = last;
      last = undefined; // one activation answers once, whether or not it was accepted
      return g !== undefined && g.trusted && g.decision === decision && now() - g.at <= GESTURE_WINDOW_MS;
    },
  };
}
