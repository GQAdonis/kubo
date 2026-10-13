import { z } from 'zod';
import { TOOL_META, type ToolName } from '../contract/generated/tool-meta';
import { plural } from '../lib/format';
import { UiScope } from '../scope/UiScope';
import { Badge } from '../ui/primitives';

const toolNames = Object.keys(TOOL_META) as [ToolName, ...ToolName[]];

export const EgressBadgeDataSchema = z.strictObject({
  /** The tool whose result this marks. Only tools with contentEgress in the catalog render a badge. */
  tool: z.enum(toolNames),
  /** Hosts the operator acknowledged in egress_ack, as the server reports them. */
  acknowledged_hosts: z.array(z.string().min(1).max(253)),
  /** True when the server refused to send the content (error code egress_not_acknowledged). */
  refused: z.boolean().optional(),
});
export type EgressBadgeData = z.infer<typeof EgressBadgeDataSchema>;

export type EgressBadgeProps = EgressBadgeData;

/**
 * Marks a result that carries node content and states which acknowledged host may receive it. A text
 * statement, not an icon: the brand does not use padlocks or shields.
 */
export function EgressBadge({ tool, acknowledged_hosts, refused = false }: EgressBadgeProps) {
  if (!TOOL_META[tool].contentEgress) return null;
  const hosts = acknowledged_hosts;
  let tone: 'err' | 'info' | 'warn';
  let word: string;
  let detail: string;
  if (refused) {
    tone = 'err';
    word = 'Content not sent';
    detail =
      hosts.length > 0
        ? `The destination is not in the acknowledged hosts (${hosts.join(', ')}), so the node content stayed on this machine.`
        : 'No host is acknowledged, so the node content stayed on this machine.';
  } else if (hosts.length > 0) {
    tone = 'info';
    word = 'Carries node content';
    detail = `May reach ${plural(hosts.length, 'acknowledged host')}: ${hosts.join(', ')}. No other host receives it.`;
  } else {
    tone = 'warn';
    word = 'Carries node content';
    detail = 'No host is acknowledged, so a model host or embedder is refused this content.';
  }
  return (
    <UiScope>
      <p className="iu-egress">
        <Badge tone={tone}>{word}</Badge> <span>{detail}</span>
      </p>
    </UiScope>
  );
}
