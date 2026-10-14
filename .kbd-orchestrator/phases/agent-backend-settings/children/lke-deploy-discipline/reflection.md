# Reflection — agent-backend-settings › lke-deploy-discipline

Date: 2026-10-14. Author: the lead (Claude Code).

## Delta between plan and delivery

| Planned | Delivered |
|---|---|
| Skill `lke-gitops-change` with preflight, verify, inventory; validated; live self-test that fails on the broken console | Delivered and merged (know-me-cluster #28). The first version failed an independent review (permanently red preflight, redundant certificate as INFO, HTTP-01 check by pattern, kubectl errors as silent PASS); rewritten before merge and tested against synthetic reproductions of the #24, #24 (DNS-01) and #26 failures, each failing for the right reason |
| PR-A: console on the wildcard; PR-B: remove the HTTP-01 Issuer and port-80 listeners after ACME clean-up | PR-A (#29) and PR-B (#31) merged, each with a live preflight PASS posted before the merge and a verify PASS after. Between them, Let's Encrypt issued `console-tls` through #27 and the operator first chose to keep it; #29 was merged at the same moment the lead tried to close it. The lead reported it as closed; corrected afterwards |
| OG-6 restart only after verify passes and with no PR open or unsynced; bounded; stop and hand over on a hang | Restart ran 10:41:44Z-10:42:17Z (33 s). It overlapped #29's Argo sync: the pre-restart snapshot read Argo eleven seconds before the merge reached it. Post-restart verify PASS after one transient timeout; unauthenticated isolation checks PASS |
| Gates G-2 (strip the key annotation), G-3 (rotation and renewal owner), G-4 (orphan cleanup) | Open; filed as know-me-cluster #30 for the operator |

## What the live checks found that offline work had not

- `prometheusags.ai` is on GoDaddy DNS and `know-me.tools` on Cloudflare; only the latter can use DNS-01.
- A ZeroSSL wildcard `*.prometheusags.ai` (valid until 2027-02-13) is copied into three namespaces and is renewed by nobody.
- The wildcard Secrets carry their private key in `last-applied-configuration`.
- Two ClusterIssuers reference gateways that do not exist.
- Envoy isolates hosts that share a certificate and an address: a console Host on an ipfs SNI connection gets 404.

## Lessons

- An offline render and an artifact-only review cannot see live state. The lead must run a read-only live check and post it before asking for a merge. Now enforced by the skill and recorded in `.prometheus/gotchas.md` and lead memory.
- Checks that config exists and matches are not checks that it works. The skill pairs a preflight with a post-sync verify that fetches the hosts from outside.
- A gate that is always red teaches people to ignore it. Scope FAILs to what the change touches.
- Read merge state immediately before any cluster mutation, not only in a snapshot taken earlier. The restart overlapped a merge by seconds. Follow-up: add a "re-check open and just-merged PRs and Argo revision immediately before the restart command" step to the skill's restart section.
- Reporting an action as done from the command's output, without checking the resulting state, produced a false report (#29 "closed"). Verify the state after an outward action.
- `kbd-apply mark-done` resolves tasks against the active phase: while this child was active it registered a stray `webui-promethity-foundation` entry inside the child phase and updated only the parent's projection. Typed `prometheus kbd task transition --phase <id>` calls are the reliable path while a child is active.

## Handoff to the parent

- Every later cluster change (`lke-agent-settings`, `lke-vector-search`, the redirect PR) must use `.claude/skills/lke-gitops-change/` in know-me-cluster: preflight posted before merge, verify posted after sync.
- `lke-origin-isolation` 1.8: the unauthenticated part is evidenced here (`evidence/origin-isolation-unauth-*.txt`); the credentialed part is the operator's.
- Operator items: delete Secrets `ipfs/console-tls` and `ipfs/letsencrypt-http01-ipfs-account`; decide on the key annotation and wildcard renewal (#30).
