import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react';
import { Badge, Button, Notice, interruptExpired, type Tone, type UiTheme } from '@gqadonis/ipfs-mcp-ui';
import { X } from 'lucide-react';
import type { AguiClient } from '../agui/client';
import { Sidebar, SidebarContent, SidebarFooter, SidebarHeader, useSidebar } from '../ui/sidebar';
import { Composer } from './Composer';
import { RunNotice, noticeFor } from './RunNotice';
import { createChatSession, type SessionIds } from './session';
import { buildTimeline } from './timeline';
import { Transcript } from './Transcript';

// One conversation: the sidebar with its header, transcript and composer. The session is created once
// per mount; the island remounts this component (by key) when the endpoint changes, so a thread never
// continues against a different server.

export type Readiness =
  | { readonly state: 'ready' }
  | { readonly state: 'no-endpoint'; readonly reason: string }
  | { readonly state: 'no-token' };

export interface ConversationProps {
  readonly client: AguiClient | null;
  readonly readiness: Readiness;
  readonly theme: UiTheme;
  readonly ids: SessionIds;
  /** The clock, epoch milliseconds. Defaults to Date.now; a test seam the custom element never sets. */
  readonly now?: () => number;
}

/** The host clock as an ISO string, advanced exactly when the next interrupt expiry passes. */
function useNowIso(expiries: readonly string[], clock: () => number): string {
  const [now, setNow] = useState(clock);
  const next = useMemo(() => {
    const times = expiries.map((e) => Date.parse(e)).filter((t) => Number.isFinite(t) && t > now);
    return times.length > 0 ? Math.min(...times) : undefined;
  }, [expiries, now]);
  useEffect(() => {
    if (next === undefined) return;
    const timer = setTimeout(() => setNow(clock()), Math.max(0, next - clock()) + 25);
    return () => clearTimeout(timer);
  }, [next, clock]);
  return new Date(now).toISOString();
}

const EMPTY: Record<Readiness['state'], { title: string; body: string }> = {
  ready: {
    title: 'Ask about your node',
    body: 'Reads run at once. A change waits for your approval in this panel. A removal waits for the system prompt, where you can also deny it.',
  },
  'no-endpoint': {
    title: 'Chat is not configured',
    body: 'The host page sets the ipfs mcp address. Until it does, this panel sends nothing.',
  },
  'no-token': {
    title: 'Chat has no token',
    body: 'The host page supplies the token for the ui principal. Until it does, this panel sends nothing.',
  },
};

export function Conversation({ client, readiness, theme, ids, now = Date.now }: ConversationProps) {
  const [session] = useState(() => createChatSession({ client, ids, now }));
  const { chat, running, observing, error } = useSyncExternalStore(session.subscribe, session.getSnapshot);
  const { open, setOpen } = useSidebar();
  const panel = useRef<HTMLElement>(null);

  // Opening moves focus into the panel (the message box, or the title when sending is blocked), so a
  // keyboard user lands where they can act. Closing returns focus to the host's opener; the element does that.
  useEffect(() => {
    if (!open) return;
    const target = panel.current?.querySelector<HTMLElement>('textarea:not([disabled])') ?? panel.current?.querySelector<HTMLElement>('h3');
    target?.focus({ preventScroll: true });
  }, [open]);

  // A conversation that unmounts (the element left the page) must not keep a stream open.
  useEffect(() => () => session.stop(), [session]);

  const items = useMemo(() => buildTimeline(chat), [chat]);
  const expiries = useMemo(
    () => chat.interrupts.filter((i) => i.resolution === undefined && i.interrupt.expiresAt).map((i) => i.interrupt.expiresAt as string),
    [chat.interrupts],
  );
  const notice = noticeFor(error, chat.run);
  // The retry hold of a 503 ends at an instant, like an interrupt's expiry, so the clock stops there too.
  const holdUntil = error?.retryNotBefore;
  const ticks = useMemo(() => (holdUntil === undefined ? expiries : [...expiries, new Date(holdUntil).toISOString()]), [expiries, holdUntil]);
  const nowIso = useNowIso(ticks, now);
  const nowMs = Date.parse(nowIso);
  const held = holdUntil !== undefined && nowMs < holdUntil;
  const waiting = chat.interrupts.some((i) => i.resolution === undefined && !interruptExpired(i.interrupt, nowIso));

  const blockedReason =
    readiness.state === 'no-endpoint'
      ? 'Not configured.'
      : readiness.state === 'no-token'
        ? 'No token.'
        : waiting
          ? 'Answer the approval above first.'
          : held
            ? 'The node asked for a short wait.'
            : undefined;

  const status: { word: string; tone: Tone } =
    readiness.state === 'no-endpoint'
      ? { word: 'Not configured', tone: 'neutral' }
      : readiness.state === 'no-token'
        ? { word: 'Needs token', tone: 'warn' }
        : observing.length > 0
          ? { word: 'Waiting for system prompt', tone: 'warn' }
          : running
          ? { word: 'Running', tone: 'info' }
          : waiting
            ? { word: 'Waiting for approval', tone: 'warn' }
            : notice
              ? { word: notice.status, tone: notice.tone }
              : { word: 'Ready', tone: 'neutral' };

  const empty = EMPTY[readiness.state];
  const answer = (interruptId: string, decision: 'approve' | 'deny') => void session.answer(interruptId, decision);
  // The watch of a system prompt stops when the stream drops or the person stops the run; the
  // confirmation stays pending until it expires, so offer to watch again while an error is showing.
  const watchAgain = !running && error !== undefined ? (interruptId: string) => void session.observe(interruptId) : undefined;

  return (
    <Sidebar label="Agent chat" ref={panel}>
      <SidebarHeader>
        <div className="flex items-center justify-between gap-s2">
          <h3 tabIndex={-1} className="font-ui-heading text-[17px] leading-[1.3] font-semibold">
            Agent chat
          </h3>
          <Button variant="secondary" onClick={() => setOpen(false)}>
            <X size={16} strokeWidth={1.5} aria-hidden="true" />
            Close
          </Button>
        </div>
        {/* Status and the thread action share a row, so the title row stays short at 320px. */}
        <div className="flex items-center justify-between gap-s2">
          <Badge tone={status.tone}>{status.word}</Badge>
          <Button variant="text" onClick={() => session.reset()} disabled={items.length === 0 && !running}>
            New chat
          </Button>
        </div>
      </SidebarHeader>

      <SidebarContent>
        {items.length === 0 ? (
          <div className="flex flex-col gap-s2 px-s3 py-s3">
            <h4 className="font-ui-heading text-[17px] leading-[1.3] font-semibold">{empty.title}</h4>
            <p className="max-w-[66ch] text-muted-foreground">
              {readiness.state === 'no-endpoint' ? `${empty.body} ${readiness.reason}` : empty.body}
            </p>
          </div>
        ) : (
          <Transcript items={items} theme={theme} nowIso={nowIso} running={running} observing={observing} onAnswer={answer} onWatchAgain={watchAgain} />
        )}
        {notice ? <RunNotice model={notice} nowMs={nowMs} onRetry={() => void session.retry()} /> : null}
      </SidebarContent>

      <SidebarFooter>
        <Composer blockedReason={blockedReason} running={running} onSend={(text) => void session.send(text)} onStop={() => session.stop()} />
      </SidebarFooter>
    </Sidebar>
  );
}
