// The island talks to exactly one place: the `ipfs mcp` /agui endpoint the host configured. This
// module decides whether a configured value is acceptable, and gives every request one guard that
// refuses any other origin. There is no default endpoint: with none configured the island makes no
// request at all (design.md: "no default endpoint").

/** The address `ipfs mcp --http` is documented to use for the chat. A host may pass it; the island never assumes it. */
export const SUGGESTED_AGUI_ENDPOINT = 'http://127.0.0.1:5050/agui';

const LOOPBACK_HOSTS = new Set(['127.0.0.1', 'localhost', '[::1]']);

export type EndpointCheck =
  | { readonly ok: true; readonly url: URL; readonly origin: string }
  | { readonly ok: false; readonly reason: string };

/**
 * Accept an absolute https URL, or an http URL on a loopback host, whose path ends in /agui and that
 * carries no credentials, query or fragment. The bearer is sent in a header only, so a URL that could
 * carry a token is refused rather than cleaned.
 */
export function checkEndpoint(raw: string | null | undefined): EndpointCheck {
  if (!raw) return { ok: false, reason: 'No endpoint is configured.' };
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return { ok: false, reason: 'The endpoint is not an absolute URL.' };
  }
  if (url.protocol !== 'https:' && url.protocol !== 'http:') {
    return { ok: false, reason: 'The endpoint must use https, or http on a loopback address.' };
  }
  if (url.protocol === 'http:' && !LOOPBACK_HOSTS.has(url.hostname)) {
    return { ok: false, reason: 'The endpoint uses http on a non-loopback host. The bearer would travel in clear text.' };
  }
  if (url.username || url.password) return { ok: false, reason: 'The endpoint must not carry credentials in the URL.' };
  if (url.search || url.hash) return { ok: false, reason: 'The endpoint must not carry a query or fragment.' };
  if (!url.pathname.endsWith('/agui')) return { ok: false, reason: 'The endpoint path must end in /agui.' };
  return { ok: true, url, origin: url.origin };
}

export class OriginRefusedError extends Error {
  override name = 'OriginRefusedError';
}

/** Wrap fetch so a request to any origin but the configured one throws before it is sent. */
export function originGuardedFetch(origin: string, impl: typeof fetch): typeof fetch {
  return ((input: RequestInfo | URL, init?: RequestInit) => {
    const target = input instanceof Request ? input.url : String(input);
    let targetOrigin: string;
    try {
      targetOrigin = new URL(target).origin;
    } catch {
      throw new OriginRefusedError('The request URL is not absolute.');
    }
    if (targetOrigin !== origin) {
      throw new OriginRefusedError(`Refused a request to ${targetOrigin}. The chat talks only to ${origin}.`);
    }
    return impl(input, init);
  }) as typeof fetch;
}
