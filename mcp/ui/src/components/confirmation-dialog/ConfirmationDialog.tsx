import { z } from 'zod';
import { ApprovalPanel } from '../../approval/ApprovalPanel';
import type { ApprovalModel, ApprovalPhase } from '../../approval/model';
import { ConfirmationRequiredSchema, type ConfirmationRequired } from '../../contract/generated/confirmation';
import { UiScope } from '../../scope/UiScope';

/**
 * Data for the confirmation dialog: the contract's ConfirmationRequired payload (carried in
 * ToolError.error.confirmation), which itself says through requires_native_prompt whether the system
 * prompt decides, plus the decision already made on this surface.
 */
export const ConfirmationDialogDataSchema = z.strictObject({
  confirmation: ConfirmationRequiredSchema,
  /** A decision already made on this surface. Without it the phase follows confirmation.state. */
  resolution: z.enum(['approved', 'denied']).optional(),
});
export type ConfirmationDialogData = z.infer<typeof ConfirmationDialogDataSchema>;

export type ConfirmationDialogProps = ConfirmationDialogData & {
  /**
   * Handlers make the dialog answerable. When approval happens elsewhere (an OS prompt, another
   * principal) pass neither: the dialog then shows no approve control and says where approval happens.
   */
  onApprove?: () => void;
  onDeny?: () => void;
};

export function approvalModelFromConfirmation(data: ConfirmationDialogData): ApprovalModel {
  const c: ConfirmationRequired = data.confirmation;
  const native = c.requires_native_prompt === true;
  let phase: ApprovalPhase = c.state;
  if (data.resolution === 'denied') phase = 'denied';
  else if (native && c.state === 'pending') phase = 'waiting_system_prompt'; // a click here never approves
  else if (data.resolution) phase = data.resolution;
  return {
    confirmationId: c.confirmation_id,
    tool: c.summary.tool,
    class: c.summary.class,
    summary: c.summary.text,
    targets: c.summary.targets,
    expiresAt: c.expires_at,
    resolvers: c.resolvers,
    phase,
  };
}

/** Renders a confirmation Summary for surfaces that can elicit a decision. */
export function ConfirmationDialog({ onApprove, onDeny, ...data }: ConfirmationDialogProps) {
  const model = approvalModelFromConfirmation(data);
  return (
    <UiScope>
      <ApprovalPanel
        model={model}
        onApprove={onApprove}
        onDeny={onDeny}
        focusSafeAction
        variant="alertdialog"
      />
    </UiScope>
  );
}
