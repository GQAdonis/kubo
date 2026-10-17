import { loadIsland } from './island-loader';
import type { IslandHandle } from './island/mount';
import type { IslandProps } from './island/IslandApp';

// <ipfs-agent-chat>: the part of the island a host page loads. It is small on purpose. It defines the
// custom element, owns the shadow root and the host-facing properties, and loads React and everything
// else (island/mount.tsx) with a dynamic import() the first time the sidebar opens. A page that loads
// this module but never opens the sidebar runs no React, adds no stylesheet to the document, registers
// no font and makes no request.
//
// Host API
//   endpoint      attribute or property  the ipfs mcp /agui URL. No default: unset, the element sends nothing.
//   open          attribute or property  whether the sidebar is shown.
//   theme         attribute or property  'dark' | 'light' | 'auto' (default 'auto').
//   token         PROPERTY ONLY, write-only  the bearer for the `ui` principal. Never an attribute, never read
//                 back, never written to any storage, never in a URL.
//   tokenProvider PROPERTY ONLY  () => string | Promise<string>, for a host that holds the token elsewhere
//                 (the Desktop safeStorage bridge). Called per request.
//   session       attribute or property  session mode (console-session-auth): the page's own cookie session
//                 is the credential. Requests go same-origin with credentials and carry no Authorization
//                 header; the token is neither needed nor read.
//   openPanel(), close(), toggle()
//   event 'ipfs-agent-chat-openchange' { detail: { open } }, bubbles and composed.
//   event 'ipfs-agent-chat-session-expired' (session mode only) when the node answers 401, bubbles and composed.

declare const __AC_OPEN_SHADOW__: boolean;

export const ELEMENT_TAG = 'ipfs-agent-chat';
export const OPEN_CHANGE_EVENT = 'ipfs-agent-chat-openchange';
export const NOTICE_EVENT = 'ipfs-agent-chat-notice';
export const SESSION_EXPIRED_EVENT = 'ipfs-agent-chat-session-expired';

export type ChatTheme = 'dark' | 'light' | 'auto';
const THEMES: readonly ChatTheme[] = ['dark', 'light', 'auto'];

type TokenProvider = () => string | undefined | Promise<string | undefined>;

// Placement only. Everything visual about the panel lives inside the shadow root, in the lazy chunk.
const HOST_CSS = `
:host { all: initial; position: fixed; inset: 0; z-index: var(--ipfs-agent-chat-z-index, 1000); display: block;
  overflow: hidden; pointer-events: none; contain: layout style; }
:host(:not([data-active])) { display: none; }
`;

const afterPaint = (): Promise<void> =>
  new Promise((resolve) => {
    const raf = globalThis.requestAnimationFrame;
    if (typeof raf !== 'function') return void setTimeout(resolve, 16);
    raf(() => raf(() => resolve()));
  });

export class IpfsAgentChatElement extends HTMLElement {
  static readonly observedAttributes = ['endpoint', 'open', 'theme', 'session'] as const;

  // The bearer and its provider are private fields: no other code, and no page script, can read them.
  #token: string | undefined;
  #provider: TokenProvider | undefined;
  #shadow: ShadowRoot | undefined;
  #handle: IslandHandle | undefined;
  #loading: Promise<void> | undefined;
  #returnFocus: Element | null = null;
  #generation = 0;

  // ---- host-facing properties

  get endpoint(): string | undefined {
    return this.getAttribute('endpoint') ?? undefined;
  }
  set endpoint(value: string | undefined) {
    if (value === undefined || value === '') this.removeAttribute('endpoint');
    else this.setAttribute('endpoint', value);
  }

  get open(): boolean {
    return this.hasAttribute('open');
  }
  set open(value: boolean) {
    this.toggleAttribute('open', Boolean(value));
  }

  get session(): boolean {
    return this.hasAttribute('session');
  }
  set session(value: boolean) {
    this.toggleAttribute('session', Boolean(value));
  }

  get theme(): ChatTheme {
    const t = this.getAttribute('theme');
    return THEMES.includes(t as ChatTheme) ? (t as ChatTheme) : 'auto';
  }
  set theme(value: ChatTheme) {
    this.setAttribute('theme', value);
  }

  /** Write-only. Reading it returns undefined, always. */
  get token(): undefined {
    return undefined;
  }
  set token(value: string | undefined) {
    this.#token = typeof value === 'string' && value !== '' ? value : undefined;
    this.#schedule();
  }

  /** Write-only, like token. */
  get tokenProvider(): undefined {
    return undefined;
  }
  set tokenProvider(value: TokenProvider | undefined) {
    this.#provider = typeof value === 'function' ? value : undefined;
    this.#schedule();
  }

  /** Whether the host has supplied a token. Safe to read. */
  get tokenSet(): boolean {
    return this.session || this.#token !== undefined || this.#provider !== undefined;
  }

  openPanel(): void {
    this.open = true;
  }
  close(): void {
    this.open = false;
  }
  toggle(): void {
    this.open = !this.open;
  }

  // ---- lifecycle

  connectedCallback(): void {
    if (!this.#shadow) {
      // Closed in production: page script has no reference to the island's DOM or to the React tree
      // behind it. Test builds define __AC_OPEN_SHADOW__ so Playwright and Testing Library can pierce it.
      this.#shadow = this.attachShadow({ mode: __AC_OPEN_SHADOW__ ? 'open' : 'closed' });
      adoptHostStyles(this.#shadow);
    }
    // A bearer in an attribute sits in the DOM, in saved HTML and in devtools. It is never read; it is removed.
    if (this.hasAttribute('token')) {
      this.removeAttribute('token');
      this.dispatchEvent(
        new CustomEvent(NOTICE_EVENT, { detail: { message: 'The token attribute is ignored and removed. Set the token property.' } }),
      );
    }
    this.#schedule();
  }

  disconnectedCallback(): void {
    this.#generation++;
    this.#handle?.unmount();
    this.#handle = undefined;
    this.#loading = undefined;
    this.removeAttribute('data-active');
  }

  attributeChangedCallback(name: string, oldValue: string | null, newValue: string | null): void {
    if (oldValue === newValue) return;
    if (name === 'open') this.#onOpenAttribute(newValue !== null);
    this.#schedule();
  }

  // ---- internals

  readonly #getToken = async (): Promise<string | undefined> => {
    if (this.#token !== undefined) return this.#token;
    try {
      return (await this.#provider?.()) || undefined;
    } catch {
      return undefined;
    }
  };

  readonly #onSessionExpired = (): void => {
    this.dispatchEvent(new CustomEvent(SESSION_EXPIRED_EVENT, { bubbles: true, composed: true }));
  };

  #onOpenAttribute(open: boolean): void {
    if (open) {
      const active = document.activeElement;
      this.#returnFocus = active && active !== this ? active : null;
    } else {
      const target = this.#returnFocus;
      this.#returnFocus = null;
      if (target instanceof HTMLElement && target.isConnected) target.focus({ preventScroll: true });
    }
    this.dispatchEvent(new CustomEvent(OPEN_CHANGE_EVENT, { detail: { open }, bubbles: true, composed: true }));
  }

  #props(open: boolean): IslandProps {
    return {
      endpoint: this.endpoint,
      getToken: this.#getToken,
      tokenSet: this.tokenSet,
      session: this.session,
      onSessionExpired: this.#onSessionExpired,
      open,
      theme: this.theme,
      onOpenChange: (next) => {
        this.open = next;
      },
    };
  }

  #schedule(): void {
    if (!this.isConnected || !this.#shadow) return;
    // Closed and never opened: nothing to load, nothing to draw.
    if (!this.#handle && !this.open) return;
    void this.#sync();
  }

  async #sync(): Promise<void> {
    const generation = this.#generation;
    const shadow = this.#shadow;
    if (!shadow) return;
    if (this.#handle) {
      this.#handle.update(this.#props(this.open));
      return;
    }
    this.#loading ??= loadIsland().then(async ({ mountIsland }) => {
      if (generation !== this.#generation || this.#handle) return;
      // First render closed, then open on the next frames, so the first opening slides like every other.
      this.#handle = mountIsland(shadow, this.#props(false));
      this.setAttribute('data-active', '');
      if (this.open) {
        await afterPaint();
        if (generation === this.#generation) this.#handle?.update(this.#props(this.open));
      }
    });
    await this.#loading;
  }
}

function adoptHostStyles(shadow: ShadowRoot): void {
  if (typeof CSSStyleSheet !== 'undefined' && 'adoptedStyleSheets' in shadow) {
    try {
      const sheet = new CSSStyleSheet();
      sheet.replaceSync(HOST_CSS);
      shadow.adoptedStyleSheets = [sheet];
      return;
    } catch {
      // fall through
    }
  }
  const style = document.createElement('style');
  style.textContent = HOST_CSS;
  shadow.append(style);
}

/** Register <ipfs-agent-chat>. Safe to call more than once. */
export function defineIpfsAgentChat(tag: string = ELEMENT_TAG): void {
  if (!customElements.get(tag)) customElements.define(tag, IpfsAgentChatElement);
}

declare global {
  interface HTMLElementTagNameMap {
    'ipfs-agent-chat': IpfsAgentChatElement;
  }
}
