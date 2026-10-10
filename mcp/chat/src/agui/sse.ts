// Server-sent events over a fetch body. A small, strict parser for the subset `ipfs mcp` /agui sends:
// `data:` lines, blank-line dispatch, `:` comments (keep-alives), CRLF, LF or CR line ends. It has no
// network access: callers hand it bytes.

export interface SseMessage {
  readonly event: string;
  readonly data: string;
  readonly id?: string;
}

/** One event may not exceed this many characters. A server that sends more is broken or hostile. */
export const MAX_EVENT_CHARS = 1_048_576;

export class SseProtocolError extends Error {
  override name = 'SseProtocolError';
}

/**
 * Incremental parser. Feed it decoded text with push(); it calls onMessage once per complete event.
 * A trailing CR is held back until the next chunk, because it may be the first half of CRLF.
 */
export function createSseParser(onMessage: (message: SseMessage) => void) {
  let buffer = '';
  let started = false;
  let event = '';
  let data: string[] = [];
  let dataChars = 0;
  let id: string | undefined;

  const dispatch = () => {
    if (data.length > 0) {
      const message: SseMessage = { event: event || 'message', data: data.join('\n'), ...(id !== undefined && { id }) };
      onMessage(message);
    }
    event = '';
    data = [];
    dataChars = 0;
  };

  const line = (text: string) => {
    if (text === '') return dispatch();
    if (text.startsWith(':')) return;
    const colon = text.indexOf(':');
    const field = colon === -1 ? text : text.slice(0, colon);
    let value = colon === -1 ? '' : text.slice(colon + 1);
    if (value.startsWith(' ')) value = value.slice(1);
    if (field === 'data') {
      dataChars += value.length + 1;
      if (dataChars > MAX_EVENT_CHARS) throw new SseProtocolError('An event exceeded the size limit.');
      data.push(value);
    } else if (field === 'event') event = value;
    else if (field === 'id' && !value.includes('\0')) id = value;
    // retry and unknown fields carry nothing this client uses.
  };

  const drain = (final: boolean) => {
    let start = 0;
    for (let i = 0; i < buffer.length; i++) {
      const ch = buffer[i];
      if (ch !== '\n' && ch !== '\r') continue;
      if (ch === '\r' && i === buffer.length - 1 && !final) break; // maybe CRLF; wait for the next chunk
      line(buffer.slice(start, i));
      if (ch === '\r' && buffer[i + 1] === '\n') i++;
      start = i + 1;
    }
    buffer = buffer.slice(start);
    if (buffer.length > MAX_EVENT_CHARS) throw new SseProtocolError('A line exceeded the size limit.');
  };

  return {
    push(chunk: string) {
      if (!started) {
        started = true;
        if (chunk.startsWith('﻿')) chunk = chunk.slice(1);
      }
      buffer += chunk;
      drain(false);
    },
    /** End of stream. An event without its closing blank line is dropped, as the SSE spec says. */
    end() {
      drain(true);
      buffer = '';
      data = [];
      event = '';
    },
  };
}

/** Read a response body as SSE messages. Stops when the body ends or the signal aborts. */
export async function* readSse(body: ReadableStream<Uint8Array>, signal?: AbortSignal): AsyncGenerator<SseMessage> {
  const reader = body.getReader();
  const decoder = new TextDecoder('utf-8');
  const queue: SseMessage[] = [];
  const parser = createSseParser((m) => queue.push(m));
  const onAbort = () => void reader.cancel().catch(() => undefined);
  signal?.addEventListener('abort', onAbort, { once: true });
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) {
        parser.push(decoder.decode());
        parser.end();
        break;
      }
      parser.push(decoder.decode(value, { stream: true }));
      while (queue.length > 0) yield queue.shift() as SseMessage;
    }
    while (queue.length > 0) yield queue.shift() as SseMessage;
  } finally {
    signal?.removeEventListener('abort', onAbort);
    reader.releaseLock();
  }
}
