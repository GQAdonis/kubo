# Proposal: uar-integration-spike

Phase `ipfs-mcp-server`, plan revision 2 (new change). N2 (CONDITIONAL on the topology-B gate): UAR integration spike over `--http`. See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

**CONDITIONAL.** This change runs only if its gate passes (see What Changes).

**Gate result (2026-10-09): NO-GO** (ADR-0008, `docs/mcp/0008-topology-b-gate-no-go.md`). Task 1.1 is done; tasks 1.2 to 1.5 are deferred and reported unmet for phase `ipfs-mcp-server`.

## Why

Topology B (UAR hosts AG-UI and A2A and calls `ipfs mcp --http` as a `kind: uar` principal) is supported only if the gate passes. Topology-B gate (addendum revision 3): UAR's conformance items contained in a published, installable UAR release (tag plus image digest or native package) by the end of Round 4. Merged-but-unreleased items do not count. The spike then tests the riskiest assumptions against the pinned published release: an A2A-delegated write cannot run without a human, setup cost, defaults with and without a draft profile, and `terminal_exec` refusal.

## What Changes

- Record the gate result (tag and digest or package, and the contained items). If it fails, stop: N2 and N3 are deferred and topology B is reported unmet; topology A is unaffected.
- Operator-measured setup cost from published artifacts on clean macOS arm64 and Linux x86_64 (G10-5: 30 minutes or 15 steps; above it, the UAR backend is documented as advanced).
- A2A-delegated write through UAR (G9-2 analogue) executes only after the native prompt, with UAR governance on and off; UAR-side approval behavior recorded verbatim.
- Self-confirm attempt via `terminal_exec` (enabled in a test-only profile) and via `ipfs mcp confirm`; check-uar must refuse write groups for that instance (HG-1, HG-2).
- Defaults with and without a draft profile (in `mcp/uar/spike/`), loopback-only outbound check, settings readability; `mcp/uar/spike/REPORT.md` with GO/NO-GO per criterion.

## Capabilities

### New Capabilities

None. Spike or request with no product behavior (`skip_specs: true`).

### Modified Capabilities

None.

## Impact

`mcp/uar/spike/**` (mcp-server-engineer) and operator time. No product code.
