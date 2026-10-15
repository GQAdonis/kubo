# Round 5 evidence: agent settings page and chat island integration

Changes: `webui-agent-settings-page` 1.1-1.7, `agent-chat-island-integration` 1.1-1.4. Run date 2026-10-14.

## Setup (all on non-default ports, own `IPFS_PATH`, fake keys)

| Process | Address |
|---|---|
| temporary Kubo node (`test` profile, bootstrap emptied, `Routing.Type none`) | swarm 4299, API 5299, gateway 8299 |
| `ipfs mcp --http --agui --mcp-config ...` built by `make build` (`cmd/ipfs/ipfs`), `admin` and `ui` principals from env, `--settings-dir` in a temp dir | 127.0.0.1:5287 |
| fake OpenAI-compatible model server (`scripts/fake-llm.mjs`, lists four models) | 127.0.0.1:5286 |
| Web UI dev server (`npm start`, branch `feat/ipfs-mcp-server`) | 3299 |
| one-origin proxy (`scripts/proxy.mjs`): `/agent/*` and `/agui` to ipfs mcp, the rest to the dev server | localhost:3297 |

The proxy exists because the settings route is same-origin by design: its CORS answer allows only `GET, POST, DELETE, OPTIONS` and no `If-Match` header, so a cross-origin PUT from a Web UI on another origin is refused at the preflight. In the cloud deployment the console origin serves both. Screenshots are taken by `scripts/cdp.mjs`, a raw Chrome DevTools Protocol driver (headless Chrome 155, no Playwright), through the flows in `scripts/run-a.mjs`, `run-b.mjs`, `run-c.mjs`. Tokens are fake (`adm-evidence-fake-0001`, `ui-evidence-fake-0001`, key `sk-evidence-fake-0001`). Every state is captured four ways: `-1440-dark`, `-1440-light`, `-375-dark`, `-375-light`. The run was done twice; the first pass found the defects listed below, which were fixed, and every shot here is from the confirmation pass.

## States

| File prefix | State | What to see |
|---|---|---|
| `01-first-run` | `GET` returns `backend: null` | No backend selected, two-step list, no field set mounted, Apply off, status says "No backend" |
| `02-chat-no-backend` | chat island, node answers `503 backend_not_configured` | "Pick a model to start", a "Choose a backend" link to `#/settings/agent`, the refused message kept below with "Send message again", badge "No backend" |
| `03-openai-tested` | OpenAI-compatible form, connection tested | "Reachable", "4 models listed", model combobox with typeahead open |
| `04-openai-confirm` | host changed | Typed re-confirmation: "Confirm the model host: type 127.0.0.1:5286"; Apply stays off until typed |
| `05-key-stored` | key saved | Field replaced by "Stored ... never shown again" with Replace and Remove; key text appears nowhere in the DOM |
| `06-apply-success` | `PUT` with `If-Match` and `egress_confirmation` | "Applied" with time, model and host, and the in-flight-run sentence |
| `07-configured-header` | header after apply | Backend, host and model in one status line |
| `08-apply-error` | another client changed the node first | `412 revision_conflict`: "Not applied. Nothing was changed. The previous backend is still in use." Focus moves to the failure; the button reads Retry |
| `09-uar-unavailable` | UAR chosen | "Not available", what is missing, every control disabled, Apply off with the reason, no success state |
| `10-sign-in-again` | wrong admin token, `401` | "Sign in again" in the header; the Connection section holds the token field |
| `11-settings-summary` | `/settings` | The one-line Agent row replacing the old chat and WebMCP boxes, with a link |
| `12-secrets-not-accepted` | node with `agent.settings.accept_secrets: false` | Key field replaced by "Set on the server (Kubernetes Secret)" after the `403 secrets_not_accepted` |

## Defects found by the first pass and fixed before the confirmation pass

- The island's "Choose a backend" button had an accent label on the accent fill (the library rule `[data-ipfs-ui] a` outranks `.iu-btn--primary`). Fixed in `mcp/chat/src/styles/island.css`.
- The Agent row link on `/settings` had no styles (the page stylesheet is a lazy chunk the Settings page does not load): now `agent-summary.css`.
- Duplicate words in status lines ("Applied Applied 02:57", "Off Off.", "Stored A key is stored") reworded.
- The typed-confirmation panel clipped its help text (margin collapse): `display: flow-root`.
- UAR selected showed "Unsaved changes"; it now says "Not available".

## Not captured, and why

- Desktop bridge states: the Desktop side is built in parallel; the bridge client is covered by Jest against a fake bridge (`core.test.js`, `AgentSettingsPage.test.js`).
- Settings change event (SSE) across two tabs: covered by Jest (`core.test.js` stream test, the "Changed elsewhere" test); not shown here.

## Teardown

Every process was stopped by PID (dev server tree 14275/14405/14409/14430, proxy 14273, fake model 14271, `ipfs mcp` 5583 under `timeout` 5581, Kubo 78987 under `timeout` 78984). Afterwards ports 3299, 3297, 5286, 5287, 5299, 4299, 8299 and 9333 had no listener. 4001, 5001, 8080 and 5050 were never touched.
