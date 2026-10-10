# ADR-0008: Topology-B gate is NO-GO for phase `ipfs-mcp-server`; topology B deferred, topology A unaffected

## Status

Accepted, 2026-10-09, by `ipfs-agent-architect`. Records the gate result of `uar-integration-spike` task 1.1 (plan revision 2, D1). Supersedes nothing. A later gate evaluation that passes supersedes this ADR.

## Date

2026-10-09 (recorded 22:46 UTC)

## Context

Plan revision 2, D1, and `mcp/uar/requests/CONFORMANCE.md` ("The gate rule") make topology B (UAR hosts AG-UI and A2A and calls `ipfs mcp --http` as a `kind: uar` principal) conditional: UAR's conformance items must be contained in a published, installable UAR release (git tag plus an OCI image digest or a native package with a published checksum) by the end of Round 4, and each item's acceptance check must pass against that artifact as published. Merged-but-unreleased work does not count. If the gate fails, topology B is deferred and reported unmet, and topology A is unaffected.

The gate was due at the end of Round 4 and is recorded in Round 6. It is evaluated against everything published up to the recording time, which is more lenient than the rule.

## Evidence

| # | Gate condition | Observed | Holds |
|---|---|---|---|
| E1 | The request reached the UAR team | `mcp/uar/requests/CONFORMANCE.md` line 8 reads `Filed: <link> on <date>`, unfilled; `uar-conformance-request` task 1.2 (operator files it) is open. | No |
| E2 | A published UAR release is pinned | `mcp/contract/agentic/uar-preflight.json`: `minVersion`, `settingsPath` and `pinnedFrom` are `null` (ADR-0007, decision 2). | No |
| E3 | The examined source contains the items | Local clone `/Users/gqadonis/Projects/prometheus/universal-agent-runtime`, HEAD `a7cb9729` (2026-10-01): no `agent-card.json` route in `src/`; the settings API returns per-key records behind `x-uar-admin-key` (ADR-0007, Context). | No |
| E4 | Newest version-numbered tag | `v1.0.0-rc.9` (2026-07-13), older than `a7cb9729`, so it cannot contain anything `a7cb9729` lacks. | No |
| E5 | Newest published GitHub releases | `gh release list -R Prometheus-AGS/universal-agent-runtime` at 22:46 UTC: `boss-sidecar-<platform>-v1.0.0-2.2.25-60b5922e` (published 16:07 to 16:23 UTC; assets `uar-sidecar-<platform>.tar.gz` plus a `.json` manifest) and `boss-sidecar-*-v1.0.0-c10-6b2ad902-20261009`. These are sidecar payloads for another application, not UAR releases that name the conformance items. Their source commits were checked anyway (E6 to E8). | n/a |
| E6 | Item 1, A2A 1.0 card at `/.well-known/agent-card.json` | `60b5922e`, `src/uar/api/a2a/mod.rs`: line 75 routes only `/agent.json`; the module doc (line 6) names `GET /.well-known/agent.json`. Same at `6b2ad902`. | No |
| E7 | Item 8, readable effective settings with a bearer credential | `60b5922e`, `src/uar/api/settings.rs`: `list_settings` (line 616) and the `x-uar-admin-key` check (line 369), unchanged from `a7cb9729`; no endpoint matching `mcp/contract/agentic/uar-effective-settings.schema.json`. | No |
| E8 | Item 9, release version in the agent card | `60b5922e`, `src/uar/api/a2a/agent_card.rs` line 22: `version: "0.1.0".into()`, the literal item 9 asks to replace. | No |

Items 2 to 7 were not checked one by one: items 1, 8 and 9 each block topology B alone (the card path is how `check-uar` finds UAR, item 8 is what `check: verify` reads, item 9 is what the version pin compares). No UAR was installed or started and no acceptance check was run against a live instance. The local clone was read with `git log`, `git tag` and `git grep` (no fetch); published commits were read through the GitHub API. No UAR file was changed.

## Decision

**NO-GO.** Topology B is deferred and reported unmet for phase `ipfs-mcp-server`. Topology A is unaffected.

Deferred (unmet), not done; none of these tasks is checked off:

- `uar-integration-spike` 1.2 (operator setup-cost measurement, G10-5), 1.3 (A2A-delegated write through UAR, governance on and off), 1.4 (self-confirm attempts and the `check-uar` downgrade against a UAR instance), 1.5 (defaults with and without a draft profile, GO/NO-GO per criterion, `mcp/uar/spike/REPORT.md`).
- `agentic-uar-integration` 1.1 (shipped profile), 1.2 (principal snippet and the `uar-preflight.json` pin), 1.3 (topology-B end-to-end tests), 1.4 (profile README), 1.5 (UAR backend upstream-provider check per agent session; moved there from `mcp-llm-proxy-loop` in Round 6).
- `uar-preflight.json` keeps `minVersion`, `settingsPath` and `pinnedFrom` at `null`. A `kind: uar` principal is refused with `min_version_not_set` unless the operator sets `uar.min_version` and uses `check: assert` on their own verification (ADR-0007).

Done and unaffected:

- `uar-integration-spike` 1.1: this record.
- `uar-conformance-request` 1.1 (the request text). Task 1.2 (filing) stays an open operator action; filing it later does not reopen this phase's gate.
- `check-uar`, the stdio guard and `kind: uar` handling from `mcp-agent-principal-enforcement`. They ship and fail closed, which is correct while no conformant release exists.
- Topology A: `ipfs mcp --http` serves `/mcp`, `/agui`, `/a2a` and the agent card itself with any OpenAI-compatible backend. A UAR instance may still be configured as the LLM backend there; egress is then governed only by the principal's `egress_ack` naming the UAR host, with the limitation stated in ADR-0004 (Kubo cannot see where UAR forwards content).

## Alternatives considered

| Alternative | Rejected because |
|---|---|
| Evaluate against UAR `main` or the sidecar payload commits as if they were releases | Merged-but-unreleased work does not count (gate rule), and those commits fail items 1, 8 and 9 anyway. |
| Keep the gate open into Round 7 | The gate exists so topology B cannot delay topology A; the request is not even filed, so waiting has no event to wait for. |
| Mark the deferred tasks done with a "not applicable" note | They are unmet, not inapplicable. Marking them done would report delivery of topology B that did not happen. |

## Consequences

- The phase reports G9/G10 through topology A only. The operator's "Both" answer is half delivered.
- `agentic-uar-integration` and tasks 1.2 to 1.5 of `uar-integration-spike` carry over to a later phase unchanged, gated on a new evaluation recorded with a release tag, a digest or checksum and the passing items.
- The uncomfortable part: the request that would have started UAR's side of the work was never sent. The roughly 14 tasks planned for topology B produced the request text, a fail-closed `check-uar` and this record, and nothing a user can run against UAR.

## Note on location

`uar-integration-spike` task 1.1 named `mcp/uar/spike/REPORT.md` as the place for this record. The gate result is recorded here instead, as a decision record, because writing that file was blocked in the architect's session. `REPORT.md` remains the output of task 1.5 when the spike runs; it should cite this ADR for the Round 6 evaluation.

## References

- `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`: D1 (gate), Round 6 and Round 7 schedule
- `mcp/uar/requests/CONFORMANCE.md`: gate rule, items 1 to 9
- ADR-0004 (agentic endpoints, egress limitation), ADR-0007 (check-uar settings shape and version pin)
- `openspec/changes/uar-integration-spike/`, `openspec/changes/agentic-uar-integration/`, `openspec/changes/uar-conformance-request/`
