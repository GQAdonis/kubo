import { defineConfig } from '@playwright/test';

// Visual regression, real-browser axe (with layout rules: contrast, target size), keyboard and
// offline specs. They run against the built gallery served with the MCP Apps default CSP.
export default defineConfig({
  testDir: 'tests/visual',
  outputDir: 'test-results',
  fullyParallel: true,
  retries: 0,
  reporter: [['list']],
  // Baselines are per platform (font rasterisation differs). Commit the ones for the platforms you gate on.
  snapshotPathTemplate: '{testDir}/__screenshots__/{projectName}/{testFileName}/{arg}{ext}',
  expect: { toHaveScreenshot: { maxDiffPixelRatio: 0.005, animations: 'disabled', caret: 'hide' } },
  use: {
    baseURL: 'http://127.0.0.1:4173',
    colorScheme: 'dark',
    locale: 'en-US',
    timezoneId: 'UTC',
    deviceScaleFactor: 1,
    // Blocks the system's reduced-motion preference from leaking into baselines; specs opt in explicitly.
    reducedMotion: 'no-preference',
  },
  projects: [
    {
      name: 'chromium',
      use: { browserName: 'chromium' },
    },
  ],
  webServer: {
    command: 'pnpm exec vite build --config vite.gallery.config.ts && pnpm exec vite preview --config vite.gallery.config.ts',
    url: 'http://127.0.0.1:4173/',
    reuseExistingServer: true,
    timeout: 120_000,
  },
});
