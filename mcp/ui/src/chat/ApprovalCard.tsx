import { z } from 'zod';
import { ApprovalPanel, type ApprovalActivation } from '../approval/ApprovalPanel';
import type { ApprovalModel, ApprovalPhase } from '../approval/model';
import { AguiInterruptSchema, type AguiInterrupt } from '../contract/generated/agui';
import { UiScope } from '../scope/UiScope';

export const ApprovalCardDataSchema = z.strictObject({
  /** The AG-UI interrupt exactly as RUN_FINISHED carries it. */
  interrupt: AguiInterruptSchema,
  /** The user's answer in this chat, once given. */
  resolution: z.enum(['approved', 'denied', 'expired']).optional(),
  /** Set by the host from its clock when expiresAt has passed. The card never reads the clock. */
  expired: z.boolean().optional(),
});
export type ApprovalCardData = z.infer<typeof ApprovalCardDataSchema>;

export type ApprovalCardProps = ApprovalCardData & {
  /**
   * Resume the run with approved: true. Never rendered as a control when the system prompt decides.
   * Receives the click that caused it; read activation.isTrusted to require a real pointer or key press.
   */
  onApprove?: (activation: ApprovalActivation) => void;
  /**
   * Deny. Denial is always safe, so it stays available while waiting for the system prompt. Receives
   * the click that caused it, like onApprove.
   */
  onDeny?: (activation: ApprovalActivation) => void;
  /**
   * Put focus on Deny when the card mounts, so Enter cannot approve by accident. Off by default: a
   * transcript that holds the reader's place should not have focus pulled by every card.
   */
  focusSafeAction?: boolean;
};

/**
 * Pure: turn an interrupt and what is known about it into the model the panel renders. The server's
 * message is shown as sent; for a native-prompt call it already says to confirm in the system prompt.
 * requires_native_prompt comes from the interrupt metadata and is the only thing that removes Approve.
 */
export function approvalModelFromInterrupt(data: ApprovalCardData): ApprovalModel {
  const meta = data.interrupt.metadata['ipfs-mcp'];
  const native = meta.requires_native_prompt;
  let phase: ApprovalPhase;
  if (data.resolution === 'denied') phase = 'denied';
  else if (data.expired || data.resolution === 'expired') phase = 'expired';
  else if (data.resolution === 'approved') phase = 'approved';
  else phase = native ? 'waiting_system_prompt' : 'pending';
  return {
    confirmationId: meta.confirmation_id,
    tool: meta.tool,
    class: meta.class,
    summary: data.interrupt.message,
    targets: [],
    ...(data.interrupt.expiresAt && { expiresAt: data.interrupt.expiresAt }),
    resolvers: native ? [] : ['agui-interrupt'],
    phase,
  };
}

/** True when the interrupt's expiresAt is at or before `nowIso`. Pass the host clock in. */
export function interruptExpired(interrupt: AguiInterrupt, nowIso: string): boolean {
  if (!interrupt.expiresAt) return false;
  return Date.parse(interrupt.expiresAt) <= Date.parse(nowIso);
}

/**
 * The interrupt card of the agent chat: pending, approved, denied, expired, or waiting for the
 * system prompt. It offers Approve only when this chat can answer; when the native prompt decides,
 * it says so and offers no approve control.
 */
export function ApprovalCard({ onApprove, onDeny, focusSafeAction, ...data }: ApprovalCardProps) {
  const model = approvalModelFromInterrupt(data);
  return (
    <UiScope>
      <ApprovalPanel
        model={model}
        onApprove={onApprove}
        onDeny={onDeny}
        {...(focusSafeAction !== undefined && { focusSafeAction })}
        variant="group"
        headingLevel={4}
      />
    </UiScope>
  );
}
