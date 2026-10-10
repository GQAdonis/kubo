import { z } from 'zod';
import { displayText } from '../lib/safe-text';
import { UiScope } from '../scope/UiScope';
import type { ChatRole } from './agui';

export const MessageStreamDataSchema = z.strictObject({
  messages: z.array(
    z.strictObject({
      id: z.string().min(1),
      role: z.enum(['developer', 'system', 'assistant', 'user', 'tool']),
      content: z.string(),
      streaming: z.boolean().optional(),
    }),
  ),
});
export type MessageStreamData = z.infer<typeof MessageStreamDataSchema>;

export interface ChatMessageItemProps {
  readonly message: {
    readonly id: string;
    readonly role: ChatRole;
    readonly content: string;
    readonly streaming?: boolean | undefined;
  };
  /** The element to draw. MessageStream uses 'li'; a host that owns its own list or log uses 'div'. */
  readonly as?: 'div' | 'li';
}

/**
 * One text message: who spoke and what they said, as plain text (model output is never interpreted
 * as markup). Draws nothing for tool and developer messages. It does not wrap itself in UiScope or a
 * log, so a host can place it among other transcript items such as tool call cards in one live log.
 */
export function ChatMessageItem({ message, as: Tag = 'div' }: ChatMessageItemProps) {
  const who = ROLE_LABEL[message.role];
  if (who === undefined) return null;
  return (
    <Tag className={`iu-msg iu-msg--${message.role}`}>
      <span className="iu-msg__who">{who}</span>
      <p className="iu-msg__text">
        {displayText(message.content)}
        {message.streaming ? <span className="iu-msg__more"> Writing</span> : null}
      </p>
    </Tag>
  );
}

export interface MessageStreamProps {
  messages: readonly {
    readonly id: string;
    readonly role: ChatRole;
    readonly content: string;
    readonly streaming?: boolean | undefined;
  }[];
  /** Accessible name of the conversation. */
  label?: string;
}

const ROLE_LABEL: Partial<Record<ChatRole, string>> = {
  user: 'You',
  assistant: 'Agent',
  system: 'System',
};

/**
 * The AG-UI text message stream. Plain text only: model output is never interpreted as markup, so a
 * message cannot inject UI. Tool and developer messages are not drawn; tool calls have their own card.
 */
export function MessageStream({ messages, label = 'Conversation' }: MessageStreamProps) {
  // A message with no text yet and no stream in progress (a tool-call-only assistant turn) has nothing to show.
  const visible = messages.filter((m) => ROLE_LABEL[m.role] !== undefined && (m.content !== '' || m.streaming === true));
  const busy = visible.some((m) => m.streaming === true);
  return (
    <UiScope>
      {/* The log role goes on a wrapper: an <ol> with role=log would lose its list semantics. */}
      <div role="log" aria-label={label} aria-live="polite" aria-relevant="additions text" aria-busy={busy}>
        <ol className="iu-stream">
          {visible.map((m) => (
            <ChatMessageItem key={m.id} as="li" message={m} />
          ))}
        </ol>
      </div>
    </UiScope>
  );
}
