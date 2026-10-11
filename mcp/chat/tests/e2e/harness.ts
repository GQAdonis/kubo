import { existsSync } from 'node:fs';
import { createRequire } from 'node:module';
import { resolve } from 'node:path';
import { _electron as electron, expect, test as base, type ElectronApplication, type Page } from '@playwright/test';
import type { E2eEnv } from './global-setup';

// One way to open the stand-in Web UI page in either runtime, so every spec runs in Chromium and in
// Electron 44 unchanged:
//   chromium  Playwright's Chromium, page loaded over http from the harness server;
//   electron  Electron 44 (the version IPFS Desktop pins), page loaded from webui://localhost, the way
//             Desktop loads the Web UI, with webSecurity, contextIsolation and the sandbox on.

export type Runtime = 'chromium' | 'electron';
export type Scheme = 'dark' | 'light';

export interface OpenOptions {
  /** none: the baseline page. closed: module loaded, element mounted, sidebar closed. open: sidebar open. */
  readonly island: 'none' | 'closed' | 'open';
  readonly scheme?: Scheme;
  /** The element's theme attribute. Default: auto, so it follows the scheme. */
  readonly theme?: 'dark' | 'light' | 'auto';
  readonly width?: number;
  readonly height?: number;
  readonly csp?: 'strict';
}

export interface Harness {
  readonly env: E2eEnv;
  readonly runtime: Runtime;
  open(options: OpenOptions): Promise<Page>;
  /** Requests the fixture server (standing in for ipfs mcp /agui) has received since the last reset. */
  fixtureRequests(): Promise<FixtureRequest[]>;
  resetFixtureRequests(): Promise<void>;
  /** Paths the harness page server has served since open() last reset its log (works in Electron, where the page cannot list requests). */
  harnessRequests(): Promise<string[]>;
  /** The runtime's own version string, to assert which engine ran. */
  engineVersion(): Promise<string>;
}

export interface FixtureRequest {
  method: string;
  url: string;
  headers: Record<string, string | undefined>;
  body: { resume?: { interruptId: string; status: string; payload?: { approved: boolean } }[]; messages?: unknown[] } | undefined;
  matched: string | undefined;
}

const SIZE = { width: 1024, height: 768 };

function readEnv(): E2eEnv {
  const raw = process.env['CHAT_E2E'];
  if (!raw) throw new Error('CHAT_E2E is not set: tests/e2e/global-setup.ts did not run.');
  return JSON.parse(raw) as E2eEnv;
}

/**
 * Electron 44's executable. ELECTRON_EXEC_PATH wins. Otherwise the `electron` package in node_modules, whose
 * binary is downloaded by its install script (pnpm runs it only when approved: `node node_modules/electron/install.js`).
 * There is no silent skip: a gate that cannot run Electron 44 has not passed G11-1.
 */
function electronExecutable(): string {
  const fromEnv = process.env['ELECTRON_EXEC_PATH'];
  if (fromEnv) {
    if (!existsSync(fromEnv)) throw new Error(`ELECTRON_EXEC_PATH does not exist: ${fromEnv}`);
    return fromEnv;
  }
  try {
    return createRequire(import.meta.url)('electron') as string;
  } catch (cause) {
    throw new Error(
      'Electron 44 is not available. Set ELECTRON_EXEC_PATH to an Electron 44 executable (IPFS Desktop pins 44.3.0), ' +
        'or run `node node_modules/electron/install.js` in mcp/chat to download the binary.',
      { cause },
    );
  }
}

export const test = base.extend<{ runtime: Runtime; harness: Harness }>({
  runtime: ['chromium', { option: true }],

  harness: async ({ page, runtime }, use) => {
    const env = readEnv();
    let app: ElectronApplication | undefined;

    const pagePath = (o: OpenOptions) => {
      const q = new URLSearchParams({ island: o.island, theme: o.theme ?? 'auto' });
      if (o.csp) q.set('csp', o.csp);
      return `/host.html?${q.toString()}`;
    };

    const harness: Harness = {
      env,
      runtime,
      async open(o) {
        const width = o.width ?? SIZE.width;
        const height = o.height ?? SIZE.height;
        const scheme = o.scheme ?? 'dark';
        await fetch(`${env.harnessOrigin}/__test/requests`, { method: 'DELETE' });
        let target: Page;
        if (runtime === 'chromium') {
          await page.setViewportSize({ width, height });
          await page.emulateMedia({ colorScheme: scheme, reducedMotion: 'no-preference' });
          await page.goto(env.harnessOrigin + pagePath(o));
          target = page;
        } else {
          await app?.close(); // one window at a time; a spec that opens twice gets a fresh Electron each time
          app = await electron.launch({
            executablePath: electronExecutable(),
            args: [resolve(import.meta.dirname, 'electron-main.cjs')],
            env: {
              ...(process.env as Record<string, string>),
              HARNESS_ORIGIN: env.harnessOrigin,
              HARNESS_PATH: pagePath(o),
              WINDOW_WIDTH: String(width),
              WINDOW_HEIGHT: String(height),
            },
          });
          target = await app.firstWindow();
          await target.waitForLoadState('domcontentloaded');
          // Electron's prefers-color-scheme ignores nativeTheme here; emulate it the way the Chromium branch does.
          await target.emulateMedia({ colorScheme: scheme, reducedMotion: 'no-preference' });
        }
        await target.waitForFunction(() => (window as unknown as { __harnessReady?: boolean }).__harnessReady === true);
        if (o.island === 'open') await expect(target.locator('ipfs-agent-chat aside[data-state="open"]')).toBeVisible();
        return target;
      },
      async fixtureRequests() {
        const res = await fetch(`${env.fixtureOrigin}/__test/requests`);
        return (await res.json()) as FixtureRequest[];
      },
      async resetFixtureRequests() {
        await fetch(`${env.fixtureOrigin}/__test/requests`, { method: 'DELETE' });
      },
      async harnessRequests() {
        const res = await fetch(`${env.harnessOrigin}/__test/requests`);
        return (await res.json()) as string[];
      },
      async engineVersion() {
        if (runtime === 'electron' && app) return app.evaluate(() => `electron ${process.versions.electron} chrome ${process.versions.chrome}`);
        return page.evaluate(() => navigator.userAgent);
      },
    };

    await harness.resetFixtureRequests();
    await use(harness);
    await app?.close();
  },
});

export { expect };
