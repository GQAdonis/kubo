import { expect, test } from '@playwright/test';
import { SCENE_IDS, THEMES, VIEWPORTS, openScene } from './helpers';

test.describe('keyboard', () => {
  for (const theme of THEMES) {
    test(`focus ring is a solid 2px accent outline offset 2px on every control, ${theme}`, async ({ page }) => {
      await openScene(page, 'confirmation-pending', theme);
      const accent = await page.evaluate(() => getComputedStyle(document.querySelector('[data-ipfs-ui]') as Element).getPropertyValue('--accent').trim());
      const seen: { name: string; style: string; width: string; offset: string; color: string }[] = [];
      // The dialog autofocuses Deny, so a plain Tab run starts on Approve and then leaves the page.
      // Walk forward, back, forward so both controls are visited with keyboard modality.
      for (const key of ['Tab', 'Shift+Tab', 'Tab']) {
        await page.keyboard.press(key);
        const info = await page.evaluate(() => {
          const el = document.activeElement as HTMLElement;
          const cs = getComputedStyle(el);
          return { name: (el.getAttribute('aria-label') ?? el.textContent ?? '').trim().slice(0, 30), style: cs.outlineStyle, width: cs.outlineWidth, offset: cs.outlineOffset, color: cs.outlineColor, tag: el.tagName };
        });
        if (info.tag === 'BODY') break;
        seen.push(info);
      }
      expect(seen.length).toBeGreaterThanOrEqual(2);
      const hex = accent.replace('#', '');
      const rgb = `rgb(${parseInt(hex.slice(0, 2), 16)}, ${parseInt(hex.slice(2, 4), 16)}, ${parseInt(hex.slice(4, 6), 16)})`;
      for (const s of seen) {
        expect(s.style, s.name).toBe('solid');
        expect(s.width, s.name).toBe('2px');
        expect(s.offset, s.name).toBe('2px');
        expect(s.color, s.name).toBe(rgb);
      }
    });
  }

  test('confirmation dialog: Deny has focus first, Approve is next, Enter on Deny does nothing else', async ({ page }) => {
    await openScene(page, 'confirmation-pending', 'dark');
    const focused = () => page.evaluate(() => (document.activeElement as HTMLElement).textContent?.trim());
    // autoFocus puts the safe action first
    expect(await focused()).toBe('Deny');
    await page.keyboard.press('Tab');
    expect(await focused()).toBe('Approve pin removal');
    await page.keyboard.press('Shift+Tab');
    expect(await focused()).toBe('Deny');
  });

  test('IPLD disclosures open with Enter and with Space', async ({ page }) => {
    await openScene(page, 'dag', 'dark');
    const summary = page.locator('.iu-json__group > summary', { hasText: 'nested' }).first();
    const details = summary.locator('xpath=..');
    await summary.focus();
    await expect(details).not.toHaveAttribute('open', '');
    await page.keyboard.press('Enter');
    await expect(details).toHaveAttribute('open', '');
    await page.keyboard.press('Space');
    await expect(details).not.toHaveAttribute('open', '');
    await page.keyboard.press('Space');
    await expect(details).toHaveAttribute('open', '');
  });

  test('wide tables scroll sideways inside their own frame and are reachable by keyboard at 320px', async ({ page }) => {
    await page.setViewportSize(VIEWPORTS[320]);
    await openScene(page, 'pins', 'dark');
    const region = page.getByRole('region', { name: 'Pins' }).first();
    await region.focus();
    await expect(region).toBeFocused();
    const { scrolls, pageOverflow } = await region.evaluate((el) => ({
      scrolls: el.scrollWidth > el.clientWidth,
      pageOverflow: document.documentElement.scrollWidth > document.documentElement.clientWidth,
    }));
    expect(scrolls).toBe(true);
    expect(pageOverflow).toBe(false);
    await page.keyboard.press('ArrowRight');
    // Chromium animates keyboard scrolling, so the offset is not final on the next tick.
    await expect.poll(() => region.evaluate((el) => el.scrollLeft)).toBeGreaterThan(0);
  });
});

test.describe('reduced motion', () => {
  test('removes every transition and animation when the user asks for none', async ({ browser }) => {
    const ctx = await browser.newContext({ reducedMotion: 'reduce', baseURL: 'http://127.0.0.1:4173' });
    const page = await ctx.newPage();
    await openScene(page, 'all', 'dark');
    const moving = await page.evaluate(() => {
      const out: string[] = [];
      for (const el of document.querySelectorAll('[data-ipfs-ui], [data-ipfs-ui] *')) {
        const cs = getComputedStyle(el);
        const anim = cs.animationName !== 'none' && cs.animationName !== '';
        const trans = cs.transitionDuration.split(',').some((d) => parseFloat(d) > 0);
        if (anim || trans) out.push(`${el.tagName.toLowerCase()}.${(el as HTMLElement).className}`);
      }
      return out;
    });
    expect(moving).toEqual([]);
    await ctx.close();
  });

  test('keeps short state transitions when motion is allowed', async ({ page }) => {
    await openScene(page, 'confirmation-pending', 'dark');
    const d = await page.locator('.iu-btn--primary').evaluate((el) => getComputedStyle(el).transitionDuration);
    expect(parseFloat(d) * 1000).toBeGreaterThanOrEqual(150);
    expect(parseFloat(d) * 1000).toBeLessThanOrEqual(250);
  });
});

test.describe('offline and CSP', () => {
  test('every scene loads with only same-origin documents and scripts, under the MCP Apps default CSP, with brand fonts from bytes', async ({ page }) => {
    const requests: { url: string; type: string }[] = [];
    const csp: string[] = [];
    page.on('request', (r) => requests.push({ url: r.url(), type: r.resourceType() }));
    page.on('console', (m) => {
      if (/Content Security Policy|Refused to/i.test(m.text())) csp.push(m.text());
    });
    page.on('pageerror', (e) => csp.push(`pageerror: ${e.message}`));
    for (const scene of SCENE_IDS) await openScene(page, scene, 'dark');
    const origin = new URL(page.url()).origin;
    const foreign = requests.filter((r) => !r.url.startsWith(origin) && !r.url.startsWith('data:'));
    expect(foreign).toEqual([]);
    expect(requests.filter((r) => r.type === 'font')).toEqual([]); // no font is requested by URL
    expect(requests.filter((r) => ['fetch', 'xhr', 'websocket', 'eventsource', 'ping'].includes(r.type))).toEqual([]);
    expect(csp).toEqual([]);
  });

  test('the response carries the deny-by-default CSP, and the brand faces are registered anyway', async ({ page }) => {
    const res = await page.goto('/?scene=pins&theme=dark');
    expect(res?.headers()['content-security-policy']).toContain("default-src 'none'");
    expect(res?.headers()['content-security-policy']).toContain("connect-src 'none'");
    expect(res?.headers()['content-security-policy']).not.toContain('font-src');
    await page.waitForSelector('html[data-ready="true"]');
    const faces = await page.evaluate(async () => {
      await document.fonts.ready;
      return ['700 17px "IPFS UI Syne"', '400 15px "IPFS UI Hanken Grotesk"', '600 14px "IPFS UI Hanken Grotesk"', '400 13px "IPFS UI JetBrains Mono"'].map((f) => [f, document.fonts.check(f)]);
    });
    for (const [face, ok] of faces) expect(ok, String(face)).toBe(true);
    const loaded = await page.evaluate(() => [...document.fonts].filter((f) => f.status === 'loaded').map((f) => `${f.family} ${f.weight}`));
    expect(loaded.length).toBeGreaterThanOrEqual(9);
  });

  test('still renders with the network cut after load', async ({ page, context }) => {
    await openScene(page, 'pins', 'dark');
    await context.setOffline(true);
    await expect(page.getByRole('table', { name: 'Pins' }).first()).toBeVisible();
    await page.getByRole('button', { name: 'Load more pins' }).first().click(); // a host callback, not a request
    await context.setOffline(false);
  });
});

test.describe('theme', () => {
  test('auto follows the system, dark and light are explicit', async ({ browser }) => {
    for (const scheme of ['dark', 'light'] as const) {
      const ctx = await browser.newContext({ colorScheme: scheme, baseURL: 'http://127.0.0.1:4173' });
      const page = await ctx.newPage();
      await openScene(page, 'pins', 'dark');
      await page.goto(`/?scene=pins&theme=auto`);
      await page.waitForSelector('html[data-ready="true"]');
      const ground = await page.locator('.iu-ground').evaluate((el) => getComputedStyle(el).backgroundColor);
      expect(ground).toBe(scheme === 'dark' ? 'rgb(12, 11, 10)' : 'rgb(245, 242, 238)');
      await ctx.close();
    }
  });
});

test.describe('shadow root isolation', () => {
  test('a hostile host page cannot restyle the island, and the island does not restyle the host', async ({ page }) => {
    await page.setViewportSize(VIEWPORTS[1024]);
    await openScene(page, 'chat-write-pending', 'dark', '/shadow.html');
    // Host styles still apply to the host.
    const outside = await page.locator('#outside').evaluate((el) => {
      const cs = getComputedStyle(el);
      return { transform: cs.textTransform, bg: cs.backgroundColor };
    });
    expect(outside.transform).toBe('uppercase');
    expect(outside.bg).toBe('rgb(0, 255, 0)');

    const inside = await page.evaluate(() => {
      const root = document.getElementById('island-host')?.shadowRoot as ShadowRoot;
      const pick = (sel: string) => {
        const el = root.querySelector(sel) as HTMLElement;
        const cs = getComputedStyle(el);
        return { transform: cs.textTransform, spacing: cs.letterSpacing, family: cs.fontFamily, bg: cs.backgroundColor, borderTop: cs.borderTopStyle, align: cs.textAlign, italic: cs.fontStyle };
      };
      return {
        title: pick('.iu-approval__title'),
        message: pick('.iu-msg__text'),
        button: pick('.iu-btn--secondary'),
        hasStyles: root.querySelectorAll('style[data-ipfs-ui-styles]').length,
      };
    });
    expect(inside.hasStyles).toBe(1);
    for (const k of ['title', 'message', 'button'] as const) {
      expect(inside[k].transform, k).toBe('none');
      expect(inside[k].spacing, k).not.toBe('4px');
      expect(inside[k].borderTop, k).toBe('none');
      expect(inside[k].italic, k).toBe('normal');
      expect(inside[k].family, k).not.toMatch(/Comic|Times/);
    }
    expect(inside.title.family).toMatch(/IPFS UI Syne/);
    expect(inside.message.family).toMatch(/IPFS UI Hanken Grotesk/);
    expect(inside.button.bg).toBe('rgb(45, 44, 42)'); // --control, not the host's lime
    await expect(page).toHaveScreenshot('shadow-island-hostile-host.png', { fullPage: true });
  });
});
