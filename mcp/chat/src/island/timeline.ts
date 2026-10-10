import type { ChatInterrupt, ChatMessage, ChatState, ChatToolCall } from '@gqadonis/ipfs-mcp-ui';

// The transcript is the chat state laid out in the order things happened: a message, then the tool
// calls that message made, then the approval each interrupted call is waiting on.

export type TimelineItem =
  | { readonly kind: 'message'; readonly key: string; readonly message: ChatMessage }
  | { readonly kind: 'call'; readonly key: string; readonly call: ChatToolCall; readonly interrupt?: ChatInterrupt };

const SPEAKERS = new Set<ChatMessage['role']>(['user', 'assistant', 'system']);

export function buildTimeline(chat: ChatState): readonly TimelineItem[] {
  const items: TimelineItem[] = [];
  const placed = new Set<string>();
  const interruptFor = (callId: string) => chat.interrupts.find((i) => i.interrupt.toolCallId === callId);

  const placeCalls = (parentId: string | undefined) => {
    for (const call of chat.toolCalls) {
      if (placed.has(call.id) || call.parentMessageId !== parentId) continue;
      placed.add(call.id);
      const interrupt = interruptFor(call.id);
      items.push({ kind: 'call', key: `call:${call.id}`, call, ...(interrupt && { interrupt }) });
    }
  };

  for (const message of chat.messages) {
    // A turn that only made tool calls has no text and draws no bubble, but its calls still appear.
    if (SPEAKERS.has(message.role) && (message.content !== '' || message.streaming)) {
      items.push({ kind: 'message', key: `msg:${message.id}`, message });
    }
    placeCalls(message.id);
  }
  // Calls whose parent message is unknown still belong in the transcript.
  for (const call of chat.toolCalls) {
    if (placed.has(call.id)) continue;
    placed.add(call.id);
    const interrupt = interruptFor(call.id);
    items.push({ kind: 'call', key: `call:${call.id}`, call, ...(interrupt && { interrupt }) });
  }
  return items;
}
