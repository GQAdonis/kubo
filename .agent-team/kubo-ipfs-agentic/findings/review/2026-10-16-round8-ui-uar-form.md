# UI review: Round 8, phase `agent-backend-settings`, UAR backend form

Reviewer: `reviewer` (Claude Opus), separate read-only context, `prometheus-ui-review` loaded from SKILL.md. Static review (Read, Grep, Glob only); no Jest, eslint, axe, browser or captures; contrast, both themes and widths not verified. Lead-reported: Jest 374/374, eslint clean. Written to disk by the lead. Scope: uncommitted working tree of `ipfs-webui` (`feat/ipfs-mcp-server`).

**Verdict: BLOCK.** Contract fit correct, important absences tested, no mocked UAR state. Two HIGH copy findings misstate where the UAR credential goes (same class as Round 6 N1).

## Contract fit (clean)

`buildSetRequest` sends `model` only for openai (`core/form.js:314`; asserted `AgentSettingsPage.test.js:604-608`, `core.test.js:104-108`). Test-connection sends `{base_url, kind:'uar'}` only (`AgentSettings.js:109`; `:567`). Key save and clear carry `kind:'uar'` (`AgentSettings.js:124,141`). Kind switching restores the node's values for its own kind and starts empty for the other; key, test and confirmation reset (`form.js:96,164-168`; `core.test.js:111-128`, `AgentSettingsPage.test.js:548-554`). Typed host confirmation applies to UAR (`form.js:260`; `:592-612`, `:623-640`). Server detail shown on errors (`UarBackendForm.js:68`, `ApplyStatus.js:42`).

## Findings

- **H1 HIGH:** every test of the current UAR backend with a stored credential reads "Reachable, key not sent ... this instance is not it" (`UarBackendForm.js:58-60`; `agentSettings.json:140,143,165`). `probeUAR` never sets `key_sent` (kubo `mcp/server/settings_probe.go:339-349`), so the sentence is false; `test.keyNotSentServer` too; `uar.test.noKey` implies a stored credential would be sent. Fix: for UAR drop the `keyStored`/`keySent` branch, tone ok, "The test reads the instance's public agent card and sends no credential."; test with `SERVER(UAR)` + `{ok:true}` asserting "Reachable" and `queryByText(/this instance is not it/)` null.
- **H2 HIGH:** "the credential stays on the node" is false (`agentSettings.json:173`, rendered `UarBackendForm.js:78`); the node sends it as `Authorization: Bearer` (`mcp/server/uar_client.go:94`). Fix: "The node keeps the credential in a file only it can read and sends it only to this instance, to create and run the agent." plus a Jest assertion.
- **M1 MEDIUM:** two terms for one secret: UAR strings say "credential", the shared field, buttons, confirmation help and "Key not sent" say "API key"/"key" (`ApiKeyField.js:69,81,89,94`, `ConfirmHost.js:51`, `ApplyStatus.js:58`). Pick one or give the field a kind-specific label.
- **M2 MEDIUM:** OpenAI-specific copy in the UAR flow: `url.error.empty` (`:77`), `apply.blocked.url` (`:226`, asserted `:530`), `apply.dirty.text` "backend, URL and model" (`:222`), `confirm.label` "model host" (`:178`), `firstRun.step2` (`:64-65`), `apply.firstRun.text` (`:232`). Fix: kind-neutral strings or `uar.*` variants.
- **M3 MEDIUM:** the runtime lead "The node could not set up the runtime" is shown for `network` and `protocol` failures, which are not runtime failures (`ApplyStatus.js:11`). Limit `RUNTIME_FAILURES` to `http` and `backend_not_configured`.
- **L1 LOW:** `gatesTest` holds back Test connection on a UAR host move although the UAR probe never carries the key (`form.js:267`); gate only openai, or document why.
- **L2 LOW (optional):** duplicated test block `UarBackendForm.js:50-74` vs `OpenAiBackendForm.js:59-77`; a shared `TestConnection` would have prevented H1 drift.
- **Nit:** failed UAR test prefixes "Detail:", OpenAI shows it bare (`UarBackendForm.js:68` vs `OpenAiBackendForm.js:74`).

## Copy, accessibility, visual (static)

Sentence case, no exclamation marks, no unqualified banned words. Field order matches the OpenAI form; URL errors tied to the field (`aria-invalid`, `:544`); test note via `aria-describedby` (`:51`); results in a polite live region; axe in both themes over empty, tested, unreachable and failed (`:971-991`) but with `color-contrast` disabled. CSS adds only token values (`agent-settings.css:130-136`). Not verified: 320/768/1440, both themes visually, keyboard pass, reduced motion, real contrast.

## Acceptance

`agent-backend-uar` 1.6 "no mocked UAR state in production code": met (grep of `src/` outside tests finds none; every UAR state comes from the node or a request the page made). 1.6 as a whole not accepted until H1 and H2 are fixed and asserted; M1-M3 recommended in the same batch. One batched correction and confirmation cycle.
