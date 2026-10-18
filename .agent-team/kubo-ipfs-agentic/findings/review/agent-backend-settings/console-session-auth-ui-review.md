# UI review: console-session-auth group 3 (task 3.5)

Reviewer: reviewer (independent, separate context). Date: 2026-10-17.
Artifact: ipfs-webui feat/ipfs-mcp-server (session.js, session-boot.js, SessionPanel.js, SessionBanner.js, session.css, AgentSettingsPage.js, ConnectionSection.js, use-settings-client.js, http-client.js, search/client.js, locales en), kubo mcp/chat (element.ts, IslandApp.tsx, agui/client.ts), evidence mcp/ui/evidence/2026-10-17-console-session-auth/.
Requirements: openspec/changes/console-session-auth/tasks.md group 3, specs/webui-console-session/spec.md, ADR-0013 sections 5, 7, 9, 10.
Method limits: prometheus-ui-review skill not loadable in this context (no Skill tool); followed .agents/UI_UX_PROTOCOL.md. No shell: commit range not diffed, tests not run, contrast not measured. Files reviewed at current branch state.

Relayed verbatim by the lead (the reviewer context has no Write tool).

Verdict: BLOCK. One blocking finding (B1).

## BLOCKING
B1. Step-up "Sign in again" is a no-op. ipfs-webui/src/agent-chat/session.js:155-157 (reload), SessionPanel.js:36-37, public/locales/en/agentChat.json:48. In step-up the console cookie is valid; reload passes Envoy and keeps the same ID token amr/auth_time, so the page returns to step-up. The copy promises a second-factor sign-in the button cannot start. Fix: in step-up make the primary action signOut() (/oauth2/logout -> post-logout console root -> fresh Hydra login), and rewrite stepUp.text to say the second factor must be on the know-me.tools account, which may need signing out there too (ADR-0013 sections 5, 9). Add a test asserting the step-up primary action navigates to /oauth2/logout.

## MAJOR
M1. /api/v0 401 does not raise "Sign in again". src/bundles/ipfs-provider.js has no 401 path to markSessionExpired; only settings (http-client.js:79), search (search/client.js:229) and the chat island report it. Spec: "A 401 from any same-origin call in session mode SHALL show Sign in again". Today it surfaces only via the 5-minute revalidate or visibilitychange (SessionBanner.js:26-29).
M2. Chat-expired path has no reachable action. kubo mcp/chat/src/island/IslandApp.tsx:54 notifies the host; the island notice says "Sign in again" with no control; the banner button is occluded by the chat panel at 1440 (chat-expired-1440-dark.png) and fully covered at 320 (chat-expired-320-light.png). The sidebar is not inert, so keyboard focus can land on the hidden button (WCAG 2.2 SC 2.4.11). Fix: add a "Sign in again" action to the island's unauthorized notice in session mode, or close/shift the panel when the session expires.
M3. 3.4 is ticked without its acceptance run. test/e2e/console-session.test.js has not executed; the screenshots come from another harness (the spec writes no capture-results.json and its chat-signed-in shots are absent). Not blocking for 3.5 (the run belongs to the phase-gate integration run), but annotate 3.4 as held for the gate or untick it until the run passes.

## MINOR
m1. ADR-0013 section 9 says the Web UI "Sign out" "offers both"; the UI offers the console sign-out plus a note (agentChat.json:53). Copy is accurate; amend the ADR wording (a hardcoded know-me.tools link would violate the no-hardcoded-endpoint rule) or make the link configurable.
m2. Duplicate announcements. Session ended: banner role=alert plus AgentSettingsPage.js:40-41 role=alert nested in aria-live=polite (line 64). Step-up: header (agentSettings.json:55-57) and panel (SessionPanel.js:33) both say "Verify again". SessionPanel.js:29 claims a single announcement.
m3. Evidence gaps vs protocol: no keyboard-focus or reduced-motion captures; step-up 1440 captures scrolled past the step-up line and header.
m4. Captures show "Revision 21baf2bb" (pre-change base) in the footer: taken from an uncommitted tree, not tied to 3dd965ba.
m5. admin "not_eligible" has no state copy; settings 403 falls to phase "error" with generic text and Retry (src/settings/agent/AgentSettings.js:44-51).
m6. At 320 px the floating "Agent chat" button covers body text (step-up-320-light.png). Likely not new with this change.

## NIT
n1. src/index.js:37 awaits bootSession before first render; up to 4 s (session.js:20) on a hung origin. Loopback: one fast 404, behaviour unchanged.
n2. session-boot.js:37 treats a boot-time 401 as token mode silently; low likelihood behind Envoy's navigation redirect.

## Passed
- Promethity identity preserved (tokens only, StatusLine/Button reuse, both themes coherent).
- Focus order: banner first in DOM; Connection first on unauthorized/step-up; native buttons with focus-outline.
- Banner live region: role=alert via StatusLine.
- Loopback/Desktop unchanged when /auth/session is not 200 (session-boot.js:35-37; session.test.js:64-119).
- No credential written or displayed: readReport drops sub/token; store writes refused (settings-store.js:31,41,103; admin-token.js:65,78; ipfs-provider.js:456); only email shown.
- 320 px: 0 px horizontal overflow on all 16 captures (capture-results.json).

## e2e gap ruling
Not executing test/e2e/console-session.test.js is not blocking for 3.5. It belongs to the phase-gate integration run and must pass there; until then 3.4's acceptance is unverified (M3).

## To clear the block
Fix B1 (action plus copy plus test). M1 and M2 should be fixed in the same correction cycle. One batched confirmation pass then follows.
