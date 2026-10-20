# Proposal: lke-agent-settings

Phase `agent-backend-settings`, change 25 of 28 in plan revision 3, Round 9. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (C-A).

Revised 2026-10-20 to the session model. The 2026-10-16 plan revision note ("Plan revision note (2026-10-16): console-session-auth") amended 1.1, 1.2 and 1.8. ADR-0004's amendment of 2026-10-20 then required this change to keep the `sub` binding and to set `auth.oidc.routes` and `agent.origin_allowlist` explicitly. The earlier text assumed bearer-only `admin` and `ui` tokens on the console and no SecurityPolicy on the new rules. That no longer holds: `console-session-auth` is complete, and the console is session-only.

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
  
  No `admin` principal is reachable from the console session. How admin settings changes are made on LKE is open operator question OQ-1.
- 1.2: A console rule for `/agui` on HTTPRoute `ipfs-webui`, to `ipfs-mcp:5050`, covered by `ipfs-webui-auth`, plus a streaming timeout policy. The settings route is not added to the console unless OQ-1 is answered with console admin.
- 1.3: Document LKE confirmation behavior (S2) in `docs/mcp/chat-setup.md`.
- 1.4: Standalone RWO PersistentVolumeClaim `ipfs-mcp-settings` (1Gi) at `/mcp-settings`.
- 1.5: Rollback and safety (R9; W9).
- 1.6: PR body file and offline render.
- 1.7: Security-reviewer read of the PR diff before OG-13 (C-B).
- 1.8: Post-merge check script `lke-checks/agent-settings.sh`: unauthenticated checks, plus the operator's signed-in browser steps. No bearer tokens.
- 1.9: After OG-13, record the operator's signed-in output and run the unauthenticated part (read-only).
- 1.10 (new): Image check. The running sidecar image contains the `mcp-oidc-hardening` merge.

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
  - OG-12 and OG-14 before OG-13. The image roll that carries `mcp-oidc-hardening` is OQ-2.
- Size: M-L; goals: 1, 4.
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md):
  - `--agui` absent unmounts the chat;
  - removing `agui` from `auth.oidc.routes` refuses session tokens there;
  - `auth.oidc.enabled: false` turns off the verifier.

## Open operator questions

- **OQ-1, the admin path on LKE.** ADR-0013 decision 3 (answered 2026-10-17) chose option (a): console admin with TOTP or WebAuthn step-up. This revision follows the lead's direction that admin is not reachable from the console session. The operator must choose one:
  - (i) keep 3(a) for a later change, after a second factor is enabled in Kratos and enrolled; settings stay off the console until then;
  - (ii) supersede 3(a) with 3(c): settings changes only through `kubectl --context know-me port-forward` with a static `admin` bearer read from an out-of-band Secret (`optional: true`);
  - (iii) no admin principal on LKE at all, with settings changed through the config in git.
  
  Until the operator answers, 1.1 adds no `admin` principal and 1.2 adds no settings route.
- **OQ-2, the image roll.** The image that carries `mcp-oidc-hardening` must reach LKE before OG-13. Either OG-12 is re-run with a new candidate from `kubo-candidate-image`, or the image bump is folded into the OG-13 PR. The operator chooses.
