import { expect, test } from './harness';
import { RGB } from './helpers';

// Keyboard behavior of a non-modal, right-side complementary panel (WCAG 2.1.1, 2.4.3, 2.4.7, 2.4.11).

test('opening from the launcher moves focus into the panel, and Escape returns it', async ({ harness }) => {
  const page = await harness.open({ island: 'closed' });
  await page.locator('#chat-launcher').focus();
  await page.keyboard.press('Enter');
  await expect(page.locator('ipfs-agent-chat aside[data-state="open"]')).toBeVisible();
  await expect(page.getByRole('textbox', { name: 'Message to the agent' })).toBeFocused();

  await page.keyboard.press('Escape');
  await expect(page.locator('ipfs-agent-chat aside[data-state="closed"]')).toBeAttached();
  await expect(page.locator('#chat-launcher')).toBeFocused();
  await expect(page.locator('#chat-launcher')).toHaveAttribute('aria-expanded', 'false');
});

test('the Close button closes and returns focus too', async ({ harness }) => {
  const page = await harness.open({ island: 'closed' });
  await page.getByRole('button', { name: 'Ask the agent' }).click();
  await page.getByRole('button', { name: 'Close' }).click();
  await expect(page.locator('ipfs-agent-chat aside[data-state="closed"]')).toBeAttached();
  await expect(page.locator('#chat-launcher')).toBeFocused();
});

test('every focusable control shows a solid 2px accent ring', async ({ harness }) => {
  const page = await harness.open({ island: 'open', scheme: 'dark' });
  await page.getByRole('textbox', { name: 'Message to the agent' }).fill('x');
  const seen: string[] = [];
  await page.getByRole('button', { name: 'Close' }).focus();
  for (let i = 0; i < 3; i++) {
    const ring = await page.evaluate(() => {
      const a = document.querySelector('ipfs-agent-chat')?.shadowRoot?.activeElement;
      if (!a) return undefined;
      const cs = getComputedStyle(a);
      return { name: (a.getAttribute('aria-label') ?? a.textContent ?? a.tagName).trim().slice(0, 24), style: cs.outlineStyle, width: cs.outlineWidth, color: cs.outlineColor };
    });
    expect(ring, 'focus is inside the panel').toBeDefined();
    expect(ring).toMatchObject({ style: 'solid', width: '2px', color: RGB.dark.accent });
    seen.push(ring?.name ?? '');
    await page.keyboard.press('Tab');
  }
  expect(new Set(seen).size).toBeGreaterThan(2);
});

test('Tab order inside the panel follows reading order: actions, then the message box, then Send', async ({ harness }) => {
  const page = await harness.open({ island: 'open' });
  // New chat is disabled while the conversation is empty, so the order starts at Close.
  await page.getByRole('button', { name: 'Close' }).focus();
  const active = () => page.evaluate(() => document.querySelector('ipfs-agent-chat')?.shadowRoot?.activeElement?.tagName ?? '');
  expect(await active()).toBe('BUTTON'); // Close
  await page.keyboard.press('Tab');
  expect(await active()).toBe('TEXTAREA'); // the empty state has no log to scroll
  await page.getByRole('textbox', { name: 'Message to the agent' }).fill('hello');
  await page.keyboard.press('Tab');
  expect(await active()).toBe('BUTTON'); // Send message, enabled now there is text
});
