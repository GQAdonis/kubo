# ADR-0010: Kubo provisions and proxies a UAR agent behind `ipfs mcp /agui`; the release gate

## Status

**Accepted (2026-10-16), OG-4 met; real-service verification (`agent-backend-uar` 1.5) open.** Proposed 2026-10-15 by `ipfs-agent-architect`, recording the assumed defaults D13 and D14 of `.kbd-orchestrator/phases/agent-backend-settings/plan.md` revision 4 (section 1) and plan item 22 (`agent-backend-uar` task 1.1). The published UAR v1.1.0 release is pinned (see the amendment of 2026-10-16), so the section 4 gate is met; what has not run is the test against the published image (task 1.5), and the amendment lists what only that run can show.

This ADR supersedes the topology-B design of `agentic-uar-integration` (all tasks) and `uar-integration-spike` tasks 1.2-1.5. It does not supersede ADR-0008, whose recorded NO-GO stands as history; it replaces the topology that ADR deferred.

## Date

2026-10-15

## Context

- Goal 2 of phase `agent-backend-settings`: a Universal Agent Runtime (UAR) agent with its own system prompt, the kubo MCP tools and the IPFS skills, reachable from the Web UI and Desktop chat through `ipfs mcp /agui` so the island contract (one endpoint, one token model, one interrupt shape) stays unchanged.
- The previous design (ADR-0004 topology B, ADR-0007, ADR-0008) had UAR host AG-UI and A2A itself and call `ipfs mcp --http` as a `kind: uar` principal. Its gate (a published, installable UAR release containing the conformance items) failed on 2026-10-09 and the work was deferred (ADR-0008). The island would have needed a second endpoint profile and a second interrupt shape to talk to UAR directly.
- Settings contract (ADR-0009, `agent-settings-contract` 1.2): the settings resource already carries `backend.kind: openai | uar` (`mcp/contract/settings/settings-read.schema.json`, `settings-write.schema.json`, `test-connection.schema.json`, `model-list.schema.json`). The `uar` kind is declared in the contract and has no implementation behind it.
- UAR facts that drive the design (follow-up request `mcp/uar/requests/2026-10-13-v1.1.md`, filed as a comment on `Prometheus-AGS/universal-agent-runtime#379` on 2026-10-13): the requested `v1.1.0` is to carry AG-UI at `/api/uar/ag-ui` with documented interrupt shapes, binding an external MCP server to a specific agent with a run-time credential, remote skill upload, and elicitation for globally registered servers. At HEAD `b4244aea` an external MCP server can only be registered globally with a credential taken from UAR's own process environment, and skill import previews a server-local path only.
- Release state, observed 2026-10-15 on `Prometheus-AGS/universal-agent-runtime`: only `boss-sidecar-<platform>-v1.0.0-*` builds exist. They are sidecar payloads for another application; none is a `v1.1.0` tag with an OCI digest, and none names the conformance items. The newest version-numbered tag is older than the working tree the request describes. No UAR code is changed by kubo and none is installed here.
- `AGENTS.md` Stability and User Agency rules outrank this ADR: no `/api/v0`, gateway, CID or wire change; every new feature has an off switch; no hardcoded third-party endpoint. UAR is an operator-configured backend with no default URL, exactly like the OpenAI-compatible backend (ADR-0004, E6/G10 decisions of record).

## Decision

### 1. Kubo creates and proxies the UAR agent (D14)

When `backend.kind` is `uar`, `ipfs mcp`:

1. **Provisions** one UAR agent through UAR's released management API: system prompt equal to kubo's configured prompt (the fixed safety prefix included), an MCP binding naming this `ipfs mcp` server, and skill bindings equal to the directory names under `mcp/skills/` (acceptance of `agent-backend-uar` 1.2).
2. **Proxies** each `/agui` run to UAR's AG-UI endpoint for that agent and relays the event stream back unchanged except for the interrupt translation below. The island, the principal model (`ui` token on `/agui`) and the confirmation classes of ADR-0003 are unchanged.
3. **Translates interrupts** (`tool_approval`, `mcp_elicitation`) between UAR's shape and kubo's shape (`agent-backend-uar` 1.3). Kubo's confirmation gate stays the authority for every write and destructive tool call: a UAR-side approval never substitutes for it, and a proxied approval is only an input to kubo's own confirmation (capability inversion: the agent kernel does not write).
4. **Holds a UAR credential** supplied by the operator by reference (file or environment name, never a literal in config; see the amendment of 2026-10-16 for the settings API). An absent credential disables only the `uar` backend and the dependent route answers `backend_not_configured`; it does not stop startup (CRITICAL-2, ADR-0009 section 1).

The binding UAR makes to `ipfs mcp` uses a run-time credential that is scoped to that agent. The `ipfs mcp` listener stays loopback or console-bearer as configured; UAR gets the same `ui`/tool-principal grants and `egress_ack` as any other backend and no wider.

### 2. Egress and reachability

- A UAR instance is an LLM backend host for `egress_ack` purposes. Content-egress tool results go to it only if its host is `local`, covered by the principal's `egress_ack`, or the write that selected it carried the D4 confirmation (ADR-0009). The UAR instance's own upstream model provider is not visible to kubo unless the released settings endpoint exposes it; where it is not readable, `contentEgress` results are refused, as `check-uar` already does (ADR-0007).
- Redirects from UAR are not followed. The base URL has no default.

### 3. Off switch and defaults

- Default: `backend.kind` is `openai` (or no backend). The `uar` kind is selected only by the operator through the settings API, the config file or the settings page. Nothing contacts a UAR instance unless it is configured.
- Off switches: set `backend.kind` back to `openai`, remove the UAR URL, or leave `llm` unset (`backend_not_configured`); `make build GOTAGS=noagent` removes all agent code. Disabled-state tests: `TestAgentOffWhenUnconfigured` and `TestAgentHasNoDefaultEndpointOrModel` cover the unconfigured state, `TestResolveUARDefaultsAndRefusals` and `TestSettingsWriteSwitchesToAndFromUAR` cover the `uar` kind defaulting to `openai` and switching back, and `TestUARWithoutMCPPrincipalIsRefused` covers the fail-closed rule that a `uar` backend without `llm.uar.mcp_principal` is refused. They exist; `agent-backend-uar` 1.5 adds only the run against the published image.

### 4. The release gate (OG-4)

`agent-backend-uar` tasks 1.2-1.6 do not start, and no UAR code is merged as complete, until OG-4: the operator confirms a **published, installable UAR release** with a git tag and an OCI image digest (or a native package with a published checksum) that contains the items below. Merged-but-unreleased work does not count, as in ADR-0008.

| # | Required in the release | Needed by |
|---|---|---|
| G1 | Tag `v1.1.0` (or later) on `Prometheus-AGS/universal-agent-runtime` and an OCI digest `sha256:...` recorded in the release notes | 1.4 |
| G2 | Agent management API sufficient to create an agent with a system prompt and to read it back (`GET /api/agents/{id}`) | 1.2 |
| G3 | Per-agent binding of an external MCP server with a run-time credential (request item 2) | 1.2 |
| G4 | Remote skill upload or equivalent binding by name (request item 3) | 1.2 |
| G5 | AG-UI endpoint for an agent with documented interrupt shapes for approval and elicitation (request item 4 and #379 items 3, 6) | 1.3 |
| G6 | Release version readable from the agent card, so `check-uar` can compare it with the pinned minimum (#379 item 9) | 1.4 |
| G7 | Effective settings readable with a bearer credential (#379 item 8), or an explicit statement that `contentEgress` is refused | 2 above |

**State on 2026-10-15: not met.** Only `boss-sidecar-*-v1.0.0-*` builds exist; none is a UAR release carrying G1-G7. Per D13 the phase may close goals 1, 3, 4 and 5 while `agent-backend-uar` 1.2-1.6 wait; goal 2 is carried as an explicit open change (OG-10). `mcp/contract/agentic/uar-preflight.json` stays `null` until 1.4, and the contract validator keeps refusing a `uar` principal for `check: verify` until it is pinned.

When OG-4 passes: record tag and digest (`uar-conformance-release` 1.3), pin `uar-preflight.json` (no `null`), run the real-service test (1.5), then enable the UAR form in the settings page (1.6, no mocked UAR state in production code).

## Alternatives considered

### (b) A second island endpoint profile, UAR hosting AG-UI directly (topology B)

The island would talk to UAR's AG-UI endpoint with UAR's token and interrupt shape. Rejected under D14: two token models, a second interrupt shape in the Web UI and Desktop, and kubo's confirmation gate would sit behind a UAR-side approval it does not control. This is also the design that failed its gate once (ADR-0008).

### (c) Keep the `uar` kind as an OpenAI-compatible endpoint only

Use UAR's `/v1/chat/completions` as an ordinary model backend (topology A, unchanged by this ADR). This already works and stays available; it gives the model but not a UAR agent with its own prompt, skills and tool bindings, so it does not satisfy goal 2.

### (d) Hold the phase until UAR ships

Rejected under D13: it would delay goals 1, 3, 4 and 5 on an external team's schedule.

## Consequences

- One island contract, one token model, one interrupt shape; the UAR agent sits behind it.
- Kubo holds a UAR credential and creates an agent on a remote runtime on the operator's behalf. That is a trust decision: the credential is by reference, write-only through the settings API, and the typed D4/D2 confirmation applies to the backend change. An `admin` bearer holder can repoint kubo at a different UAR instance (ADR-0009 residual 1 applies unchanged).
- The uncomfortable part, stated once: this repeats the dependency that failed in ADR-0008. If the UAR team does not publish a release carrying G1-G7, goal 2 stays open and the only UAR path that works is topology A (UAR as an OpenAI-compatible model endpoint), with no agent-level prompt, skills or tool binding. The settings page must say so honestly (`webui-agent-settings-page` 1.3: honest unavailable states, no fake success).
- Superseded: `agentic-uar-integration` and `uar-integration-spike` 1.2-1.5 carry a superseded note in their `tasks.md`; their checkboxes are unchanged.
- Follow-up work after OG-4: `agent-backend-uar` 1.2-1.6 and the `phase-integration-gate` 1.5 UAR item.

## Amendment (2026-10-16, Round 8: implementation notes)

### The UAR credential follows the settings schema

Section 1 item 4 and the Consequences say the UAR credential is by reference and never a literal in the settings API. The implementation follows the settings schema instead, as the OpenAI backend does: `settings-write.schema.json` carries a write-only `api_key`, which the settings API stores as a 0600 file in the settings directory (never returned by a read; the read shape reports only `api_key: {set, source}`), and the credential can equally be given by reference (`llm.api_key` as an environment name or file) in the configuration file. This is consistent with ADR-0012: the key scope rules (a key stored through the API is cleared on a host move, a key supplied by reference stays) apply to the `uar` kind unchanged, because the UAR instance is the backend host. The by-reference statement still holds for the configuration file and for LKE, where `agent.settings.accept_secrets: false` refuses a key in a write. `llm.uar.*` (`agent_id`, `mcp_principal`, `mcp_credential_revision`, `skills_dir`, and the other keys in that object) is configuration-file only; the settings API neither reads nor writes it.

### Verified against the running release (2026-10-16)

The real-service test passed against UAR v1.1.0 (image digest `sha256:064f5004...`) with the Qwen `qwen3.8-max` model; evidence is in `mcp/server/evidence/2026-10-16-uar-real-service/README.md` (commits `f0eb68447`, `e24e4ff3a`). Verified against the running release: agent create, skill import and bind, run and resume, and the `tool_approval` translation (a write run raises the interrupt, resumes after approval and finishes; a denied write ends with nothing executed).

Findings from that run:

- UAR requires `version` in the `SKILL.md` frontmatter (`src/uar/domain/skills.rs:241-246`; import answers 400 `skill_markdown_invalid` without it). Kubo adds `version: 1.0.0` to uploaded skills that lack one.
- The operator's MCP destination registration may precede the agent's existence, and must happen before the first run (422 `run_mcp_destination_unregistered` otherwise).
- UAR refuses a private-HTTP MCP destination whose address is not RFC1918 (422 `run_mcp_destination_invalid`).
- A failed UAR request now logs op, method, path, status and a 300-byte excerpt of UAR's error, scrubbed of the key and grant. The client-facing error is unchanged.

Still unverified: kubo's own write confirmation answered inside a UAR run (the test leaves it unanswered, by design), the live placement of the interrupt fixtures in `mcp/uar/fixtures/v1.1.0/` beyond the paths the run exercised, and runs on amd64 (the run was arm64).

### Shipped skills need `llm.uar.skills_dir`

Section 1 item 1 says the skill bindings equal the directory names under `mcp/skills/`. There is no embed package in `mcp/skills/` yet, so the binary cannot carry the skills. Until one exists, the operator points `llm.uar.skills_dir` at a directory of skills (a checkout of `mcp/skills/` or an installed copy); without it no skill is uploaded or bound and the agent is created without the IPFS skills.

### Gate state

The section 4 line "State on 2026-10-15: not met" is superseded. `mcp/contract/agentic/uar-preflight.json` now pins the published release v1.1.0, and the `llm.kind: uar` backend is implemented behind that pin.

## Amendment (2026-10-16, Round 8 corrections: lead rulings)

Rulings from the Round 8 security, Go and UI reviews (`.agent-team/kubo-ipfs-agentic/findings/`). IDs refer to those reviews.

### Registering kubo's MCP server on UAR is an operator step

Registering `ipfs mcp` as a destination on UAR (`PUT /api/uar/mcp/servers/{name}` with a `grant_policy` that carries `trusted_hosts`) is done by the operator after the first provisioning. Kubo does not do it, because kubo's configuration has no UAR-reachable URL for itself and no trusted host id: `ipfs mcp` listens on loopback by default and has no way to know the address UAR would use. Follow-up: `llm.uar.mcp_url` and `llm.uar.host_id`, so that kubo can register itself. Until then `docs/mcp/agentic.md` lists the step.

### The credential is a trusted-host JWT

`llm.api_key` for the `uar` kind must be a trusted-host JWT (the issuer-signed token of an entry in UAR's `security.trusted_host_principals`), not an ordinary API key. The settings page and the docs say so.

### Ownership of the remote agent (R8-07)

- Kubo owns the agent's system prompt and the skills it uploads. A `PUT` of an agent with the same `llm.uar.agent_id` overwrites its prompt.
- `llm.uar.mcp_principal` is required for the `uar` kind. An agent with an empty tool allowlist gets UAR's own native tools (Auto selection), so kubo refuses the backend instead of creating an agent without a kubo binding.
- Kubo binds and unbinds only skill ids it bound in this process. It does not unbind a skill someone else bound, and with `llm.uar.skills_dir` unset it unbinds nothing it did not bind.
- `llm.uar.agent_id` must be unique per node on a shared UAR. The default `kubo-ipfs` collides across nodes: two nodes with the default overwrite each other's agent.
- A skill with the same name already on UAR is bound in place of kubo's only if its `artifact_digest` equals the digest of the bytes kubo holds. A different or missing digest fails provisioning closed (U7), and nothing is bound.

### The relay is an allowlist (U3)

The proxy relays only `TEXT_MESSAGE_*` and `TOOL_CALL_*` events and, within them, only the listed fields. It writes `RUN_STARTED`, `RUN_FINISHED` and `RUN_ERROR` itself, with the island's thread and run ids, never UAR's. Everything else is dropped: `CUSTOM` (including an A2UI surface), `STATE_*`, `MESSAGES_SNAPSHOT`, `RAW`, `STEP_*` and the `*_CHUNK` events. A UAR agent therefore cannot make the island render a surface that kubo did not produce.

### Secrets are scrubbed from relayed strings (U8)

The UAR credential and the run grant are replaced in every relayed string, including a relayed `RUN_ERROR` message and approval card text. Residual: a secret split across two events is not detected, because each string is scrubbed on its own. Accepted; the secrets are by-reference tokens that rotate, and the exposure is to the same island session that holds the run.

### Approval cards are labelled (U9)

A UAR approval card is labelled "UAR approval, not a node confirmation" and carries a `cf_uar_` confirmation id. The island schema requires the `cf_` prefix, so the id cannot drop it. Residual: the card still shares the confirmation-card shape and prefix. Telling them apart in structure needs a contract change (a distinct kind or a field the island schema defines); it is recorded as a follow-up, not made this round.

### The grant is the principal's static bearer (U4)

The run grant sent in `forwardedProps.uar.mcp_servers` is the `uar` principal's static bearer. `expires_at_unix` is advisory: UAR may honour it, `ipfs mcp` does not enforce it, and a leaked grant stays usable until that principal's secret is rotated, whatever `llm.uar.mcp_grant_ttl` says. Lead ruling: accepted residual. Follow-up: mint short-lived tokens that `ipfs mcp` verifies itself.

### Plain http is refused for the grant too (U5)

The refusal to send a credential over plain http to a non-loopback host covers the grant as well as `llm.api_key`. The exemption is the operator's `embeddings.insecure_http_hosts` list, as for the key. The typed confirmation names the kubo MCP grant, not only the model key.

### Interrupts survive a failed resume (R8-03)

An open UAR interrupt is forgotten after a resume stream that returned 200, and also when UAR refuses the thread (400, 404, 409, 410 or 422) or a hop fails where nobody needs to answer; in those two cases the thread moves to a fresh UAR thread id, so a new message cannot land on a thread where UAR still holds the interrupt. A transport error, a 401 or a 5xx on resume withdraws only the answer to the interrupt shown at the failure, plus any denials it filled in; earlier answers stay and the shown interrupt stays answerable (Go N1). Once the stream has started, a failure settles the interrupts; a failure before it starts is an HTTP 502 and the shown interrupt stays answerable, so no `RUN_ERROR` field was added (N6 rebutted). After an idle eviction or an abandoned run the id changes through a per-thread sequence number; the per-backend generation explains the change only after a settings apply. Every settings apply rotates the UAR thread ids, so UAR-side conversation history is lost even on a prompt-only change.

Residual (N2): a skill removed from `llm.uar.skills_dir` stays bound on UAR after a restart, because the record of what kubo bound (`ownedSkills`) lives on one backend instance only. Unbind it on UAR by hand.

### Other consequences

- A2A with a UAR backend (U6): a non-POST request gets 405 and a non-`a2a` principal 403. If UAR is the backend at startup, `--a2a` is ignored and `/a2a` and the agent card are not mounted (404). The 503 applies only after a runtime switch to UAR, and the public agent card keeps answering 200 then. A task in flight at the switch ends FAILED instead of crashing.
- A backend kind change is a move for the key-scope rule of ADR-0012 (U2); see its amendment.
- A `uar` backend with a grant that moves to a new host, or a first UAR backend, needs the typed `egress_confirmation` even when `egress_ack` covers the host (N2).
- The relay byte budget counts every SSE line, not only `data:` lines (N4).
- With `--api-auth`, an api file re-read during the daemon wait that names a non-loopback address is refused, so the RPC `Authorization` is not sent to it (N5).
- The provisioning warning "llm.uar.mcp_principal is not set" is gone: that configuration is refused.
