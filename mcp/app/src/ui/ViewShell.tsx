import { UiScope, type UiTheme } from '@gqadonis/ipfs-mcp-ui';
import { mapHostStyle, safeAreaPadding, type HostStyleInput } from '@gqadonis/ipfs-mcp-ui/host';
import { useMemo, type CSSProperties } from 'react';
import type { ToolResultLike } from '../core/result';
import { copyText } from '../host/clipboard';
import type { HostPort } from '../host/port';
import { VIEWS, type ViewId } from '../views';
import { ViewSession } from './ViewSession';

/** The part of hostContext the shell reads. */
export interface HostView {
  readonly theme?: 'light' | 'dark' | undefined;
  readonly variables?: HostStyleInput['variables'];
  readonly safeAreaInsets?: { readonly top: number; readonly right: number; readonly bottom: number; readonly left: number } | undefined;
  readonly toolInfoName?: string | undefined;
}

export interface ViewShellProps {
  readonly view: ViewId;
  readonly port: HostPort;
  readonly host: HostView;
  /** Complete arguments of the tool call (ui/notifications/tool-input). */
  readonly input: Readonly<Record<string, unknown>> | undefined;
  /** The tool result (ui/notifications/tool-result), absent until the host sends it. */
  readonly result: ToolResultLike | undefined;
  /** Changes with every tool result, so a new result starts a clean navigation history. */
  readonly resultKey: number;
  /** Connection problem to report instead of waiting forever. */
  readonly error?: string | undefined;
}

const ACTIONS = { copy: copyText } as const;

/**
 * Theme, tokens, safe area and the waiting state around a session. Host style variables go through
 * mapHostStyle under the identity policy: the host chooses light or dark, the brand keeps its
 * colors, fonts and radii (DESIGN.md, Theming across hosts).
 */
export function ViewShell({ view, port, host, input, result, resultKey, error }: ViewShellProps) {
  const spec = VIEWS[view];
  const themed = useMemo(
    () => mapHostStyle({ theme: host.theme, variables: host.variables }, { policy: 'identity' }),
    [host.theme, host.variables],
  );
  const style = useMemo<CSSProperties>(
    () => ({ ...themed.tokenOverrides, ...safeAreaPadding(host.safeAreaInsets) }),
    [themed.tokenOverrides, host.safeAreaInsets],
  );
  const theme: UiTheme = themed.theme;

  return (
    <UiScope theme={theme} paint actions={ACTIONS} style={style}>
      <main className="app" aria-label={spec.title} data-view={view}>
        {error ? (
          <div className="iu-notice iu-notice--err" role="alert">
            <strong className="iu-notice__label">Not connected.</strong> {error}
          </div>
        ) : result ? (
          <ViewSession
            key={resultKey}
            view={view}
            port={port}
            input={input}
            result={result}
            toolInfoName={host.toolInfoName}
          />
        ) : (
          <p className="iu-muted app-waiting" role="status">
            Waiting for the result from your agent.
          </p>
        )}
      </main>
    </UiScope>
  );
}
