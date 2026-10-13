import { defineConfig } from '@playwright/test';

// Visual regression, real-browser axe (with layout rules: contrast, target size) and CSP specs for the
// built single-file views. Each spec loads dist/<view>.html into a sandboxed iframe whose CSP is the
// MCP Apps default (tests/visual/host.ts), then plays the host side of the protocol.
// Run `pnpm build` first. Baselines are per platform; commit the ones for the platforms you gate on.
export default defineConfig({
  testDir: 'tests/visual',
  outputDir: 'test-results',
  fullyParallel: true,
  retries: 0,
  reporter: [['list']],
  snapshotPathTemplate: '{testDir}/__screenshots__/{projectName}/{testFileName}/{arg}{ext}',
  expect: { toHaveScreenshot: { maxDiffPixelRatio: 0.005, animations: 'disabled', caret: 'hide' } },
  use: {
    colorScheme: 'dark',
    locale: 'en-US',
    timezoneId: 'UTC',
    deviceScaleFactor: 1,
    reducedMotion: 'no-preference',
  },
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
});
