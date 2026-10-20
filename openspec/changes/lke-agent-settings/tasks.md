# Tasks: lke-agent-settings

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `lke-agent-settings`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9. Route, model and prerequisites come from those rows only. Task 1.10 is new in the 2026-10-20 revision and follows the 1.1 route.

Revision 2026-10-20 (session model). Tasks 1.1, 1.2, 1.7, 1.8 and 1.9 are rewritten, and 1.10 is added. 1.3 to 1.6 are kept, with session wording where they named bearers. No task is renumbered. The source is the plan revision note of 2026-10-16 and ADR-0004's amendment of 2026-10-20 (`docs/mcp/0004-agentic-endpoints-and-llm.md`). See `proposal.md`, including open operator questions OQ-1 (admin path) and OQ-2 (image roll).

Change-level prerequisites (plan revision 3): every task in this change also requires completion of lke-origin-isolation, kubo-candidate-image, agent-features-default-on and settings-security-review; OG-12 and OG-14 before OG-13. The 2026-10-20 revision adds:

- console-session-auth (complete);
- **mcp-oidc-hardening, complete, including its 5.1 and 5.2 reviews, before 1.1 starts.**

Scheduled in Round 9. Cluster tasks follow the know-me-cluster skill `lke-gitops-change`: offline checks, a live preflight posted on the PR, then stop for the operator's merge, then verify. Agents never apply, sync, restart or read Secret values.

## 1. Tasks

- [ ] 1.1 Sidecar and config:
  - `--agui` and `--settings-dir=/mcp-settings`;
  - the `console-ui` principal (kind `ui`) is unchanged, with the same `subjects` and no `credential`, so the `sub` binding of console-session-auth is kept;
  - `auth.oidc.routes` set explicitly to exactly `["session", "agui"]`;
  - `agent.origin_allowlist` exactly `["https://console.prometheusags.ai"]`;
  - `auth.oidc.require_origin` absent or `true`;
  - `agent.settings.accept_secrets: false`;
  - `llm.api_key` as a file reference to the out-of-band LLM key Secret (S1: the operator sets the cloud key out of band, never through the browser and never in git).
  
  No `admin` principal is added, and none is given `subjects`, until the operator answers OQ-1. If the answer is OQ-1 (ii), an `admin` principal with a static `credential` from an out-of-band Secret and no `subjects` is added in this task. Every new Secret reference carries `optional: true`.
  
  Acceptance:
  - the diff shows these keys;
  - an offline check asserts `routes` equals `["session","agui"]` exactly, `origin_allowlist` equals the console origin exactly, no principal of kind `admin` has `subjects`, every new `secretKeyRef` and Secret volume has `optional: true`, and `lke-checks/session-allowlist.sh` still exits zero;
  - the absent-Secret behavior is the `kubo-candidate-image` 1.3 smoke.
- [ ] 1.2 A console rule on HTTPRoute `ipfs-webui` for `/agui`, to `ipfs-mcp:5050`, plus a streaming timeout policy targeting that rule. The rule is covered by SecurityPolicy `ipfs-webui-auth` (session: OIDC, JWT, the `sub` RBAC allowlist and CSRF). It is not bearer-only, and no rule is outside the policy. The settings route is not added to the console unless OQ-1 is answered with console admin; if it is, that is a separate amendment of this task. Acceptance: an offline check asserts all of the following.
  - `ipfs-webui-auth` targets HTTPRoute `ipfs-webui` with no `sectionName` that would exclude the `/agui` rule.
  - The `/api/v0`, `/webui`, `/oauth2/` and `/auth/session` rules are still covered.
  - No other HTTPRoute on `console.prometheusags.ai` reaches `ipfs-mcp:5050`.
  - No console rule matches the settings path.
  - The timeout policy targets the `/agui` rule.
- [ ] 1.3 Document LKE confirmation behavior (S2) in `docs/mcp/chat-setup.md`. With `confirmation.modes: []`:
  - MCP clients on `/mcp` need elicitation for writes and destructive tools;
  - chat writes from the signed-in console are confirmed by the island interrupt answered with a user gesture;
  - destructive tools from chat are refused with `confirmation_unavailable`.
  
  Acceptance: the section states all three cases, and says that the console chat authenticates through the session, not a stored token.
- [ ] 1.4 Standalone RWO PersistentVolumeClaim `ipfs-mcp-settings` (1Gi) mounted at `/mcp-settings` in the sidecar. The StatefulSet `volumeClaimTemplates` stay unchanged because they are immutable (WARNING-5). Pod `fsGroup` 1000 already applies. Acceptance: `git diff` shows a standalone PVC object and no change under `volumeClaimTemplates`.
- [ ] 1.5 Rollback and safety (R9; W9). Acceptance: the PR body has a "Rollback" section with:
  - the literal `git revert <merge-sha>`;
  - the Argo application name;
  - the settings reset (delete the overlay file on the PVC);
  - the image rollback reference;
  - the planned-restart steps under the 2026-10-13 gotcha;
  - a note that the revert leaves `ipfs-webui-auth` and `console-ui` as console-session-auth left them.
  
  `git diff` adds no `lifecycle:` key.
- [ ] 1.6 PR body file and offline render. Acceptance: render output attached; the driver pushes and opens a draft PR under OG-1.
- [ ] 1.7 Security-reviewer read of the PR diff before OG-13 (C-B). The scope includes the following.
  - `/agui` is covered by `ipfs-webui-auth`.
  - `auth.oidc.routes` and `agent.origin_allowlist` are exact.
  - No admin is reachable from the session.
  - The `mcp-oidc-hardening` 5.1 findings closed 8.1 LOW-1 to LOW-5.
  
  Acceptance: findings file; no open CRITICAL.
- [ ] 1.8 Post-merge check script `lke-checks/agent-settings.sh`, read-only. It never prints tokens, cookies or Secret values.
  - Unauthenticated part:
    - console `/api/v0/version` with `Sec-Fetch-Mode: cors` and no session is still 401 (first check);
    - a `/webui/` navigation gets 302 to `https://oauth.know-me.tools`;
    - `/agui` with `Sec-Fetch-Mode: cors` and no session is 401;
    - a POST to `/agui` with `Origin: https://ipfs.prometheusags.ai` is 403;
    - the settings path on the console gets no `ipfs-mcp` answer (404 from the gateway);
    - `/mcp` on `ipfs.prometheusags.ai` without a bearer is still 401.
  - Operator part, signed-in browser, run inside OG-13 (WARNING-4):
    - an allowlisted account signed in at `/webui/` opens chat, and `/agui` answers 503 `backend_not_configured` or runs;
    - a chat write raises the island interrupt and completes only after the gesture;
    - a destructive chat request is refused with `confirmation_unavailable`;
    - `/auth/session` still reports the `ui` principal;
    - a second, non-allowlisted Kratos account gets 403 on `/agui`;
    - `/oauth2/logout` ends the session, and `/agui` then answers 401.
  
  Acceptance: the script and an expected-results table in the PR body.
- [ ] 1.9 After OG-13, record the operator's signed-in output and run the unauthenticated part (read-only). Acceptance: a README in a new dated evidence folder marks each 1.8 check pass or fail. On any fail, the 1.5 revert was run and is recorded.
- [ ] 1.10 Image check (new, 2026-10-20). The running `ipfs` sidecar image must contain the `mcp-oidc-hardening` merge. An older image would accept JWTs with no `azp` rule or staleness cap, and its default routes would differ. Acceptance: the preflight records the running image digest and the commit it was built from, and that commit contains the `mcp-oidc-hardening` merge. If it does not, stop until the image is rolled under the gate the operator chooses for OQ-2.
