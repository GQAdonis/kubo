---
type: SessionRecord
id: karpathy-session-905f86427db8d2e9
title: Karpathy session 905f86427db8
tags:
- karpathy
- session-learning
sources:
- resource: session:43b10494-d504-43f2-9ba1-af8ce53bff62
generated:
  by: pk/1.11.0
  at: 2026-10-20T09:32:00.382941+00:00
created_at: 2026-10-20T09:32:00.382941+00:00
updated_at: 2026-10-20T09:32:00.382941+00:00
revision: 0
---

## Delta

```
KBD STATUS — Kubo
phase: agent-backend-settings › console-session-auth › task:7.1
worktree: /Volumes/T7/Projects/references/kubo  ⚠ outside worktreeRoot (/Users/gqadonis/.claude/worktrees)
Last updated by: kbd-runtime (2026-10-20T01:39:44Z, revision 2157)

Implementation: 32/37 IN_PROGRESS
Evidence:       NOT_TRACKED
Certification:  NOT_TRACKED
Publication:    NOT_TRACKED

Goals:
  [🔄] Runtime agent-backend settings UI (Desktop + cloud Web UI): built; cloud rollout waits on lke-agent-settings
  [🔄] Backend option 1, UAR: agent-backend-uar done; cloud rollout pending
  [🔄] Backend option 2, OpenAI-compatible proxy: AG-UI/A2UI registry and hot reload done; cloud rollout pending
  [✅] Agent features on by default, each with an off switch (agent-features-default-on)
  [🔄] Promethity rebrand: Web UI and Desktop changes done; promethity-delivery pending
  [🔄] Vector search: contract, extractor, describer, indexing, UI and Desktop sidecar done; lke-vector-search pending

Changes:
  IN_PROGRESS: console-session-auth (45/47 tasks; 7.1 and 7.2 open, waiting on the OG-27 soak)
  PENDING:     lke-agent-settings (0/9)
  PENDING:     phase-integration-gate (0/10)
  PENDING:     promethity-delivery (0/7)
  PENDING:     lke-vector-search (0/9)
  DONE:        32 others (team-ownership-agent-backend … vector-search-security-review)

Next work (derived from task state): console-session-auth, task 7.1
```

**What has to happen next:**
1. **7.1 waits on you.** It starts when you declare the OG-27 soak over. It removes the console `ui` principal's password and its token Secret. After that you delete `ipfs-webui-basicauth`, and 7.2 closes the ADR-0009 and ADR-0004 residuals. The new `rpc.prometheusags.ai` Secret (`ipfs-rpc-basicauth`) is separate and stays.
2. **The four PENDING changes can't start yet.** The adopted contract covers only console-session-auth, so they each need a contract amendment first.
3. **`lke-agent-settings` has extra prerequisites:** the 8.1 LOW-1 to LOW-5 fixes and the 8.2 changelog note (MAJOR-1).

**Not in the KBD ledger:** today's machine RPC host (know-me-cluster PR #51, merged and checked live) is outside KBD tracking. It is worth a line in `.prometheus/decisions.md`, because it reopens a public write path that is protected by a long-lived password.

Completed kbd-status — agent-backend-settings

## Root Cause

No explicit root-cause section was captured; preserve this as a session record, not an inferred diagnosis.

## Corrective Actions

Review and promote only reusable findings.

## Session Metadata

- Harness: claude-code
- Session: 43b10494-d504-43f2-9ba1-af8ce53bff62
- Captured: 2026-10-20T09:24:37.446756Z
- Project: /Volumes/T7/Projects/references/kubo

## Changed Paths

- No changed paths detected.
