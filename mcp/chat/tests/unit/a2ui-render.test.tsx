import { screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { A2UI_CATALOG_ID } from '@gqadonis/ipfs-mcp-ui/a2ui';
import { fixture } from '../helpers/fixtures';
import { idsFor, mountIsland, say, untilIdle } from '../helpers/island';

// Tool results in the chat are drawn by the shared A2UI catalog (ipfsCatalog), validated against the
// contract's output schema first. These tests run whole fixture conversations and look at the DOM.

describe('A2UI rendering of tool results in the chat', () => {
  it('draws pin_ls as the PinList view from the catalog, inside a ToolCallCard', async () => {
    const f = fixture('read-pin-ls');
    const m = mountIsland({ props: { ids: idsFor(f, ['run-1']) } });
    await say(m, String(f.input.messages[0]?.['content']));
    await untilIdle();

    const card = await screen.findByRole('article', { name: 'Tool call pin_ls' });
    const table = within(card).getByRole('table', { name: 'Pins' });
    expect(within(table).getByText('wiki')).toBeInTheDocument();
    expect(within(card).getByText('Done')).toBeInTheDocument();
    expect(within(card).getByText('Read only')).toBeInTheDocument();
    expect(screen.getByText('You have one recursive pin, named wiki.')).toBeInTheDocument();
  });

  it('shows the contract refusal as a failure and the refused egress badge, never the content', async () => {
    const f = fixture('egress-refused');
    const m = mountIsland({ props: { ids: idsFor(f, ['run-1']) } });
    await say(m, String(f.input.messages[0]?.['content']));
    await untilIdle();

    const card = await screen.findByRole('article', { name: 'Tool call ipfs_cat' });
    expect(within(card).getByText('Failed')).toBeInTheDocument();
    expect(card.textContent).toMatch(/Content not sent/);
    expect(screen.getByText(/I cannot read that file/)).toBeInTheDocument();
  });

  it('says a pending call has not run, and shows the approval beside it', async () => {
    const f = fixture('write-interrupt');
    const m = mountIsland({ props: { ids: idsFor(f, ['run-1']) } });
    await say(m, String(f.input.messages[0]?.['content']));
    const card = await screen.findByRole('article', { name: 'Tool call pin_add' });
    expect(within(card).getByText('Waiting for approval')).toBeInTheDocument();
    expect(within(card).getByText(/Nothing has run/)).toBeInTheDocument();
    expect(screen.getAllByText('Waiting for approval').length).toBeGreaterThanOrEqual(2); // the header badge and the call card
  });

  it('draws a result the contract does not describe as data, not as a view', async () => {
    const f = fixture('read-pin-ls');
    const bad = {
      ...f,
      events: f.events.map((e) =>
        e.type === 'TOOL_CALL_RESULT' ? { ...e, content: JSON.stringify({ pins: [{ cid: 'bafy', type: 'sideways' }], page: { has_more: false } }) } : e,
      ),
    };
    const { createFakeFetch } = await import('../helpers/fake-fetch');
    const m = mountIsland({ fake: createFakeFetch({ fixtures: [bad], token: 'ui-bearer-0123456789abcdef' }), props: { ids: idsFor(f, ['run-1']) } });
    await say(m, String(f.input.messages[0]?.['content']));
    await untilIdle();
    await screen.findByText(/Result does not match the contract/);
    expect(screen.queryByRole('table', { name: 'Pins' })).not.toBeInTheDocument();
  });

  it('uses the published catalog id for its surfaces', () => {
    expect(A2UI_CATALOG_ID).toMatch(/^https:\/\//);
  });

  it('treats model text as plain text, not markup', async () => {
    const f = fixture('read-pin-ls');
    const evil = {
      ...f,
      events: f.events.map((e) =>
        e.type === 'TEXT_MESSAGE_CONTENT' ? { ...e, delta: '<img src=x onerror="alert(1)"><b>bold</b>' } : e,
      ),
    };
    const { createFakeFetch } = await import('../helpers/fake-fetch');
    const m = mountIsland({ fake: createFakeFetch({ fixtures: [evil], token: 'ui-bearer-0123456789abcdef' }), props: { ids: idsFor(f, ['run-1']) } });
    await say(m, String(f.input.messages[0]?.['content']));
    await untilIdle();
    const log = await screen.findByRole('log', { name: 'Conversation' });
    await screen.findByText(/<img src=x/);
    expect(log.querySelector('img')).toBeNull();
    expect(log.querySelector('b')).toBeNull();
  });
});
