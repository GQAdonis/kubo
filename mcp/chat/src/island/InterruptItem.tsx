import { createContext, use, useMemo, useState } from 'react';
import { ApprovalCard, Button, Notice, interruptExpired, type ChatInterrupt } from '@gqadonis/ipfs-mcp-ui';
import { createGestureGate } from '../agui/gesture';

/**
 * How the island decides an activation came from a person. The default reads Event.isTrusted. A test
 * that must drive the card with synthetic events (jsdom marks every event untrusted) provides another
 * value around the island; the custom element never does, so page script has no way to.
 */
export const TrustProbeContext = createContext<(event: { readonly isTrusted: boolean }) => boolean>((event) => event.isTrusted === true);

// One pending approval in the transcript. This is where G11-3 is enforced for the UI: an answer is
// accepted only when the activation that caused it was a real pointer or key event (isTrusted), on the
// control that matches the answer. A scripted click, a synthetic event or a direct call to the handler
// has no trusted activation behind it and is refused, with a visible reason.
//
// Native-prompt interrupts (requires_native_prompt) get Deny only. The card draws no Approve control
// for them, and the session refuses an approve for them regardless of what called it (docs/mcp/0003,
// Amendment item 4). While the session watches the system prompt, Deny stays live: it stops the watch
// and sends the denial.

export interface InterruptItemProps {
  readonly item: ChatInterrupt;
  /** The host clock as an ISO string, advanced when an interrupt's expiry passes. */
  readonly nowIso: string;
  /** Resolve. The island passes the session; tests may pass a stub. */
  readonly onAnswer: (interruptId: string, decision: 'approve' | 'deny') => void;
  /** Disabled while a run is in flight, except Deny on a call whose system prompt is being watched. */
  readonly busy: boolean;
  /** The session is waiting on the system prompt for this interrupt. */
  readonly observing: boolean;
  /** Offered when the watch stopped or dropped; asks the session to watch the system prompt again. */
  readonly onWatchAgain?: (interruptId: string) => void;
}

export function InterruptItem({ item, nowIso, onAnswer, busy, observing, onWatchAgain }: InterruptItemProps) {
  const { interrupt, resolution } = item;
  const gate = useMemo(() => createGestureGate(), []);
  const probe = use(TrustProbeContext);
  const [refused, setRefused] = useState(false);
  const expired = resolution === undefined && interruptExpired(interrupt, nowIso);
  const open = resolution === undefined && !expired;
  const native = interrupt.metadata['ipfs-mcp'].requires_native_prompt;
  const interactive = open && (!busy || observing);

  const answer = (decision: 'approve' | 'deny') => {
    if (!gate.consume(decision)) {
      setRefused(true);
      return;
    }
    setRefused(false);
    onAnswer(interrupt.id, decision);
  };

  return (
    <div
      className="flex flex-col gap-s2"
      // Capture phase: runs before the button's own handler, so the gate knows what activated it.
      onClickCapture={(event) => gate.record({ isTrusted: probe(event.nativeEvent), target: event.target })}
    >
      {/* Initial focus lands on Deny, the safe action (DESIGN.md). */}
      <ApprovalCard
        interrupt={interrupt}
        {...(resolution && { resolution })}
        expired={expired}
        focusSafeAction
        {...(interactive && { onApprove: () => answer('approve'), onDeny: () => answer('deny') })}
      />
      {native && open && !busy && onWatchAgain ? (
        <div>
          <Button variant="text" onClick={() => onWatchAgain(interrupt.id)}>
            Check the system prompt again
          </Button>
        </div>
      ) : null}
      {refused ? (
        <div role="alert">
          <Notice tone="err" label="Not answered">
            An answer needs a click or key press from you on the button itself.
          </Notice>
        </div>
      ) : null}
    </div>
  );
}
