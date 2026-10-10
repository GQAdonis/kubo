import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { fixture } from '../helpers/fixtures';
import { TOKEN, idsFor, mountIsland, say, untilIdle } from '../helpers/island';
import { createFakeFetch } from '../helpers/fake-fetch';

// Configuration and failure states of the panel, in the DESIGN.md voice.

describe('panel states', () => {
  it('is not configured without an endpoint, and sends nothing', async () => {
    const m = mountIsland({ props: { endpoint: undefined } });
    expect(screen.getByText('Chat is not configured')).toBeInTheDocument();
    expect(screen.getByText('Not configured', { selector: '.iu-badge' })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Message to the agent' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Send message' })).toBeDisabled();
    expect(m.fake.calls).toHaveLength(0);
  });

  it('explains why an endpoint was refused', () => {
    mountIsland({ props: { endpoint: 'http://node.example.net/agui' } });
    expect(screen.getByText(/uses http on a non-loopback host/)).toBeInTheDocument();
  });

  it('needs a token before it will send, and says where the token comes from', () => {
    const m = mountIsland({ props: { tokenSet: false } });
    expect(screen.getByText('Chat has no token')).toBeInTheDocument();
    expect(screen.getByText(/The host page supplies the token/)).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Message to the agent' })).toBeDisabled();
    expect(m.fake.calls).toHaveLength(0);
  });

  it('is ready with an invitation that states the confirmation rules', () => {
    mountIsland();
    expect(screen.getByText('Ask about your node')).toBeInTheDocument();
    expect(screen.getByText(/A change waits for your approval in this panel/)).toBeInTheDocument();
    expect(screen.getByRole('complementary', { name: 'Agent chat' })).toBeInTheDocument();
    expect(screen.getByText('Ready', { selector: '.iu-badge' })).toBeInTheDocument();
  });

  it('reports a refused token in words and stays usable', async () => {
    const m = mountIsland({ fake: createFakeFetch({ token: 'a-different-token' }), props: { ids: idsFor(fixture('read-pin-ls'), ['run-1']) } });
    await say(m, 'hello');
    await screen.findByText(/The token was refused/);
    expect(screen.getByText('Failed', { selector: '.iu-badge' })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Message to the agent' })).toBeEnabled();
  });

  it('sends with Enter, adds a line with Shift and Enter, and leaves IME composition alone', async () => {
    const f = fixture('read-pin-ls');
    const m = mountIsland({ props: { ids: idsFor(f, ['run-1']) } });
    const box = screen.getByRole('textbox', { name: 'Message to the agent' });
    await m.user.type(box, 'one{Shift>}{Enter}{/Shift}two');
    expect(box).toHaveValue('one\ntwo');
    expect(m.fake.calls).toHaveLength(0);
    await m.user.clear(box);
    await m.user.type(box, String(f.input.messages[0]?.['content']) + '{Enter}');
    await untilIdle();
    expect(m.fake.calls).toHaveLength(1);
    expect(box).toHaveValue('');
  });

  it('starts a new chat on request', async () => {
    const f = fixture('read-pin-ls');
    const m = mountIsland({ props: { ids: idsFor(f, ['run-1']) } });
    await say(m, String(f.input.messages[0]?.['content']));
    await untilIdle();
    await screen.findByText('You have one recursive pin, named wiki.');
    await m.user.click(screen.getByRole('button', { name: 'New chat' }));
    expect(screen.queryByText('You have one recursive pin, named wiki.')).not.toBeInTheDocument();
    expect(screen.getByText('Ask about your node')).toBeInTheDocument();
  });

  it('closes from the Close button and from Escape', async () => {
    const closed: boolean[] = [];
    const m = mountIsland({ props: { onOpenChange: (o) => closed.push(o) } });
    await m.user.click(screen.getByRole('button', { name: 'Close' }));
    await m.user.keyboard('{Escape}');
    expect(closed).toEqual([false, false]);
    expect(TOKEN).toBeTruthy();
  });

  it('is unreachable when closed: inert, not just off screen', () => {
    mountIsland({ props: { open: false } });
    const panel = document.querySelector('aside[data-slot="sidebar"]');
    expect(panel).toHaveAttribute('data-state', 'closed');
    expect(panel).toHaveAttribute('inert');
  });
});
