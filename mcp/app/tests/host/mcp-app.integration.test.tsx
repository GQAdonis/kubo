// Real SDK on both sides: the MCP Apps App (view side) connected to an AppBridge (host side) over an
// in-memory transport. Proves the wiring in McpAppRoot and createMcpPort against the protocol
// itself: ui/initialize, tool input and result, tools/call proxying, ui/message,
// ui/update-model-context and host context changes. Only the transport is substituted for the
// browser's postMessage; useApp is replaced by an equivalent that uses it.
import { App } from '@modelcontextprotocol/ext-apps';
import { AppBridge } from '@modelcontextprotocol/ext-apps/app-bridge';
import { InMemoryTransport } from '@modelcontextprotocol/client';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useEffect, useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { OUTPUTS, CID_C, CONFIRMATIONS } from '../helpers/fixtures';
import { errorResult, okResult } from '../helpers/port';

interface Harness {
  bridge: AppBridge;
  appTransport: InMemoryTransport;
  hostTransport: InMemoryTransport;
}
let harness: Harness | undefined;
let hostCaps: ConstructorParameters<typeof AppBridge>[2] = {};
let toolHandler: (name: string, args: Record<string, unknown> | undefined) => unknown = () => ({ content: [] });
const received = { messages: [] as unknown[], contexts: [] as unknown[], toolCalls: [] as { name: string; args: unknown }[] };

vi.mock('@modelcontextprotocol/ext-apps/react', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@modelcontextprotocol/ext-apps/react')>();
  return {
    ...actual,
    useApp: ({ appInfo, capabilities, onAppCreated }: Parameters<typeof actual.useApp>[0]) => {
      const [state, setState] = useState<{ app: App | null; isConnected: boolean; error: Error | null }>({ app: null, isConnected: false, error: null });
      useEffect(() => {
        const app = new App(appInfo, capabilities, { autoResize: false });
        onAppCreated?.(app);
        const h = harness as Harness;
        void app.connect(h.appTransport).then(
          () => setState({ app, isConnected: true, error: null }),
          (error: Error) => setState({ app: null, isConnected: false, error }),
        );
        // eslint-disable-next-line react-hooks/exhaustive-deps
      }, []);
      return state;
    },
  };
});

function startHost(caps: typeof hostCaps): Promise<void> {
  hostCaps = caps;
  const [appTransport, hostTransport] = InMemoryTransport.createLinkedPair();
  const bridge = new AppBridge(null, { name: 'test-host', version: '1.0.0' }, hostCaps, {
    hostContext: { theme: 'light', toolInfo: { id: 1, tool: { name: 'pin_ls', inputSchema: { type: 'object' } } } },
  });
  bridge.oncalltool = async (params) => {
    received.toolCalls.push({ name: params.name, args: params.arguments });
    return toolHandler(params.name, params.arguments) as never;
  };
  bridge.onmessage = async (params) => {
    received.messages.push(params);
    return {};
  };
  bridge.onupdatemodelcontext = async (params) => {
    received.contexts.push(params);
    return {};
  };
  const ready = new Promise<void>((resolve) => (bridge.oninitialized = () => resolve()));
  harness = { bridge, appTransport, hostTransport };
  return bridge.connect(hostTransport).then(() => ready);
}

async function mount() {
  const { McpAppRoot } = await import('../../src/ui/McpAppRoot');
  const ready = startHost(hostCaps);
  const view = render(<McpAppRoot view="pins" />);
  await ready;
  return view;
}

afterEach(async () => {
  await harness?.bridge.close();
  harness = undefined;
  received.messages.length = received.contexts.length = received.toolCalls.length = 0;
  toolHandler = () => ({ content: [] });
});

const FULL_CAPS = { serverTools: {}, message: { text: {} }, updateModelContext: { text: {} } };

describe('MCP App view against a real AppBridge', () => {
  it('waits, then draws the tool result and applies the host theme', async () => {
    hostCaps = FULL_CAPS;
    const { container } = await mount();
    expect(screen.getByText('Waiting for the result from your agent.')).toBeInTheDocument();

    await act(async () => {
      await harness!.bridge.sendToolInput({ arguments: { type: 'all' } });
      await harness!.bridge.sendToolResult(okResult('pin_ls', OUTPUTS.pin_ls[0]) as never);
    });

    await screen.findByText('wiki');
    expect(container.querySelector('[data-ipfs-ui]')).toHaveAttribute('data-theme', 'light');
  });

  it('proxies Load more through callServerTool with the tool input plus the cursor', async () => {
    hostCaps = FULL_CAPS;
    toolHandler = () => okResult('pin_ls', { pins: [{ cid: CID_C, type: 'direct', name: 'page-two' }], page: { has_more: false } });
    await mount();
    await act(async () => {
      await harness!.bridge.sendToolInput({ arguments: { type: 'all', limit: 3 } });
      await harness!.bridge.sendToolResult(okResult('pin_ls', OUTPUTS.pin_ls[0]) as never);
    });
    await userEvent.click(await screen.findByRole('button', { name: /Load more pins/ }));
    await screen.findByText('page-two');
    expect(received.toolCalls).toEqual([{ name: 'pin_ls', args: { type: 'all', limit: 3, cursor: 'cur_pin_2' } }]);
  });

  it('carries Request unpin as a user message and never as a tool call', async () => {
    hostCaps = FULL_CAPS;
    await mount();
    await act(async () => {
      await harness!.bridge.sendToolResult(okResult('pin_ls', OUTPUTS.pin_ls[0]) as never);
    });
    await userEvent.click((await screen.findAllByRole('button', { name: /Request unpin/ }))[0] as HTMLElement);
    await waitFor(() => expect(received.messages).toHaveLength(1));
    expect(JSON.stringify(received.messages[0])).toContain('Please remove the pin');
    expect(received.toolCalls).toHaveLength(0);
  });

  it('reports a denial to the model as context, with no tool call', async () => {
    hostCaps = FULL_CAPS;
    await mount();
    await act(async () => {
      await harness!.bridge.sendToolResult(
        errorResult({ code: 'confirmation_required', message: 'Confirm first.', tool: 'pin_rm', confirmation: CONFIRMATIONS.humanWithToken }) as never,
      );
    });
    await userEvent.click(await screen.findByRole('button', { name: /deny/i }));
    await waitFor(() => expect(received.contexts).toHaveLength(1));
    expect(JSON.stringify(received.contexts[0])).toContain('cf_3pQm8vK2xR7tLw9a');
    expect(received.toolCalls).toHaveLength(0);
  });

  it('follows host context changes', async () => {
    hostCaps = FULL_CAPS;
    const { container } = await mount();
    await act(async () => {
      await harness!.bridge.sendToolResult(okResult('pin_ls', OUTPUTS.pin_ls[0]) as never);
    });
    await screen.findByText('wiki');
    await act(async () => {
      harness!.bridge.setHostContext({ theme: 'dark', toolInfo: { id: 1, tool: { name: 'pin_ls', inputSchema: { type: 'object' } } } });
    });
    await waitFor(() => expect(container.querySelector('[data-ipfs-ui]')).toHaveAttribute('data-theme', 'dark'));
  });

  it('degrades to a static view when the host declares no serverTools, message or model context', async () => {
    hostCaps = {};
    await mount();
    await act(async () => {
      await harness!.bridge.sendToolResult(okResult('pin_ls', OUTPUTS.pin_ls[0]) as never);
    });
    await screen.findByText('wiki');
    expect(screen.queryByRole('button', { name: /Load more/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Request unpin/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Refresh' })).not.toBeInTheDocument();
  });
});
