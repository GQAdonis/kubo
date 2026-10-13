import { useLayoutEffect, useRef } from 'react';
import { ChatMessageItem, type UiTheme } from '@gqadonis/ipfs-mcp-ui';
import { InterruptItem } from './InterruptItem';
import { ToolCallSurface } from './ToolCallSurface';
import type { TimelineItem } from './timeline';

// The conversation, as a live log. Message text is plain text: ChatMessageItem never interprets model
// output as markup, so a message cannot inject UI. Tool calls are drawn by the A2UI catalog.

export interface TranscriptProps {
  readonly items: readonly TimelineItem[];
  readonly theme: UiTheme;
  readonly nowIso: string;
  readonly running: boolean;
  /** Interrupts whose system prompt the session is watching. */
  readonly observing: readonly string[];
  readonly onAnswer: (interruptId: string, decision: 'approve' | 'deny') => void;
  /** Offered on a native-prompt card when the watch stopped; absent while there is nothing to retry. */
  readonly onWatchAgain?: ((interruptId: string) => void) | undefined;
}

export function Transcript({ items, theme, nowIso, running, observing, onAnswer, onWatchAgain }: TranscriptProps) {
  const log = useRef<HTMLDivElement>(null);
  const pinned = useRef(true);

  // Follow new output only while the reader is at the bottom; scrolling up to read stops the follow.
  useLayoutEffect(() => {
    const el = log.current;
    if (el && pinned.current) el.scrollTop = el.scrollHeight;
  });

  return (
    <div
      ref={log}
      role="log"
      aria-label="Conversation"
      aria-live="polite"
      aria-relevant="additions text"
      aria-busy={running}
      // The log scrolls, so a keyboard user must be able to reach it.
      tabIndex={0}
      onScroll={(event) => {
        const el = event.currentTarget;
        pinned.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
      }}
      className="min-h-0 flex-1 overflow-y-auto px-s3 py-s2"
    >
      <ol className="flex flex-col gap-s4">
        {items.map((item) => (
          <li key={item.key} className="min-w-0">
            {item.kind === 'message' ? (
              <ChatMessageItem message={item.message} />
            ) : (
              <div className="flex flex-col gap-s3">
                <ToolCallSurface call={item.call} theme={theme} />
                {item.interrupt ? (
                  <InterruptItem
                    item={item.interrupt}
                    nowIso={nowIso}
                    onAnswer={onAnswer}
                    busy={running}
                    observing={observing.includes(item.interrupt.interrupt.id)}
                    {...(onWatchAgain && { onWatchAgain })}
                  />
                ) : null}
              </div>
            )}
          </li>
        ))}
      </ol>
    </div>
  );
}
