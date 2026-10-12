# ADR-0002: WebMCP tools in the IPFS Web UI

## Status

Deferred, 2026-10-12, by operator decision. The spike report `mcp/webmcp/spike/REPORT.md` (`webmcp-host-spike` task 1.5) does not exist, because the host probe has not run. The operator has not run the probe in Codex desktop or Kimi desktop: `webmcp-host-spike` tasks 1.2 and 1.3 are open, and `mcp/webmcp/spike/evidence/codex/` and `mcp/webmcp/spike/evidence/kimi/` hold only `.gitkeep`. The probe page and the operator runbook (`mcp/webmcp/spike/RUNBOOK.md`) are built (task 1.1). Fallback evidence about MCP and MCP App support is recorded in `mcp/webmcp/spike/evidence/FALLBACK.md` (task 1.4), and by the runbook's rule it is not WebMCP GO.

This is a decision not to decide yet. The draft below (from `.kbd-orchestrator/phases/ipfs-mcp-server/assessment.md`, "ADR-0002 draft") is the intended design. Nothing in it is built.

## Date

2026-10-12

## Context

- Goal G8: WebMCP tools in the IPFS Web UI. Operator answer 7 (`.kbd-orchestrator/phases/ipfs-mcp-server/operator-answers.md`) names Codex desktop and Kimi desktop as the first hosts. The answers also record that host support is unverified and that G8 implementation is gated on a host spike. Web UI delivery (Q3) is decided after the spike.
- WebMCP is a Draft Community Group Report (8 October 2026). Pages register tools with `document.modelContext.registerTool`, gated by the `tools` permissions policy. No host has been shown to call a tool registered by our page.
- Kubo serves a pinned Web UI CID on the RPC origin (`core/corehttp/webui.go`). The Web UI is React 16 with `kubo-rpc-client` (GQAdonis/ipfs-webui fork).

## Decision

Deferred: no WebMCP code is written, and no Web UI CID change is made, until the spike report shows GO for at least one host.

### What moves this to Accepted

The operator runs the probe per `mcp/webmcp/spike/RUNBOOK.md` and saves screenshots and the agent transcript under `evidence/codex/` and `evidence/kimi/`. `REPORT.md` then records GO or NO-GO per host. A host is GO only when all four criteria hold, each with cited evidence:

1. `document.modelContext` is present.
2. The `echo` tool registers.
3. The host agent invokes `echo`.
4. The agent transcript contains the per-load nonce.

This ADR becomes Accepted for each GO host, scoped to that host. If both hosts are NO-GO, it becomes Rejected for this phase and G8 is recorded as not met.

### Intended design if accepted (the assessment draft, unchanged)

- A fork-only `src/webmcp/` bundle in GQAdonis/ipfs-webui registers the tools.
- The tools are built from the contract-generated, framework-free package `mcp/webmcp`, with WebMCP annotations derived from the MCP ones.
- The feature is off by default. Writes need a second toggle and an in-page confirmation per call. There are no destructive tools.
- Delivery is through Desktop fork bundling and opt-in subdomain-gateway use. Kubo's `WebUIPath` does not change in this phase.

### What stays parked

| Change | What waits |
|---|---|
| `webmcp-tool-package` | The `mcp/webmcp` package (0/3 tasks) |
| `webui-webmcp-integration` | The Web UI fork registration bundle (0/4 tasks) |
| `webui-cid-delivery` | The operator's choice of D1, D2 or D3, which follows the spike report (0/3 tasks); `core/corehttp/webui.go` stays unchanged |
| `webui-agent-chat` | Its change-level prerequisite on `webui-webmcp-integration` (sequenced after it because both touch Settings). The chat itself is built and does not use WebMCP; only task 1.3, the operator's live run, is open |
| `mcp-architecture-adrs` | Nothing: task 1.2 is met by this Deferred record |

## Consequences

- **No in-browser agent tools.** A Web UI user cannot hand IPFS actions to a browser-hosted agent until a host is proven, whatever Codex or Kimi ship in the meantime. G8 is not delivered in this phase.
- The rest of the MCP work is unaffected. MCP agents use `ipfs mcp` over stdio or Streamable HTTP, and the Web UI chat uses `/agui` (ADR-0004, ADR-0005).
- No fork drift, no second RPC client stack implementing the contract, and no change to the admin origin.
- The probe page and runbook can go stale as the draft spec and the hosts change. They must be re-read before the run.

## Rejected alternatives

| Alternative | Rejected because |
|---|---|
| Build the WebMCP package and Web UI integration now, unverified | Code against a draft API with no proven host; it may never be invoked. The operator answers gate G8 on the spike |
| Treat MCP or MCP App support in Codex or Kimi (`FALLBACK.md`) as GO | The runbook's GO criteria require a WebMCP tool call with the nonce in the transcript; MCP support says nothing about WebMCP |
| Treat Codex's documented "site tools" as GO | A vendor doc is not the four-criteria probe result on our page |

## References

- `mcp/webmcp/spike/RUNBOOK.md`, `mcp/webmcp/spike/evidence/FALLBACK.md`; `mcp/webmcp/spike/REPORT.md` (not yet written)
- `openspec/changes/webmcp-host-spike/tasks.md`, `openspec/changes/mcp-architecture-adrs/tasks.md`
- `.kbd-orchestrator/phases/ipfs-mcp-server/assessment.md`, "ADR-0002 draft: WebMCP in the IPFS Web UI"
- `.kbd-orchestrator/phases/ipfs-mcp-server/operator-answers.md`, answer 7 and Q3
- ADR-0004 (agentic endpoints), ADR-0005 (agent chat UI)
