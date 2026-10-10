import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { expect, type Page } from '@playwright/test';
import { fixture } from '../helpers/fixtures';

// Shared helpers for the browser specs. Everything here runs through the page the way a person would:
// the island's shadow root is open in the harness build, and Playwright locators pierce open roots.

export const userTextOf = (name: string): string => String(fixture(name).input.messages[0]?.['content']);

/** Type a message and send it with the real Send control (trusted input). */
export async function ask(page: Page, text: string): Promise<void> {
  const box = page.getByRole('textbox', { name: 'Message to the agent' });
  await box.click();
  await box.fill(text);
  await page.getByRole('button', { name: 'Send message' }).click();
}

/** Settle on the panel's resting states, in words. */
export const waits = {
  answer: (page: Page, text: string | RegExp) => expect(page.getByText(text).first()).toBeVisible(),
  idle: (page: Page) => expect(page.getByRole('button', { name: 'Stop run' })).toHaveCount(0),
};

const HOST_SELECTORS = ['body', 'header', 'nav', 'h1', 'h2', 'p', 'a', 'table', 'th', 'td', 'label', 'input', 'button'] as const;
const PROPS = ['fontFamily', 'fontSize', 'fontWeight', 'lineHeight', 'letterSpacing', 'wordSpacing', 'textTransform', 'color', 'backgroundColor', 'margin', 'padding', 'borderTopWidth', 'borderRadius', 'textShadow'] as const;

export interface HostSnapshot {
  head: string;
  sheets: number;
  adopted: number;
  fontFaces: number;
  body: string;
  computed: Record<string, Record<string, string>>;
  rects: Record<string, { x: number; y: number; width: number; height: number }[]>;
  scrollWidth: number;
  clientWidth: number;
}

/** What a host page is, structurally and visually, excluding the island element itself. */
export function snapshotHost(page: Page): Promise<HostSnapshot> {
  return page.evaluate(
    ({ selectors, props }) => {
      const body = document.body.cloneNode(true) as HTMLElement;
      body.querySelectorAll('ipfs-agent-chat, script').forEach((n) => n.remove());
      const computed: HostSnapshot['computed'] = {};
      const rects: HostSnapshot['rects'] = {};
      for (const sel of selectors) {
        const el = document.querySelector(sel);
        if (!el) continue;
        const cs = getComputedStyle(el);
        computed[sel] = Object.fromEntries(props.map((p) => [p, cs[p as keyof CSSStyleDeclaration] as string]));
        rects[sel] = [...document.querySelectorAll(sel)].map((e) => {
          const r = e.getBoundingClientRect();
          return { x: r.x, y: r.y, width: r.width, height: r.height };
        });
      }
      return {
        head: document.head.innerHTML,
        sheets: document.styleSheets.length,
        adopted: document.adoptedStyleSheets.length,
        fontFaces: document.fonts.size,
        body: body.innerHTML,
        computed,
        rects,
        scrollWidth: document.documentElement.scrollWidth,
        clientWidth: document.documentElement.clientWidth,
      };
    },
    { selectors: [...HOST_SELECTORS], props: [...PROPS] },
  );
}

/** Brand tokens as computed rgb(), for assertions about styled rendering. */
export const RGB = {
  dark: { surface: 'rgb(23, 22, 20)', raised: 'rgb(34, 33, 32)', control: 'rgb(45, 44, 42)', text: 'rgb(245, 242, 238)', accent: 'rgb(233, 106, 18)', onAccent: 'rgb(12, 11, 10)' },
  light: { surface: 'rgb(251, 249, 246)', raised: 'rgb(236, 231, 224)', control: 'rgb(225, 219, 210)', text: 'rgb(20, 18, 16)', accent: 'rgb(206, 48, 18)', onAccent: 'rgb(255, 255, 255)' },
} as const;

export const AXE_SOURCE = (): string => readFileSync(resolve(import.meta.dirname, '../../node_modules/axe-core/axe.min.js'), 'utf8');
