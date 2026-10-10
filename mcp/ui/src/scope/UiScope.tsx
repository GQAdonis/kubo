import { createContext, use, useMemo, useState, type CSSProperties, type ReactNode } from 'react';
import { STYLE_TEXT } from '../styles/style-text';

export type UiTheme = 'dark' | 'light' | 'auto';

export interface UiHostActions {
  /** Copy text to the clipboard. When absent the library tries navigator.clipboard and reports failure in place. */
  readonly copy?: (text: string) => Promise<void> | void;
}

interface ScopeValue {
  readonly actions: UiHostActions;
  readonly announce: (message: string) => void;
}

const ScopeContext = createContext<ScopeValue | null>(null);
const EMPTY_ACTIONS: UiHostActions = {};
const NO_ANNOUNCE = (): void => undefined;

/** Host callbacks supplied by the nearest UiScope. */
export function useHostActions(): UiHostActions {
  return use(ScopeContext)?.actions ?? EMPTY_ACTIONS;
}

/**
 * Say something to assistive technology through the scope's single polite live region. One region
 * per scope, not one per button: a table of fifty CIDs must not create fifty live regions.
 */
export function useAnnounce(): (message: string) => void {
  return use(ScopeContext)?.announce ?? NO_ANNOUNCE;
}

export interface UiScopeProps {
  /** dark (default, the brand's primary theme), light, or auto (prefers-color-scheme). */
  theme?: UiTheme;
  /**
   * Inject the library stylesheet as a <style> element inside this scope. Leave on for iframes and
   * shadow roots; turn off when the host loads ipfs-ui.css itself or adopts the sheet (see adoptStyles).
   */
  injectStyles?: boolean;
  /** Paint the `ground` background. The MCP App root does; embedded uses (chat island) do not. */
  paint?: boolean;
  actions?: UiHostActions;
  className?: string;
  style?: CSSProperties;
  children: ReactNode;
}

/**
 * Root of every component. Tokens, base rules and component rules are all keyed on the
 * data-ipfs-ui attribute, so the library works in an iframe, a shadow root or a plain document without
 * touching the host's styles. Nested scopes without their own theme or actions reuse the parent's.
 */
export function UiScope({
  theme,
  injectStyles = true,
  paint = false,
  actions,
  className,
  style,
  children,
}: UiScopeProps) {
  const parent = use(ScopeContext);
  const [message, setMessage] = useState('');
  const mergedActions = useMemo<UiHostActions>(() => ({ ...parent?.actions, ...actions }), [parent, actions]);
  const value = useMemo<ScopeValue>(() => ({ actions: mergedActions, announce: setMessage }), [mergedActions]);

  if (parent && theme === undefined && actions === undefined && style === undefined) {
    return <>{children}</>;
  }
  const classes = ['iu-root', paint ? 'iu-ground' : '', className ?? ''].filter(Boolean).join(' ');
  return (
    <ScopeContext value={value}>
      <div data-ipfs-ui="" data-theme={theme ?? 'dark'} className={classes} style={style}>
        {injectStyles && !parent ? (
          // Trusted constant from this package's own CSS files; plain <style> (no href/precedence)
          // so React 19 keeps it inside this root instead of hoisting it to document.head.
          <style data-ipfs-ui-styles="" dangerouslySetInnerHTML={{ __html: STYLE_TEXT }} />
        ) : null}
        {children}
        <div role="status" aria-live="polite" aria-atomic="true" className="iu-sr-only" data-ipfs-ui-live="">
          {message}
        </div>
      </div>
    </ScopeContext>
  );
}
