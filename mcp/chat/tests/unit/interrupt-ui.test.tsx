import { act, fireEvent, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { fixture, loadFixtures } from '../helpers/fixtures';
import { createFakeFetch } from '../helpers/fake-fetch';
import { idsFor, mountIsland, say, untilIdle, TOKEN } from '../helpers/island';

// G11-3 in the island: an interrupt is answered only by a person. These tests use the real card and the
// real DOM. jsdom marks every synthetic event untrusted, which is exactly what a script's click is, so
// "no answer without a gesture" is observable here; the positive path provides a trust probe, and the
// Playwright spec (tests/e2e) proves the same with real browser input.

const pin = fixture('write-interrupt');
const userText = String(pin.input.messages[0]?.['content']);

async function reachApproval(trustAll: boolean) {
  const m = mountIsland({ trustAll, props: { ids: idsFor(pin, ['run-1', 'run-2']) } });
  await say(m, userText);
  await screen.findByRole('group', { name: /Pin bafkrei/ });
  return m;
}

describe('answering an interrupt', () => {
  afterEach(() => vi.useRealTimers());

  it('shows the approval with Deny and Approve, and holds the run until a person answers', async () => {
    const m = await reachApproval(false);
    expect(screen.getByRole('button', { name: 'Deny' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Approve change' })).toBeInTheDocument();
    expect(screen.getByText('Awaiting a person')).toBeInTheDocument();
    expect(m.fake.calls).toHaveLength(1);
    // The composer is locked until the approval is answered.
    expect(screen.getByRole('textbox', { name: 'Message to the agent' })).toBeDisabled();
  });

  it('moves focus to Deny when the approval appears', async () => {
    await reachApproval(false);
    expect(screen.getByRole('button', { name: 'Deny' })).toHaveFocus();
  });

  it('refuses a scripted click: fireEvent and element.click() produce untrusted events and send nothing', async () => {
    const m = await reachApproval(false);
    const approve = screen.getByRole('button', { name: 'Approve change' });

    fireEvent.click(approve);
    await screen.findByText(/An answer needs a click or key press from you/);
    approve.click();
    approve.dispatchEvent(new MouseEvent('click', { bubbles: true, composed: true }));

    expect(m.fake.calls).toHaveLength(1); // only the first run was ever sent
    expect(screen.getByText('Awaiting a person')).toBeInTheDocument();
    expect(screen.queryByText('Approved')).not.toBeInTheDocument();
  });

  it('refuses a scripted Deny too: no answer of either kind without a gesture', async () => {
    const m = await reachApproval(false);
    fireEvent.click(screen.getByRole('button', { name: 'Deny' }));
    await screen.findByText(/An answer needs a click or key press from you/);
    expect(m.fake.calls).toHaveLength(1);
  });

  it('resumes with exactly the recorded resume input when a person approves', async () => {
    const m = await reachApproval(true);
    await m.user.click(screen.getByRole('button', { name: 'Approve change' }));
    await untilIdle();

    expect(m.fake.calls).toHaveLength(2);
    expect(m.fake.calls[1]?.body).toEqual(fixture('write-resume-approved').input);
    expect(m.fake.calls[1]?.matched).toBe('write-resume-approved');
    await screen.findByText('Pinned.');
    expect(screen.getByText('Approved')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Approve change' })).not.toBeInTheDocument();
  });

  it('works from the keyboard: Enter on the focused control is a key press the browser reports as a click', async () => {
    const m = await reachApproval(true);
    screen.getByRole('button', { name: 'Approve change' }).focus();
    await m.user.keyboard('{Enter}');
    await untilIdle();
    expect(m.fake.calls).toHaveLength(2);
  });

  it('sends a denial as resolved with approved: false, and never as an approval', async () => {
    const m = await reachApproval(true);
    await m.user.click(screen.getByRole('button', { name: 'Deny' }));
    await waitFor(() => expect(m.fake.calls).toHaveLength(2));
    expect(m.fake.calls[1]?.body).toMatchObject({
      resume: [{ interruptId: 'int-cf_Zr4Yt1nB6kq0HdUe', status: 'resolved', payload: { approved: false } }],
    });
    expect(screen.getByText('Denied')).toBeInTheDocument();
  });

  describe('a call that needs the system prompt', () => {
    const gc = fixture('destructive-native-prompt');
    const gcText = String(gc.input.messages[0]?.['content']);
    const mountHeld = () => {
      const fake = createFakeFetch({ token: TOKEN, holdWait: true });
      return mountIsland({ trustAll: true, fake, props: { ids: idsFor(gc, ['run-1', 'run-2', 'run-3']) } });
    };

    it('is watched by itself: {wait: true} goes out after the card shows, with no Approve control, only Deny', async () => {
      const m = mountHeld();
      await say(m, gcText);
      await screen.findByRole('group', { name: /Run garbage collection/ });
      await waitFor(() => expect(m.fake.calls).toHaveLength(2));
      expect(m.fake.calls[1]?.body).toMatchObject({
        runId: 'run-2',
        resume: [{ interruptId: 'int-cf_Ui7dEstrUct1veXy', status: 'resolved', payload: { wait: true } }],
      });
      const card = screen.getByRole('group', { name: /Run garbage collection/ });
      expect(within(card).getByText('Waiting for system prompt')).toBeInTheDocument();
      expect(screen.queryByRole('button', { name: /^Approve/ })).not.toBeInTheDocument();
      expect(within(card).getByRole('button', { name: 'Deny' })).toBeEnabled();
      expect(within(card).getByRole('button', { name: 'Deny' })).toHaveFocus();
      expect(within(card).getByText(/Approval happens in the system prompt on this machine/i)).toBeInTheDocument();
      expect(screen.getAllByText('Destructive')).toHaveLength(2); // the tool call's class and the approval's
    });

    it('stops watching and sends the denial when the person denies in the chat', async () => {
      const m = mountHeld();
      await say(m, gcText);
      await waitFor(() => expect(m.fake.calls).toHaveLength(2));
      const watch = m.fake.calls[1]?.init.signal as AbortSignal;
      expect(watch.aborted).toBe(false);

      await m.user.click(within(screen.getByRole('group', { name: /Run garbage collection/ })).getByRole('button', { name: 'Deny' }));
      await waitFor(() => expect(m.fake.calls).toHaveLength(3));
      expect(watch.aborted).toBe(true);
      expect(m.fake.calls[2]?.body).toMatchObject({
        runId: 'run-3',
        resume: [{ interruptId: 'int-cf_Ui7dEstrUct1veXy', status: 'resolved', payload: { approved: false } }],
      });
      expect(within(screen.getByRole('group', { name: /Run garbage collection/ })).getByText('Denied')).toBeInTheDocument();
    });

    it('never shows Approve, and never sends an approval, after the system prompt approved', async () => {
      const m = mountIsland({ trustAll: true, props: { ids: idsFor(gc, ['run-1', 'run-2']) } });
      await say(m, gcText);
      // The recorded observation: the person approved in the system prompt, the call ran once.
      await screen.findByText('Garbage collection removed 42 blocks.');
      const card = screen.getByRole('group', { name: /Run garbage collection/ });
      expect(within(card).getByText('Approved')).toBeInTheDocument();
      expect(within(card).queryByRole('button')).not.toBeInTheDocument();
      expect(m.fake.calls.map((c) => c.matched)).toEqual(['destructive-native-prompt', 'destructive-native-prompt-wait-approved']);
      expect(JSON.stringify(m.fake.calls.map((c) => c.body))).not.toContain('"approved":true');
    });

    it('shows a declined or expired outcome from the observation, and lets the person write again', async () => {
      for (const [outcome, word] of [
        ['destructive-native-prompt-wait-denied', 'Denied'],
        ['destructive-native-prompt-wait-expired', 'Expired'],
      ] as const) {
        const fixtures = loadFixtures().filter((f) => !f.name.startsWith('destructive-native-prompt-wait-') || f.name === outcome);
        const fake = createFakeFetch({ token: TOKEN, fixtures });
        const m = mountIsland({ trustAll: true, fake, props: { ids: idsFor(gc, ['run-1', 'run-2']) } });
        await say(m, gcText);
        await untilIdle();
        const card = await screen.findByRole('group', { name: /Run garbage collection/ });
        await waitFor(() => expect(within(card).getAllByText(word).length).toBeGreaterThan(0));
        expect(within(card).queryByRole('button')).not.toBeInTheDocument();
        expect(screen.getByRole('textbox', { name: 'Message to the agent' })).toBeEnabled();
        m.view.unmount();
      }
    });

    it('offers to watch again when the watch dropped, and watches again on request', async () => {
      // The first observation answers with a stream that ends before the run does.
      const complete = createFakeFetch({ token: TOKEN, holdWait: true });
      let waits = 0;
      const dropping = (async (input: RequestInfo | URL, init?: RequestInit) => {
        if (typeof init?.body === 'string' && init.body.includes('"wait":true') && waits++ === 0) {
          return new Response('data: {"type":"RUN_STARTED","threadId":"thread-gc-1","runId":"run-2"}\n\n', {
            status: 200,
            headers: { 'content-type': 'text/event-stream' },
          });
        }
        return complete.fetch(input, init);
      }) as typeof fetch;
      // The retry is the same observation again, so it carries the run id the recorded fixture expects.
      const m = mountIsland({ trustAll: true, props: { fetchImpl: dropping, ids: idsFor(gc, ['run-1', 'run-2', 'run-2']) } });
      await say(m, gcText);
      const again = await screen.findByRole('button', { name: 'Check the system prompt again' });
      await m.user.click(again);
      await waitFor(() => expect(complete.calls.filter((c) => JSON.stringify(c.body).includes('"wait":true'))).toHaveLength(1));
      await waitFor(() => expect(screen.queryByRole('button', { name: 'Check the system prompt again' })).not.toBeInTheDocument());
    });
  });

  it('shows an expired interrupt as expired with no controls, and lets the person write again', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-10-09T12:02:30Z')); // the fixture expires at 12:02:00Z
    const m = mountIsland({ trustAll: true, props: { ids: idsFor(pin, ['run-1']), now: () => Date.now() } });
    // userEvent and the stream use real timers; only Date is pinned.
    await say(m, userText);
    await screen.findByText('Expired');
    expect(screen.queryByRole('button', { name: 'Approve change' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Deny' })).not.toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Message to the agent' })).toBeEnabled();
  });

  it('flips a pending approval to expired when its expiry passes', async () => {
    vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout'] });
    vi.setSystemTime(new Date('2026-10-09T12:01:58Z'));
    const m = mountIsland({ trustAll: true, props: { ids: idsFor(pin, ['run-1']), now: () => Date.now() } });
    fireEvent.change(screen.getByRole('textbox', { name: 'Message to the agent' }), { target: { value: userText } });
    fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
    await vi.waitFor(() => expect(screen.getByRole('group', { name: /Pin bafkrei/ })).toBeInTheDocument());
    expect(screen.getByText('Awaiting a person')).toBeInTheDocument();

    await act(async () => {
      vi.setSystemTime(new Date('2026-10-09T12:02:01Z'));
      await vi.advanceTimersByTimeAsync(3000);
    });
    expect(within(screen.getByRole('group', { name: /Pin bafkrei/ })).getByText('Expired')).toBeInTheDocument();
    expect(m.fake.calls).toHaveLength(1);
  });
});
