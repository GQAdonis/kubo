import './zod-config'; // first: must run before any other module uses zod
import { createRoot } from 'react-dom/client';
import { STYLE_TEXT, registerBundledFonts } from '@gqadonis/ipfs-mcp-ui';
import islandCss from '../generated/island.css?inline';
import { IslandApp, type IslandProps } from './IslandApp';

// The lazy half of the island: everything that needs React. It is reached only through a dynamic
// import() from the element when the sidebar is first opened, so a host page that loads the island
// but never opens it downloads and runs none of this.

export interface IslandHandle {
  update(props: IslandProps): void;
  unmount(): void;
}

/**
 * Put a stylesheet inside the shadow root. A constructable stylesheet is not governed by the page's
 * style-src (no inline <style> needs permitting). Engines without them get a <style> element, still
 * inside the root, so nothing reaches the host document either way.
 */
function adopt(shadow: ShadowRoot, css: string): void {
  if (typeof CSSStyleSheet !== 'undefined' && 'adoptedStyleSheets' in shadow) {
    try {
      const sheet = new CSSStyleSheet();
      sheet.replaceSync(css);
      shadow.adoptedStyleSheets = [...shadow.adoptedStyleSheets, sheet];
      return;
    } catch {
      // fall through to the element form
    }
  }
  const style = document.createElement('style');
  style.textContent = css;
  shadow.append(style);
}

export function mountIsland(shadow: ShadowRoot, props: IslandProps): IslandHandle {
  // Library tokens, base and component rules, then the island's Tailwind output.
  adopt(shadow, STYLE_TEXT);
  adopt(shadow, islandCss);
  // Brand faces from embedded bytes (FontFace from an ArrayBuffer: no fetch, no font-src).
  void registerBundledFonts();

  const container = document.createElement('div');
  container.dataset['acMount'] = '';
  shadow.append(container);
  const root = createRoot(container);
  root.render(<IslandApp {...props} />);

  return {
    update: (next) => root.render(<IslandApp {...next} />),
    unmount: () => {
      root.unmount();
      container.remove();
    },
  };
}
