import { useId, type MouseEvent } from 'react';
import { formatUtc } from '../lib/format';
import { displayText } from '../lib/safe-text';
import { IconAlert, IconClock, IconDenied, IconOk } from '../ui/icons';
import { Badge, Button } from '../ui/primitives';
import { PHASE_WORD, phaseSentence, type ApprovalModel } from './model';

/**
 * The activation behind a decision: the click on the Approve or Deny button. A pointer press or a key
 * press on the focused button arrives as a click whose isTrusted is true; element.click() and
 * dispatchEvent from script arrive with isTrusted false. Hosts that must tie an answer to a person
 * read isTrusted here. Handlers that ignore the argument keep working.
 */
export type ApprovalActivation = MouseEvent<HTMLButtonElement>;

export interface ApprovalPanelProps {
  model: ApprovalModel;
  /**
   * Decision handlers. They decide what this panel can do: with no onApprove there is no approve
   * control, and the text says where approval happens. The server still enforces confirmation.
   */
  onApprove?: (activation: ApprovalActivation) => void;
  onDeny?: (activation: ApprovalActivation) => void;
  /** Put focus on Deny when the panel mounts, so Enter cannot approve by accident. */
  focusSafeAction?: boolean;
  headingLevel?: 3 | 4;
  /** Use a dialog role (confirmation dialog) or a plain group (chat card). */
  variant: 'alertdialog' | 'group';
}

const PHASE_ICON = {
  pending: <IconClock />,
  waiting_system_prompt: <IconClock />,
  approved: <IconOk />,
  denied: <IconDenied />,
  expired: <IconAlert />,
} as const;

/**
 * One agent action and what was decided about it (the Promethity PolicyDecision pattern). Shared by
 * ConfirmationDialog and ApprovalCard so both tell the same truth in the same words.
 */
export function ApprovalPanel({
  model,
  onApprove,
  onDeny,
  focusSafeAction = false,
  headingLevel = 3,
  variant,
}: ApprovalPanelProps) {
  const titleId = useId();
  const statusId = useId();
  const Heading = `h${headingLevel}` as const;
  const phase = PHASE_WORD[model.phase];
  const canApprove = model.phase === 'pending' && onApprove !== undefined;
  const canDeny = (model.phase === 'pending' || model.phase === 'waiting_system_prompt') && onDeny !== undefined;
  const destructive = model.class === 'destructive';
  const sentence = phaseSentence(model, canApprove);

  return (
    <section
      className={`iu-approval iu-approval--${model.phase}`}
      role={variant === 'alertdialog' ? 'alertdialog' : 'group'}
      aria-modal={variant === 'alertdialog' ? 'false' : undefined}
      aria-labelledby={titleId}
      aria-describedby={statusId}
      data-phase={model.phase}
    >
      <div className="iu-approval__row">
        <Badge tone={phase.tone} icon={PHASE_ICON[model.phase]}>
          {phase.word}
        </Badge>
        <Badge tone={destructive ? 'err' : 'neutral'}>{destructive ? 'Destructive' : 'Write'}</Badge>
      </div>
      <Heading id={titleId} className="iu-approval__title">
        {displayText(model.summary)}
      </Heading>
      <p id={statusId} role="status" className="iu-approval__status">
        <strong>{phase.word}.</strong> {sentence}
      </p>
      <dl className="iu-approval__meta">
        <div>
          <dt>Tool</dt>
          <dd>{model.tool}</dd>
        </div>
        {model.targets.map((t) => (
          <div key={`${t.kind}:${t.value}`}>
            <dt>{t.kind.replace(/_/g, ' ')}</dt>
            <dd className="iu-wrap">{displayText(t.value)}</dd>
          </div>
        ))}
        {model.expiresAt ? (
          <div>
            <dt>Expires</dt>
            <dd>{formatUtc(model.expiresAt)}</dd>
          </div>
        ) : null}
        <div>
          <dt>Confirmation</dt>
          <dd>{model.confirmationId}</dd>
        </div>
      </dl>
      {canApprove || canDeny ? (
        <div className="iu-approval__actions" role="group" aria-label="Decision">
          {canDeny ? (
            <Button variant="secondary" onClick={onDeny} autoFocus={focusSafeAction}>
              Deny
            </Button>
          ) : null}
          {canApprove ? (
            <Button variant="primary" onClick={onApprove}>
              {destructive ? 'Approve removal' : 'Approve change'}
            </Button>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}
