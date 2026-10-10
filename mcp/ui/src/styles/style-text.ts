import tokens from './tokens.css?inline';
import base from './base.css?inline';
import components from './components.css?inline';
import views from './views.css?inline';
import chat from './chat.css?inline';

/** The whole stylesheet as text, in STYLE_FILES order. */
export const STYLE_TEXT: string = [tokens, base, components, views, chat].join('\n');

/**
 * Adopt the stylesheet into a Document or ShadowRoot through a constructable stylesheet. Use this
 * instead of an inline <style> when the host CSP forbids inline styles.
 */
export function adoptStyles(root: Document | ShadowRoot): CSSStyleSheet | null {
  if (typeof CSSStyleSheet === 'undefined' || !('adoptedStyleSheets' in root)) return null;
  const sheet = new CSSStyleSheet();
  sheet.replaceSync(STYLE_TEXT);
  root.adoptedStyleSheets = [...root.adoptedStyleSheets, sheet];
  return sheet;
}
