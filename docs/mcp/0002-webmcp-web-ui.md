# ADR-0002: WebMCP tools in the IPFS Web UI

## Status

Accepted per host, 2026-10-12, by the rule in "What moves this to Accepted" below, applied to the spike report `mcp/webmcp/spike/REPORT.md` (`webmcp-host-spike` task 1.5) and its evidence in `mcp/webmcp/spike/evidence/codex/` and `mcp/webmcp/spike/evidence/kimi/`:

- **Codex** (ChatGPT desktop app in Codex mode, `com.openai.codex` 26.1007.30912, GPT-6 Sol, built-in browser): GO on all four criteria. Accepted.
- **Kimi** (`Kimi.app`, `com.moonshot.kimichat` 3.2.15, Work mode, Kimi K3): GO with a caveat. Accepted for this product and version. The agent first printed the nonce as `<untrusted>` and quoted it only after one follow-up, so criterion 4 rests partly on the agent's statement that it took the value from the tool result. This is not Kimi Code Desktop, the product the runbook names; that product is untested.

Unparked by this status: `webmcp-tool-package`, `webui-webmcp-integration` and `webui-cid-delivery`, and with them `webui-agent-chat`'s prerequisite on `webui-webmcp-integration`. Web UI delivery: the operator chose D2 on 2026-10-12 (`operator-answers.md`, "Q3 Web UI delivery"), so `webui-cid-delivery` 1.1 is done. The design below is now the decision, with the spike's implications added (section "Amendment (2026-10-12): spike results").

History. First recorded as Deferred, 2026-10-12, by operator decision, because the probe had not run. The text of that status follows, kept for the record. The spike report `mcp/webmcp/spike/REPORT.md` (`webmcp-host-spike` task 1.5) does not exist, because the host probe has not run. The operator has not run the probe in Codex desktop or Kimi desktop: `webmcp-host-spike` tasks 1.2 and 1.3 are open, and `mcp/webmcp/spike/evidence/codex/` and `mcp/webmcp/spike/evidence/kimi/` hold only `.gitkeep`. The probe page and the operator runbook (`mcp/webmcp/spike/RUNBOOK.md`) are built (task 1.1). Fallback evidence about MCP and MCP App support is recorded in `mcp/webmcp/spike/evidence/FALLBACK.md` (task 1.4), and by the runbook's rule it is not WebMCP GO.

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

## Amendment (2026-10-12): spike results

The probe ran in both hosts on 2026-10-12 (`mcp/webmcp/spike/REPORT.md`). Where this disagrees with the text above, this amendment governs.

- **Status.** Accepted for Codex (GO) and for `Kimi.app` 3.2.15 (GO with the caveat in Status). Deferred is superseded. The "Consequences" bullet "No in-browser agent tools ... until a host is proven" no longer holds: hosts are proven, and the tools wait only on the parked changes being built.
- **Design constraints for `webmcp-tool-package`**, from the observed differences:
  - Register through `document.modelContext.registerTool` only. Codex has no `navigator.modelContext`, and Kimi exposes it as deprecated.
  - Treat the `execute` options and their `AbortSignal` as optional. Codex passed one; Kimi did not.
  - Return plain JSON objects.
  - Kimi treats tool output as untrusted and masked a secret-looking value until the person asked for it. Return what the person needs (CIDs, names, counts) as plain, clearly named fields plus a short `summary` string the agent can relay. Whether Kimi masks CIDs is unverified; the first real tool must test it.
  - The Web UI must not send a `Permissions-Policy` that disables `tools`.
- **Delivery: D2, superseding the draft.** The operator chose D2 on 2026-10-12: the fork bumps `WebUIPath`/`WebUIPaths` in `core/corehttp/webui.go` to the CID of its WebMCP-enabled Web UI build, so every fork user's `/webui` changes. This replaces the draft's "no `WebUIPath` change in this phase" and moves D2 out of the rejected alternatives. The behavior is specified in `openspec/changes/webui-cid-delivery/specs/kubo-webui-serving/spec.md`. The bump (`webui-cid-delivery` 1.2, with `TestWebUI` coverage of the `/webui` redirect and the `Gateway.NoFetch` 503 path) and its changelog Highlights entry (1.3) wait for that build, which `webmcp-tool-package` and `webui-webmcp-integration` produce. Making the build CID retrievable on the network is an operator action.
- **Unchanged.** Off by default; writes need a second toggle and an in-page confirmation per call; no destructive tools.
- **Limits.** One run per host. Kimi Code Desktop is untested. In both hosts the agent could also see the nonce on the page; criterion 3 does not depend on that, because the page counts a call only when the host invokes `execute`.

## Amendment (2026-10-12, R13): the in-page approval is not a security boundary; DNSLink behind its own toggle

The R13 reviews of `webmcp-tool-package` and `webui-webmcp-integration` (`.agent-team/kubo-ipfs-agentic/findings/security/r13-webmcp.md`, `.agent-team/kubo-ipfs-agentic/findings/review/r13-webmcp.md`) found that the in-page write approval can be answered by the agent it guards. These are the operator's decisions of 2026-10-12 on that finding. Where this disagrees with the text above, this amendment governs.

### The finding

Security H1: the approval dialog is page DOM. A host agent that can operate the page can click Approve, or a script can call `.click()`, and approve its own write. Requests waited up to 120 s, and Kimi passes no `AbortSignal` (spike report). Scenario: injected content asks the agent for `ipns_publish` of an attacker's CID, then the agent approves it. Both spike hosts drive their built-in browser, so this is the normal case, not an edge case.

### Decision A: keep the in-page approval, harden it, and label it honestly

- **What it is.** An intent and accident check. It stops a write the agent did not mean to send, a write the person did not expect, and a double-click that would approve a request nobody read. A host agent that drives the page can answer it. It is not a security boundary, and the Settings text and the docs say so.
- **Why it stays, and why a modal.**
  - The page is the host's surface. An `alertdialog` that makes the background inert is the WAI-ARIA Authoring Practices pattern for a decision that must be made before anything else continues, and Nielsen Norman Group's guidance reserves modals for exactly that kind of interruption.
  - DESIGN.md line 128 ("modals are avoided") is scoped to `mcp/ui` and `mcp/app`, where the host decides placement. It does not govern the Web UI fork's own pages.
  - Removing the dialog would remove the accident check and leave nothing in its place.
- **Hardening**, required by `webui-webmcp-integration` (see its spec):
  - Approve acts only on a trusted activation: `event.isTrusted` and `event.detail >= 1`. It is armed only after the request has been visible for at least 1 s and a fresh pointerdown follows.
  - The background is inert while a request is open, and the document title shows a pending-approval cue.
  - At most 3 requests wait; extras are declined at once. A "Deny all and turn off agent writes" action is always reachable.
  - A shorter expiry applies when the host passes no `AbortSignal`.
  - Every `execute` checks the live Settings (enabled, writes on, registration generation) and declines with no RPC call when any is off, whether or not the host honored unregistration.
  - The writes toggle text says plainly that an agent which controls the browser can approve its own writes.
- **Later, opt-in strict mode.** Writes approved only through the `ipfs mcp` confirmation broker with an OS presence factor (ADR-0003). Not built in this phase.
- **Future host-native path.** The WebMCP draft defines `requestUserInteraction()` on the `ModelContextClient` a host passes to `execute`. Its use for elicitation is discussed in [webmachinelearning/webmcp#165](https://github.com/webmachinelearning/webmcp/issues/165) ("WebMCP elicitation via requestUserInteraction()", open; read 2026-10-12). When a host provides it, detected at call time, the approval goes through the host's own UI instead of page DOM. The spike did not record whether either host provides it; it is unverified.

### Decision B: DNSLink resolution behind its own off-by-default toggle

- **Finding.** Security H2: the read tools accept `/ipns/<domain>` and `ipns_resolve` of any domain, and private reads (`mfs_read`, `mfs_ls`, `key_list`, `pin_ls`) need no confirmation. Injected content can ask the agent to read a secret and resolve `/ipns/<secret>.attacker.example`, leaking it through the node's DNS lookups.
- **Decision.** Resolving DNSLink names from WebMCP read tools requires a separate Settings toggle, off by default. IPNS names that are libp2p-key CIDs resolve freely.
- **Path tricks closed in the contract.** Every path type rejects `.` and `..` segments, and MFS paths also reject an empty first segment, so path cleaning cannot turn `/a/../ipns/x` into a DNSLink lookup (security L2; `mcp/contract/schemas/common.json`). The Go server must reject them too.

### Residual risks, accepted with these decisions

- **A host that drives the page or has a shell can write.** With agent writes on, such a host can approve its own writes through the page, and a shell-capable host can call the RPC API directly. Recommendation in the docs: keep agent writes off while the host runs in browser-control or fully automatic modes.
- **DHT lookups remain a channel.** Fetching an agent-chosen CID asks the network who provides it. A CID can be any well-formed string, so injected content can have the agent encode data in the CID it asks for; DHT peers that handle the lookup see it. Blocks fetched this way may also be announced under the default `Provide.Strategy` (security I3). The DNSLink toggle does not close this.
- **Peer-supplied addresses are untrusted in the browser** (security L5, operator decision 2026-10-12; ADR-0004 amendment of the same date). There is no `trusted_peers` list in WebMCP this phase, so `mcp/webmcp` marks every entry of `swarm_peers`, `routing_findpeer` and `routing_findprovs` `untrusted: true`, and these tools carry `untrustedContentHint`.
- **Tool results go to the agent's model provider** (security M6). The contract's `contentEgress` and `egress_ack` controls are server-side and are not carried into WebMCP. File contents, listen addresses and peer IPs read through WebMCP leave the machine for the host's AI provider after one Settings toggle. The toggle text says so.

## References

- `mcp/webmcp/spike/RUNBOOK.md`, `mcp/webmcp/spike/evidence/FALLBACK.md`; `mcp/webmcp/spike/REPORT.md` (not yet written)
- `openspec/changes/webmcp-host-spike/tasks.md`, `openspec/changes/mcp-architecture-adrs/tasks.md`
- `.kbd-orchestrator/phases/ipfs-mcp-server/assessment.md`, "ADR-0002 draft: WebMCP in the IPFS Web UI"
- `.kbd-orchestrator/phases/ipfs-mcp-server/operator-answers.md`, answer 7 and Q3
- ADR-0004 (agentic endpoints), ADR-0005 (agent chat UI)
