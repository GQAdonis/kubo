import { z } from 'zod';
import { ConfirmationClassSchema, ResolverSchema, TargetSchema } from '../contract/generated/confirmation';
import type { ConfirmationClass, Resolver, Target } from '../contract/generated/confirmation';

/**
 * The five confirmation states the UI can show. They are the contract's states (pending, approved,
 * denied, expired) plus the one the contract expresses as requires_native_prompt: the chat answer
 * alone does not approve, the system prompt does.
 */
export const APPROVAL_PHASES = ['pending', 'waiting_system_prompt', 'approved', 'denied', 'expired'] as const;
export const ApprovalPhaseSchema = z.enum(APPROVAL_PHASES);
export type ApprovalPhase = z.infer<typeof ApprovalPhaseSchema>;

/** What both the confirmation dialog and the chat approval card render. */
export const ApprovalModelSchema = z.strictObject({
  confirmationId: z.string().min(1),
  tool: z.string().min(1),
  class: ConfirmationClassSchema,
  /** Rendered from the catalog confirmationSummary template by the server. */
  summary: z.string().min(1),
  targets: z.array(TargetSchema),
  expiresAt: z.iso.datetime({ offset: true }).optional(),
  resolvers: z.array(ResolverSchema),
  phase: ApprovalPhaseSchema,
});
export type ApprovalModel = z.infer<typeof ApprovalModelSchema>;

export type { ConfirmationClass, Resolver, Target };

/**
 * The Approve button names the action it approves, so a person who tabs to it hears what will run
 * ("Approve pin move"), not a generic "Approve change". Keyed by the catalog tools that need a
 * confirmation; a tool not listed falls back to the class wording.
 */
export const APPROVE_LABEL: Readonly<Record<string, string>> = {
  ipfs_add: 'Approve add',
  pin_add: 'Approve pin',
  pin_update: 'Approve pin move',
  pin_rm: 'Approve pin removal',
  mfs_write: 'Approve file write',
  mfs_mkdir: 'Approve new folder',
  mfs_cp: 'Approve copy',
  mfs_mv: 'Approve move',
  mfs_rm: 'Approve MFS removal',
  ipns_publish: 'Approve IPNS publish',
  key_gen: 'Approve new key',
  key_rename: 'Approve key rename',
  key_rm: 'Approve key removal',
  dag_put: 'Approve DAG store',
  car_import: 'Approve CAR import',
  routing_provide: 'Approve announce',
  repo_gc: 'Approve garbage collection',
  ipfs_index_cid: 'Approve indexing',
  ipfs_index_reconcile: 'Approve index cleanup',
};

export function approveLabel(tool: string, cls: ConfirmationClass): string {
  return APPROVE_LABEL[tool] ?? (cls === 'destructive' ? 'Approve removal' : 'Approve change');
}

/** Where a person approves, in words. Resolver ids are from the contract's Resolver enum. */
export const RESOLVER_PLACE: Record<Resolver, string> = {
  elicitation: "your agent host's confirmation prompt",
  'agui-interrupt': 'the chat',
  desktop: 'the IPFS Desktop confirmation window',
  'macos-localauth': 'the macOS system prompt',
  'linux-polkit': 'the system authentication prompt',
  cli: 'the terminal where ipfs mcp runs',
};

export function placeOf(resolvers: readonly Resolver[]): string {
  const places = resolvers.map((r) => RESOLVER_PLACE[r]);
  if (places.length === 0) return 'a prompt outside this panel';
  if (places.length === 1) return places[0] as string;
  return `${places.slice(0, -1).join(', ')} or ${places[places.length - 1]}`;
}

export interface PhaseCopy {
  readonly word: string;
  readonly tone: 'warn' | 'ok' | 'err';
}

export const PHASE_WORD: Record<ApprovalPhase, PhaseCopy> = {
  pending: { word: 'Awaiting a person', tone: 'warn' },
  waiting_system_prompt: { word: 'Waiting for system prompt', tone: 'warn' },
  approved: { word: 'Approved', tone: 'ok' },
  denied: { word: 'Denied', tone: 'err' },
  expired: { word: 'Expired', tone: 'err' },
};

/** The single status sentence, present tense, stating what is true now. */
export function phaseSentence(model: Pick<ApprovalModel, 'phase' | 'resolvers'>, canAnswerHere: boolean): string {
  switch (model.phase) {
    case 'pending':
      return canAnswerHere
        ? 'Nothing has run. Approve to run this call once, or deny to cancel it.'
        : `Nothing has run. Approval happens in ${placeOf(model.resolvers)}. This panel cannot approve.`;
    case 'waiting_system_prompt':
      return 'Nothing has run. Approval happens in the system prompt on this machine, not in this panel. Answering here does not approve.';
    case 'approved':
      return 'The call runs once.';
    case 'denied':
      return 'Nothing was changed. Deny is final.';
    case 'expired':
      return 'Expiry is denial. Nothing was changed.';
  }
}
