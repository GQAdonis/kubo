import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { NOTICE_EVENT, OPEN_CHANGE_EVENT, defineIpfsAgentChat, IpfsAgentChatElement } from '../../src/element';
import { fixture, sseText } from '../helpers/fixtures';

// The custom element as a host page sees it: what it loads, what it adds to the page, and what it does
// with the bearer. The island chunk is real; only fetch is replaced, by a recorder that answers a chat
// turn with a recorded fixture.

const loads = vi.hoisted(() => ({ count: 0 }));
vi.mock('../../src/island-loader', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../src/island-loader')>();
  return {
    loadIsland: () => {
      loads.count++;
      return actual.loadIsland();
    },
  };
});

const TOKEN = 'secret-bearer-0123456789abcdef';
const ENDPOINT = 'http://127.0.0.1:34567/agui';

interface Recorded {
  url: string;
  headers: Record<string, string>;
}
let recorded: Recorded[];

beforeEach(() => {
  recorded = [];
  defineIpfsAgentChat();
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: RequestInfo | URL, init: RequestInit = {}) => {
      recorded.push({ url: String(url), headers: Object.fromEntries(new Headers(init.headers).entries()) });
      return new Response(sseText(fixture('read-pin-ls').events), { status: 200, headers: { 'content-type': 'text/event-stream' } });
    }),
  );
});

afterEach(() => {
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

function create(attrs: Record<string, string> = {}): IpfsAgentChatElement {
  const el = document.createElement('ipfs-agent-chat');
  for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
  document.body.append(el);
  return el;
}

// user-event prepares elements on a document-level focus listener, which sees the shadow host, not the textarea inside
// it, so its typing never reaches React's value tracking. A native-setter change event is what React expects.
const typeText = (box: HTMLElement, text: string) => fireEvent.change(box, { target: { value: text } });

const panel = (el: IpfsAgentChatElement) => within(el.shadowRoot?.querySelector('[data-ac-mount]') as HTMLElement);
const mounted = (el: IpfsAgentChatElement) => waitFor(() => expect(el.shadowRoot?.querySelector('[data-ac-mount]')).not.toBeNull(), { timeout: 20_000 });

describe('<ipfs-agent-chat> loaded but closed', () => {
  it('defines the element once, however often it is asked', () => {
    defineIpfsAgentChat();
    defineIpfsAgentChat();
    expect(customElements.get('ipfs-agent-chat')).toBe(IpfsAgentChatElement);
  });

  it('loads no React, adds nothing to the document, and makes no request', async () => {
    const headBefore = document.head.innerHTML;
    const el = create({ endpoint: ENDPOINT });
    el.token = TOKEN;
    await new Promise((r) => setTimeout(r, 30));

    expect(loads.count).toBe(0); // the island chunk was never imported
    expect(el.shadowRoot?.querySelector('[data-ac-mount]')).toBeNull();
    expect(document.head.innerHTML).toBe(headBefore);
    expect(document.body.children).toHaveLength(1); // the element itself
    expect(el.hasAttribute('data-active')).toBe(false); // host css keeps it display: none
    expect(recorded).toHaveLength(0);
  });
});

describe('<ipfs-agent-chat> opened', () => {
  it('loads the island on first open and draws the sidebar inside its shadow root only', async () => {
    const el = create({ endpoint: ENDPOINT });
    el.token = TOKEN;
    el.open = true;
    await mounted(el);

    expect(loads.count).toBeGreaterThan(0);
    const aside = el.shadowRoot?.querySelector('aside[data-slot="sidebar"]');
    expect(aside).not.toBeNull();
    expect(el.hasAttribute('data-active')).toBe(true);
    // The host document gained no node outside the element, and no stylesheet or style element.
    expect(document.body.querySelectorAll('*')).toHaveLength(1); // only <ipfs-agent-chat>; its contents are in the shadow root
    expect(document.head.querySelectorAll('style, link').length, document.head.innerHTML.slice(0, 400)).toBe(0);
    expect(document.querySelector('aside')).toBeNull(); // not in the light DOM
    await waitFor(() => expect(aside).toHaveAttribute('data-state', 'open'));
  });

  it('fires openchange when the sidebar opens and when it closes', async () => {
    const el = create({ endpoint: ENDPOINT });
    const events: boolean[] = [];
    el.addEventListener(OPEN_CHANGE_EVENT, (e) => events.push((e as CustomEvent<{ open: boolean }>).detail.open));
    el.openPanel();
    await mounted(el);
    el.close();
    expect(events).toEqual([true, false]);
  });

  it('closes from inside (Escape) and tells the host', async () => {
    const el = create({ endpoint: ENDPOINT });
    el.token = TOKEN;
    el.open = true;
    await mounted(el);
    await waitFor(() => expect(el.shadowRoot?.querySelector('aside')).toHaveAttribute('data-state', 'open'));
    // Escape is handled on the panel, so focus must be inside it.
    const box = panel(el).getByRole('textbox', { name: 'Message to the agent' });
    box.focus();
    await userEvent.setup().keyboard('{Escape}');
    await waitFor(() => expect(el.open).toBe(false));
  });

  it('returns focus to the opener when it closes', async () => {
    const opener = document.createElement('button');
    document.body.append(opener);
    opener.focus();
    const el = create({ endpoint: ENDPOINT });
    el.open = true;
    await mounted(el);
    el.close();
    expect(document.activeElement).toBe(opener);
  });

  it('reflects theme and ignores values it does not know', () => {
    const el = create();
    expect(el.theme).toBe('auto');
    el.theme = 'light';
    expect(el.getAttribute('theme')).toBe('light');
    el.setAttribute('theme', 'neon');
    expect(el.theme).toBe('auto');
  });

  it('unmounts when removed from the page, and mounts again when added back', async () => {
    const el = create({ endpoint: ENDPOINT });
    el.open = true;
    await mounted(el);
    el.remove();
    expect(el.shadowRoot?.querySelector('[data-ac-mount]')).toBeNull();
    document.body.append(el);
    await mounted(el);
  });
});

describe('<ipfs-agent-chat> and the bearer', () => {
  it('takes the token as a property and never gives it back', () => {
    const el = create();
    el.token = TOKEN;
    expect(el.token).toBeUndefined();
    expect(el.tokenSet).toBe(true);
    expect(el.getAttribute('token')).toBeNull();
    expect(JSON.stringify(el)).not.toContain(TOKEN);
    expect(Object.getOwnPropertyNames(el).join()).not.toContain('token');
    expect(Object.values(el).includes(TOKEN)).toBe(false);
    expect(el.outerHTML).not.toContain(TOKEN);
  });

  it('removes a token attribute and never reads it', async () => {
    document.body.innerHTML = `<ipfs-agent-chat endpoint="${ENDPOINT}" token="${TOKEN}" open></ipfs-agent-chat>`;
    const el = document.querySelector('ipfs-agent-chat') as IpfsAgentChatElement;
    expect(el.hasAttribute('token')).toBe(false);
    expect(el.tokenSet).toBe(false);
    await mounted(el);
    expect(panel(el).getByText('Chat has no token')).toBeInTheDocument();
    expect(document.documentElement.outerHTML).not.toContain(TOKEN);
  });

  it('announces the ignored attribute to the host', () => {
    const el = document.createElement('ipfs-agent-chat');
    const notices: string[] = [];
    el.addEventListener(NOTICE_EVENT, (e) => notices.push((e as CustomEvent<{ message: string }>).detail.message));
    el.setAttribute('token', TOKEN);
    document.body.append(el);
    expect(notices).toEqual(['The token attribute is ignored and removed. Set the token property.']);
  });

  it('sends the bearer only as an Authorization header, to the configured origin, and stores it nowhere', async () => {
    const setItem = vi.spyOn(Storage.prototype, 'setItem');
    const el = create({ endpoint: ENDPOINT });
    el.token = TOKEN;
    el.open = true;
    await mounted(el);
    const ui = panel(el);
    const user = userEvent.setup();
    typeText(ui.getByRole('textbox', { name: 'Message to the agent' }), 'Which recursive pins do I have?');
    await user.click(ui.getByRole('button', { name: 'Send message' }));
    await waitFor(() => expect(recorded).toHaveLength(1));
    await ui.findByText('You have one recursive pin, named wiki.');

    expect(recorded[0]?.url).toBe(ENDPOINT);
    expect(recorded[0]?.headers['authorization']).toBe(`Bearer ${TOKEN}`);
    expect(recorded.every((r) => new URL(r.url).origin === new URL(ENDPOINT).origin)).toBe(true);
    expect(recorded[0]?.url).not.toContain(TOKEN);

    // Nowhere readable from the page.
    expect(setItem).not.toHaveBeenCalled();
    expect(localStorage.length).toBe(0);
    expect(sessionStorage.length).toBe(0);
    expect(document.cookie).toBe('');
    expect(document.documentElement.outerHTML).not.toContain(TOKEN);
    expect(el.shadowRoot?.innerHTML).not.toContain(TOKEN);
  });

  it('asks the token provider for each request', async () => {
    const provider = vi.fn(async () => TOKEN);
    const el = create({ endpoint: ENDPOINT });
    el.tokenProvider = provider;
    expect(el.tokenProvider).toBeUndefined();
    el.open = true;
    await mounted(el);
    const ui = panel(el);
    const user = userEvent.setup();
    typeText(ui.getByRole('textbox', { name: 'Message to the agent' }), 'Which recursive pins do I have?');
    await user.click(ui.getByRole('button', { name: 'Send message' }));
    await waitFor(() => expect(recorded).toHaveLength(1));
    expect(provider).toHaveBeenCalledTimes(1);
    expect(recorded[0]?.headers['authorization']).toBe(`Bearer ${TOKEN}`);
  });

  it('sends nothing when the endpoint is not acceptable, even with a token', async () => {
    const el = create({ endpoint: 'http://attacker.example.net/agui' });
    el.token = TOKEN;
    el.open = true;
    await mounted(el);
    expect(panel(el).getByRole('textbox', { name: 'Message to the agent' })).toBeDisabled();
    expect(recorded).toHaveLength(0);
    expect(screen.queryByText(TOKEN)).toBeNull();
  });
});
