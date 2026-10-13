import { useId, useRef, useState, type KeyboardEvent } from 'react';
import { Button } from '@gqadonis/ipfs-mcp-ui';

// The message box. Enter sends, Shift and Enter adds a line, and Enter during IME composition is left
// to the input method. The send control names its object ("Send message"), as DESIGN.md asks.

/** Messages longer than this are refused before they leave the box. */
export const MAX_MESSAGE_CHARS = 8000;

export interface ComposerProps {
  /** Why sending is not possible right now, in words. Undefined when it is. */
  readonly blockedReason?: string | undefined;
  readonly running: boolean;
  readonly onSend: (text: string) => void;
  readonly onStop: () => void;
}

export function Composer({ blockedReason, running, onSend, onStop }: ComposerProps) {
  const [text, setText] = useState('');
  const field = useRef<HTMLTextAreaElement>(null);
  const fieldId = useId();
  const hintId = useId();
  const blocked = blockedReason !== undefined;
  const tooLong = text.length > MAX_MESSAGE_CHARS;
  const canSend = !blocked && !running && text.trim() !== '' && !tooLong;

  const submit = () => {
    if (!canSend) return;
    onSend(text.trim());
    setText('');
  };

  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key !== 'Enter' || event.shiftKey || event.nativeEvent.isComposing) return;
    event.preventDefault();
    submit();
  };

  return (
    <div role="group" aria-label="Message the agent" className="flex flex-col gap-s2">
      <label htmlFor={fieldId} className="iu-sr-only">
        Message to the agent
      </label>
      <textarea
        ref={field}
        id={fieldId}
        name="message"
        rows={1}
        value={text}
        disabled={blocked}
        aria-describedby={hintId}
        aria-invalid={tooLong}
        placeholder="Ask about your node"
        onChange={(event) => setText(event.target.value)}
        onKeyDown={onKeyDown}
        className="ac-field w-full rounded-ui-sm bg-control px-s3 py-s2 text-[length:var(--iu-text-body)] leading-[1.55] text-foreground disabled:opacity-45"
      />
      <div className="flex items-center justify-between gap-s3">
        <p id={hintId} className="min-w-0 text-[length:var(--iu-text-small)] text-muted-foreground">
          {blocked
            ? blockedReason
            : tooLong
              ? `Too long by ${(text.length - MAX_MESSAGE_CHARS).toLocaleString('en-US')} characters.`
              : 'Enter sends. Shift and Enter adds a line.'}
        </p>
        {running ? (
          <Button variant="secondary" onClick={onStop}>
            Stop run
          </Button>
        ) : (
          <Button variant="primary" onClick={submit} disabled={!canSend}>
            Send message
          </Button>
        )}
      </div>
    </div>
  );
}
