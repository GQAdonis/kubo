import { render, screen, waitFor, type RenderResult } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { randomInt } from 'node:crypto';
import type { ReactNode } from 'react';
import { expect } from 'vitest';
import { IslandApp, type IslandProps } from '../../src/island/IslandApp';
import { TrustProbeContext } from '../../src/island/InterruptItem';
import type { SessionIds } from '../../src/island/session';
import { createFakeFetch, type FakeFetch } from './fake-fetch';
import { FIXTURE_NOW, type AguiFixture } from './fixtures';

export const TOKEN = 'ui-bearer-0123456789abcdef';

/** A loopback endpoint on a random port. Nothing listens there: tests use an injected fetch. */
export const randomEndpoint = () => `http://127.0.0.1:${randomInt(20000, 60000)}/agui`;

export function idsFor(fixture: AguiFixture, runs: string[]): SessionIds {
  const queue = [...runs];
  return { thread: () => fixture.input.threadId, run: () => queue.shift() ?? 'run-exhausted', message: () => String(fixture.input.messages[0]?.['id']) };
}

export interface MountedIsland {
  readonly view: RenderResult;
  readonly fake: FakeFetch;
  readonly user: ReturnType<typeof userEvent.setup>;
  readonly endpoint: string;
}

export interface MountOptions {
  readonly props?: Partial<IslandProps>;
  /** Treat every activation as a person's (jsdom marks all synthetic events untrusted). Default false. */
  readonly trustAll?: boolean;
  readonly fake?: FakeFetch;
  readonly wrap?: (node: ReactNode) => ReactNode;
}

export function mountIsland(options: MountOptions = {}): MountedIsland {
  const fake = options.fake ?? createFakeFetch({ token: TOKEN });
  const endpoint = options.props?.endpoint ?? randomEndpoint();
  const props: IslandProps = {
    endpoint,
    getToken: () => TOKEN,
    tokenSet: true,
    open: true,
    theme: 'dark',
    onOpenChange: () => undefined,
    fetchImpl: fake.fetch,
    now: () => FIXTURE_NOW,
    ...options.props,
  };
  const island = <IslandApp {...props} />;
  const tree = options.trustAll ? <TrustProbeContext value={() => true}>{island}</TrustProbeContext> : island;
  const view = render(options.wrap ? options.wrap(tree) : tree);
  return { view, fake, user: userEvent.setup(), endpoint };
}

/** Type a message and send it with the real Send control. */
export async function say(m: MountedIsland, text: string): Promise<void> {
  await m.user.type(screen.getByRole('textbox', { name: 'Message to the agent' }), text);
  await m.user.click(screen.getByRole('button', { name: 'Send message' }));
}

export async function untilIdle(): Promise<void> {
  await waitFor(() => expect(screen.queryByRole('button', { name: 'Stop run' })).not.toBeInTheDocument());
}
