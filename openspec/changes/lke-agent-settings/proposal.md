# Proposal: lke-agent-settings

Phase `agent-backend-settings`, change 25 of 28 in plan revision 3, Round 9. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (C-A).

Revised 2026-10-20 to the session model, then revised again on 2026-10-20 with the operator's OQ-1 and OQ-2 rulings (see "Operator rulings" below). The 2026-10-16 plan revision note ("Plan revision note (2026-10-16): console-session-auth") amended 1.1, 1.2 and 1.8. ADR-0004's amendment of 2026-10-20 then required this change to keep the `sub` binding and to set `auth.oidc.routes` and `agent.origin_allowlist` explicitly. The earlier text assumed bearer-only `admin` and `ui` tokens on the console and no SecurityPolicy on the new rules. That no longer holds: `console-session-auth` is complete, and the console is session-only.

## Why

The candidate image can serve `/agui` on the console host. The console is now protected by the session policy `ipfs-webui-auth`: Envoy OIDC through Hydra, the ID token forwarded as `Authorization: Bearer`, a JWT check, an RBAC allowlist on `sub`, and CSRF. `ipfs mcp` verifies that token and maps its `sub` to the `console-ui` principal. Today the only `ipfs-mcp` path on the console is `/auth/session`. This change adds the chat route under the same policy, with out-of-band Secrets for the LLM key and a standalone settings PVC.

## What Changes

- 1.1: Sidecar and config:
  - `--agui` and `--settings-dir=/mcp-settings`;
  - the existing `console-ui` principal (kind `ui`), unchanged: `subjects` only, no `credential`;
  - `auth.oidc.routes` exactly `["session", "agui"]`;
  - `agent.origin_allowlist` exactly `["https://console.prometheusags.ai"]`, with `auth.oidc.require_origin` left at its default, true;
  - `agent.settings.accept_secrets: false`;
  - `llm.api_key` as a file reference to the out-of-band LLM key Secret (S1).
  
  - an `admin` principal (OQ-1 ruling, "Port-forward only") with a static `credential` read from a file in an out-of-band Secret mounted with `optional: true`, and no `subjects`. It is reachable only through `kubectl port-forward` to the `ipfs-mcp` port of the pod; it is never routed on the console, and no session identity can select it.
  
  No `admin` principal is reachable from the console session.
- 1.2: A console rule for `/agui` on HTTPRoute `ipfs-webui`, to `ipfs-mcp:5050`, covered by `ipfs-webui-auth`, plus a streaming timeout policy. No settings route is added to the console (OQ-1 ruling).
- 1.3: Document LKE confirmation behavior (S2) and the port-forward admin path in `docs/mcp/chat-setup.md`.
- 1.4: Standalone RWO PersistentVolumeClaim `ipfs-mcp-settings` (1Gi) at `/mcp-settings`.
- 1.5: Rollback and safety (R9; W9).
- 1.6: PR body file and offline render.
- 1.7: Security-reviewer read of the PR diff before OG-13 (C-B).
- 1.8: Post-merge check script `lke-checks/agent-settings.sh`: unauthenticated checks, plus the operator's signed-in browser steps and the operator's port-forward admin steps. The script prints no bearer token.
- 1.9: After OG-13, record the operator's signed-in output and run the unauthenticated part (read-only).
- 1.10 (new): Image check. The running sidecar image contains the `mcp-oidc-hardening` merge. Per the OQ-2 ruling, the image bump is part of the OG-13 PR.

Full task text and acceptance checks are in `tasks.md`. Code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: this is know-me-cluster deployment configuration outside this repository, and no kubo behavior changes here. The `ipfs mcp` behavior it relies on is specified by `console-session-auth` and `mcp-oidc-hardening`.

### Modified Capabilities

None.

## Impact

- Owner: ipfs-agent-architect.
- Scope: know-me-cluster feature branch, `namespaces/ipfs/**`, `docs/mcp/chat-setup.md`.
- Depends on:
  - lke-origin-isolation, kubo-candidate-image, agent-features-default-on, settings-security-review;
  - console-session-auth (complete);
  - **mcp-oidc-hardening, which must be complete first.** Its 8.1 LOW gates (azp, the staleness cap, per-subject binding, the `session`-only routes default, the safeguard tests) are the precondition for accepting session tokens on `/agui`.
  - OG-12 and OG-14 before OG-13. The image that carries `mcp-oidc-hardening` is bumped in the OG-13 PR itself (OQ-2 ruling), with one planned `ipfs-0` restart.
- Size: M-L; goals: 1, 4.
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md):
  - `--agui` absent unmounts the chat;
  - removing `agui` from `auth.oidc.routes` refuses session tokens there;
  - `auth.oidc.enabled: false` turns off the verifier;
  - removing the `admin` principal (or its Secret, which is `optional: true`) unmounts the settings route (404).

## Operator rulings (2026-10-20)

Both former open questions are answered. The operator's words are quoted verbatim from `.prometheus/decisions.md` (entry "2026-10-20: mcp-oidc-hardening and lke-agent-settings operator rulings"); ADR-0013 records them in its amendment of 2026-10-20.

- **OQ-1, the admin path on LKE.** Operator: "Port-forward only". This is the former option (ii): ADR-0013 decision 3 moves from (a) to (c), superseding the 2026-10-17 answer (a). Admin settings changes on the cloud node go only through `kubectl --context know-me port-forward` to the `ipfs-mcp` port of the pod, with a static `admin` bearer read from an out-of-band Secret (`optional: true`). No admin principal is reachable through the console session: the `admin` principal has no `subjects`, and no console rule routes the settings path. Consequences for this change:
  - 1.1 adds the `admin` principal with a file `credential` from the out-of-band Secret and no `subjects`;
  - 1.2 adds no settings route on the console;
  - 1.3 documents the port-forward procedure;
  - 1.8 checks that the console never answers the settings path, and adds the operator's port-forward steps.
  
  With the Secret absent, the `admin` credential does not resolve; per the `principals` off switch, startup continues with the principals that did resolve, so the chat and `/auth/session` keep working. What the settings route answers in that state (404 or 401) is not pinned by any test read so far; 1.1 records the observed answer from the `kubo-candidate-image` smoke or the preflight, and does not assume it.
- **OQ-2, the image roll.** Operator: "Fold into OG-13 PR". The image that carries `mcp-oidc-hardening` ships in the same know-me-cluster PR that adds `/agui`, with one planned `ipfs-0` restart. OG-12 is not re-run for it. Task 1.10 checks the image in that PR's preflight.
