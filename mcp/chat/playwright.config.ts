import { defineConfig } from '@playwright/test';
import type { Runtime } from './tests/e2e/harness';

// Visual isolation, real-browser accessibility, keyboard, gesture and CSP specs for <ipfs-agent-chat>,
// run in Chromium and in Electron 44 (falsifier G11-1). They run at the phase-boundary gate, not during
// implementation. `pnpm test:visual` builds the island (production and harness), then runs this.
//
// The two local servers (the AG-UI fixture server and the harness page server) start in globalSetup on
// ports the OS picks. Specs run one at a time because they share the fixture server's request log.
export default defineConfig<{ runtime: Runtime }>({
  testDir: 'tests/e2e',
  testMatch: '**/*.spec.ts',
  outputDir: 'test-results',
  globalSetup: './tests/e2e/global-setup.ts',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 60_000,
  reporter: [['list']],
  // Baselines are per project and platform (font rasterisation differs). Commit the ones the gate uses.
  snapshotPathTemplate: '{testDir}/__screenshots__/{projectName}/{testFileName}/{arg}{ext}',
  expect: { toHaveScreenshot: { maxDiffPixelRatio: 0.005, animations: 'disabled', caret: 'hide' } },
  use: { locale: 'en-US', timezoneId: 'UTC', deviceScaleFactor: 1 },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium', runtime: 'chromium' } },
    { name: 'electron44', use: { runtime: 'electron' } },
  ],
});
