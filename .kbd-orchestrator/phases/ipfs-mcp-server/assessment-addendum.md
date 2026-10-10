```
ADDENDUM: ipfs-mcp-server (G9-G12)
Project: Kubo (fork GQAdonis/kubo, 0.44.0-dev, HEAD 833e04a24)
Date: 2026-10-09
Base: assessment.md revision 2 (decision of record F4, W1+W4), plan.md revision 2 (22 changes)
Scope of this addendum: G9 AG-UI/A2A, G10 LLM backend, G11 chat UI, G12 embeddings/SurrealDB
Revision: 3 (consolidation, 2026-10-09): the decision of record follows the round-2 adjudication and the operator answers of 2026-10-09; revision 2 followed review round 1 (BLOCK)
```

Assessor: `ipfs-agent-architect` (KBD Assess stage, addendum). The only file written is this one. No code, no OpenSpec change, and no file in another repository was touched.

### Revision 2 changes (review round 1)

| Finding | How revision 2 addresses it |
|---|---|
| CRITICAL A: no falsifier; non-delivery defaulted to "ship non-conformant" | New section ASSUMPTIONS AND FALSIFIERS, per goal. The safe default on non-delivery is **defer**, never "ship non-conformant", with a dated delivery gate (end of Round 4). The N2 spike now tests the addendum's own top risks: an A2A-delegated write, setup cost against a threshold, and UAR behavior with and without the shipped profile. |
| CRITICAL B: G10/G12 guarantees delegated to a runtime that breaks them by default | New section KUBO-SIDE ENFORCEMENT. Four things Kubo ships in `ipfs mcp`: a principal policy, an egress acknowledgement, an out-of-band confirmation channel, and a `check-uar` preflight. Kubo also ships a tested UAR profile under `mcp/uar/**`. Acceptance criteria are tied to these. Guarantees Kubo cannot enforce are named and the goal is marked **partially unmet** for them. "Nothing leaves the machine by default" is removed from ADR-0006. |
| WARNING B: "the human gate holds today" over A2A | Claim removed. New evidence: UAR bypasses tool approval entirely when its governance gate is disabled. That is possible in loopback mode without JWT (`uar:src/uar/runtime/manager.rs:5469-5480`, `uar:src/uar/governance/runtime_control.rs:455-478, 662-663`). The A2A human gate is **unknown** until N2 proves it, and the Kubo-side out-of-band confirmation becomes the enforceable gate. |
| WARNING C: G10 reinterpreted | The G10 decision is **PENDING operator question 2**. L3 (thin proxy) is re-presented with its real gain (a Kubo-controlled origin/auth boundary) and its real cost. |
| WARNING D: scores without weights; (d) Security and Rules fit inflated | Weights and the aggregation rule are stated. Options with goal fit at or below 2 are excluded before aggregation. Every table is re-scored, (d) and E1 are scored "as shipped" and "with Kubo enforcement", and the margins and sensitivities are stated. |
| WARNING E: G12 silently narrowed to UAR-routed content | Listed as an explicit limitation, with an opt-in `ipfs mcp` embedding hook as the option that closes it (N9, conditional). |
| SUGGESTION: deletion checks remaining pins | Adopted. Vectors are deleted only when `pin ls --type=all <cid>` shows no direct, recursive or indirect pin. On `repo_gc`, deletion is limited to CIDs that GC reports as removed. |
| Consolidation (2026-10-09, after round 2 and the full cross-model review `findings.full.json`) | The DECISION and the ADR-0004 to ADR-0006 drafts are rewritten as the self-contained decision of record:<ul><li>G9: topology A, AG-UI and A2A in `ipfs mcp` at default `127.0.0.1:5050`, with topology B (UAR-hosted) gated on a published UAR release by the end of Round 4</li><li>G10: L3, `ipfs mcp` proxies to an operator-configured UAR or OpenAI-compatible server</li><li>Human gate: native OS prompt with a presence factor</li><li>G11: Web UI island, Promethity, talking only to `ipfs mcp`; Desktop fork in scope</li><li>G12: requirements, concrete per-mode mechanisms and acceptance tests, with the component choice left to plan revision 2</li></ul>Load-bearing assumptions and falsifiers now sit inside DECISION per goal. Stdio-spawned UAR is forbidden wherever node content reaches a model, and the general "nothing leaves the machine" claim is withdrawn and replaced by a stated partial guarantee. Plan impact is rewritten: 34 changes. The revision-2 DECISION, ADRs and plan impact are kept under "SUPERSEDED (2026-10-09)". The review sections at the end are unchanged. |
Evidence conventions (same as `assessment.md`, plus two prefixes):

- `path:line` is a Kubo working-tree fact read this session.
- `uar:path:line` is from `/Users/gqadonis/Projects/prometheus/universal-agent-runtime` (HEAD `a7cb9729`, 2026-10-01), read-only.
- `webui:path:line` is from `/Users/gqadonis/Projects/references/ipfs-webui` (HEAD `24ddaa89`), and `desktop:path:line` is from `/Users/gqadonis/Projects/references/ipfs-desktop` (HEAD `9bd9904`, origin `tribe-health/ipfs-desktop`). Both are read-only.
- **[sub]** marks facts from read-only Explore sub-investigations run this session. The lead re-read these key ones and they hold:
  - `uar:src/uar/api/a2a/types.rs:116-132`, `uar:src/uar/api/a2a/mod.rs:75`, `uar:src/uar/api/a2a/agent_card.rs:26-30`
  - `uar:src/server.rs:2040-2051, 2528-2533`
  - `uar:src/config.rs:1409-1414, 1476` (no `cors` key in `uar:src/config.rs`: `grep -n cors` returns nothing)
  - `uar:src/uar/api/adapters.rs:361-381`, `uar:src/mcp/registry.rs:140-173`
  - `uar:src/uar/runtime/manager.rs:647-652, 5450-5510`, `uar:src/uar/governance/runtime_control.rs:455-478, 533, 662-663`
  - `uar:src/uar/api/a2a/task_execution.rs:568`
  - `webui:package.json:83, 91, 100, 104, 313-314`, `desktop:src/webui/index.js:41-47`
  - the greps for `elicit`, `ui://` and `RunAgentInput` in UAR `src` (no relevant hits; the only `elicit` match is the token "felicity" in a tokenizer JSON file)
- **[external]** marks facts from outside the repositories, with the URL and the date fetched (2026-10-09). **[unverified]** marks claims that were not checked.

---

## ADDENDUM: ipfs-mcp-server (G9-G12)

### Summary of findings

1. **None of G9-G12 exists in Kubo.** UAR (MIT, `uar:LICENSE:1`, `uar:Cargo.toml:35`) already has most of the parts:
   - an agent loop;
   - an OpenAI-compatible LLM client over liter-llm, with a configurable `base_url` and env-sourced keys;
   - an MCP client over stdio and Streamable HTTP;
   - Cedar plus a tool-approval pause;
   - an AG-UI SSE profile;
   - an A2A JSON-RPC server;
   - local FastEmbed embeddings;
   - SurrealDB in embedded (surrealkv) and remote modes;
   - knowledge-base ingest and search APIs.

   With that, Kubo's side of G9-G12 can stay at MCP tools, the contract, an enforcement layer inside `ipfs mcp`, and documentation.

   > **Superseded 2026-10-09:** the decision of record (DECISION, revision 3) puts AG-UI, A2A and a bounded agent loop with an LLM proxy in `ipfs mcp` (topology A). UAR is a supported backend and a gated second topology.
2. **UAR is not conformant enough to be used as-is.**
   - **A2A.**
     - The agent card is at `/.well-known/agent.json` (`uar:src/uar/api/a2a/mod.rs:75`). A2A 1.0 specifies `/.well-known/agent-card.json` [external].
     - `TaskState` uses `rename_all = "lowercase"` (`uar:src/uar/api/a2a/types.rs:118`), so `InputRequired` goes on the wire as `"inputrequired"`.
     - There is no `auth-required` state, no `message/stream`, and no push notifications (`agent_card.rs:28-29`).
     - Tool approval never surfaces as A2A input-required: the only mapping arm (`RunStatus::Paused`) is never assigned [sub].
   - **AG-UI.**
     - There is no `RunAgentInput` POST endpoint.
     - There are no spec interrupts. Approval goes out as the `CUSTOM` event `uar.tool.approval_required` (`uar:src/uar/api/adapters.rs:361-381`).
   - **MCP client.** It has no elicitation support and no MCP Apps `ui://` support.
   - **CORS.** CORS is `allow_origin(Any)` (`uar:src/server.rs:2528-2533`) on every non-sidecar instance (`:2047-2051`), and **it is not configurable**: `uar:src/config.rs` has no CORS key.
3. **UAR's defaults conflict with the Kubo rules.**
   - The default model is `openai/gpt-4o` (`uar:src/config.rs:1476`), an implicit default endpoint.
   - The default bind is `0.0.0.0:1906` (`:1411-1412`).
   - The memory subsystem defaults to OpenAI `text-embedding-3-small` (`uar:src/config.rs:1464-1465`) [sub].
   - The dev configs disable JWT (`config.embedded.yaml:19`) [sub].
4. **The UAR human gate is conditional.**
   - Every MCP tool is marked `ApprovalClass::Required` (`uar:src/mcp/registry.rs:166-167`).
   - But `governance_bypass_decision` returns `GovernanceBypassed` whenever the governance gate is not effectively enabled (`uar:src/uar/runtime/manager.rs:647-652`). The bypass is taken before the approval check, when the host does not require approval, the run is not a child run, and the policy is `Auto` (`manager.rs:5469-5480`).
   - Governance may be disabled when four things are all true: the configured host is `localhost`/`127.0.0.1`, JWT is not required, no host token is in use, and every ingress is loopback (`uar:src/uar/governance/runtime_control.rs:455-478, 662-663`). In that case the effective state follows the operator's stored preference (`:533`).
   - A2A-delegated tasks are built with `tool_approval: ToolApprovalPolicy::Auto`, as constructed at `uar:src/uar/api/a2a/task_execution.rs:568`; that line may be test or fixture code, and its production meaning is [unverified].
   - **Consequence: UAR's approval cannot be the only human gate for node writes.**
5. **G12 must not embed SurrealDB into the `ipfs` binary.**
   - The only embedded path from Go is `github.com/surrealdb/surrealdb.c.go`, described as "Go bindings for embedded SurrealDB via the surrealdb.c C FFI library" [external, https://github.com/surrealdb/surrealdb.c.go, fetched 2026-10-09]. That means cgo plus a Rust native library.
   - Kubo has no cgo today: `grep -rln 'import "C"' --include=*.go .` (excluding tests) returns nothing.
   - SurrealDB is licensed "Business Source License 1.1", with "Change License: Apache License, Version 2.0" [external, https://github.com/surrealdb/surrealdb/blob/main/LICENSE, fetched 2026-10-09].
   - The pure-Go SDK `surrealdb.go` is a network client (`FromEndpointURLString(ctx, "ws://localhost:8000")`) [external, https://surrealdb.com/docs/languages/golang, fetched 2026-10-09].
6. **G11 cannot reuse `mcp/ui` (React 19) inside the Web UI (React 16) by import.**
   - The Web UI is React 16.14 on CRA 5 with Tachyons (`webui:package.json:83, 91, 100, 104`). React is forced to 16 via `overrides` (`:313-314`).
   - It still uses `ReactDOM.render` (`webui:src/index.js:35`) [sub].
   - At least eleven dependencies cap peer React at 16 [sub].

---

## GOAL PROGRESS

| Goal | Status | Evidence |
|---|---|---|
| G9 AG-UI and A2A endpoints for IPFS services | MISSING | No AG-UI, A2A, SSE event or agent-card code in Kubo: `grep -rniE "ag-ui\|agui\|a2a\|agent-card" --include=*.go .` returns nothing relevant. `mcp/` and `docs/mcp/` do not exist. The plan's 22 changes contain no G9 task (`plan.md`, `tasks.md`). UAR has partial, non-conformant AG-UI/A2A (Summary 2). |
| G10 LLM backend (UAR or OpenAI-compatible) | MISSING | No LLM client in Kubo (`grep -n "openai" go.mod` returns nothing); no change in `plan.md` covers it. UAR has the client (`uar:Cargo.toml:579`) and configurable providers (`uar:src/config.rs:50, 54, 1642-1659`) [sub]. |
| G11 chat UI in Desktop and Web UI | MISSING | No chat code in the Web UI: the grep for chat, assistant, llm and openai matches only the icons `webui:src/icons/GlyphChat.tsx` and `StrokeChat.tsx` [sub]. No React in Desktop [sub]. `mcp/ui` does not exist yet. |
| G12 embeddings into SurrealDB | MISSING | Nothing in Kubo (`grep -n "surreal" go.mod` returns nothing). UAR has FastEmbed (`uar:src/uar/rag/embeddings/fastembed.rs:87-101`), SurrealDB 3.3.0 surrealkv (`uar:Cargo.toml:502`) and knowledge-base routes (`uar:src/uar/api/knowledge.rs:155-169`) [sub]. |

G1-G8 are unchanged from `assessment.md`: no implementation has started, and 8 of the 22 changes are registered with 0 complete (`progress.json`).

---

## ARCHITECTURE OPTIONS

### Facts that constrain every G9-G12 option

1. **The F4 boundary.**
   - `ipfs mcp` is a local command that talks to the daemon only over `/api/v0` and never opens the repo.
   - The daemon, `config/` and `docs/config.md` do not change in this phase.
   - An option that adds a daemon listener reopens C1/F1.
2. **Kubo rules that apply to G10-G12.**
   - No hardcoded endpoints; every endpoint must be configurable with an off switch.
   - No tracking.
   - No silent CID injection.
   - No resource appropriation.

   Sending node content to an LLM or a remote embedder is data leaving the machine: off unless configured, the destination named, and never a default.
3. **The confirmation gate.**
   - It uses elicitation when the client supports it, and a two-step confirm token otherwise.
   - The token proves that the agent asked twice, not that a human agreed (`plan.md` T2).
   - The plan cut the out-of-band confirmation channel and left it as plan question Q2.
4. **UAR as an MCP client.**
   - It spawns stdio servers (`uar:src/mcp/registry.rs:693`) or connects to Streamable HTTP (`:720-723`).
   - It is configured in `mcp.json`, with header secrets as `{secret_ref: "env:VAR"}` (`uar:src/mcp/config.rs:10-12, 45-83`).
   - It does not implement elicitation.
   - Its approval pause is bypassable (Summary 4).
   - **Consequence:** through UAR, `ipfs mcp` always takes the token fallback, and no human gate is guaranteed on the UAR side.
5. **Protocol versions** [external, fetched 2026-10-09]:
   - **A2A.** `a2aproject/A2A` `docs/specification.md` is at 1.0.0. Agent card at `/.well-known/agent-card.json`. Task states include `TASK_STATE_INPUT_REQUIRED` and `TASK_STATE_AUTH_REQUIRED`. Bindings are JSON-RPC, gRPC and HTTP+JSON. Security schemes are API key, HTTP, OAuth2, OIDC and mTLS. The release date came from an LLM-extracted summary and is [unverified].
   - **A2A Go SDK.** `github.com/a2aproject/a2a-go/v2` exists.
   - **AG-UI.** The run is interrupt-aware: `RUN_FINISHED` carries `outcome: {type: "interrupt", interrupts: [...]}`, and the client resumes with a new run whose input carries `resume: [...]` (https://docs.ag-ui.com/concepts/interrupts).
   - **AG-UI Go SDK.** Listed as a community SDK (https://docs.ag-ui.com/introduction).
6. **go-sdk can reach the host's LLM without an LLM client.** It has MCP sampling (`ServerSession.CreateMessage`/`CreateMessageWithTools`) and `Elicit` (`$GOMODCACHE/github.com/modelcontextprotocol/go-sdk@v1.8.0/mcp/server.go:1664, 1710, 1729`). Sampling only works inside an MCP host, so it is not a G10 backend.
7. **The Web UI origin is admin-privileged.**
   - The Web UI is served on the RPC origin and holds admin RPC (`core/corehttp/webui.go:93-147`).
   - Kubo sets no CSP: `grep -rn "Content-Security-Policy" core/ config/ docs/config.md` returns nothing.
   - Desktop sets no CSP and runs `webSecurity: false` (`desktop:src/webui/index.js:41-47`).

### Scoring method (applies to every table below)

- **Scale.** Each criterion is scored 1 (worst) to 5 (best).
- **Goal-fit gate.** An option with goal fit at or below 2 is **excluded** before aggregation: it does not meet the operator's goal, however cheap it is.
- **Aggregation.** The rest are compared by a weighted sum, with the weights listed per table. Ties within 3 points are reported as **not decisive**, and the decision then rests on the stated qualitative argument and the falsifiers.
- **Two rows for UAR-based options.** They are scored twice:
  - **as shipped**, with UAR's verified defaults (Summary 3, 4);
  - **with Kubo enforcement**, meaning the N8 controls in KUBO-SIDE ENFORCEMENT, which Kubo can deliver without another team.
- **What is not scored.** Items that need UAR-team delivery (N1) are not credited in either row.

### G9: AG-UI and A2A endpoints

**Options.**

- **(a) Inside the `ipfs mcp` process, beside Streamable HTTP.**
  - Add `/agui`, `/a2a` and `/.well-known/agent-card.json` to the `--http` listener.
  - It reuses the `AuthVerifier`, the scope map and the confirmation gate in-process: an AG-UI interrupt or an A2A input-required maps one-to-one to a pending confirmation.
  - It needs a Go agent loop and LLM client (G10 L2), plus a2a-go v2 and an AG-UI Go implementation in the root `go.mod`.
- **(b) A daemon-hosted listener.** This is C1 again, plus an LLM client in the daemon.
- **(c) A separate service or binary in this repo.** A second product with its own release tooling; it still needs the Go loop.
- **(d) Hosted by UAR, which calls IPFS through `ipfs mcp` as a tool provider.** UAR serves AG-UI and A2A and owns the loop, the model, approvals and sessions. `ipfs mcp` is an entry in UAR's `mcp.json`.
- **(e1) An LLM-free A2A layer in `ipfs mcp`.** A `DataPart` `{tool, arguments}` maps to one contract tool, and confirmation maps to input-required. There is no model.
- **(e2) A hybrid: (d) with (a) as a pre-designed fallback.** Both reuse the same contract.

**How each option reuses the shared pieces:**

| Option | Contract reuse | Auth reuse | Confirmation-gate reuse |
|---|---|---|---|
| (a) | Direct, in-process | Same `AuthVerifier` | Native; human-visible through the AG-UI interrupt or A2A input-required |
| (b) | Direct | A second auth system in the daemon | Native, inside the daemon |
| (c) | Generated copy | Duplicated verifier | Duplicated gate |
| (d) | Through MCP `tools/list` | UAR authenticates its own clients; UAR authenticates to `ipfs mcp` with a scoped bearer | **As shipped:** only the token fallback is guaranteed, because UAR's approval is bypassable (Summary 4). **With N8:** out-of-band confirmation, enforced by `ipfs mcp`. |
| (e1) | Direct | Same verifier | Native, input-required (but the A2A caller is an agent, so the human gate needs N8 out-of-band confirmation anyway) |

**Weights:**

| Criterion | Weight |
|---|---|
| Goal fit | 3 |
| Kubo surface (5 = nothing added) | 2 |
| Rules fit | 3 |
| Security | 3 |
| Maintenance in this repo | 2 |
| Time to working result | 1 |
| Interop/conformance | 1 |
| **Maximum** | **75** |

**Scores:**

| Option | Goal fit | Kubo surface | Rules fit | Security | Maint. | Time | Interop | Weighted |
|---|---|---|---|---|---|---|---|---|
| (a) in `ipfs mcp` | 5 | 2 | 4 | 4 | 1 | 2 | 4 | **51** |
| (b) daemon listener | 4 | 1 | 2 | 2 | 1 | 1 | 4 | 33 |
| (c) separate binary | 4 | 3 | 4 | 3 | 2 | 2 | 4 | 49 |
| (d) UAR-hosted, as shipped | 4 | 5 | 2 | 2 | 5 | 3 | 2 | 49 |
| **(d) UAR-hosted, with Kubo enforcement (N8)** | **4** | **4** | **4** | **3** | **4** | **3** | **2** | **54** |
| (e1) LLM-free A2A layer | 2 | | | | | | | excluded (goal fit 2) |

**Reading the scores.**

- (d) as shipped does not win; it ties (c) and loses to (a).
- (d) with N8 leads (a) by 3 points, which is **not decisive** under the rule above.
- **Sensitivity.** With Maintenance weighted 1 and Time weighted 0, (a) scores 48 and (d)+N8 scores 47. The choice turns entirely on how much a Go agent runtime inside the Kubo repo should cost.
- **Why (d)+N8 anyway.** The decision rests on that maintenance argument and is guarded by the falsifiers in ASSUMPTIONS AND FALSIFIERS.
- **Security is 3, not 4.** UAR's CORS cannot be configured, and its approval is bypassable; N8 covers the node boundary but not UAR itself.

### G10: LLM backend

**Options.**

- **(L1) UAR is the only agent loop.** The "OpenAI-compatible server plus key" is a UAR provider config (`--llm-base-url`/`--llm-api-key`, `uar:src/config.rs:50, 54`; YAML `providers` with `${ENV}` keys) [sub]. This **narrows** G10, which reads as two alternatives, one of them without UAR: the non-UAR alternative would be unmet.
- **(L2) Minimal Go loop in `ipfs mcp`** for the OpenAI-compatible case, with UAR as the other option. It adds:
  - a chat-completions client with streaming and tool-call assembly;
  - a loop with budgets;
  - sessions;
  - AG-UI/A2A emission.

  The goal is met literally. It costs a second agent runtime in Kubo.
- **(L3) A thin proxy in `ipfs mcp --http`.**
  - **What it is.** `ipfs mcp` exposes one AG-UI route and forwards it (SSE pass-through) to an operator-configured UAR instance.
  - **What the browser sees.** Kubo's origin allowlist, auth and egress acknowledgement; the UAR key stays inside `ipfs mcp` and never reaches the browser.
  - **Why it matters.** It answers the goal's own word, "proxy", and is the only Kubo-side fix for UAR's unconfigurable CORS (G11).
  - **Cost.**
    - An SSE proxy and auth translation in `ipfs mcp`, about one change.
    - An extra hop.
    - The OpenAI-compatible case still runs through UAR, because a raw OpenAI-compatible server does not call MCP tools.
- **(L4) MCP sampling only.** Not a G10 backend.

**Weights:**

| Criterion | Weight |
|---|---|
| Goal fit | 3 |
| Kubo surface | 2 |
| Rules fit | 3 |
| Maintenance | 2 |
| Operator setup cost (5 = cheapest) | 2 |
| **Maximum** | **60** |

**Scores:**

| Option | Goal fit | Kubo surface | Rules fit | Maint. | Setup | Weighted |
|---|---|---|---|---|---|---|
| L1 UAR loop for both backends, with N8 | 3 (narrows the goal) | 5 | 3 | 5 | 2 | **42** |
| L2 Go loop + UAR | 5 | 1 | 4 | 1 | 4 | 39 |
| L3 thin proxy in `ipfs mcp` to UAR, with N8 | 4 | 3 | 4 | 3 | 2 | 40 |
| L4 sampling only | 1 | | | | | excluded |

All three surviving options are within 3 points: **not decisive**. The choice between them is the operator's (question 2).

**Rules applied to G10, whatever is chosen:**

- There is no default endpoint.
- Keys come from env or a secret file, never a config literal.
- Nothing in Kubo starts or contacts UAR unless configured.
- Node content that reaches a model is visible to the operator and acknowledged by them (N8).

### G11: chat UI in Desktop and the Web UI

**Compatibility problem** (all facts [sub], with lead spot-checks):

- **The Web UI:**
  - React 16.14 and CRA 5 via `react-app-rewired-esm`;
  - Tachyons and `ipfs-css` for styling, no Tailwind;
  - TypeScript 4.x, Storybook 6.5 and Enzyme with the React 16 adapter;
  - `ReactDOM.render` at `webui:src/index.js:35`;
  - a class-component shell with the legacy `DropTarget` HOC (`webui:src/App.js:9, 127`).
- **Peers capped at React 16:** at least eleven, including `@tableflip/react-dropdown`, `react-ace` 8, `simplebar-react` 1 and `@reach/router` 1.3.4.
- **More are capped at React 17 and 18.**
- **shadcn/ui on Tailwind 4** assumes React 19 and Tailwind's global preflight, which would restyle Tachyons elements.

**Options.**

- **(U1) Upgrade the Web UI fork.** About 20 dependencies, CRA, Storybook and Enzyme, plus a shell rewrite. A permanent upstream-rebase cost.
- **(U2) A React 19 island.**
  - A shadow-root custom element (`<ipfs-agent-chat>`) built in-tree from `mcp/ui`.
  - The Web UI mounts it next to `<Notify/>` (`webui:src/App.js:63-106`) behind an off-by-default toggle, loaded by dynamic import.
  - Desktop gets it through the bundled Web UI (D1, `desktop:package.json:24`).
  - It runs in the admin origin.
- **(U3) A Desktop-only side view** in its own non-admin origin (Electron `WebContentsView`). Browser `/webui` users get nothing.
- **(U4) Iframe UAR's own UI.** Not IPFS-specific, not branded, no shared catalog.

**How the chat reaches AG-UI.**

- **Direct to UAR (U2/U3 as designed).**
  - It needs UAR to allow the specific origins. UAR's CORS is `Any` and **not configurable**, so the allowlist cannot be enforced on the UAR side today.
  - The API key would live in the browser, at the origin's trust level (Web UI) or in `safeStorage` behind a new preload IPC (Desktop; today's bridge exposes only analytics consent and language, `desktop:src/webui/preload.js:55-90`) [sub].
- **Through `ipfs mcp` (L3).**
  - The browser talks only to `ipfs mcp --http`, which enforces an origin allowlist (`webui://localhost`, `http://127.0.0.1:5001`, `http://localhost:5001`), its own bearer and the egress acknowledgement.
  - The UAR key never reaches the browser.
  - This is the only path where Kubo controls the browser-facing boundary.
- **CSP.**
  - The island needs no `unsafe-eval` and no remote assets.
  - Desktop should set a CSP for `webui://localhost` whose `connect-src` lists the RPC and the chat endpoint (a Desktop-fork change).
- **Confirmation in chat.** The island renders UAR's `uar.tool.approval_required` events with the shared confirmation dialog. Kubo's out-of-band confirmation (N8) is shown through that same dialog when the chat is the out-of-band surface (see KUBO-SIDE ENFORCEMENT item 3).

**Brand.** A shadow-root island can apply Promethity fully without touching Web UI styles. Whether it should inside the Web UI is the PRODUCT.md open decision (question 6).

**Ownership.**

- New grants are required: `mcp/chat/**` and Web UI `src/agent-chat/**` for ui-ux-engineer.
- No role owns the Desktop fork (question 7).

**Weights:**

| Criterion | Weight |
|---|---|
| Goal fit | 3 |
| Upstream-rebase cost (5 = low) | 2 |
| Security | 3 |
| Reach | 2 |
| Effort (5 = low) | 1 |
| Brand control | 1 |
| **Maximum** | **60** |

**Scores:**

| Option | Goal fit | Rebase | Security | Reach | Effort | Brand | Weighted |
|---|---|---|---|---|---|---|---|
| U1 upgrade the fork | 5 | 1 | 3 | 5 | 1 | 3 | 40 |
| U2 island, direct to UAR | 5 | 4 | 2 | 5 | 3 | 5 | 47 |
| **U2 island, through the `ipfs mcp` proxy (L3)** | **5** | **4** | **3** | **5** | **2** | **5** | **49** |
| U3 Desktop-only side view | 3 | 5 | 5 | 2 | 3 | 5 | 46 |
| U4 iframe UAR UI | 2 | | | | | | excluded |

U2 through the proxy leads U3 by 3 points: **not decisive**. With the Security weight at 4, U3 scores 51 and U2 through the proxy 52. With Security at 5, U3 wins. Keeping the chat out of the admin origin is a legitimate operator preference (question 6).

### G12: embeddings into SurrealDB

**Verified constraints.**

- **SurrealDB from Go** (Summary 5): embedded mode means cgo plus a native library; the engine is BSL 1.1.
- **UAR already provides:**
  - surrealkv embedded storage by default (`uar:src/config.rs:1439-1440`) and remote ws/http;
  - FastEmbed BGE-small (384 dimensions) loaded from committed local weights (`uar:src/uar/rag/embeddings/fastembed.rs:49-55, 87-101`);
  - remote embedders (OpenAI, Voyage, Cohere);
  - chunking;
  - per-knowledge-base upload and search [sub].
- **UAR weaknesses:**
  - Knowledge search is in-memory cosine with no ANN index (`uar:src/uar/persistence/providers/surreal.rs:1738-1739, 2073`) [sub].
  - `/api/ingest` hardcodes the "default" knowledge base [sub].
  - The memory embedder defaults to OpenAI (Summary 3).
  - `fastembed` fetches ONNX Runtime at build time (`uar:Cargo.toml:450-455`) [sub].
- **Kubo has no GC or unpin event stream.**

**Options.**

- **(E1) UAR owns embedding, the store and search**, triggered by agent actions that pass through UAR.
- **(E2) `ipfs mcp` embeds via surrealdb.c.go (cgo).**
- **(E3) `ipfs mcp` manages a SurrealDB sidecar.**
- **(E4) A different vector store inside Kubo.**
- **(E5) A daemon-wide hook that embeds everything added.** **Refused** under User Agency (no per-item local cause; it would index all node content). There is no flagged variant.

**Weights:**

| Criterion | Weight |
|---|---|
| Goal fit | 3 |
| Kubo surface | 2 |
| Rules fit | 3 |
| License/build fit | 2 |
| Maintenance | 2 |
| Search quality now | 1 |
| **Maximum** | **65** |

**Scores:**

| Option | Goal fit | Kubo surface | Rules fit | License/build | Maint. | Search | Weighted |
|---|---|---|---|---|---|---|---|
| E1 UAR, as shipped (remote memory embedder default) | 4 | 5 | 2 | 4 | 5 | 2 | 48 |
| **E1 UAR, with Kubo enforcement (N8 profile and preflight)** | **4** | **5** | **4** | **4** | **5** | **2** | **54** |
| E2 cgo embedded in `ipfs` | 4 | 1 | 3 | 1 | 1 | 4 | 31 |
| E3 sidecar managed by `ipfs mcp` | 4 | 2 | 2 | 3 | 2 | 4 | 36 |
| E4 other store in Kubo | 2 | | | | | | excluded |
| E5 daemon-wide hook | refused | | | | | | |

E1 with enforcement is decisive against E2 and E3. E1 **as shipped** is not acceptable: it fails Rules fit because of the remote memory-embedder default.

---

## KUBO-SIDE ENFORCEMENT (new in revision 2)

> **Partly superseded 2026-10-09 by DECISION (revision 3):**
>
> - Item 3 (CLI over a 0600 socket or a loopback page as the human gate) is replaced by the native OS prompt with a presence factor; CLI confirm is headless opt-in only.
> - Stdio-spawned UAR is forbidden, not "documented as outside the guarantee".
> - `check-uar` re-runs per session and on settings change.
> - Items 1, 2, 4 and 5 stand, and apply to topology B and to the G10 backend.

The guarantees in G10 ("off unless configured", "no default endpoint") and G12 ("no content leaves the machine without explicit operator configuration"), and the confirmation guarantee from `operator-answers.md` 5, must be enforced by something Kubo ships. Everything below lives inside `ipfs mcp` (flags, its own MCP config file, and the contract). There is no daemon change, no `config/` change and no `docs/config.md` key. It is delivered as one new change, N8 `mcp-agent-principal-enforcement`.

1. **Principal policy.**
   - **Remote principals are opt-in.** `ipfs mcp --http` serves a non-human, non-elicitation client such as UAR only as an explicitly configured principal in the `ipfs mcp` config file, never through a generic token.
   - **Default grant.** A principal's default tool groups are read-only.
   - **Writes.** Write and destructive groups must be granted by name.
   - **Off switch.** Remove the principal or its token, and nothing is served to it.
2. **Egress acknowledgement.**
   - **Content-returning tools.** These are the tools marked `contentEgress: true` in the contract: `ipfs_cat`, `ipfs_get`, `dag_get`, `ipfs_ls` and others. They are refused to a principal unless its config carries `egress_ack` listing the model endpoint host(s) and embedder host(s) the operator accepts. The local-only form `egress_ack: ["local"]` is valid only when the preflight (item 4) confirms local providers.
   - **Refusal.** A refused call returns a structured error naming the missing acknowledgement. It never returns an empty result.
3. **Out-of-band confirmation for principals without elicitation.**
   - **No self-confirmation.** The two-step token is not accepted from such principals, because the agent could confirm by itself.
   - **The gate.** Writes and destructive calls wait for a human decision on a surface `ipfs mcp` controls:
     - `ipfs mcp confirm <id>` / `ipfs mcp deny <id>` on the local CLI, through a local control socket under the user's runtime directory, mode 0600;
     - or a loopback-only confirmation page served by `ipfs mcp --http`, with a CSRF token. The chat island may embed it.
   - **Expiry.** A pending confirmation expires, and an expired one is a denial ("Deny is final.").
   - **Plan impact.** This pulls the cut out-of-band channel (`plan.md` Scope cuts, Q2) back into scope, for non-elicitation principals only. Clients with elicitation keep elicitation.
4. **`ipfs mcp check-uar <url>` preflight.** It also runs automatically at startup for each principal marked `kind: uar`.
   - **What it checks.** It reads the UAR agent card and, with an operator-supplied admin credential, the settings API (`GET /api/uar/settings`, Admin, `uar:src/uar/api/administration_capabilities.rs:148`). The settings fields that expose the effective model, embedders and governance state are [unverified]; N2 confirms them.
   - **What it refuses**, unless the principal's config lists the host in `egress_ack`:
     - the bind is not loopback, or an unauthenticated request is accepted (a probe returns 200 without credentials);
     - the effective chat model provider is remote;
     - the effective knowledge-base embedder or memory embedder is remote;
     - governance is effectively disabled while the principal has write groups.
   - **Fallback when settings cannot be read.** The principal is refused unless the operator sets `check: assert`, a recorded operator assertion that the check is waived for that principal. It is printed at startup.
   - **Failure mode.** Failure disables that principal only. The server and the other principals keep running.
   - **Outbound calls.** The preflight is an outbound call to an operator-configured URL only. There is no default URL.
5. **Shipped UAR profile, `mcp/uar/profile/`** (tested in N3):
   - an explicit `llm.model` placeholder that must be edited; whether UAR refuses to start with it unset is tested in N2;
   - memory and knowledge-base embedders set to `fastembed` (`bge-small-en-v1.5`, 384), with `persistence.vector_dimension: 384`;
   - `server.host: 127.0.0.1`;
   - `security.jwt_required: true`;
   - `UAR_LOCAL_ONLY` documented as the start mode;
   - an `mcp.json` that connects to `ipfs mcp --http` as the configured principal. The UAR profile does not spawn `ipfs mcp` over stdio, because a stdio client cannot be identified as UAR.

**What Kubo cannot enforce (stated, not hidden):**

| Guarantee | Enforceable by Kubo? | Status if UAR is unchanged |
|---|---|---|
| No node content reaches a remote model without operator configuration | **Yes, over HTTP** (items 1, 2, 4). **No over stdio**: a stdio client that spawns `ipfs mcp` cannot be identified, and its model choice is the host's. That is the same position as Claude Code or Codex using `ipfs mcp` today. | G10/G12 **partially met**: met over HTTP; the stdio-spawned UAR path is documented as unsupported for the egress guarantee. |
| UAR's own default model and memory embedder never contact a remote provider | **No.** Kubo can refuse to serve such an instance (item 4) but cannot change UAR. | Met at the node-data boundary only. A misconfigured UAR still calls its provider with non-node data (user prompts). |
| Browser chat origin allowlist on UAR | **No** (UAR CORS is not configurable). **Yes through L3**: the browser talks only to `ipfs mcp`. | G11 direct-to-UAR **unmet**. Met through L3. |
| A human confirms every node write | **Yes** (item 3; elicitation otherwise). | Met, independent of UAR's approval and its governance bypass. |
| UAR answers A2A and AG-UI per spec | **No** (UAR code). | G9 conformance unmet until N1 delivers (see the delivery gate). |

**Acceptance criteria tied to enforcement** (they replace "N1 acknowledged"):

- **G10:**
  - `test/cli` proves that a principal without `egress_ack` gets refused on every `contentEgress` tool;
  - that `check-uar` refuses a UAR instance running UAR's own defaults (`0.0.0.0`, `openai/gpt-4o`, JWT off), and accepts the shipped profile with a local provider;
  - that a principal with no configuration receives nothing.
- **G12:** a real-service test shows that with the shipped profile, ingesting a document produces no outbound connection except to the configured local endpoints. It is measured by running UAR with the network restricted to loopback (`UAR_LOCAL_ONLY` plus an OS-level loopback-only check in the test) and asserting success. A remote embedder is accepted only with a matching `egress_ack`.
- **Confirmation:** a real-service test through UAR shows that a write from a non-elicitation principal does not execute until `ipfs mcp confirm` runs, executes exactly once after it, and never executes after `deny` or expiry. The test runs with UAR governance both enabled and disabled.

---

## ASSUMPTIONS AND FALSIFIERS (new in revision 2)

> **Superseded 2026-10-09** where it conflicts with the per-goal assumptions and falsifiers now inside DECISION (revision 3). Two corrections apply in particular:
>
> - The topology-B gate counts only items contained in a **published, installable** UAR release, not merges on `main`.
> - A9.x and A10.1 refer to the revision-2 UAR-hosted primary.
>
> This section is kept as the record of the revision-2 analysis.

The delivery gate is **the end of Round 4**, the last round before any G9-G12 delivery change (N3, N5, N7) starts. The orchestrator checks it and records the result in `progress.json`. **The default on any falsified assumption is the safe one: the dependent changes are deferred and the goal is reported unmet.** A different outcome (a Kubo-side fallback, or shipping with a stated non-conformance) happens only on an explicit operator answer.

### G9

| # | Assumption | Falsifier (observable) | Consequence if falsified |
|---|---|---|---|
| A9.1 | The UAR team accepts and merges the core conformance items: agent card at `agent-card.json`, TaskState wire values, approval surfaced as input-required, an AG-UI `RunAgentInput` endpoint with spec interrupts | By the end of Round 4, those items are not merged on UAR `main` (checked by commit or PR state) | **Default:** N3's A2A/AG-UI conformance tasks and any G9 conformance claim are deferred; G9 is reported unmet. **Operator may choose:** fallback (a) (about +3 changes) or "ship as UAR profile, stated non-conformant". |
| A9.2 | An A2A-delegated write cannot bypass the human gate | N2 case (a): an A2A `message/send` from a test peer agent asks UAR to `pin_add`, and the pin is created without an `ipfs mcp confirm` (or before it) | **Block:** A2A access to write groups is refused by N8 policy, and N8 does not ship until it is fixed. A2A stays read-only. |
| A9.3 | `ipfs mcp` can serve UAR as a principal over HTTP with tools listed and invoked correctly | N2: `tools/list` through UAR differs from the stdio list, or a read tool fails | N2 is NO-GO: N3, N5, N6 and N7 are deferred. |

### G10

| # | Assumption | Falsifier | Consequence |
|---|---|---|---|
| A10.1 | The operator accepts G10's OpenAI-compatible path running through UAR (question 2) | The operator answers "no" | Switch to L2: Go loop, about +2 changes. Or switch to L3 proxy plus L2 for the raw OpenAI path. |
| A10.2 | UAR can be set up on target machines from published artifacts within an acceptable cost | N2 case (b): from a clean macOS arm64 machine and a clean Linux x86_64 machine, using only published artifacts (Docker image or native package, no source build), a working chat with one tool call takes **more than 30 minutes or more than 15 operator steps** | **Default:** G10/G11 delivery is deferred. The operator is offered L2 (no UAR) or acceptance of the measured cost. |
| A10.3 | UAR's effective model and embedder can be read, so `check-uar` can verify them | N2 case (c): the settings API does not expose the effective provider or embedder | `check-uar` falls back to `check: assert` (a recorded operator assertion). G10's egress guarantee is marked **partially met** (operator-asserted, not verified). |
| A10.4 | The shipped profile overrides every remote default | N2 case (c): with the profile applied and loopback-only networking, UAR attempts an outbound connection (model, embedder, any other) | Fix the profile. If the attempt cannot be configured away, G10/G12 are reported partially met and the leaking component is named in the docs. |

### G11

| # | Assumption | Falsifier | Consequence |
|---|---|---|---|
| A11.1 | A shadow-root island isolates Tailwind 4 from Tachyons and works in Electron 44 and current browsers | N4 visual tests: any Web UI page changes appearance with the island loaded but closed, or the island renders unstyled in Electron 44 | Switch to U3 (Desktop-only, own origin) for this phase. |
| A11.2 | The browser can reach the chat backend under a Kubo-controlled origin policy | UAR CORS stays non-configurable (A9.1-style check on the N1 CORS item at the end of Round 4) **and** the operator does not approve L3 | **Default:** N5 and N6 are deferred; G11 is unmet in this phase. |

### G12

| # | Assumption | Falsifier | Consequence |
|---|---|---|---|
| A12.1 | UAR can ingest into a named, per-node knowledge base keyed by CID | By the end of Round 4, `/api/ingest` is still hardcoded to "default", and the per-knowledge-base upload route cannot carry CID metadata (N2 checks) | **Default:** N7 is deferred, and G12 is unmet. |
| A12.2 | Local FastEmbed and surrealkv work with the profile, with no network | N2 case (c): ingest fails or connects outward under loopback-only networking | Same as A10.4. |
| A12.3 | Vector deletion can stay consistent with the pin state | The N7 test pins a CID twice (recursive root and direct), removes one pin, and the vectors are deleted | **Block** N7 until deletion checks `pin ls --type=all <cid>`. |

### N2 `uar-integration-spike`, extended GO criteria (all required for GO)

1. **Plumbing.** UAR with the shipped profile and a local provider connects to `ipfs mcp --http` as a configured principal. The `tools/list` result matches stdio. One read tool works through the AG-UI stream.
2. **(a) Writes via AG-UI and via A2A.** A gated write requested via AG-UI **and** a gated write requested via A2A (`message/send` from a test peer agent) each execute only after `ipfs mcp confirm`. Each is denied by `ipfs mcp deny`. Both are tested with UAR governance enabled **and** disabled. The UAR-side approval behavior on the A2A path (pause, hang, time out or bypass) is recorded verbatim.
3. **(b) Setup cost.** It is measured and recorded: steps and wall-clock time from a clean macOS arm64 machine and a clean Linux x86_64 machine to a working chat with one tool call, using published artifacts only. The threshold is A10.2.
4. **(c) Defaults, with and without the profile.**
   - **Without the profile:** UAR's own defaults are recorded (bind, JWT, model, memory embedder, CORS headers on a cross-origin preflight, governance state), and `check-uar` refuses that instance.
   - **With the profile under loopback-only networking:** there are zero outbound connection attempts.
   - **Settings API:** whether the effective model, embedder and governance state are readable (A10.3).
   - **Unset model:** whether UAR starts with `llm.model` unset.

Any failed criterion is NO-GO for the changes that depend on it, as listed in the tables above. The spike report is written to `mcp/uar/spike/REPORT.md` (architect-owned path, granted in Round 0).

---

## DECISION

**Decision of record, revision 3 (consolidation, 2026-10-09).** This section is self-contained. It supersedes the revision-2 DECISION ("UAR-hosted primary"), which is kept unchanged under "SUPERSEDED (2026-10-09)" near the end of this file. Its basis:

- the orchestrator adjudication of the round-2 CRITICAL, applied here: the addendum's own goal-fit gate excludes UAR-hosted (d) when undelivered UAR work is not credited, which leaves (a) at 51 against (c) at 49;
- the operator's binding answers of 2026-10-09 (`operator-answers.md`, "Answers to the G9-G12 addendum");
- the orchestrator defaults for questions 3, 5 and 8-11 recorded there.

**Shared principle (replaces the revision-2 principle).** `ipfs mcp` is the agentic surface of the node:

- It serves MCP, AG-UI and A2A from one process.
- It runs a minimal, bounded agent loop over contract tools only.
- It proxies model calls to an operator-configured OpenAI-compatible endpoint: UAR or any other server.
- It reuses one `AuthVerifier`, one scope map and one confirmation gate in-process.

The daemon, `/api/v0`, the gateway, `config/` and `docs/config.md` stay unchanged (F4). UAR is a supported LLM backend in topology A, and a second, gated topology (B) in which UAR hosts the agent endpoints.

**This reverses one revision-2 claim, and the consequence is stated, not hidden:** Kubo's fork now contains an LLM HTTP client and a minimal agent loop. Both live inside `ipfs mcp`, behind the build tag `noagent` (and `nomcp`). There is no default endpoint, and nothing is mounted unless configured.

### G9: topology A, AG-UI and A2A served by `ipfs mcp`; topology B (UAR-hosted) is gated

**Topology A (primary, delivered this phase).**

- **Listener.**
  - `ipfs mcp --http` gains AG-UI and A2A on the same listener as MCP Streamable HTTP, on distinct paths: `/mcp`, `/agui`, `/a2a`, and `/.well-known/agent-card.json`.
  - The listener's default address is **`127.0.0.1:5050`**, overridable with `--http=<multiaddr>`.
  - Port 4000 is never used as a default; it is the local liter-llm gateway port in this environment and would collide.
  - Kubo's defaults 4001, 5001 and 8080 are not used either.
  - The F4 bind rules carry over: a non-loopback bind is refused without a configured verifier.
- **AG-UI.**
  - `POST /agui` accepts `RunAgentInput` and streams spec events over SSE.
  - A pending confirmation ends the run with `RUN_FINISHED` `outcome: {type: "interrupt", interrupts: [...]}` (https://docs.ag-ui.com/concepts/interrupts, fetched 2026-10-09).
  - The resume input carries `resume: [{interruptId, status, payload}]`.
- **A2A 1.0.**
  - The agent card lives at `/.well-known/agent-card.json`, with the JSON-RPC binding at `/a2a`: `message/send`, `message/stream`, `tasks/get`, `tasks/cancel`.
  - A pending confirmation is `TASK_STATE_INPUT_REQUIRED`, and a missing or insufficient credential is `TASK_STATE_AUTH_REQUIRED`.
  - The skills are generated from the contract's A2A skill metadata.
- **In-process reuse.**
  - All three protocols authenticate through the same `AuthVerifier` and authorize through the same scope map.
  - A tool call from AG-UI or A2A goes through the same confirmation gate as an MCP call.
- **Off switches.**
  - `--agui` and `--a2a` are off by default.
  - They are mounted only when an LLM backend is configured (G10).
  - `noagent` compiles them out.
- **Protocol implementation.** The plan prefers hand-written types generated from the spec schemas over `net/http`, with no new SDK. An SDK (a2a-go v2, a community AG-UI Go SDK) is taken only if plan revision 2 measures its dependency weight and the operator accepts it; operator answer 2 covered go-sdk only.

**Topology B (second supported topology, gated).**

- **What it is.** UAR hosts AG-UI and A2A, and calls `ipfs mcp --http` as a configured principal (kind `uar`) with read-only default groups, `egress_ack`, the native confirmation prompt, and `check-uar`.
- **Stdio is forbidden.** Stdio-spawned UAR is forbidden for topology B. Over stdio, `ipfs mcp` cannot identify the client or enforce egress.
  - The shipped profile never spawns `ipfs mcp`.
  - `ipfs mcp` refuses `contentEgress` tools to a stdio client whose MCP `clientInfo` identifies UAR. This is a misconfiguration guard, not a security boundary, because `clientInfo` is self-reported.
- **Delivery gate.** B is supported only if UAR's conformance items are **contained in a published, installable UAR release (tag plus image digest or native package) by the end of Round 4**: agent card at `agent-card.json`, A2A 1.0 TaskState wire values, approval surfaced as input-required, `RunAgentInput` with spec interrupts, a configurable CORS allowlist, and safe defaults.
  - **Version pin.** The shipped `mcp/uar/profile/` pins that minimum UAR version, and `check-uar` refuses older versions via the agent card.
  - **If the gate fails:** topology B is deferred and reported unmet. There is no non-conformant B release. G9 is still delivered through topology A.

**Assumptions and falsifiers (G9).** Each row's observable is measurable; the default on falsification is the safe one.

| # | Assumption | Falsifier (observable) | Consequence |
|---|---|---|---|
| G9-1 | AG-UI and A2A can be served conformantly from `ipfs mcp` without an SDK | Plan revision 2's conformance tests (AG-UI event schema validation against the published `@ag-ui/core` schemas; A2A JSON-RPC schema validation against `a2aproject/A2A` 1.0 JSON schema) fail and cannot be fixed within the change | Take the SDK (operator approval of the root `go.mod` addition), or ship A2A only and defer AG-UI |
| G9-2 | A2A-delegated writes cannot run without a human | A test peer agent's `message/send` asks for `pin_add`, and the pin exists before a human approves the native prompt | Block: write groups are refused to every A2A principal until fixed |
| G9-3 | Topology B's UAR fixes ship in a published release by the end of Round 4 | No UAR tag or image digest containing all listed items exists at the end of Round 4 | Topology B deferred and reported unmet; A unaffected |
| G9-4 | The single `--http` listener can carry MCP, AG-UI and A2A without cross-protocol auth confusion | A token scoped for one protocol's principal is accepted on another protocol's path for a tool outside its groups (`test/cli` matrix) | Block until fixed; separate listeners if needed |

### G10: L3, `ipfs mcp` proxies model calls to an operator-configured UAR instance or OpenAI-compatible server

- **The loop.**
  - The agent loop for topology A runs in `ipfs mcp`.
  - It is minimal: a step limit, a token budget and a wall-clock limit, all configurable with conservative defaults.
  - Its tools are only the contract tools enabled for the calling principal.
  - It has no shell, no filesystem and no network tools.
- **The model endpoint.** The model is reached over the OpenAI-compatible chat-completions API (`POST <base_url>/chat/completions` with `tools`). Both UAR (`/v1/chat/completions`, `uar:src/server.rs:1901`) [sub] and other OpenAI-compatible servers provide it.
- **Configuration.**
  - `--llm-base-url`, `--llm-model` and `--llm-api-key-env=<VAR>` (or `--llm-api-key-file`), or the same keys in the `ipfs mcp` config file.
  - **There is no default URL and no default model.**
  - The key is read from the named env var or file, never accepted as a literal.
  - Unset means G9/G10 surfaces are not mounted and no model call is ever made.
- **Egress, enforced in-process.**
  - A tool result marked `contentEgress` (`ipfs_cat`, `ipfs_get`, `dag_get`, `ipfs_ls` and others) is included in a model request only if the backend's host is listed in `egress_ack`.
  - Otherwise the loop returns a structured refusal to the model and the user ("Content not sent: `<host>` is not in egress_ack.").
  - With a UAR backend, `check-uar` also verifies UAR's effective upstream provider (re-checked per session and when UAR's settings report a change) where its settings API exposes it.
- **Remaining partial guarantee (limitation).** Kubo cannot verify what an acknowledged endpoint does with data. The cases:
  - a loopback server that forwards upstream;
  - a UAR whose effective provider is not readable;
  - stdio MCP hosts (Claude Code, Codex, OpenCode) that send tool results to their own models;
  - WebMCP tools executing in the browser.

  The operator's `egress_ack` is the control, and docs state this. **This addendum does not claim "no content leaves the machine" in general.**

**Assumptions and falsifiers (G10).**

| # | Assumption | Falsifier (observable) | Consequence |
|---|---|---|---|
| G10-1 | UAR's `/v1/chat/completions` accepts client-supplied `tools` and returns `tool_calls` without executing its own tools | The integration test against a UAR built from the pinned published release shows UAR executing a tool itself, or dropping client tools | UAR as a G10 backend is unsupported until fixed. OpenAI-compatible servers remain. |
| G10-2 | At least one local OpenAI-compatible server supports tool calling well enough for the contract tools | The real-service test with one named local server (chosen in plan revision 2) fails the tool-call round trip | G10 is local-unmet; a remote backend with `egress_ack` is still supported. Reported. |
| G10-3 | No model call happens without configuration | `test/cli`: with no `--llm-*` set, `/agui` and `/a2a` return 404, and a network capture shows zero outbound connections from `ipfs mcp` | Block |
| G10-4 | `egress_ack` is enforced for every `contentEgress` result | `test/cli` with a fake model server: content from `ipfs_cat` reaches the model request body without its host in `egress_ack` | Block |
| G10-5 | The UAR backend setup is acceptable | Measured from published artifacts on clean macOS arm64 and Linux x86_64: more than 30 minutes or more than 15 steps (threshold per operator default for question 11) | The UAR backend is documented as "advanced". OpenAI-compatible local servers are the recommended path. |

### Human gate (applies to G9, G10, G11 and topology B)

- **Priority order.** For clients that can elicit, MCP elicitation stays the primary gate. For every principal that cannot elicit, the gate is a **native OS prompt**:
  - an IPFS Desktop confirmation window, when Desktop is running and paired with `ipfs mcp` over a local authenticated channel;
  - otherwise a macOS or Linux system prompt raised by `ipfs mcp`.

  Both show the tool, the CID, path or name, and the effect.
- **Human presence.** The prompt must require presence that a same-user background process cannot simulate. Candidate mechanisms, chosen in plan revision 2 and verified by the falsifier below:
  - macOS LocalAuthentication (Touch ID or password);
  - a Linux polkit authentication agent;
  - for Desktop, a window that requires a user gesture and is not reachable through the Web UI's renderer.
- **Headless only.** `ipfs mcp confirm <id>` exists only for headless servers behind an explicit opt-in (`--confirm=cli`). It checks for a TTY and prints, at startup and per request, that same-user processes can reach it.
- **Shell-capable tools.** They are refused for principals with write scope.
  - In topology A, the loop exposes no shell tools.
  - In topology B, `check-uar` refuses write groups when UAR reports `terminal_exec` enabled (`uar:src/uar/tools/terminal_exec.rs:22-27`, opt-in at `uar:src/config.rs:2523-2525`, per the round-2 finding) or any configured MCP server other than `ipfs mcp`.
  - The shipped profile pins `terminal_exec` off.
- **AG-UI chat interrupts.** An interrupt answered in the chat island counts as the human gate for the chat principal (kind `ui`), the same way elicitation does. Destructive tools additionally raise the native prompt. This default is revisited if G11-3 is falsified.
- **Expiry.** An expired or unanswered prompt is a denial ("Deny is final.").

**Assumptions and falsifiers (human gate).**

| # | Assumption | Falsifier (observable) | Consequence |
|---|---|---|---|
| HG-1 | A same-user process cannot approve a pending write | Test: an agent with shell access (topology B UAR with `terminal_exec` enabled in a test-only profile, and a plain same-user script) tries to approve via AppleScript/`System Events`, `xdotool`, the control socket or the Desktop channel, and succeeds | Block that mechanism. Fall back to the presence-factor mechanism or refuse writes for non-elicitation principals on that platform. |
| HG-2 | `check-uar` sees UAR's tool inventory | The UAR settings or tools API does not expose `terminal_exec` or the MCP server list from the pinned release | Write groups are refused to topology-B principals (fail closed) |

### G11: React 19 island in the Web UI fork, Promethity, talking only to `ipfs mcp`; Desktop fork in scope

- **The `mcp/chat` package.**
  - A shadow-root custom element (React 19, Tailwind 4, shadcn, the A2UI renderer from `mcp/ui`, an AG-UI client), with Promethity tokens and fonts bundled.
  - No remote assets, no `unsafe-eval`.
- **Mount.** It is mounted in the GQAdonis/ipfs-webui fork behind an off-by-default Settings toggle and loaded by dynamic import. Desktop gets it through the bundled Web UI (D1).
- **Backend.**
  - The island talks **only** to `ipfs mcp --http` `/agui`, **never** to UAR.
  - `ipfs mcp` enforces an origin allowlist: `webui://localhost`, `http://127.0.0.1:5001`, `http://localhost:5001`, plus configured subdomain-gateway origins.
  - The chat principal is kind `ui`, with its own bearer token.
  - **Desktop:** the token is held by the main process with `safeStorage` and attached via a preload bridge, so it is never in page storage.
  - **Browser `/webui`:** the user enters it once, and it is held per-origin. This is the stated admin-origin risk, accepted by the operator.
- **Desktop fork (tribe-health/ipfs-desktop), in scope this phase.**
  - Owned by ui-ux-engineer, reviewed by security-reviewer.
  - It adds a CSP for `webui://localhost` whose `connect-src` lists the RPC and `ipfs mcp` origins.
  - It re-enables `webSecurity` (`desktop:src/webui/index.js:41-47`).
  - It adds the `safeStorage` key bridge.
  - It adds the Desktop confirmation window and its local channel to `ipfs mcp` (human gate).

**Assumptions and falsifiers (G11).**

| # | Assumption | Falsifier (observable) | Consequence |
|---|---|---|---|
| G11-1 | The shadow root isolates Tailwind 4 from Tachyons in browsers and Electron 44 | Visual regression: any Web UI page differs with the island loaded and closed, or the island renders unstyled in Electron 44 | Desktop-only side view (U3) for this phase |
| G11-2 | Re-enabling `webSecurity` and adding the CSP does not break Desktop | Desktop smoke test (files, explore, peers, settings, add, pin) fails with the change | Fix allowlists. Never ship with `webSecurity: false` restored silently; report the regression. |
| G11-3 | Chat interrupts can only be resolved by a user gesture | Security review or test shows script in the page resolves an interrupt without a user gesture | Require the native prompt for all chat writes |
| G11-4 | The admin-origin exposure is bounded | Security review finds the island bundle loads any remote resource, or the `ipfs mcp` token is readable from Desktop page storage | Block |

### G12: requirements, mechanisms and falsifiers stated; the component choice is made by plan revision 2's re-score

**G12 is not decided beyond this section.** The adjudication found that revision 2's E1 goal-fit score has the same defect as (d). Plan revision 2 re-scores G12 under the goal-fit gate and chooses which component performs embedding and storage: `ipfs mcp`, or UAR in topology B.

**Requirements** (binding, from G12 and the operator defaults):

- off by default;
- operator-configured embedding provider, local or remote, never a hardcoded endpoint;
- a remote embedder needs `egress_ack`;
- both embedded and remote SurrealDB modes;
- chunking, model, dimension, content types and trigger are configurable;
- storage keyed by CID;
- pin-aware deletion;
- no daemon-wide hook (E5 refused).

**Remote mode, concrete.** Either:

- `ipfs mcp` connects with `github.com/surrealdb/surrealdb.go`;
  - v1.7.0, published 2026-09-07 per `proxy.golang.org/.../@latest`;
  - Apache-2.0 per its `LICENSE`;
  - its `go.mod` requires only pure-Go modules (`fxamacker/cbor/v2`, `gorilla/websocket`, `lxzan/gws`, `golang-jwt/jwt/v5`, `gofrs/uuid`) [external, fetched 2026-10-09; cgo-freeness of the full build is unverified until plan revision 2 builds it];
  - the URL and namespace/database come from config, and credentials come from env or file;
- or UAR's remote configuration (`config.remote.surreal.yaml`) in topology B.

**Embedded mode, concrete.** Either:

- **(i) a managed local `surreal` process.**
  - `ipfs mcp` starts `surreal start --bind 127.0.0.1:<ephemeral> surrealkv://<operator data dir>` with credentials generated per start, and stops it on exit. It connects with `surrealdb.go`.
  - It runs only when embedded mode is configured.
  - The `surreal` binary path is operator-configured. Kubo never downloads it or ships it (BSL 1.1 stays outside Kubo's artifacts).
- **or (ii) UAR-embedded surrealkv** (`uar:src/config.rs:1439-1440`) in topology B.

`surrealdb.c.go` (cgo plus a Rust native library) is excluded: Kubo has no cgo.

**Embedding computation.** Either:

- an operator-configured OpenAI-compatible `/embeddings` endpoint called by `ipfs mcp` (a local server, or UAR's endpoint if it exposes one [unverified]);
- or UAR's FastEmbed in topology B.

There is no default.

**Trigger.**

- On a successful `ipfs_add` from any `ipfs mcp` transport (stdio, HTTP, AG-UI chat, A2A), when indexing is configured.
- Subject to the MIME allowlist and a size cap.
- Plus an explicit "index this CID" tool.
- **Limitation:** WebMCP tools in the browser call the RPC directly, not through `ipfs mcp`, so content they add is not indexed unless re-indexed explicitly.

**Deletion.**

- Delete a CID's vectors only when `pin ls --type=all <cid>` reports no direct, recursive or indirect pin.
- On `repo_gc`, only for CIDs that GC reports as removed.
- Operator-triggered reconciliation for content unpinned outside `ipfs mcp`.

**Acceptance tests per mode** (plan revision 2 turns them into tasks):

| Mode | Test |
|---|---|
| Disabled (default) | No `surreal` process spawned. No connection to any store or embedder (network capture). Index tools absent. |
| Embedded (i) | Start and stop lifecycle, including crash cleanup. Loopback-only bind. Data under the configured directory. Ingest, then search, then pin-aware delete against a harness daemon. Zero outbound connections with a local embedder. |
| Embedded (ii) / remote via UAR (topology B only) | The same flow through the pinned UAR release, gated like topology B. |
| Remote (surrealdb.go) | ws/wss URL from config, credentials from env, the same flow against a real `surreal` server started by the test. Refusal when the embedder host is not in `egress_ack`. |

**Assumptions and falsifiers (G12).**

| # | Assumption | Falsifier (observable) | Consequence |
|---|---|---|---|
| G12-1 | `surrealdb.go` builds into `ipfs` without cgo and at an acceptable size | `CGO_ENABLED=0 make build` fails with it, or the measured binary delta exceeds the limit plan revision 2 sets | Remote and embedded (i) via `ipfs mcp` are unsupported; topology B only, or G12 deferred |
| G12-2 | SurrealDB vector search (HNSW) works for the configured dimension via `surrealdb.go` | A real-service test cannot define or query an HNSW index from Go | In-memory fallback is not accepted; G12 deferred |
| G12-3 | The managed process can be supervised safely | The test leaves an orphaned `surreal` process after `ipfs mcp` is killed, or it binds non-loopback | Embedded (i) unsupported until fixed |
| G12-4 | Deletion stays consistent with the pin state | A CID pinned twice (recursive root and direct) loses its vectors when one pin is removed | Block |

---

## ADR DRAFTS

The plan stage materializes these in `docs/mcp/`, after ADR-0001 to ADR-0003. The revision-2 drafts are kept under "SUPERSEDED (2026-10-09)".

### ADR-0004 draft: AG-UI and A2A are served by `ipfs mcp`, and model calls are proxied to an operator-configured endpoint

- **Status:** Proposed, decision of record per operator answers (2026-10-09). Accepted once plan revision 2 is approved.
- **Context.**
  - G9 and G10.
  - F4 keeps Kubo's agent surface in `ipfs mcp` (RPC-only, no daemon change).
  - AGENTS.md: no default endpoints, an off switch for every feature, no tracking.
  - UAR (MIT) is non-conformant to A2A 1.0/AG-UI today, and its approval can be bypassed when governance is disabled (Summary 2, 4).
  - The goal-fit gate excludes UAR-hosted endpoints while UAR's fixes are undelivered (adjudication).
  - The operator chose "Both".
- **Decision.**
  - **Topology A.** `ipfs mcp --http` (default `127.0.0.1:5050`) serves AG-UI (`/agui`, spec interrupts) and A2A 1.0 (`/.well-known/agent-card.json`, `/a2a`, input-required and auth-required). Both reuse the `AuthVerifier`, the scope map and the confirmation gate in-process.
  - **The loop and the model.** A bounded loop over contract tools calls an operator-configured OpenAI-compatible endpoint (UAR or another server). There is no default URL or model, and the key comes from env or file.
  - **Egress.** `contentEgress` results reach a model only for hosts in `egress_ack`.
  - **Human gate.** For non-eliciting clients, a native OS prompt with a presence factor. CLI confirm is headless opt-in only. Shell-capable tools are refused for write principals.
  - **Topology B.** UAR-hosted, over `--http` only, with stdio forbidden. Supported once UAR's fixes ship in a published release by the end of Round 4; otherwise deferred and reported unmet.
  - **Off switches.** `--agui`/`--a2a` are off by default and mounted only with a configured backend; `noagent` compiles them out.
- **Consequences.**
  - **Positive:** conformance is under this team's control; one gate and one auth path; works with any OpenAI-compatible server, local or remote; UAR is optional.
  - **Negative:**
    - The fork now carries an LLM client and a minimal agent loop (maintenance in this repo).
    - Possible new root `go.mod` modules if an SDK is taken (G9-1).
    - The egress guarantee is limited to what `egress_ack` and `check-uar` can see.
    - Native prompts need per-OS implementations.
- **Rejected or deferred alternatives:**

  | Alternative | Rejected or deferred because |
  |---|---|
  | (d) UAR-hosted as primary | Excluded by the goal-fit gate while UAR's fixes are undelivered. Kept as topology B. |
  | (b) daemon listener | C1 plus an LLM client in the daemon |
  | (c) separate binary | A second product in the repo |
  | L1 UAR-only backend | Fails G10's "or an OpenAI-compatible server" |
  | Token fallback or a 0600 socket as the human gate | Reachable by same-user processes (round-2 finding) |
  | Stdio-spawned UAR | Egress unenforceable |

### ADR-0005 draft: chat UI as a React 19 island in the Web UI fork, talking only to `ipfs mcp`; Desktop hardened

- **Status:** Proposed, decision of record per operator answers (2026-10-09).
- **Context.**
  - G11.
  - The Web UI is React 16 / CRA 5 / Tachyons with React-capped dependencies (Summary 6).
  - The Web UI origin holds admin RPC.
  - Desktop has no CSP and runs `webSecurity: false`.
  - The operator chose the Web UI island, Promethity, and the Desktop fork in scope.
- **Decision.**
  - `mcp/chat` is a shadow-root React 19 element (Tailwind 4, shadcn, A2UI, an AG-UI client, Promethity), mounted off by default in the Web UI fork and reaching Desktop through D1.
  - It talks only to `ipfs mcp` `/agui`, behind an origin allowlist and a `ui` principal token.
  - In the Desktop fork: CSP, `webSecurity` re-enabled, a `safeStorage` token bridge, and the native confirmation window.
  - Owner: ui-ux-engineer. Reviewer: security-reviewer.
- **Consequences.**
  - **Positive:** the fork stays near upstream; one component set; brand control; no browser credential to UAR.
  - **Negative:**
    - The chat runs in the admin origin (operator-accepted risk, bounded by G11-3/G11-4).
    - Two React copies when enabled.
    - Desktop fork divergence (CSP, `webSecurity`).
- **Rejected alternatives:**

  | Alternative | Rejected because |
  |---|---|
  | U1 upgrade the fork | Rewrite-sized |
  | U3 Desktop-only | Leaves browser users out. It is the flip if G11-1 fails. |
  | U4 iframe UAR | Not IPFS-specific |
  | Direct-to-UAR from the browser | UAR CORS is not configurable; the operator chose "never directly to UAR" |

### ADR-0006 draft: G12 requirements and mechanisms; component choice deferred to plan revision 2

- **Status:** Proposed, requirements only. The component choice (`ipfs mcp` or UAR topology B) is recorded by plan revision 2's re-score under the goal-fit gate. This ADR does not claim G12 decided.
- **Context.**
  - G12 requires both embedded and remote SurrealDB, an operator-configured embedder, off by default.
  - Embedded SurrealDB from Go via `surrealdb.c.go` needs cgo; Kubo has none.
  - `surrealdb.go` v1.7.0 is an Apache-2.0 network client.
  - The SurrealDB engine is BSL 1.1.
  - Kubo has no pin-removal event stream.
- **Decision.**
  - **Remote mode:** `surrealdb.go` from `ipfs mcp`, or UAR remote config (topology B).
  - **Embedded mode:** a managed local `surreal` process (surrealkv, loopback, operator-provided binary, started and stopped by `ipfs mcp` only when configured), or UAR-embedded surrealkv (topology B).
  - **Embedding:** an operator-configured OpenAI-compatible endpoint or UAR FastEmbed. There is no default, and a remote embedder needs `egress_ack`.
  - **Trigger:** `ipfs_add` through `ipfs mcp` or an explicit index tool.
  - **Deletion:** pin-aware.
  - **Refused:** a daemon-wide hook. Excluded: cgo.
- **Consequences.**
  - **Positive:** no cgo and no BSL code in Kubo artifacts; both modes have a concrete mechanism and per-mode acceptance tests.
  - **Negative:**
    - Kubo (`ipfs mcp`) may supervise a third-party process (embedded (i)).
    - WebMCP-added content is not indexed automatically.
    - Content unpinned outside `ipfs mcp` stays indexed until reconciliation.
    - Egress for a remote embedder is limited to the `egress_ack` control.

---

## IMPACT ON F4 AND THE CURRENT PLAN

This section replaces the revision-2 plan impact (kept under "SUPERSEDED (2026-10-09)").

### F4 itself

- **Placement unchanged.** `ipfs mcp` stays a local, RPC-only command.
- **Additions inside `ipfs mcp`:**
  - the `--http` default address `127.0.0.1:5050`;
  - AG-UI and A2A routes;
  - a bounded agent loop and an OpenAI-compatible client;
  - principal policy, `egress_ack`, the native confirmation prompt and `check-uar`;
  - optional G12 indexing with a managed `surreal` process or a remote client.
- **Build tag.** `noagent` removes everything except MCP.
- **Still unchanged:** the daemon, `config/`, `docs/config.md`, `/api/v0` and the gateway.
- **Possible new root `go.mod` modules:** `surrealdb.go` (G12) and a protocol SDK only if G9-1 requires one. Operator answer 2 covered go-sdk only, so plan revision 2 must get explicit approval for each.

### Changes to existing plan items (22)

| # | Change | Impact |
|---|---|---|
| 0 | team-ownership-grant | Add grants:<ul><li>ui-ux-engineer: `mcp/chat/**`, Web UI `src/agent-chat/**`, and the Desktop fork (`tribe-health/ipfs-desktop`: `src/webui/**`, the confirmation window, CSP)</li><li>architect: `mcp/uar/**`</li><li>mcp-server-engineer: `mcp/server/**` (already granted) and `test/cli/mcp_agent_*_test.go`</li></ul>The batched question now only confirms the go.mod additions and the remaining defaults. |
| 1 | mcp-tool-contract | Add `contentEgress`, A2A skill metadata, A2UI binding hints, principal kinds (`human-mcp`, `ui`, `a2a`, `uar`) and scopes, and the `egress_ack` and LLM-backend config schemas. |
| 2 | mcp-architecture-adrs | Add ADR-0004 and ADR-0005 (decision of record) and ADR-0006 (requirements; status completed after plan revision 2's re-score). |
| 6 | mcp-pinning-with-confirmation | The gate state machine supports external resolution (native prompt, AG-UI interrupt, A2A input-required). The token fallback is not accepted from non-eliciting configured principals. |
| 7 | mcp-http-transport-auth | Default `127.0.0.1:5050`; configured principals with kinds; path-scoped auth for `/mcp`, `/agui` and `/a2a` (G9-4). |
| 5 | mcp-ui-component-library | Chat primitives (message stream, tool-call card, interrupt/approval card, egress badge), shadow-DOM-safe. |
| 16 | webui-webmcp-integration | Sequence it before N5 (shared Settings). |
| 20 | mcp-user-docs | Agent endpoints, LLM backend config (no defaults), `egress_ack`, the native prompt, topology B, G12 modes, and the limitations. |
| 21 | phase-boundary-review | All falsifier tests from DECISION. Security review of the loop, the proxy, the prompts, the Desktop changes and the chat bundle. |
| others | 3, 4, 8-15, 17-19 | No change. |

### New changes (12, three conditional)

| New change | Owner | Depends on | Round | Notes |
|---|---|---|---|---|
| N1 `uar-conformance-request` | architect (`team-request`; no UAR edits) | team-ownership-grant | 1 | For topology B and the UAR backend (G10-1). Delivery is evaluated against **published releases** at the end of Round 4. |
| N4 `agent-chat-island` | ui-ux-engineer | mcp-ui-component-library | 3 | Island and AG-UI client, Promethity. G11-1 and G11-4 tests. |
| N8 `mcp-agent-principal-enforcement` | mcp-server-engineer | mcp-http-transport-auth, mcp-pinning-with-confirmation | 4 (merges after http-transport-auth) | Principal kinds, `egress_ack`, `check-uar` (per session and on settings change), shell-tool refusal for write principals, the stdio `clientInfo` guard. |
| N14 `native-confirmation-prompt` | mcp-server-engineer | mcp-pinning-with-confirmation | 4 | macOS and Linux presence-factor prompts, the Desktop pairing channel protocol, `--confirm=cli` headless opt-in. HG-1 tests. |
| N11 `mcp-llm-proxy-loop` | mcp-server-engineer | N8 | 5 | G10 L3: OpenAI-compatible client and bounded loop, no defaults, key from env or file. Tests G10-1 to G10-4 against UAR (pinned release) and one local server. |
| N12 `mcp-agui-endpoint` | mcp-server-engineer | N11, N14 | 6 | `/agui` with spec events and interrupts. G9-1 and G9-4. |
| N13 `mcp-a2a-endpoint` | mcp-server-engineer | N11, N14 | 6 | A2A 1.0 agent card and JSON-RPC, input-required and auth-required. G9-1, G9-2 and G9-4. |
| N6 `desktop-agent-chat` | ui-ux-engineer (security-reviewer reviews) | N4, N14 | 6 | Desktop fork: CSP, `webSecurity` re-enabled, `safeStorage` bridge, the confirmation window. G11-2. |
| N7 `mcp-embeddings-surrealdb` | per plan revision 2's re-score (mcp-server-engineer if `ipfs mcp`) | N8, N11 | 6 | G12 modes and the per-mode acceptance tests in DECISION, the trigger (it absorbs the former N9 hook), pin-aware deletion, reconciliation. G12-1 to G12-4. |
| N5 `webui-agent-chat` | ui-ux-engineer | N4, N12, webui-webmcp-integration (sequencing) | 7 | Mount, toggle, i18n, a real run against `ipfs mcp`. G11-3. |
| N2 `uar-integration-spike` (CONDITIONAL: the topology-B gate passes) | mcp-server-engineer, plus the operator for the setup measurement | gate at the end of Round 4, N8, N14 | 5 | Topology B with the pinned published UAR over `--http`: the A2A-delegated write (G9-2 analogue), the setup measurement (G10-5), defaults with and without the profile, `terminal_exec` refusal (HG-2). |
| N3 `agentic-uar-integration` (CONDITIONAL: N2 GO) | architect + mcp-server-engineer | N2 | 6 | Shipped `mcp/uar/profile/` with the pinned minimum version, and the topology-B e2e tests. |

**Removed or superseded:** revision-2 N9 (`mcp-embedding-hook`, folded into N7) and N10 (`mcp-agui-proxy`, replaced by N12).

### Count and rounds

- **Count:** 22 existing + 12 new = **34 changes**. Three are conditional: `webui-cid-delivery`, N2 and N3.
- **Rounds:**

  | Round | Additions |
  |---|---|
  | 0 | Grant |
  | 1 | N1 |
  | 3 | N4 |
  | 4 | N8, N14 (Round 4 now has 9 parallel changes; the `plan.md` T1 merge risk rises; merge order is http-transport-auth, then N8, then N14, then the tool groups) |
  | End of Round 4 | **Topology-B gate** (published UAR release) |
  | 5 | N11, N2 (conditional) |
  | 6 | N12, N13, N6, N7, N3 (conditional) |
  | 7 | N5 (alongside marketplaces and user-docs; user-docs now also depends on N11-N13 and N7) |
  | 8 | Review |

- **Critical path.** pinning (R3), then http-transport-auth and N8 (R4), then N11 (R5), then N12 (R6), then N5 (R7), then review (R8). That fits the existing 0-8 round structure, so the phase does not get longer.
- **Failure isolation.** A failed topology-B gate removes only N2 and N3. Failed G9-G12 falsifiers block or defer only their own changes. G1-G8 are unaffected.

## RISKS AND OPEN QUESTIONS

### Top risks (including risks against this addendum's own decisions)

> **Note (2026-10-09).** Risks 1, 2 and 10 were written against the revision-2 UAR-hosted primary. Under the revision-3 decision:
>
> - The main risks are the maintenance of an LLM client and agent loop in the fork, the per-OS native-prompt implementations, and possible new root `go.mod` modules.
> - UAR delivery affects only topology B and the UAR backend.
> - Risks 3-9 stand as written for topology B, with the egress limitation now restated in DECISION G10.

1. **Dependency on another team.** Conformance, CORS, elicitation, safe defaults and ANN search all need UAR changes. The default on non-delivery is now deferral, which means **G9-G12 may deliver nothing in this phase**. That is the honest cost of the safe default.
2. **Setup cost.** It is measured in N2 against an explicit threshold (A10.2). If it fails, the principle itself is in question, and L2 becomes the likely answer.
3. **UAR's unsafe and non-configurable defaults.**
   - `0.0.0.0`, the remote model, the remote memory embedder, CORS `Any` (not configurable) and the governance bypass in local mode.
   - N8 enforces the node-data boundary only. A UAR instance an operator runs raw is still unsafe for other reasons, outside Kubo's control.
4. **Out-of-band confirmation UX.**
   - One extra human step per write, on a second surface (CLI or the loopback page).
   - This is safer than relying on UAR, but it may still drive rubber-stamping.
   - The control socket and the confirmation page are new attack surface in `ipfs mcp`: CSRF on the page, socket permissions.
5. **Egress over stdio is not enforceable.** A UAR or any host spawning `ipfs mcp` over stdio sends whatever content the agent reads to the host's model. This is documented as outside the guarantee, which is the same position as every other stdio MCP host today.
6. **`check-uar` couples Kubo to UAR's settings API shape.** A UAR release that changes it makes the principal fail closed, which is safe but disruptive. A10.3 records whether verification is possible at all.
7. **Chat in the admin origin.** Through the L3 proxy, the key is no longer in the browser, but an XSS in the island is still admin RPC.
8. **Embeddings drift and coverage.** Index drift from out-of-agent unpins, and non-UAR surfaces are not indexed without N9.
9. **SurrealDB BSL 1.1** is inherited by UAR deployers.
10. **The weighted margins for G9 and G11 are not decisive.** The decisions rest on qualitative arguments (maintenance location, reach) that a reasonable operator could weigh differently.

### Questions only the operator can answer

> **Answered 2026-10-09.**
>
> - Q1, Q2, Q4, Q6 and Q7 by the operator (binding).
> - Q3, Q5 and Q8-Q11 by orchestrator defaults the operator may overrule.
>
> See `operator-answers.md` and the appended "Answers to the G9-G12 addendum". The questions are kept as asked. New open items for plan revision 2:
>
> - approval of any new root `go.mod` modules (`surrealdb.go`, any protocol SDK);
> - the G12 component choice from the re-score;
> - the local OpenAI-compatible server used for G10-2.

1. **UAR as a required component.** Do you accept UAR as the runtime for G9-G12, with Kubo adding the N8 enforcement but no LLM client, vector store or chat backend? A "no" moves to (a)+L2 for G9/G10 and to E3 or deferral for G12.
2. **G10 narrowing.** Should "OpenAI-compatible server plus key" run through UAR (L1, narrows G10), through a Go loop in `ipfs mcp` (L2, about +2 changes), or behind an `ipfs mcp` proxy (L3, +1 change, also the G11 browser boundary)?
3. **Delivery gate.** The default at the end of Round 4, if UAR has not delivered the conformance items, is to defer G9 conformance and report it unmet. Do you instead want, in advance, the Kubo fallback (a) or an explicitly non-conformant release?
4. **Out-of-band confirmation.** Is a local CLI (`ipfs mcp confirm`) plus a loopback confirmation page acceptable as the human gate for agents without elicitation, including UAR? That reverses the plan's cut of this channel for these principals.
5. **Egress.** Is per-principal `egress_ack` (naming the model and embedder hosts) the right control, and should content tools stay refused for every non-elicitation principal without it?
6. **Chat placement and brand.** Web UI island (reaching Desktop) or Desktop-only (U3)? Promethity or IPFS-family branding inside the Web UI?
7. **Desktop fork.** Is `tribe-health/ipfs-desktop` in scope for this phase (CSP, key bridge)? Which role owns it?
8. **Embedding scope.** Embed only UAR-routed agent actions, or also enable the opt-in N9 hook so content added from stdio hosts, the MCP App and WebMCP is indexed? And is the pin-aware deletion plus operator-triggered reconciliation acceptable?
9. **SurrealDB license.** Is SurrealDB's BSL 1.1 acceptable for your deployments, through UAR?
10. **Remote modes.** Are a remote SurrealDB or a remote embedder needed this phase, or is local-only enough?
11. **Setup threshold.** Is the setup-cost threshold in A10.2 (30 minutes or 15 steps from published artifacts) the right falsifier, or do you want a different number?

---

## APPLIED LESSONS

- **"For every external integration, run at least one test against the real service before claiming done."**
  - N2 now tests the addendum's own riskiest assumptions against real UAR: the A2A write gate, setup cost, and default behavior with and without the profile. It no longer tests only the plumbing.
  - The UAR facts here come from its source, and the source contradicted its docs three times:
    - the port 3928 in `A2A_PROTOCOL.md`;
    - "postgres" as the default in `configuration.md`;
    - "FastEmbed 4" in a comment against `version = "5"`.
  - Reading the source also found the governance bypass, which the docs do not mention.
- **"Never call a CLI that takes a lock from inside an operation that already holds that lock, and never paper over such a failure with a default value."**
  - A `check-uar` failure disables the principal, never "assumes safe".
  - An unreadable settings API needs an explicit operator assertion, never a default pass.
  - Refused egress returns an error, not an empty result.
- **"Tests must discover their environment rather than encode one checkout's state."**
  - UAR e2e tests use a temporary data dir and non-default ports, and harness daemons with isolated `IPFS_PATH`.
  - The loopback-only network check is part of the test, not an assumption about the machine.
- **"The operator approves merges, tags and machine-level changes explicitly; a single up-front approval question at the phase boundary keeps the run unblocked."**
  - The eleven questions batch into Round 0.
  - The setup-cost measurement (N2 criterion 3) and the Desktop fork are explicit operator tasks.
  - Cross-repo issue filing (N1) is operator-visible.

## KNOWLEDGE GAPS

- **A2A 1.0:** the release date, the exact method names and the wire enum casing [partly LLM-extracted, unverified].
- **AG-UI interrupt status:** stable or draft [unverified].
- **UAR settings API fields:** whether the effective model, embedder and governance state are readable, and with which role. This decides A10.3; N2 checks it.
- **The production meaning of `uar:src/uar/api/a2a/task_execution.rs:568`** (`tool_approval: Auto` for A2A tasks), and UAR's approval behavior on the A2A path. N2 criterion 2 checks it.
- **Whether UAR starts with `llm.model` unset** (N2 criterion 4).
- **UAR SurrealDB feature unification** (`mem://`, `rocksdb://`, `http`) and its document extractors [unverified].
- **UAR prebuilt artifacts per platform** [unverified]; N2 measures them.
- **Whether UAR's A2UI surfaces render the `mcp/ui` A2UI v0.9 catalog** [unverified].
- **`surrealdb.c.go` maturity, platforms and license** [unverified].
- **Tailwind 4 shadow-root isolation under Electron 44, and the size of two React copies** [unmeasured]. N4 checks A11.1.
- **The Electron 44 `webui://` scheme and credentialed `fetch` to a loopback `ipfs mcp` origin under the proposed CSP** [unverified].

## SUPERSEDED (2026-10-09): revision-2 DECISION, ADR drafts and plan impact (UAR-hosted primary)

**Superseded 2026-10-09** by the revision-3 DECISION, ADR DRAFTS and IMPACT sections above. The basis is the orchestrator adjudication of the round-2 CRITICAL (the goal-fit gate excludes UAR-hosted (d) while UAR fixes are undelivered) and the operator answers of 2026-10-09. The text is kept verbatim (headings demoted one level) as the record of the analysis. It is not the decision of record. Where it conflicts with the sections above, the sections above win. In particular, these points no longer hold:

- UAR as the only agent loop;
- UAR-hosted AG-UI/A2A as primary;
- the CLI/0600-socket confirmation as the human gate;
- the "nothing leaves the machine" style claims;
- G12 E1 as decided;
- the change count 22 + 9 = 31.

### DECISION

**Shared principle:** UAR owns the agent loop, the LLM client, AG-UI/A2A, embeddings and SurrealDB. Kubo owns the MCP tools, the contract, documentation, real-service tests, **and the enforcement at the node-data boundary (N8)**. Kubo grows no LLM client, no vector store and no chat backend. This principle is conditional on the assumptions above. Every falsifier defers by default rather than shipping a weaker result.

#### G9: (d) UAR-hosted AG-UI and A2A, with `ipfs mcp` as an enforced tool provider (e2: (a) is the designed fallback, not built)

1. **Connection.** UAR consumes `ipfs mcp --http` as an explicitly configured principal (N8 items 1-4). The stdio-spawned UAR setup is documented as outside the egress and confirmation guarantees.
2. **No new endpoint.** Kubo adds no AG-UI or A2A endpoint of its own, unless the operator chooses L3 for G10/G11. No daemon listener, no `Addresses.*` key and no `docs/config.md` key. F4 is unchanged.
3. **Contract additions** (`mcp/contract`, architect-owned):
   - `contentEgress` per tool;
   - A2A skill metadata;
   - A2UI binding hints;
   - the principal scope list.
4. **Confirmation.**
   - Through UAR, the human gate for node writes is the Kubo-enforced out-of-band confirmation (N8 item 3), until UAR's MCP client supports elicitation.
   - **Whether UAR's own approval holds on the A2A path is unknown until N2 proves it.** This addendum makes no claim that it holds today: Summary 4 shows a bypass condition.
   - Until N2 passes criterion 2, the UAR principal is read-only.
5. **UAR conformance work.** It is requested from the UAR team (N1) and gated by A9.1 at the end of Round 4. **Default on non-delivery: defer the G9 conformance work and report G9 unmet.** It does not automatically ship as non-conformant.

**Flip conditions:**

| If... | Then |
|---|---|
| A9.1 falsified and the operator chooses the fallback | **(a)** in `ipfs mcp --http`, behind a `noagent` tag, with G10 L2 |
| The operator rejects UAR (question 1) | (a) with L2 |
| "The daemon must host it" | Refuse in this phase (same grounds as `assessment.md` C1/F1); offer (a) |

#### G10: PENDING operator question 2

- **Recommendation: L1, UAR as the only agent loop, with N8 enforcement.**
  - It **narrows** G10. The goal's "operator-configured OpenAI-compatible server with API key" becomes a UAR provider configuration, not a path without UAR.
  - If the operator does not accept that narrowing, the non-UAR path is **unmet** under L1. The choices are then L2 (Go loop) or L3 + L2.
- **L3 (thin proxy) is a real alternative, not a rejected one.**
  - **What it costs:** about one change in `ipfs mcp` and an extra hop.
  - **What it buys:** a Kubo-controlled browser boundary, which UAR cannot offer today because its CORS is not configurable.
  - **Recommended pairing:** L3 is the recommended path for G11's browser chat if the operator wants G11 in this phase without waiting for the UAR CORS fix.
- **Kubo-side deliverables in all cases:**
  - the shipped UAR profile (N8 item 5, with a local provider placeholder and no default provider);
  - `check-uar`;
  - the egress acknowledgement;
  - docs that state what leaves the machine.

#### G11: U2, a React 19 chat island in the Web UI fork, reaching Desktop through the bundled Web UI; browser-facing path through `ipfs mcp` (L3) unless UAR CORS is fixed

1. **The `mcp/chat` package.**
   - It reuses `mcp/ui` and the A2UI catalog.
   - It is a shadow-root custom element with an AG-UI client and no remote assets.
   - Brand tokens come from the bundled Promethity files, pending question 6.
2. **The Web UI fork mount.**
   - It is off by default, behind a Settings toggle, loaded by dynamic import.
   - It has its own `agentChat` i18n namespace (`webui:src/i18n.js:41`) [sub].
   - Fork changes are limited to the toggle and the launcher.
3. **Backend endpoint.**
   - **Default:** the `ipfs mcp --http` L3 proxy, with an origin allowlist and the UAR key held server-side.
   - **Direct-to-UAR:** allowed only if UAR ships a configurable CORS allowlist (N1 item), and only then.
   - **Neither available at the gate:** N5/N6 are deferred (A11.2).
4. **Desktop.**
   - It gets the island through D1.
   - Desktop-fork changes (CSP for `webui://localhost`, and `safeStorage` only if direct-to-UAR is used) are operator-gated (question 7).
5. **Confirmations.** They are rendered with the shared dialog. A denied action does not run.

**Flip conditions:**

| If... | Then |
|---|---|
| The operator wants the chat outside the admin origin, or A11.1 is falsified | **U3** |
| The operator rejects L3 and UAR CORS is not fixed | Defer G11 (A11.2) |

#### G12: E1, UAR owns embeddings and SurrealDB, under Kubo-enforced egress control

- **Trigger and storage.**
  - Embedding is triggered only by agent actions through UAR: content an agent added with `ipfs_add`, or explicitly asked to index.
  - It is subject to an operator MIME allowlist and a size cap.
  - Chunks are stored in a dedicated per-node knowledge base with `cid`, `path`, `mime`, `chunk_index` and `model`.
  - The shipped profile sets a local FastEmbed embedder and an embedded surrealkv store. A remote embedder or remote SurrealDB needs a matching `egress_ack`, enforced by `check-uar`.
- **Deletion.**
  - On agent `pin_rm`, vectors are deleted only if `pin ls --type=all <cid>` reports no remaining direct, recursive or indirect pin.
  - On agent `repo_gc`, deletion is limited to CIDs the GC output reports as removed.
  - An operator-triggered reconciliation compares indexed CIDs against `pin ls --type=all` and deletes the rest.
- **Limitation (explicit scope narrowing).** Content added through other agent surfaces is **not embedded** unless it passes through UAR. That covers stdio MCP hosts (Claude Code, Codex, OpenCode), the MCP App, and WebMCP in the Web UI (G8).
  - **Option to close it:** N9 `mcp-embedding-hook` (CONDITIONAL, question 8). An opt-in `ipfs mcp` setting that, after a successful `ipfs_add` from any principal or transport, sends the added bytes to the configured UAR knowledge-base ingest endpoint.
  - It applies the same MIME allowlist and requires an `egress_ack` naming that endpoint.
  - It is off by default, with no default URL. Its failures are visible tool warnings, and the add itself still succeeds.
- **Content unpinned outside the agent.** Content unpinned through the CLI or the Web UI stays indexed until reconciliation. This is documented. Immediate deletion would need a Kubo pin-removal event source, which is a separate Kubo change and out of this phase.
- **Refused:** any daemon-wide embedding hook (E5).

**Flip conditions:**

| If... | Then |
|---|---|
| The operator rejects UAR | E3, only with explicit approval of process management and BSL. Or defer. Never E2. |
| ANN search quality is required | A UAR change. No Kubo change. |

---

### ADR DRAFTS

The plan stage materializes these in `docs/mcp/`, after ADR-0001 to ADR-0003.

#### ADR-0004 draft: agentic endpoints (AG-UI, A2A) and the LLM backend are hosted by UAR, behind Kubo-enforced principal controls

- **Status:** Proposed. The G10 part is pending operator question 2. Acceptance also requires N2 GO.
- **Context.**
  - G9 and G10. F4 keeps Kubo's MCP surface in `ipfs mcp`, RPC-only.
  - AGENTS.md forbids default endpoints and requires off switches.
  - UAR (MIT) has a loop, an LLM client, an MCP client, AG-UI and A2A.
  - It deviates from A2A 1.0 and AG-UI (Summary 2).
  - Its defaults include a remote model, a remote memory embedder, a `0.0.0.0` bind and non-configurable CORS `Any` (Summary 3).
  - Its approval pause can be bypassed when governance is disabled in loopback mode (Summary 4).
- **Decision.**
  - UAR serves AG-UI and A2A and is the agent loop (L1 recommended; L3 proxy is the browser-facing option).
  - `ipfs mcp --http` serves UAR only as an explicitly configured principal:
    - read-only by default;
    - content tools only with an `egress_ack`;
    - writes confirmed out-of-band;
    - a `check-uar` preflight that refuses unsafe instances.
  - Kubo ships a tested UAR profile.
  - UAR conformance fixes are requested, and gated at the end of Round 4. Non-delivery defers by default.
- **Consequences.**
  - **Positive:** no LLM client, protocol SDK or agent runtime in Kubo; the node-data guarantees are enforced in Kubo code.
  - **Negative:**
    - A second runtime to install.
    - Conformance depends on another team.
    - Out-of-band confirmation adds a step per write.
    - The stdio-spawned UAR path is outside the egress guarantee.
    - `check-uar` depends on UAR's settings API shape.
- **Rejected alternatives:**

  | Alternative | Rejected because |
  |---|---|
  | (a) in `ipfs mcp` | A Go agent runtime and two protocol SDKs in Kubo. Kept as the designed fallback; the margin over it is not decisive. |
  | (b) daemon listener | C1 plus an LLM client in the daemon |
  | (c) separate binary | A second product in the Kubo repo, still with a Go loop |
  | Trusting UAR's approval as the human gate | Bypassable (Summary 4) |
  | A "host-delegated confirmation" scope | Lets one bearer token skip the gate |

#### ADR-0005 draft: chat UI as a React 19 island in the Web UI fork, behind a Kubo-controlled endpoint

- **Status:** Proposed. Depends on N2 GO and on A11.1/A11.2.
- **Context.**
  - G11.
  - The Web UI is React 16 / CRA 5 / Tachyons, with React-capped dependencies.
  - `mcp/ui` targets React 19.
  - The Web UI origin holds admin RPC.
  - Desktop has no CSP and runs `webSecurity: false`.
  - UAR's CORS is `Any` and not configurable.
- **Decision.**
  - A shadow-root React 19 island (`mcp/chat`), mounted off by default in the Web UI fork and reaching Desktop through D1.
  - The browser talks to `ipfs mcp --http` (L3 proxy, origin allowlist, server-held UAR key), unless UAR ships a CORS allowlist.
- **Consequences.**
  - **Positive:** the fork stays near upstream; one component set; brand control.
  - **Negative:**
    - Two React copies when enabled.
    - The chat runs in the admin origin.
    - L3 adds an endpoint to `ipfs mcp`.
    - The margin over Desktop-only (U3) is not decisive and flips if security is weighted higher.
- **Rejected alternatives:**

  | Alternative | Rejected because |
  |---|---|
  | U1 upgrade the fork | Rewrite-sized; permanent rebase cost |
  | U3 Desktop-only | Leaves browser users out. It is the flip. |
  | U4 iframe the UAR UI | Not IPFS-specific; cross-origin framing |
  | Direct-to-UAR from the browser | Not enforceable while UAR CORS is `Any` |

#### ADR-0006 draft: document embeddings live in UAR, triggered only by agent actions, with Kubo-enforced egress control

- **Status:** Proposed. Depends on N2 GO and on A12.1.
- **Context.**
  - G12.
  - Embedded SurrealDB from Go needs cgo plus a native library. Kubo has no cgo. SurrealDB is BSL 1.1.
  - UAR has local FastEmbed, surrealkv/remote SurrealDB and knowledge-base APIs, but defaults its memory embedder to OpenAI and has no ANN index.
  - Kubo has no unpin or GC event stream.
  - User Agency forbids daemon-wide processing of node content.
- **Decision.**
  - UAR embeds and stores, triggered only by agent actions through UAR (plus the optional N9 hook), under a MIME allowlist.
  - Chunks are keyed by CID.
  - The shipped profile sets a local embedder and storage. Remote embedder or storage needs an `egress_ack` that `check-uar` enforces.
  - Deletion follows the pin state, checked with `pin ls --type=all`.
  - A daemon-wide hook is refused.
- **Consequences.**
  - **Positive:** no cgo, no BSL code and no vector store in Kubo.
  - **Data egress:** with the shipped profile and its tests, ingestion makes no outbound connection. A UAR started without the profile is refused by `check-uar` for node content. A UAR started without the profile and connected over stdio is outside the guarantee (documented).
  - **Negative:**
    - Content from non-UAR surfaces is not indexed unless N9 is enabled.
    - Content unpinned outside the agent stays indexed until reconciliation.
    - Search quality is limited by in-memory search.
    - A second copy of node content (chunks) lives in UAR's store.
- **Rejected alternatives:**

  | Alternative | Rejected because |
  |---|---|
  | E2 cgo embedded | cgo, BSL, native-library distribution |
  | E3 Kubo-managed sidecar | Kubo supervising a third-party server |
  | E4 a vector store in Kubo | Misses SurrealDB; grows a store in Kubo |
  | E5 daemon-wide hook | Refused under User Agency |

---

### IMPACT ON F4 AND THE CURRENT PLAN

#### F4 itself

F4 is unchanged in placement. Two additions land inside `ipfs mcp`:

- the principal policy, egress acknowledgement, out-of-band confirmation and `check-uar` (N8);
- optionally, the L3 proxy route and the N9 hook.

There is still no daemon, `config/` or `docs/config.md` change, and no new Go dependency: the proxy and preflight use the standard library's `net/http`.

#### Changes to existing plan items (22)

| # | Change | Impact |
|---|---|---|
| 0 | team-ownership-grant | Add grants:<ul><li>ui-ux-engineer: `mcp/chat/**` and Web UI `src/agent-chat/**`</li><li>architect: `mcp/uar/**` (profile, spike report)</li><li>mcp-server-engineer: `test/cli/mcp_uar_*_test.go` and N8 code in `mcp/server/**` (already granted)</li><li>Desktop fork paths: only after question 7</li></ul>Add questions 1-11 to the batched Round 0 question. |
| 1 | mcp-tool-contract | Add `contentEgress`, A2A skill metadata, A2UI binding hints, the principal/scope schema and the `egress_ack` config schema. **Adds one to two tasks.** |
| 2 | mcp-architecture-adrs | Add ADR-0004, ADR-0005 and ADR-0006. **Adds three tasks.** |
| 6 | mcp-pinning-with-confirmation | The gate gains a third mode, out-of-band, for non-elicitation principals. The confirmation state machine must support external resolution. **Adds one task.** The out-of-band channel itself is N8. |
| 7 | mcp-http-transport-auth | Principals become configured entities with kinds and group grants. **Adds one task.** |
| 5 | mcp-ui-component-library | Chat primitives (message stream, tool-call card, approval card, egress badge), shadow-DOM-safe components. **Size: L to XL.** |
| 16 | webui-webmcp-integration | Sequence it before N5 because both touch Settings. |
| 20 | mcp-user-docs | Agentic setup guide: UAR install, profile, `check-uar`, egress, the stdio limitation, embedding limits. |
| 21 | phase-boundary-review | Add the G9-G12 acceptance criteria from KUBO-SIDE ENFORCEMENT and the N2 report. Security review covers the chat in the admin origin, the L3 proxy, the control socket and the confirmation page. |
| others | 3, 4, 8-15, 17-19 | No change. |

#### New changes (9, three conditional)

| New change | Owner | Depends on | Round | Notes |
|---|---|---|---|---|
| N1 `uar-conformance-request` | architect (`team-request`; no UAR edits) | team-ownership-grant | 1 | The A2A and AG-UI items, CORS allowlist, elicitation, parameterized ingest, safe defaults (no default model, local memory embedder default), and an ANN index. Its status is evaluated at the end of Round 4 (A9.1, A11.2, A12.1). |
| N2 `uar-integration-spike` (extended) | mcp-server-engineer, plus the operator for criterion 3 | mcp-server-core-stdio, mcp-pinning-with-confirmation (confirmation state machine) | 4 | GO criteria 1-4 (ASSUMPTIONS AND FALSIFIERS). Moved from Round 3 to Round 4 because criterion 2 needs the confirmation gate. Uses a minimal N8 confirm stub, finalized in N8. |
| N8 `mcp-agent-principal-enforcement` | mcp-server-engineer | mcp-http-transport-auth, mcp-pinning-with-confirmation | 4 (after http-transport-auth in merge order) | Principal policy, `egress_ack`, out-of-band confirmation (CLI over a control socket plus a loopback page), `check-uar`, startup preflight, and the `test/cli` acceptance tests. **Blocking for N3, N5 and N7.** |
| N3 `agentic-uar-integration` | architect (profile, docs) + mcp-server-engineer (e2e) | N2 GO, N8 | 5 | Shipped `mcp/uar/profile/`, e2e over `--http`, loopback-only network test, A2A/AG-UI smoke tests. Conformance claims only if A9.1 holds. |
| N10 `mcp-agui-proxy` (CONDITIONAL on question 2/6: L3) | mcp-server-engineer | N8 | 5 | AG-UI SSE pass-through to the configured UAR, origin allowlist, server-held key, disabled unless configured. |
| N4 `agent-chat-island` | ui-ux-engineer | mcp-ui-component-library | 3 | Island, AG-UI client, approval and out-of-band confirm rendering, egress badge, WCAG 2.2 AA, isolation tests for A11.1. |
| N5 `webui-agent-chat` | ui-ux-engineer | N4, N2 GO, N10 or the N1 CORS item | 6 | Mount, toggle, i18n, real run. Deferred by default per A11.2. |
| N6 `desktop-agent-chat` (CONDITIONAL on question 7) | the role named by the operator | N5 | 7 | CSP, plus `safeStorage` if direct-to-UAR. |
| N7 `agentic-embeddings-uar` | architect + mcp-server-engineer | N3, A12.1 | 6 | Per-node knowledge base, MIME allowlist, local profile, pin-aware deletion, reconciliation, loopback-only test. **No Kubo daemon code.** |
| N9 `mcp-embedding-hook` (CONDITIONAL on question 8) | mcp-server-engineer | N7, N8 | 7 | Opt-in post-`ipfs_add` ingest to the configured UAR knowledge base from any surface, with `egress_ack` required. |

#### Dependency and round impact

- The plan grows from 22 to 31 changes (four conditional: `webui-cid-delivery`, N6, N9, N10). The round structure 0 to 8 is kept:

  | Round | Additions |
  |---|---|
  | 0 | Larger batched question |
  | 1 | N1 |
  | 3 | N4 |
  | 4 | N2, N8 |
  | End of Round 4 | **Delivery gate** for A9.1, A11.2 and A12.1 |
  | 5 | N3, N10 |
  | 6 | N5, N7 |
  | 7 | N6, N9 |
  | 8 | Review with the G9-G12 acceptance criteria |

- **New critical path.** core-stdio, then pinning-with-confirmation, then http-transport-auth, then N8 and N2, then the gate, then N3, then N7, then N9. N9 lands in Round 7, alongside the existing Round 7 changes, so the phase does not get longer.
- **Round 4 load.** Round 4 grows to nine parallel changes. The `plan.md` T1 worktree-merge risk rises. N8 merges after http-transport-auth and before the tool groups.
- **Failure isolation.** Any NO-GO or gate failure defers only G9-G12 delivery changes. G1-G8 are unaffected.

---


ADDENDUM COMPLETE

## Unresolved review findings

Review of this addendum: round 1 BLOCK (`review/assess-addendum/findings.round1.json`), round 2 BLOCK (`review/assess-addendum/findings.json`). Both rounds were harness-native isolated Claude critics because the liter-llm gateway (localhost:4000) was down; producer and judge are the same model family, so this review is NOT cross-model and must be re-run through `dispatch-judge.sh` once the gateway is available. Round 2 is the last vet round, so the addendum is accepted with the round-2 findings carried verbatim below.

### [CRITICAL] Applied as written, the addendum's own scoring method excludes the option it selects: (d) gets G9 goal fit 4 although the rule says N1 delivery is not credited, and without N1 the addendum itself reports G9 as unmet, which by its goal-fit gate means exclusion; then (a) wins outright.

> Evidence: Line 149: 'Items that need UAR-team delivery (N1) are not credited in either row.' Line 144: 'An option with goal fit at or below 2 is excluded before aggregation: it does not meet the operator's goal.' Line 447 (A9.1, no N1 delivery): 'Default: N3's A2A/AG-UI conformance tasks and any G9 conformance claim are deferred; G9 is reported unmet.' Line 426: 'UAR answers A2A and AG-UI per spec | No (UAR code) | G9 conformance unmet until N1 delivers.' Yet line 196 scores '(d) UAR-hosted, with Kubo enforcement (N8) | 4' for goal fit, while (e1), which does deliver a conformant A2A layer, is excluded at goal fit 2 (197). With (d) excluded, the remaining scores are (a) 51, (c) 49 (192, 194), so the decision of record flips to (a). The same contradiction holds for G12: E1 goal fit 4 (371), but A12.1 (471) says that without a UAR change to /api/ingest 'N7 is deferred, and G12 is unmet'. The (d) 'Time to working result 3' (196) also conflicts with Risk 1 (756): 'G9-G12 may deliver nothing in this phase'.
>
> Suggested fix: Either score (d) and E1 goal fit for UAR as it is without N1 (at or below 2, which excludes them and makes (a)/E3 the in-method choice), or state openly that the goal-fit gate is waived for options contingent on another team and justify why that waiver does not also apply to (e1). Record that the G9 choice of (d) is then a judgment call outside the stated method, not its output.

### [WARNING] The out-of-band confirmation is declared to guarantee that 'a human confirms every node write', but `ipfs mcp confirm <id>` and the loopback confirmation page accept any same-user local process, and UAR can run shell commands as that user when its terminal_exec tool or any shell-capable MCP server is configured; nothing in N8 or check-uar detects that.

> Evidence: Line 425: 'A human confirms every node write | Yes (item 3; elicitation otherwise). | Met, independent of UAR's approval and its governance bypass.' Line 394: 'No self-confirmation. The two-step token is not accepted from such principals, because the agent could confirm by itself.' Line 396: '`ipfs mcp confirm <id>` ... through a local control socket under the user's runtime directory, mode 0600' (mode 0600 admits every process of the same user, which includes a loopback UAR). UAR ships 'terminal_exec' ('Execute a shell command and return its stdout, stderr, and exit code', uar:src/uar/tools/terminal_exec.rs:22-27), opt-in via config (uar:src/config.rs:2523-2525). The check-uar refusal list (402-406) and the shipped profile (410-416) say nothing about code-execution tools or other MCP servers in UAR's mcp.json. Line 397 also lets the chat island, which runs in the admin origin where Risk 7 (767) admits 'an XSS in the island is still admin RPC', embed the confirmation page. The confirmation acceptance test (435) would pass while this path stays open, so the gap would surface only in use.
>
> Suggested fix: List 'the agent runtime cannot reach the confirmation surface' as an assumption with a falsifier. Make check-uar refuse write groups when UAR reports terminal_exec or any non-Kubo MCP server with code-execution effect, pin `native_tools.terminal_exec_enabled: false` in the shipped profile, and require a human-presence factor for `confirm` (TTY check plus a code shown only on the page, or vice versa). Add an N2 case where the agent is asked to confirm its own pending write through terminal_exec.

### [WARNING] check-uar is a startup-time check against a UAR whose model, embedder and governance settings can change at runtime, and it cannot observe the bind address from a loopback client, so 'refuses unsafe instances' (ADR-0004) holds only at the moment of the check.

> Evidence: Line 400: '`ipfs mcp check-uar <url>` preflight. It also runs automatically at startup for each principal marked `kind: uar`.' UAR exposes 'settings.update', 'PUT', '/api/uar/settings/{key}', Admin, NextTurn (uar:src/uar/api/administration_capabilities.rs:157), and governance follows 'the operator's stored preference' (77). Line 403: refuses when 'the bind is not loopback', but a 0.0.0.0-bound UAR answers identically on 127.0.0.1, and the settings fields are '[unverified]' (401). If they are unreadable, line 407 falls back to 'check: assert', an operator waiver. Line 605 (ADR-0004): 'a `check-uar` preflight that refuses unsafe instances.' The acceptance test (432) checks only a UAR running all defaults at once, where the unauthenticated probe alone triggers refusal.
>
> Suggested fix: Re-run the effective-settings check before each contentEgress or write call, or periodically with fail-closed behavior on drift (UAR exposes /api/uar/settings/drift). Drop 'bind is not loopback' from what check-uar claims unless the settings API confirms it, and add an acceptance case: JWT on, bind 0.0.0.0, remote model switched in after startup.

### [WARNING] The A9.1/A11.2/A12.1 falsifier passes when the items are merged on UAR main, but A10.2 and N2 allow only published artifacts with no source build, so the gate can report the dependency delivered while no installable UAR contains it.

> Evidence: Line 447: 'By the end of Round 4, those items are not merged on UAR `main` (checked by commit or PR state)'. Line 456 (A10.2): 'using only published artifacts (Docker image or native package, no source build)'. Line 479: 'using published artifacts only'. No assumption covers a UAR release cadence or a minimum UAR version for the shipped profile.
>
> Suggested fix: Change the falsifier to 'contained in a published UAR release (tag or image digest) by the end of Round 4', pin that minimum UAR version in `mcp/uar/profile/`, and have check-uar refuse older versions via the agent card.

### [SUGGESTION] The change counts do not add up: the 'New changes' table lists 10 changes but is titled 9, and 22 + 10 is 32, not the stated 31.

> Evidence: Line 715: 'New changes (9, three conditional)'; rows N1, N2, N8, N3, N10, N4, N5, N6, N7, N9 (719-728) = 10. Line 732: 'The plan grows from 22 to 31 changes'.
>
> Suggested fix: Correct the count before plan revision 2 sizes the rounds on it.

### [SUGGESTION] The same N8 controls are scored as costing Kubo surface for G9 but not for G12, which biases E1.

> Evidence: Line 195-196: (d) Kubo surface 5 as shipped, 4 with N8. Line 370-371: E1 Kubo surface 5 both as shipped and 'with Kubo enforcement (N8 profile and preflight)'.
>
> Suggested fix: Score E1+N8 Kubo surface 4 for consistency (E1 still wins at 52 against 36 and 31).

### Orchestrator adjudication

1. CRITICAL (method excludes the chosen option): upheld. Applying the addendum's own goal-fit gate without crediting undelivered N1 items, (d) UAR-hosted AG-UI/A2A falls to goal fit 2 and is excluded; (a) AG-UI and A2A in `ipfs mcp` is the decision the method produces (51 vs 49). It also matches the operator's original wording: "the AG-UI and A2A endpoints are proxies to where when an LLM is needed" UAR or an OpenAI-compatible server is used. Provisional decision of record pending the operator's answer to question 1: **G9 = (a) AG-UI + A2A endpoints in `ipfs mcp`, reusing the AuthVerifier, scope map and confirmation gate in-process; G10 = L3, `ipfs mcp` proxies LLM calls to an operator-configured UAR instance or OpenAI-compatible server (no default, off unless configured, key from env).** The plan must not treat (d) as decided. G12's E1 goal-fit score has the same defect; plan revision 2 re-scores G12 under the gate before deciding.
2. WARNING (human gate bypassable by same-user processes / UAR terminal_exec): upheld. The out-of-band confirmation must carry a human-presence check (Desktop-native or OS-level prompt, not only a 0600 socket or loopback page), and `check-uar` plus the shipped profile must detect and refuse shell-capable tools (`terminal_exec`, shell MCP servers) for any principal with write scope. The acceptance test must include an agent attempting to self-confirm.
3. WARNING (check-uar point-in-time): upheld. Re-check on each session and on settings change where observable; document that a 0.0.0.0 bind is not detectable from loopback and require the shipped profile for the supported configuration.
4. WARNING (delivery gate counts merged-but-unpublished items): upheld. Only items in a published, installable UAR artifact count as delivered.
5. SUGGESTIONS: change counts corrected in plan revision 2 (22 + 10 = 32 if all new changes stand); Kubo-surface scoring made consistent between G9 and G12.

### Cross-model re-review (gateway restored)

Judge `gpt-6.1-sol` via liter-llm (`review/assess-addendum/findings.cross-model.json`): BLOCK, 5 CRITICAL, 4 WARNING. The packet was truncated at the `## DECISION` heading (39,613 of 74,296 bytes; build-review-packet.sh cap 40,000), so the judge did not see the decisions, the falsifiers, the Kubo-side enforcement, the operator answers or the adjudication above. Findings verbatim:

- [CRITICAL] The parsed decision has no stated assumptions or falsifier, making the commit-level decision unfalsifiable despite relying on conditional delivery.
- [CRITICAL] The decision chooses a UAR-only backend path that does not satisfy the stated G10 requirement for either UAR or an operator-configured OpenAI-compatible server.
- [CRITICAL] A load-bearing operator preference is unresolved, so the decision commits to a path before the required operator answer exists.
- [CRITICAL] The decision does not meet G12's requirement to support both embedded and remote SurrealDB modes, because it assigns embeddings and SurrealDB entirely to UAR and rejects embedding SurrealDB in Kubo.
- [CRITICAL] The selected architecture knowingly leaves the no-egress guarantee only partially met for stdio-spawned UAR, contradicting the original G12 guarantee.
- [WARNING] The enforcement design depends on unverified UAR settings fields, so the key safety check may be impossible to implement as described.
- [WARNING] The selected G9 architecture depends on another team merging core UAR conformance work by Round 4, but the decision commits to UAR ownership before that dependency is secured.
- [WARNING] The setup-cost threshold is arbitrary and could reject or accept UAR based on an unsupported number.
- [WARNING] The preferred chat UI path keeps the chat in the admin-privileged Web UI origin while acknowledging that keeping it out of that origin may be safer and undecided.

Orchestrator adjudication of the cross-model findings:

1. "No assumptions or falsifier": not upheld; truncation artifact (ASSUMPTIONS AND FALSIFIERS section exists after the cap).
2. "UAR-only backend fails G10": superseded by the operator's answer (Both; `ipfs mcp` proxies to UAR or an OpenAI-compatible server, `operator-answers.md`).
3. "Load-bearing operator preference unresolved": resolved; answers recorded 2026-10-09.
4. "G12 not met for embedded + remote SurrealDB": upheld. Plan revision 2 must deliver both modes concretely and state the mechanism for each.
5. "Egress guarantee partial for stdio-spawned UAR": upheld. In topology A (`ipfs mcp` hosts AG-UI/A2A and proxies the LLM) egress is enforced in-process; topology B (UAR-hosted) must be restricted to `ipfs mcp --http` with a principal `egress_ack`, or its partial status documented as a stated limitation.
6. Warnings (unverified UAR settings fields; UAR delivery dependency; setup-cost threshold; chat in the admin-privileged origin) carry to plan revision 2 and the phase-boundary review.

The plan review must not be truncated: reviewing plan.md requires a packet cap above its size or a reviewed excerpt that includes every decision section.
