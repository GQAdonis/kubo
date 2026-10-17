# Design: console-session-auth

The design of record is [ADR-0013](../../../docs/mcp/0013-console-session-auth.md). It covers the architecture, the verified sources and what is not verified, every decision with its alternatives, the off switches, the stability analysis and the uncomfortable consequences. This file adds only the ordering and the rollout mechanics the tasks depend on; it does not restate the ADR.

## Context

See `proposal.md` for the motivation, and ADR-0013 "Context" and "Verified sources" for the current state of the cluster and the code. Three facts constrain the order:

- The console's SecurityPolicy `ipfs-webui-auth` is `Prune=false` with sync-wave `-1`. Changing it in place under the same name is the only way to get a one-commit rollback without two policies on one HTTPRoute.
- Kratos registration is open. Gateway RBAC on `sub` must ship in the same commit as the OIDC filter, never after it.
- The Envoy OIDC flow fails closed if Hydra is unreachable. So Hydra, the Kratos wiring and the client must be live and checked before the console cutover.

## Goals / Non-Goals

**Goals:**

- Ship in an order where every intermediate state is no less safe than today, and the cutover is one revertible merge.
- Land `ipfs mcp` support (off by default) in the image before the cluster turns sessions on.

**Non-Goals:**

- Session auth on `ipfs.prometheusags.ai` (gateway host) or on `/mcp`. Both stay bearer-only.
- Keto or any role model in Kratos. Authorization stays an allowlist of identity IDs.
- Changing Desktop or the loopback Web UI.
- Per-write presence for admin (ADR-0013 section 5 states what step-up does and does not prove).

## Decisions

1. **Order.** The sequence is:
   1. ADR, contract and `go.mod`;
   2. the `ipfs mcp` verifier, merged with its off switch;
   3. the Web UI session mode, inert unless `/auth/session` answers;
   4. the cluster: (a) Hydra with its database and host, the Kratos `oauth2_provider` and the Kratos UI Hydra env, each verified live on its own, with no console change; (b) the NetworkPolicies; (c) the cutover PR, which changes `ipfs-webui-auth` to `oidc`, `jwt`, `authorization` and `csrf`, adds the `/oauth2/` rule, and sets the `ipfs-mcp-config` `auth.oidc` section and `subjects`; (d) after the soak, retire basic auth and the console `ui` token.
   
   The alternative of one big cluster PR was rejected: a failure in Hydra would then roll back the NetworkPolicy and the console together.
2. **Image dependency.** The cutover's `auth.oidc` section needs an `ipfs mcp` that knows it. The config schema rejects unknown keys, so an old image would fail to load the config. The cutover PR therefore requires the running image to contain the verifier: either the Round 8 candidate image is rebuilt after step 2, or the cutover waits for the final image. The preflight compares the running image digest with one known to contain the verifier commit.
3. **Allowlist agreement check.** One script reads the `sub` values in the SecurityPolicy `authorization` rule and the `subjects` of the `ui` and `admin` principals in `ipfs-mcp-config`. It fails unless every principal subject is also allowed at the gateway. The gateway list may be a superset, for someone who may use `/api/v0` but not chat.
4. **The `/oauth2/` rule.** Envoy's OAuth2 filter acts only on routes the policy targets, so the callback and logout paths need a console HTTPRoute rule under the policy. It is sent to `ipfs-api:5001`, which never receives the request because Envoy answers it.

## Risks / Trade-offs

- [The combination of `forwardIDToken` on `Authorization` with a `jwt` provider on the same header is rejected by Envoy Gateway] → a server-side dry run in the cutover PR's preflight. The fallback is the JWT provider reading the ID token from a dedicated header (`forwardIDToken.header: X-Console-Id-Token`, `jwt.extractFrom.headers`), with `ipfs mcp` reading the same header. That fallback changes the spec's carrier statement and needs the operator's approval.
- [Envoy forwards an expired ID token after a refresh] → equal Hydra TTLs, and a live check that runs one access-token lifetime after sign-in.
- [Calico does not enforce the NetworkPolicy, or it blocks kubelet probes] → the probe-pod check and a readiness check; revert the policy object on failure.
- [Kratos UI v1.3.1 fails the challenge flow against Kratos v26.2.0] → switch the UI image to `v26.2.0` in step 4a, before the console depends on it.
- [Hydra or Kratos outage locks the operator out of the console] → the break-glass section in the cutover PR body (`kubectl port-forward`, revert).

## Migration Plan

- **Forward:** steps 1 to 4 above.
- **Rollback of the cutover:** `git revert <merge-sha>` and an Argo sync. This restores `basicAuth` on `ipfs-webui-auth`, removes `auth.oidc` from the sidecar config, and needs the planned single-replica restart under the 2026-10-13 gotcha (no blocking lifecycle hooks, never force-delete). The htpasswd Secret stays until step 4d, so the rollback works throughout the soak.
- **Rollback of Hydra (4a):** revert. The Hydra database and PVCs are kept (`Prune=false`).
- **Clean-up:** browser profiles need no manual work. The Web UI clears the stored credentials itself on its first session-mode load.

## Open Questions

None that change the specs or tasks. The operator decisions are listed in ADR-0013, "Decisions left to the operator". They change values (host name, allowlist, soak length) or which admin path is live, not the work breakdown.
