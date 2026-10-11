import { act, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MessageProcessor } from '@a2ui/web_core/v0_9';
import { A2uiSurface, type ReactComponentImplementation } from '@a2ui/react/v0_9';
import { describe, expect, it, vi } from 'vitest';
import { A2UI_CATALOG_ID, ipfsCatalog } from '../../src/a2ui/catalog';
import { blockNetwork } from '../helpers/render';
import { OUTPUTS } from '../fixtures/outputs';
import { readAguiFixtures } from '../helpers/contract';

type Msg = Record<string, unknown>;

function mount(messages: Msg[], onAction = vi.fn()) {
  const processor = new MessageProcessor<ReactComponentImplementation>([ipfsCatalog], onAction);
  processor.processMessages(messages as never);
  const surfaces = Array.from(processor.model.surfacesMap.values());
  const view = render(
    <>
      {surfaces.map((s) => (
        <A2uiSurface key={s.id} surface={s} />
      ))}
    </>,
  );
  return { processor, view, onAction };
}

const create = (extra: Msg = {}): Msg => ({ version: 'v0.9', createSurface: { surfaceId: 's1', catalogId: A2UI_CATALOG_ID, ...extra } });
const components = (list: Msg[]): Msg => ({ version: 'v0.9', updateComponents: { surfaceId: 's1', components: list } });
const data = (value: unknown, path = '/'): Msg => ({ version: 'v0.9', updateDataModel: { surfaceId: 's1', path, value } });

describe('A2UI surface rendering with the IPFS catalog', () => {
  it('renders the pin list from a data-model path, offline', async () => {
    const net = blockNetwork();
    try {
      mount([
        create(),
        components([{ id: 'root', component: 'PinList', tool: 'pin_ls', output: { path: '/pins' } }]),
        data(OUTPUTS.pin_ls[0], '/pins'),
      ]);
      const table = await screen.findByRole('table', { name: 'Pins' });
      expect(within(table).getAllByRole('row')).toHaveLength(4);
    } finally {
      net.restore();
    }
    expect(net.calls).toEqual([]);
  });

  it('renders a literal contract output and follows data-model updates', async () => {
    const { processor } = mount([
      create(),
      components([{ id: 'root', component: 'PinList', tool: 'pin_ls', output: { path: '/o' } }]),
      data({ pins: [], page: { has_more: false } }, '/o'),
    ]);
    expect(await screen.findByText(/No pins match/)).toBeInTheDocument();
    act(() => processor.processMessages([data(OUTPUTS.pin_ls[0], '/o')] as never));
    expect(await screen.findByRole('table', { name: 'Pins' })).toBeInTheDocument();
  });

  it('says so, and draws nothing else, when bound data does not match the contract', async () => {
    mount([
      create(),
      components([{ id: 'root', component: 'PinList', tool: 'pin_ls', output: { path: '/o' } }]),
      data({ pins: [{ cid: 'bafy', type: 'sideways' }], page: { has_more: false } }, '/o'),
    ]);
    expect(await screen.findByText('Invalid data.')).toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });

  it('honours createSurface.theme.mode', async () => {
    const { view } = mount([
      create({ theme: { mode: 'light' } }),
      components([{ id: 'root', component: 'NodeStatusPanel', tool: 'stats_bw', output: { path: '/o' } }]),
      data(OUTPUTS.stats_bw[0], '/o'),
    ]);
    await screen.findByText('Receiving now');
    expect(view.container.querySelector('[data-ipfs-ui]')).toHaveAttribute('data-theme', 'light');
  });

  it('renders a confirmation with approve wired to the action, and dispatches it once', async () => {
    const onAction = vi.fn();
    mount(
      [
        create(),
        components([
          {
            id: 'root',
            component: 'ConfirmationDialog',
            confirmation: { path: '/c' },
            approveAction: { event: { name: 'approve', context: { id: 'cf' } } },
            denyAction: { event: { name: 'deny' } },
          },
        ]),
        data(
          {
            confirmation_id: 'cf_3pQm8vK2xR7tLw9a',
            state: 'pending',
            summary: { tool: 'pin_add', class: 'write', text: 'Pin bafy… (recursive=true)', targets: [{ kind: 'cid', value: 'bafy' }] },
            expires_at: '2026-10-09T12:02:00Z',
            resolvers: ['elicitation'],
          },
          '/c',
        ),
      ],
      onAction,
    );
    const approve = await screen.findByRole('button', { name: 'Approve pin' });
    await userEvent.click(approve);
    expect(onAction).toHaveBeenCalledTimes(1);
  });

  it('draws no approve control when the action is not wired', async () => {
    mount([
      create(),
      components([{ id: 'root', component: 'ConfirmationDialog', confirmation: { path: '/c' } }]),
      data(
        {
          confirmation_id: 'cf_3pQm8vK2xR7tLw9a',
          state: 'pending',
          summary: { tool: 'repo_gc', class: 'destructive', text: 'Run garbage collection', targets: [{ kind: 'repo', value: 'repo' }] },
          expires_at: '2026-10-09T12:02:00Z',
          resolvers: ['macos-localauth'],
        },
        '/c',
      ),
    ]);
    await screen.findByRole('group', { name: /garbage collection/i });
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('renders every AG-UI fixture interrupt as an ApprovalCard through the catalog', async () => {
    const interrupts = readAguiFixtures()
      .flatMap(({ fixture }) => fixture.events)
      .flatMap((e) => ((e['outcome'] as { interrupts?: unknown[] } | undefined)?.interrupts ?? []));
    expect(interrupts.length).toBeGreaterThanOrEqual(2);
    for (const interrupt of interrupts) {
      const { view } = mount([create(), components([{ id: 'root', component: 'ApprovalCard', interrupt }])]);
      expect(await screen.findAllByRole('group', { name: /./ })).not.toHaveLength(0);
      view.unmount();
    }
  });
});
