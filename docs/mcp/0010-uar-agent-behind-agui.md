# ADR-0010: Kubo provisions and proxies a UAR agent behind `ipfs mcp /agui`; the release gate

## Status

**Proposed**, 2026-10-15, by `ipfs-agent-architect`. Records the assumed defaults D13 and D14 of `.kbd-orchestrator/phases/agent-backend-settings/plan.md` revision 4 (section 1) and plan item 22 (`agent-backend-uar` task 1.1). It stays `Proposed` until the operator answers OG-1 for D13 and D14 and OG-4 confirms a release. If OG-1 overrides D14, the "Alternatives" section names what is built instead.

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
4. **Holds a UAR credential** supplied by the operator by reference (file or environment name, never a literal in config or the settings API). An absent credential disables only the `uar` backend and the dependent route answers `backend_not_configured`; it is never fatal (CRITICAL-2, ADR-0009 section 1).

The binding UAR makes to `ipfs mcp` uses a run-time credential that is scoped to that agent. The `ipfs mcp` listener stays loopback or console-bearer as configured; UAR gets the same `ui`/tool-principal grants and `egress_ack` as any other backend and no wider.

### 2. Egress and reachability

- A UAR instance is an LLM backend host for `egress_ack` purposes. Content-egress tool results go to it only if its host is `local`, covered by the principal's `egress_ack`, or the write that selected it carried the D4 confirmation (ADR-0009). The UAR instance's own upstream model provider is not visible to kubo unless the released settings endpoint exposes it; where it is not readable, `contentEgress` results are refused, as `check-uar` already does (ADR-0007).
- Redirects from UAR are not followed. The base URL has no default.

### 3. Off switch and defaults

- Default: `backend.kind` is `openai` (or no backend). The `uar` kind is selected only by the operator through the settings API, the config file or the settings page. Nothing contacts a UAR instance unless it is configured.
- Off switches: set `backend.kind` back to `openai`, remove the UAR URL, or leave `llm` unset (`backend_not_configured`); `make build GOTAGS=noagent` removes all agent code. Disabled-state test: the existing `TestAgentOffWhenUnconfigured` and `TestAgentHasNoDefaultEndpointOrModel` cover the unconfigured state; `agent-backend-uar` 1.5 adds the `uar`-kind disabled test when the release lands.

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
