import { isDeepStrictEqual } from 'node:util';
import { loadFixtures, sseText, type AguiFixture } from './fixtures';

// An in-process fetch for component tests: no socket, same contract as the fixture server. It answers
// a POST only when the body equals a fixture input, and records every call so a test can assert what
// was and was not sent.

export interface FakeCall {
  readonly url: string;
  readonly headers: Record<string, string>;
  readonly body: unknown;
  readonly init: RequestInit;
  readonly matched: string | undefined;
}

export interface FakeFetch {
  readonly fetch: typeof fetch;
  readonly calls: FakeCall[];
}

export interface FakeFetchOptions {
  readonly fixtures?: readonly AguiFixture[];
  readonly token?: string;
  /**
   * Answer a {wait: true} resume with RUN_STARTED and then nothing, until the request is aborted: a
   * system prompt that nobody has answered yet. The test ends the run by stopping or denying.
   */
  readonly holdWait?: boolean;
}

export function createFakeFetch(options: FakeFetchOptions = {}): FakeFetch {
  const fixtures = options.fixtures ?? loadFixtures();
  const calls: FakeCall[] = [];
  const impl = async (input: RequestInfo | URL, init: RequestInit = {}): Promise<Response> => {
    const url = String(input);
    const headers = Object.fromEntries(new Headers(init.headers).entries());
    const body = typeof init.body === 'string' ? (JSON.parse(init.body) as unknown) : undefined;
    const matched = fixtures.find((f) => isDeepStrictEqual(f.input, body));
    calls.push({ url, headers, body, init, matched: matched?.name });
    if (options.token !== undefined && headers['authorization'] !== `Bearer ${options.token}`) {
      return new Response('{"error":"unauthorized"}', { status: 401 });
    }
    if (!matched) return new Response('{"error":"no fixture"}', { status: 422 });
    const encoder = new TextEncoder();
    const waiting = options.holdWait === true && JSON.stringify(body).includes('"wait":true');
    if (waiting) {
      return new Response(
        new ReadableStream<Uint8Array>({
          start(controller) {
            controller.enqueue(encoder.encode(sseText(matched.events.slice(0, 1))));
            init.signal?.addEventListener('abort', () => controller.error(new DOMException('Aborted', 'AbortError')));
          },
        }),
        { status: 200, headers: { 'content-type': 'text/event-stream' } },
      );
    }
    const text = sseText(matched.events);
    return new Response(
      new ReadableStream<Uint8Array>({
        start(controller) {
          controller.enqueue(encoder.encode(text));
          controller.close();
        },
      }),
      { status: 200, headers: { 'content-type': 'text/event-stream' } },
    );
  };
  return { fetch: impl as typeof fetch, calls };
}
