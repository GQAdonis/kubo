import { useMemo } from 'react';
import { UiScope, type UiTheme } from '@gqadonis/ipfs-mcp-ui';
import { AguiError, createAguiClient, type AguiClient } from '../agui/client';
import { checkEndpoint } from '../agui/endpoint';
import { SidebarProvider } from '../ui/sidebar';
import { Conversation, type Readiness } from './Conversation';
import type { SessionIds } from './session';

// The island's root. It validates the host's configuration, builds the one client, and renders the
// sidebar inside the library scope. Styles are adopted into the shadow root by mount.tsx, so the scope
// injects none (a <style> element would need style-src 'unsafe-inline' on the host page).

export interface IslandProps {
  /** The ipfs mcp /agui URL, or undefined when the host has not configured one. */
  readonly endpoint: string | undefined;
  /** Reads the bearer for one request. The token itself never passes through props or state. */
  readonly getToken: () => string | undefined | Promise<string | undefined>;
  /** Whether the host has supplied a token. A boolean only; used to word the empty state. */
  readonly tokenSet: boolean;
  /** Session mode: same-origin cookie session, no bearer. The token is neither needed nor read. */
  readonly session?: boolean;
  /** Called when the node answers 401 in session mode, so the host can offer "Sign in again". */
  readonly onSessionExpired?: () => void;
  readonly open: boolean;
  readonly theme: UiTheme;
  readonly onOpenChange: (open: boolean) => void;
  /** Test seams. The custom element never sets these, so page script cannot. */
  readonly fetchImpl?: typeof fetch;
  readonly ids?: SessionIds;
  readonly now?: () => number;
}

let counter = 0;
const randomId = (prefix: string): string =>
  `${prefix}-${globalThis.crypto?.randomUUID?.() ?? `${Date.now().toString(36)}${(counter++).toString(36)}`}`;

const DEFAULT_IDS: SessionIds = {
  thread: () => randomId('thread'),
  run: () => randomId('run'),
  message: () => randomId('msg-u'),
};

export function IslandApp({ endpoint, getToken, tokenSet, session = false, onSessionExpired, open, theme, onOpenChange, fetchImpl, ids = DEFAULT_IDS, now }: IslandProps) {
  const check = useMemo(() => checkEndpoint(endpoint), [endpoint]);
  const client = useMemo<AguiClient | null>(() => {
    if (!check.ok) return null;
    const inner = createAguiClient({ endpoint: check.url, getToken, session, ...(fetchImpl && { fetch: fetchImpl }) });
    if (!session || !onSessionExpired) return inner;
    return {
      async *run(input, signal) {
        try {
          yield* inner.run(input, signal);
        } catch (error) {
          if (error instanceof AguiError && error.kind === 'unauthorized') onSessionExpired();
          throw error;
        }
      },
    };
  }, [check, getToken, session, onSessionExpired, fetchImpl]);
  const readiness: Readiness = !check.ok
    ? { state: 'no-endpoint', reason: endpoint ? check.reason : '' }
    : !tokenSet && !session
      ? { state: 'no-token' }
      : { state: 'ready' };

  return (
    <UiScope theme={theme} injectStyles={false} className="ac-root">
      <SidebarProvider open={open} onOpenChange={onOpenChange}>
        {/* Keyed by the endpoint: a different server is a different conversation. */}
        <Conversation key={check.ok ? check.url.href : 'unconfigured'} client={client} readiness={readiness} theme={theme} ids={ids} {...(now && { now })} />
      </SidebarProvider>
    </UiScope>
  );
}
