// @vitest-environment node
import { describe, expect, it } from 'vitest';
import { MAX_EVENT_CHARS, SseProtocolError, createSseParser, readSse, type SseMessage } from '../../src/agui/sse';

function parse(chunks: string[]): SseMessage[] {
  const out: SseMessage[] = [];
  const p = createSseParser((m) => out.push(m));
  for (const c of chunks) p.push(c);
  p.end();
  return out;
}

describe('SSE parser', () => {
  it('dispatches one message per blank line', () => {
    expect(parse(['data: {"a":1}\n\ndata: {"b":2}\n\n']).map((m) => m.data)).toEqual(['{"a":1}', '{"b":2}']);
  });

  it('handles CRLF, lone CR and LF line ends', () => {
    expect(parse(['data: x\r\n\r\ndata: y\r\rdata: z\n\n']).map((m) => m.data)).toEqual(['x', 'y', 'z']);
  });

  it('holds a trailing CR until the next chunk, so CRLF split across reads is one line end', () => {
    expect(parse(['data: x\r', '\n\r', '\n']).map((m) => m.data)).toEqual(['x']);
  });

  it('reassembles frames split at every byte', () => {
    const wire = 'data: {"type":"RUN_STARTED"}\n\n: ping\n\ndata: {"type":"RUN_FINISHED"}\n\n';
    expect(parse([...wire]).map((m) => m.data)).toEqual(['{"type":"RUN_STARTED"}', '{"type":"RUN_FINISHED"}']);
  });

  it('joins multi-line data with newlines', () => {
    expect(parse(['data: one\ndata: two\n\n'])[0]?.data).toBe('one\ntwo');
  });

  it('ignores comments, unknown fields and retry', () => {
    const msgs = parse([': keep-alive\nretry: 1000\nfoo: bar\ndata: ok\n\n']);
    expect(msgs).toHaveLength(1);
    expect(msgs[0]?.data).toBe('ok');
  });

  it('carries event and id, and strips one leading space only', () => {
    const m = parse(['event: custom\nid: 7\ndata:  two spaces\n\n'])[0];
    expect(m).toMatchObject({ event: 'custom', id: '7', data: ' two spaces' });
  });

  it('drops an event with no closing blank line at end of stream', () => {
    expect(parse(['data: partial\n'])).toEqual([]);
  });

  it('strips a byte order mark at the start of the stream', () => {
    expect(parse(['﻿data: x\n\n'])[0]?.data).toBe('x');
  });

  it('refuses an event larger than the limit', () => {
    const big = 'data: ' + 'x'.repeat(MAX_EVENT_CHARS + 1) + '\n\n';
    expect(() => parse([big])).toThrow(SseProtocolError);
  });
});

describe('readSse', () => {
  const body = (parts: string[]) =>
    new ReadableStream<Uint8Array>({
      start(c) {
        const enc = new TextEncoder();
        for (const p of parts) c.enqueue(enc.encode(p));
        c.close();
      },
    });

  it('decodes a multi-byte character split across reads', async () => {
    const bytes = new TextEncoder().encode('data: café\n\n');
    const stream = new ReadableStream<Uint8Array>({
      start(c) {
        c.enqueue(bytes.slice(0, 10)); // ends inside the two-byte é
        c.enqueue(bytes.slice(10));
        c.close();
      },
    });
    const out: string[] = [];
    for await (const m of readSse(stream)) out.push(m.data);
    expect(out).toEqual(['café']);
  });

  it('stops when the signal aborts', async () => {
    const ac = new AbortController();
    const stream = new ReadableStream<Uint8Array>({
      start(c) {
        c.enqueue(new TextEncoder().encode('data: one\n\n'));
      },
    });
    const out: string[] = [];
    for await (const m of readSse(stream, ac.signal)) {
      out.push(m.data);
      ac.abort();
    }
    expect(out).toEqual(['one']);
  });

  it('yields every message of a complete body', async () => {
    const out: string[] = [];
    for await (const m of readSse(body(['data: a\n\n', 'data: b\n\n']))) out.push(m.data);
    expect(out).toEqual(['a', 'b']);
  });
});
