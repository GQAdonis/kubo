import { screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { foldAguiEvents, toWire, type AguiEvent } from '@gqadonis/ipfs-mcp-ui';
import { A2UI_CATALOG_ID } from '@gqadonis/ipfs-mcp-ui/a2ui';
import { fixture, type AguiFixture } from '../helpers/fixtures';
import { createFakeFetch } from '../helpers/fake-fetch';
import { TOKEN, idsFor, mountIsland, say, untilIdle } from '../helpers/island';

// mcp-agui-a2ui-registry 1.3: the island renders the surfaces the server streams as CUSTOM `a2ui`
// events, and keeps its local ToolCallCard fallback for calls and streams without them. Both checks
// replay the recorded contract fixture read-pin-ls; the "with" variant inserts the three CUSTOM events
// the server emits for the tool result (mcp/server/agui_a2ui.go) right after it, in the same shapes.
//
// The visible difference between the two paths is the EgressBadge: pin_ls carries node content, and
// only the server knows the principal's acknowledged hosts. A server surface says "May reach ...";
// the local fallback has no hosts to name.

const custom = (value: Record<string, unknown>): AguiEvent => ({ type: 'CUSTOM', name: 'a2ui', value });

/** The fixture with the server's A2UI surface inserted after each TOOL_CALL_RESULT. */
function withServerSurfaces(f: AguiFixture, acknowledgedHosts: readonly string[]): AguiFixture {
  const state = foldAguiEvents(f.events);
  const events: AguiEvent[] = [];
  for (const e of f.events) {
    events.push(e);
    if (e.type !== 'TOOL_CALL_RESULT') continue;
    const call = state.toolCalls.find((c) => c.id === e['toolCallId']);
    if (!call) continue;
    const surfaceId = `call-${call.id}`;
    const card: Record<string, unknown> = { id: 'root', component: 'ToolCallCard', call: { path: '/call' } };
    if (acknowledgedHosts.length > 0) card['acknowledgedHosts'] = [...acknowledgedHosts];
    events.push(
      custom({ version: 'v0.9', createSurface: { surfaceId, catalogId: A2UI_CATALOG_ID } }),
      custom({ version: 'v0.9', updateComponents: { surfaceId, components: [card] } }),
      custom({ version: 'v0.9', updateDataModel: { surfaceId, path: '/call', value: toWire(call) } }),
    );
  }
  return { ...f, events };
}

describe('server-emitted A2UI surfaces in the chat island', () => {
  it('with server surfaces: the card is the server surface, hosts and all, over the same fixture', async () => {
    const f = fixture('read-pin-ls');
    const withSurfaces = withServerSurfaces(f, ['local']);
    expect(withSurfaces.events.some((e) => e.type === 'CUSTOM' && e['name'] === 'a2ui')).toBe(true);
    const m = mountIsland({
      fake: createFakeFetch({ fixtures: [withSurfaces], token: TOKEN }),
      props: { ids: idsFor(f, ['run-1']) },
    });
    await say(m, String(f.input.messages[0]?.['content']));
    await untilIdle();

    // The result still draws as the catalog's PinList view, inside the ToolCallCard.
    const card = await screen.findByRole('article', { name: 'Tool call pin_ls' });
    expect(within(card).getByRole('table', { name: 'Pins' })).toBeInTheDocument();
    expect(within(card).getByText('Done')).toBeInTheDocument();
    // Only the server surface can name the acknowledged hosts; the local fallback never sets them.
    expect(within(card).getByText(/May reach 1 acknowledged host: local\./)).toBeInTheDocument();
    expect(within(card).queryByText(/No host is acknowledged/)).not.toBeInTheDocument();
    expect(screen.getByText('You have one recursive pin, named wiki.')).toBeInTheDocument();
  });

  it('without server surfaces: the local ToolCallCard fallback draws the same result', async () => {
    const f = fixture('read-pin-ls');
    expect(f.events.some((e) => e.type === 'CUSTOM')).toBe(false); // the recorded stream emits none
    const m = mountIsland({ props: { ids: idsFor(f, ['run-1']) } });
    await say(m, String(f.input.messages[0]?.['content']));
    await untilIdle();

    const card = await screen.findByRole('article', { name: 'Tool call pin_ls' });
    expect(within(card).getByRole('table', { name: 'Pins' })).toBeInTheDocument();
    expect(within(card).getByText('Done')).toBeInTheDocument();
    // The fallback has no acknowledged hosts to name, so the badge says so.
    expect(within(card).getByText(/No host is acknowledged/)).toBeInTheDocument();
    expect(within(card).queryByText(/May reach/)).not.toBeInTheDocument();
  });

  it('with server surfaces for an error result: the card shows the failure and the refusal badge', async () => {
    const f = fixture('egress-refused');
    const withSurfaces = withServerSurfaces(f, []); // the principal acknowledged no host
    const m = mountIsland({
      fake: createFakeFetch({ fixtures: [withSurfaces], token: TOKEN }),
      props: { ids: idsFor(f, ['run-1']) },
    });
    await say(m, String(f.input.messages[0]?.['content']));
    await untilIdle();

    const card = await screen.findByRole('article', { name: 'Tool call ipfs_cat' });
    expect(within(card).getByText('Failed')).toBeInTheDocument();
    expect(card.textContent).toMatch(/Content not sent/);
  });
});
