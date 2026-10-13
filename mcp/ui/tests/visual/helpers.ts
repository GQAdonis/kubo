import type { Page } from '@playwright/test';

// Kept as a literal list because Playwright's loader cannot import the gallery's Vite-only modules
// (?inline CSS, JSON). tests/gallery-scenes.test.ts asserts this list equals the gallery's scenes.
export const SCENE_IDS = [
  'pins',
  'pins-writes',
  'content-text',
  'content-listing',
  'mfs',
  'dag',
  'ipns',
  'node-status',
  'node-routing',
  'node-repo',
  'confirmation-pending',
  'confirmation-states',
  'chat-read',
  'chat-write-pending',
  'chat-write-approved',
  'chat-native',
  'chat-egress',
] as const;

export const THEMES = ['dark', 'light'] as const;
export type Theme = (typeof THEMES)[number];

// The breakpoints the project's web rules name: 320, 768, 1024, 1440.
export const VIEWPORTS = {
  320: { width: 320, height: 800 },
  768: { width: 768, height: 900 },
  1024: { width: 1024, height: 900 },
  1440: { width: 1440, height: 900 },
} as const;

export async function openScene(page: Page, scene: string, theme: Theme, path = '/'): Promise<void> {
  await page.goto(`${path}?scene=${encodeURIComponent(scene)}&theme=${theme}`);
  await page.waitForSelector('html[data-ready="true"]');
}
