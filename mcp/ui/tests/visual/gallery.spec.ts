import { expect, test } from '@playwright/test';
import { SCENE_IDS, THEMES, VIEWPORTS, openScene } from './helpers';

// Visual regression: every scene in both themes at the narrowest and a desktop width, plus the whole
// gallery at 768 and 1440. Baselines live in __screenshots__/ and are reviewed like code.

for (const theme of THEMES) {
  for (const width of [320, 1024] as const) {
    for (const scene of SCENE_IDS) {
      test(`${scene} ${theme} ${width}`, async ({ page }) => {
        await page.setViewportSize(VIEWPORTS[width]);
        await openScene(page, scene, theme);
        await expect(page).toHaveScreenshot(`${scene}-${theme}-${width}.png`, { fullPage: true });
      });
    }
  }
  for (const width of [768, 1440] as const) {
    test(`all scenes ${theme} ${width}`, async ({ page }) => {
      await page.setViewportSize(VIEWPORTS[width]);
      await openScene(page, 'all', theme);
      await expect(page).toHaveScreenshot(`all-${theme}-${width}.png`, { fullPage: true });
    });
  }
}

test('no scene overflows horizontally at 320px, in either theme', async ({ page }) => {
  await page.setViewportSize(VIEWPORTS[320]);
  for (const theme of THEMES) {
    await openScene(page, 'all', theme);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow, theme).toBeLessThanOrEqual(0);
  }
});
