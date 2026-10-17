# Spec Delta: webui-console-session

## Purpose

How the Web UI behaves when served from a console whose ingress provides a cookie session (ADR-0013): it stores no credentials, calls its own origin, recovers from an expired session, and signs out. Loopback and Desktop behaviour is unchanged.

## ADDED Requirements

### Requirement: Session mode detection
The Web UI SHALL enter session mode only when a same-origin `GET /auth/session` returns 200 with a session report. In every other case it SHALL keep its existing token behaviour. This includes a loopback origin, a Desktop origin, and a missing or failing route.

#### Scenario: Loopback node
- **WHEN** the Web UI is served from `http://127.0.0.1:5001` and `/auth/session` returns 404
- **THEN** the chat token handling is exactly as before this change

### Requirement: No stored credentials in session mode
On entering session mode, the Web UI SHALL delete the stored chat token (`ipfs-webui:agent-chat:token`) and any stored API address that contains userinfo. It SHALL NOT write any credential to `localStorage` or `sessionStorage` while in session mode.

#### Scenario: Profile from the interim period
- **WHEN** a browser profile holds the chat token and `https://<user>:<password>@console.prometheusags.ai` as the API address, and the console is in session mode
- **THEN** after the first load both values are gone from storage and the API address is the console origin without userinfo

### Requirement: Same-origin calls without Authorization
In session mode, calls to `/api/v0`, `/agui`, the settings route and `/index/*` SHALL go to the page's own origin with `credentials: 'same-origin'` and SHALL NOT set an `Authorization` header.

#### Scenario: Chat run
- **WHEN** a signed-in user sends a chat message
- **THEN** the `/agui` request carries no `Authorization` header from the page and succeeds through the session

### Requirement: Expired session and step-up
A 401 from any same-origin call in session mode SHALL show "Sign in again". Activating it SHALL perform a top-level navigation to the current page. A 403 `step_up_required` from the settings route SHALL explain that settings need a recent sign-in with a second factor, and SHALL offer sign-out and sign-in.

#### Scenario: Session expired during use
- **WHEN** a call returns 401 in session mode
- **THEN** the page shows "Sign in again" and does not retry with any stored credential

### Requirement: Sign-out
In session mode, the Web UI SHALL show who is signed in (from the session report's email) and a sign-out action that navigates to `/oauth2/logout`. It SHALL also tell the user that signing out of `know-me.tools` is a separate step.

#### Scenario: Sign out
- **WHEN** the user chooses sign out
- **THEN** the browser navigates to `/oauth2/logout`
