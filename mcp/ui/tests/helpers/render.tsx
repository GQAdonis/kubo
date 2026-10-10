import { render, type RenderResult } from '@testing-library/react';
import type { ReactElement } from 'react';
import { UiScope, type UiTheme } from '../../src/scope/UiScope';

/** Render inside a UiScope without injecting CSS (jsdom has no layout). */
export function renderIn(ui: ReactElement, theme: UiTheme = 'dark'): RenderResult {
  return render(
    <UiScope theme={theme} injectStyles={false}>
      {ui}
    </UiScope>,
  );
}

/** Replace every network entry point with a function that fails the test if called. */
export function blockNetwork(): { calls: string[]; restore: () => void } {
  const calls: string[] = [];
  const g = globalThis as Record<string, unknown>;
  const saved: Record<string, unknown> = {};
  const trap = (name: string) => () => {
    calls.push(name);
    throw new Error(`network access attempted: ${name}`);
  };
  for (const name of ['fetch', 'XMLHttpRequest', 'WebSocket', 'EventSource']) {
    saved[name] = g[name];
    g[name] = trap(name);
  }
  const nav = globalThis.navigator as unknown as Record<string, unknown>;
  saved['sendBeacon'] = nav['sendBeacon'];
  nav['sendBeacon'] = trap('sendBeacon');
  return {
    calls,
    restore: () => {
      for (const [k, v] of Object.entries(saved)) {
        if (k === 'sendBeacon') nav[k] = v;
        else g[k] = v;
      }
    },
  };
}
