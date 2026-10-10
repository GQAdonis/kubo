import { readSse } from './sse';
import { originGuardedFetch } from './endpoint';
import { parseRetryAfter } from './retry-after';
import type { RunAgentInput } from './wire';
import { parseToolError, type AguiEvent } from '@gqadonis/ipfs-mcp-ui';

// The AG-UI client: POST a RunAgentInput to the configured `ipfs mcp` /agui endpoint and stream back
// the events. This is the only module in the island that calls fetch. The bearer is read from the
// host-supplied getter for each request and travels in the Authorization header: never in the URL,
// never in a cookie (credentials are omitted), never written anywhere.

export type AguiErrorKind =
  | 'config' // no endpoint or no token
  | 'unauthorized' // 401
  | 'forbidden' // 403: the page origin is not on the allowlist, or the principal lacks the group
  | 'not_found' // 404: /agui is not mounted, which is the off switch
  | 'session_busy' // 409 session_busy: another run is streaming on this thread; retryable
  | 'failed_precondition' // 409 failed_precondition: an open interrupt or a full thread; not retryable unchanged
  | 'resource_exhausted' // 503: a server-wide cap (the session limit); retryable after Retry-After
  | 'http'
  | 'network'
  | 'protocol'
  | 'aborted';

export class AguiError extends Error {
  override name = 'AguiError';
  constructor(
    readonly kind: AguiErrorKind,
    message: string,
    readonly status?: number,
    /** Seconds from the Retry-After header of a 503, clamped (see parseRetryAfter). Absent otherwise. */
    readonly retryAfterSeconds?: number,
  ) {
    super(message);
  }
}

export interface AguiClientOptions {
  /** The validated endpoint (see checkEndpoint). */
  readonly endpoint: URL;
  /** Returns the bearer. Called per request; may be async (the Desktop bridge). Never logged. */
  readonly getToken: () => string | undefined | Promise<string | undefined>;
  /** Test seam. Defaults to the global fetch. */
  readonly fetch?: typeof fetch;
  /** Test seam for an HTTP-date Retry-After. Defaults to Date.now. */
  readonly now?: () => number;
}

const STATUS_MESSAGE: Record<number, [AguiErrorKind, string]> = {
  401: ['unauthorized', 'The token was refused. Check the token for the ui principal.'],
  403: ['forbidden', 'ipfs mcp refused this page or this principal. Check the origin allowlist and the principal groups.'],
  404: ['not_found', 'ipfs mcp has no chat endpoint here. The chat is off until an LLM backend is configured.'],
};

export const SESSION_BUSY_MESSAGE = 'A reply is still streaming. Send again when it ends.';
export const FAILED_PRECONDITION_MESSAGE = 'Answer the open question first.';
export const RESOURCE_EXHAUSTED_MESSAGE = 'The node is at its session limit. Try again shortly.';

/**
 * A 409 carries a ToolError body whose code says which conflict it is. Only the code is read: the body's
 * message is not shown. A 409 whose body is missing or unreadable is treated as session_busy, the
 * retryable one, because that is the case a person can recover from by waiting.
 */
async function conflictError(response: Response): Promise<AguiError> {
  let code: string | undefined;
  try {
    code = parseToolError(await response.text())?.error.code;
  } catch {
    code = undefined;
  }
  return code === 'failed_precondition'
    ? new AguiError('failed_precondition', FAILED_PRECONDITION_MESSAGE, 409)
    : new AguiError('session_busy', SESSION_BUSY_MESSAGE, 409);
}

export interface AguiClient {
  run(input: RunAgentInput, signal?: AbortSignal): AsyncGenerator<AguiEvent>;
}

export function createAguiClient(options: AguiClientOptions): AguiClient {
  const { endpoint, getToken } = options;
  const impl = options.fetch ?? ((...a: Parameters<typeof fetch>) => globalThis.fetch(...a));
  const guarded = originGuardedFetch(endpoint.origin, impl);

  async function* run(input: RunAgentInput, signal?: AbortSignal): AsyncGenerator<AguiEvent> {
    const token = await getToken();
    if (!token) throw new AguiError('config', 'No token is set. The host page supplies it; the chat does not ask for it here.');

    let response: Response;
    try {
      response = await guarded(endpoint.href, {
        method: 'POST',
        headers: {
          'content-type': 'application/json',
          accept: 'text/event-stream',
          authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(input),
        // A bearer header is the only credential. No cookies, no referrer, no cache, no redirect.
        credentials: 'omit',
        redirect: 'error',
        referrerPolicy: 'no-referrer',
        cache: 'no-store',
        mode: 'cors',
        ...(signal && { signal }),
      });
    } catch (error) {
      if (signal?.aborted) throw new AguiError('aborted', 'Stopped.');
      if (error instanceof Error && error.name === 'OriginRefusedError') throw new AguiError('config', error.message);
      throw new AguiError('network', 'Cannot reach ipfs mcp. Check that it is running and that this page may connect to it.');
    }

    if (!response.ok) {
      const known = STATUS_MESSAGE[response.status];
      if (known) throw new AguiError(known[0], known[1], response.status);
      if (response.status === 409) throw await conflictError(response);
      if (response.status === 503) {
        const wait = parseRetryAfter(response.headers.get('retry-after'), (options.now ?? Date.now)());
        throw new AguiError('resource_exhausted', RESOURCE_EXHAUSTED_MESSAGE, 503, wait);
      }
      throw new AguiError('http', `ipfs mcp answered ${response.status}.`, response.status);
    }
    const type = response.headers.get('content-type') ?? '';
    if (!type.toLowerCase().includes('text/event-stream') || !response.body) {
      throw new AguiError('protocol', 'ipfs mcp did not answer with an event stream.');
    }

    try {
      for await (const message of readSse(response.body, signal)) {
        let parsed: unknown;
        try {
          parsed = JSON.parse(message.data);
        } catch {
          throw new AguiError('protocol', 'An event was not valid JSON.');
        }
        if (!isEvent(parsed)) throw new AguiError('protocol', 'An event had no type.');
        yield parsed;
      }
      // readSse ends quietly when the signal aborts; tell the caller why the stream stopped.
      if (signal?.aborted) throw new AguiError('aborted', 'Stopped.');
    } catch (error) {
      if (error instanceof AguiError) throw error;
      if (signal?.aborted) throw new AguiError('aborted', 'Stopped.');
      throw new AguiError('network', 'The connection to ipfs mcp ended unexpectedly.');
    }
  }

  return { run };
}

function isEvent(v: unknown): v is AguiEvent {
  return typeof v === 'object' && v !== null && typeof (v as { type?: unknown }).type === 'string';
}
