import { screen, waitFor } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { defineIpfsAgentChat, type IpfsAgentChatElement } from '../../src/element';
import { axeViolations, describeViolations } from '../helpers/axe';
import { fixture, type AguiFixture } from '../helpers/fixtures';
import { idsFor, mountIsland, say, untilIdle } from '../helpers/island';

// axe over the open panel in every state that draws something different, in both themes. Contrast and
// target size need layout; the Playwright a11y spec runs them in a real browser.

const states: { name: string; fixtureName?: string; settle: () => Promise<unknown> }[] = [
  { name: 'empty', settle: async () => screen.getByText('Ask about your node') },
  { name: 'a read result', fixtureName: 'read-pin-ls', settle: () => screen.findByRole('table', { name: 'Pins' }) },
  { name: 'a refused egress', fixtureName: 'egress-refused', settle: () => screen.findByText(/I cannot read that file/) },
  { name: 'a pending write approval', fixtureName: 'write-interrupt', settle: () => screen.findByText('Awaiting a person') },
  { name: 'a run stopped at its step limit', fixtureName: 'run-error-step-limit', settle: () => screen.findByText(/reached its step limit/) },
  { name: 'a model backend failure', fixtureName: 'run-error-backend-failure', settle: () => screen.findByText(/model backend failed/) },
  { name: 'a native-prompt approval', fixtureName: 'destructive-native-prompt', settle: () => screen.findByText('Waiting for system prompt') },
];

describe.each(['dark', 'light'] as const)('axe, %s theme', (theme) => {
  it.each(states)('finds no WCAG 2.2 AA violation with $name', async ({ fixtureName, settle }) => {
    const f: AguiFixture | undefined = fixtureName ? fixture(fixtureName) : undefined;
    const m = mountIsland({ props: { theme, ...(f && { ids: idsFor(f, ['run-1']) }) } });
    if (f) {
      await say(m, String(f.input.messages[0]?.['content']));
      await untilIdle();
    }
    await settle();
    const aside = screen.getByRole('complementary', { name: 'Agent chat' });
    const violations = await axeViolations(aside);
    expect(violations, describeViolations(violations)).toEqual([]);
  });
});

describe('axe through the shadow root', () => {
  it('finds no violation in the element as a host page sees it', async () => {
    defineIpfsAgentChat();
    const el = document.createElement('ipfs-agent-chat') as IpfsAgentChatElement;
    el.setAttribute('endpoint', 'http://127.0.0.1:34567/agui');
    el.token = 'x'.repeat(24);
    document.body.append(el);
    el.open = true;
    await waitFor(() => expect(el.shadowRoot?.querySelector('aside')).not.toBeNull());
    const violations = await axeViolations(el);
    expect(violations, describeViolations(violations)).toEqual([]);
    el.remove();
  });
});

describe.each(['dark', 'light'] as const)('axe, %s theme, node with no backend', (theme) => {
  it('finds no WCAG 2.2 AA violation in the pick-a-model state', async () => {
    const noBackend = (async () =>
      new Response(JSON.stringify({ error: { code: 'backend_not_configured', message: 'no model backend is configured', retryable: false } }), {
        status: 503,
        headers: { 'content-type': 'application/json' },
      })) as typeof fetch;
    const m = mountIsland({ props: { theme, fetchImpl: noBackend, ids: idsFor(fixture('read-pin-ls'), ['run-1']) } });
    await say(m, 'Which pins do I have?');
    await screen.findByText('Choose a backend to start');
    const violations = await axeViolations(screen.getByRole('complementary', { name: 'Agent chat' }));
    expect(violations, describeViolations(violations)).toEqual([]);
  });
});
