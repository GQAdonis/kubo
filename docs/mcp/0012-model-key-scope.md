# ADR-0012: Which hosts may receive the stored model key

## Status

**Accepted**, 2026-10-15. Operator, verbatim: "accept ADR-0012". Proposed the same day by `ipfs-agent-architect`. The open points take the recommended values: (1) on a host move a key stored through the API is cleared and a key supplied by reference stays as configured; (2) no config-declared key-host exception; (3) server, contract and both clients change in the same release. **Amended 2026-10-15** (operator, verbatim: "Let it follow, document it"): a key supplied by reference (environment variable, mounted file, flag) follows a backend host move; only a key stored through the API is withheld. The LKE case is an accepted residual (see "Accepted residual: the LKE case"). Implementation is tracked as `mcp-agent-settings-api`, `webui-agent-settings-page` and `desktop-agent-settings-bridge` follow-up tasks.

## Date

2026-10-15

## Context

The settings API (ADR-0009) stores the model API key write-only: through the settings API as a 0600 file in the settings directory, or by reference to an environment variable or file (a Kubernetes Secret mount on LKE). The key never appears in a read response. The question here is narrower: **which hosts is `ipfs mcp` willing to send that stored key to**, and under what confirmation.

Findings that raise it (all LOW, none a new exfiltration path on its own):

- **N3** (Web UI and Desktop confirm review, `.agent-team/kubo-ipfs-agentic/findings/security/2026-10-14-round5-webui-desktop-confirm.md`): "acknowledged host" now also means "may receive the model key". The probe (`probeKey` in `mcp/server/settings_probe.go`) sends the stored key to the current backend's host and port, or to any host covered by the calling principal's effective `egress_ack`, which includes hosts added by past confirmed writes and kept indefinitely. Three consequences: a once-acknowledged host keeps receiving the key on probes; `egress_ack: ["local"]` covers any loopback port, and `settings_probe_test.go` asserts it; an acknowledgement of provider B granted for content egress also lets provider A's key reach B.
- **R1** (same review, accepted residual): on the kubo-served Web UI the admin bearer can supply its own `egress_confirmation`, so the typed host in the page is renderer UX, not a server-enforced second factor. Desktop adds a main-process gate. The review asks whether a base-URL host change should withhold the stored key unless the key is re-supplied.
- **N2** (Go confirm review, `.agent-team/kubo-ipfs-agentic/findings/security/2026-10-14-round5-go-confirm.md`): a confirmed backend host move also carries the stored key (`mcp/server/settings.go` keeps the key on a `base_url` change), the D4 confirmation text mentions node content only, and the host stays in the persisted `EgressAck`, so later probes keep sending the key. Marked intentional: the admin bearer is the root of trust.

### Current behavior

A settings write that changes `backend.base_url` keeps the stored key. A test-connection or model-list probe sends the stored key only to the current backend's exact host:port or to an acknowledged host; for any other candidate host it sends no key and reports `key_sent: false`, and a 401 or 403 from that host is reported as reachable-but-key-not-sent. The D4 confirmation (typed host, states node content will go there) does not mention the key. ADR-0009 residual 1 already accepts that an `admin` bearer holder can repoint the backend; this ADR is about whether the key must follow.

### Stakes

- **Impact of a misdirected key:** whoever runs the receiving host can use the key against the provider it belongs to, at the owner's expense and within the key's scope, until it is rotated. The key cannot be read back through this API, so a leak is detectable only at the provider.
- **Who can cause it:** the `admin` bearer holder (root of trust by ADR-0009), or a person (or a browser extension or injected script acting in the Web UI) who types a wrong or stale host in a page the same holder runs. On LKE the cloud key lives in a Secret with `accept_secrets: false`, so the key is mounted by reference and an admin bearer holder who moves `base_url` sends a key they never held. That is the sharpest case, it exists today, and under the 2026-10-15 ruling it stays: a referenced key follows the move (see "Accepted residual: the LKE case").
- **Who pays for protection:** operators who legitimately switch hosts (for example Ollama on port 11434 to LM Studio on 1234, or provider A to provider B) and the clients that implement the flow.
- **Cannot be fixed by documentation alone:** a model key is a bearer secret; nothing downstream of the send can retract it.

## Options

### Current: key goes to the current backend or any acknowledged host; `local` covers any loopback port

- Pro: no change; probes of every previously used host just work; legitimate moves keep the key.
- Con: N3 and N2 stay. Acknowledgements granted for one purpose (content egress to a host) widen into a second purpose (key delivery) without the operator being told.

### Option A: the key goes only to the current backend's host:port

`probeKey` ignores `egress_ack` entirely; the key goes to the exact host and port of the current backend and nowhere else. `local` stops covering "any loopback port" for key purposes.

- Closes N3 entirely: past acknowledgements, provider B, and the wildcard loopback port stop receiving the key. Smallest change (one condition in `probeKey`, tests updated; `settings_probe_test.go:283`'s assertion flips).
- User impact is small: a candidate host other than the current backend already gets a keyless probe today, with `key_sent: false` and an honest 401/403 explanation; Option A only removes the acknowledged-host exceptions.
- Does **not** close N2 or R1: after a confirmed move the new host becomes the current backend and the stored key goes with it. A is necessary for N3, not sufficient for N2.

### Option B: withhold the stored key on a base-URL host change unless the same write carries `api_key`

When a settings write changes the backend host or port (scheme, host, port; a change of path or model alone does not count), the stored key is not applied to the new backend. The write succeeds with the key left unset for the new host, unless the same request carries a new `api_key`. The key stays in the settings directory for the old host if the operator moves back (or is cleared, a choice to make when B is accepted).

- Closes N2 and R1 at the server, where it is enforceable: moving the backend cannot, by itself, deliver an existing key to the new host. The renderer-only typed-host check stops being the only barrier.
- Cost for stored-by-API keys: after a host move the operator re-enters the key, which is the right moment to read the confirmation again. Cost for the loopback case: a local model usually needs no key, so nothing changes in practice.
- **Cost on LKE, as first proposed and since overridden:** the original proposal also withheld a referenced key, so on a node with `accept_secrets: false` a host move through the API would have left the backend keyless and a provider change would have needed a config edit and a pod roll. The operator ruled against that on 2026-10-15 (see Decision): a referenced key follows the move, B withholds only a key stored through the API, and on LKE a provider change through the API succeeds with the mounted key attached to the new host.
- Needs an additive contract change: the write response (and the read shape) must tell the client the key was not carried (for example `api_key: {set: false}` plus a stable detail), and error or warning copy in both clients. No existing field changes meaning.
- Does **not** close N3 on its own: probes still reach acknowledged hosts with the key. B without A leaves the acknowledgement widening.

### Option C: disclose the key in the confirmation copy only

The D4 confirmation (server text, Web UI, Desktop) states that the stored key will also be sent to the new host.

- Cheapest; no behavior change; makes the intentional behavior visible, which N2 already concedes is missing.
- Does not stop anything. The `admin` bearer can supply the confirmation itself (R1), so on the kubo-served Web UI the disclosure is renderer UX; on Desktop it is a main-process gate but still a person clicking through. It also leaves N3, which is about hosts the person never sees in that dialog.

## Decision

Adopt **A and B together, and add C's disclosure as the copy of B's confirmation**.

- A removes the acknowledged-host and any-loopback-port exceptions (N3).
- B makes a host move not carry a key stored through the API unless the same write supplies one (N2, R1), enforced in the server where the admin bearer cannot turn it into a UI choice. A key supplied by reference (environment variable, mounted file, flag) is not an API-stored secret and follows the move as configured (operator ruling, 2026-10-15).
- The confirmation copy for a backend move says what is actually true: node content goes to the new host; a key stored through the API does not follow unless you supply a new one; a key supplied by environment variable or file does.

### Accepted residual: the LKE case

On a node whose key is mounted by reference (LKE: a Kubernetes Secret with `accept_secrets: false`), an `admin` bearer holder who moves `backend.base_url` through the API sends the mounted cloud key to the new host, a key that holder never held. The operator accepted this on 2026-10-15 ("Let it follow, document it"). Reasoning:

- The `admin` bearer is the root of trust (ADR-0009). That holder can already repoint the backend, add principals and rewrite egress acknowledgements; withholding one key does not change who controls the node.
- A referenced key is the operator's own deployment choice. Withholding it would make a provider change on the cloud node a config edit and a pod roll, at the cost of the settings page being the wrong tool there, to defend against a holder who has every other lever.
- Desktop, the case with a person at the keyboard, stores its key through the API and deletes its own key before a move, so the case that matters on a laptop is closed by B.
- Operators who do not want the mounted key to follow a move keep the admin bearer scarce, or run with `agent.settings.enabled: false` so the route does not exist. This is documented in `docs/mcp/agentic.md` and `docs/mcp/chat-setup.md`.

Why not A alone: it leaves a move carrying a key stored through the API, which B closes. Why not B alone: probes keep sending the key to every host ever acknowledged. Why not C alone: it documents the exposure without removing it and is defeated by the same bearer R1 describes.

Open points (resolved):

1. On a host move under B, is the old key **retained** for a later move back, or **cleared**? Retained is more convenient and keeps a secret around for a host that is no longer used; cleared is safer and costs one re-entry. The recommendation is cleared for a key stored through the API; a key supplied by reference follows the move (the file or variable is the operator's), per the 2026-10-15 ruling.
2. Should the cloud node get a config-declared exception (for example a `llm.key_hosts` list naming hosts that may receive the referenced key) so a provider change does not need a config edit? It would be a new config key with its own default and off switch and is not part of this recommendation.
3. Both clients (Web UI, Desktop) and the contract change in the same release, or the server enforces first and the clients catch up with honest copy.

## Consequences if accepted

- Contract (defined 2026-10-15, `mcp/contract/settings/`), all additive and optional, no existing field changes meaning:
  - Read side: no new field. After a move the client learns the key state from the existing `backend.api_key`: `{set: false}` (the API-stored key was cleared) or `{set: true, source: "environment"|"file"}` (a referenced key follows the move).
  - Write response (the read shape): optional top-level `key_not_carried: {reason: "host_changed", cleared: boolean}` in `settings-read.schema.json` (`$defs/KeyNotCarried`). Present only in the PUT response when a change of backend scheme, host or port, without `api_key` in the same request, withheld a key stored through the API. `cleared` is true when that key was deleted. Absent on GET, on writes that do not move the host, when the same write supplies `api_key`, and for a referenced key.
  - `test-connection` `key_sent` and the `model-list` description: the key is attached only when the probed host:port is the current backend's.
- Server: `probeKey` loses its `egress_ack` branch; `apply` stops carrying the key across a host change; tests for the three cases of N3, the move (with and without `api_key` in the same write) and the LKE `accept_secrets: false` case.
- Web UI and Desktop: copy and one new state ("key not sent to the new host"), test names from the settings page and the Desktop settings service; the Desktop main-process gate stays as defense in depth.
- Docs: `docs/mcp/chat-setup.md` and the agentic guide state that an acknowledged host does not receive the key.
- Phase effect: a LOW finding closed; if not accepted, the three findings stay recorded as accepted residuals with this ADR as the reasoning.

## Alternatives not pursued

- Sending the key only over HTTPS: already enforced for remote hosts (`TestKeyNotSentOverPlainHTTPToRemoteHost`); it limits eavesdropping, not the choice of recipient.
- Per-key host binding stored beside the key (key bound to the host it was entered for): equivalent to B with a persistent record; more state for the same effect, revisit if open point 1 moves toward "retained".

## Amendment (2026-10-16, Round 8: a backend kind change is a move)

Lead ruling after the Round 8 security read (U1, U2). Rule B was written for a change of host. A change of `backend.kind` (`openai` to `uar` or back) is a move too, because the new kind sends the node's content and a credential to a backend the earlier confirmation did not cover: a write of `{"backend":{"kind":"uar"}}` with no host change would otherwise send the API-stored OpenAI key, or the `uar` principal's grant, to a host confirmed only as a model host.

- **Typed confirmation.** A kind change needs the typed `egress_confirmation` even when the host is unchanged. The server enforces it; a client that skips it gets the confirmation error.
- **No key crosses kinds.** No stored key, whether stored through the API or supplied by reference, is carried across a change of kind. An OpenAI key is not a UAR trusted-host JWT, and the reverse. The new kind starts without a key until the same write supplies one.
- **`key_not_carried` is unchanged.** The write response reports a withheld API-stored key with `reason: "host_changed"` for a kind change as well. No contract change this round (lead ruling); a client that wants a kind-specific message reads `backend.kind` from the same response. A `reason` of `kind_changed` would be an additive value and is a follow-up if a client needs it.
- **A key given by a run flag still follows.** A key given by a run flag is the operator's own deployment choice and still follows the move as configured (the 2026-10-15 ruling); the confirmation text names it, as it names a referenced key on a host move.
- **Save key and Remove key never change the kind.** They act on the current backend only (U1). The kind changes only through Apply.
- **Referenced keys are reported too (fixed in `agent-backend-uar` 1.7).** A kind change reports `key_not_carried` with reason `host_changed` for a config-referenced key as well as an API-stored one, so the client is told in both cases. (Before 1.7 the referenced case dropped the key silently, finding N3 of the confirmation read.)
