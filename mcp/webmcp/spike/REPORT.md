# WebMCP host spike report

Change `webmcp-host-spike`, task 1.5. Probe run on 2026-10-12 by following [`RUNBOOK.md`](RUNBOOK.md). The probe registers one tool, `echo`, which returns this page load's nonce. The operator asked for the run and chose the Codex model by hand. The lead (Claude Code) drove both apps through the KimiCU desktop-control MCP. Screenshots are unedited.

The probe page was served once on `http://127.0.0.1:53638/`. The server log shows only what the runbook expects: one sanity request from the lead with `curl` at 08:19 local time (status check only, no page assets), one page load per host (08:28 Codex, 08:32 Kimi), each with `probe.css`, `probe.js` and five fonts, and two `favicon.ico` 404s. There were no other requests (`evidence/*/04-server-log.txt`; the two copies are identical).

## Result

| Host | Result |
|---|---|
| Codex: ChatGPT desktop app, Codex mode (`com.openai.codex` 26.1007.30912), model GPT-6 Sol at High reasoning, built-in browser | **GO**: all four criteria met |
| Kimi: the Kimi desktop app (`Kimi.app`, `com.moonshot.kimichat` 3.2.15), Work mode, model Kimi K3, permission mode "Fully automatic", built-in browser | **GO with a caveat**: criteria 1-3 met; criterion 4 met only after one follow-up prompt |

**Product difference.** The runbook names Kimi Code Desktop as the Kimi host. That product was not used. The run used the Kimi desktop app (`Kimi.app`) in Work mode, the Kimi product installed on the operator's machine. The Kimi result applies to `Kimi.app` 3.2.15 only and says nothing about Kimi Code Desktop (`evidence/kimi/host.txt`).

## Criteria per host

| # | Criterion | Codex | Kimi |
|---|---|---|---|
| 1 | `document.modelContext` is present | Met. `evidence.json` `documentModelContextPresent: true`, `registerToolIsFunction: true`; readout "Yes, object" in `00-page-loaded.jpg` | Met. `evidence.json` `documentModelContextPresent: true`; readout in `00-page-loaded.jpg`; the address bar shows a "WebMCP" badge |
| 2 | `echo` registers | Met. `echoRegistration: "registered"` at 13:28:03.294Z; `inPageGetTools.names: ["echo"]`; "Registered" badge in `00-page-loaded.jpg` | Met. `echoRegistration: "registered"` at 13:32:55.313Z; `inPageGetTools.names: ["echo"]`; "Registered" badge |
| 3 | The host agent invokes `echo` | Met. One call at 13:29:22.175Z with message "codex probe" (`evidence.json` `calls`, `callCount: 1`); "Called 1x" in `03-page-after-call.jpg`; the agent's step list reads "Discover echo site tool", "Listed website tools", "Echo" (`02-agent-transcript.jpg`) | Met. One call at 13:34:02.489Z with message "kimi probe" (`evidence.json`); "Called 1x" in `02a-agent-transcript-first-answer.jpg`; the count stayed 1 after the follow-up (`02-agent-transcript.jpg`) |
| 4 | The agent transcript contains the nonce | Met. "The echo site tool returned nonce WMCP-CPR4-1BFY-38XM", matching `evidence.json` `nonce` (`02-agent-transcript.jpg`, `.txt`) | Met with a caveat. The first answer printed "the nonce is <untrusted>" while relaying the echoed message and call time from the same result (`02a-agent-transcript-first-answer.jpg`). After one follow-up ("Quote the exact nonce string from the echo tool's result ... Do not read it from the page or a screenshot") it quoted WMCP-VW8X-8RS4-52FQ, matching `evidence.json`, and said it took it from the tool output (`02-agent-transcript.jpg`, `.txt`) |

## Caveats and limits

- **Kimi criterion 4 rests partly on the agent's own statement.** The nonce was also visible on the page, so the transcript alone cannot prove that the second answer came from the tool result. Supporting the tool path: the first answer relayed other fields of the same result ("kimi probe", call #1, 13:34:02Z); the follow-up took 1 s with no new tool use; the call count stayed 1. Kimi masks tool output it treats as untrusted, and it masked the nonce, apparently the one field that looks like a secret.
- **The Codex site tools panel was not captured.** The address-bar control opened no visible panel (`evidence/codex/01-site-tools-list.txt`). Discovery is shown by the agent's step list and `inPageGetTools`.
- **Kimi screenshot duplication.** `evidence/kimi/03-page-after-call.jpg` is byte-identical to `02-agent-transcript.jpg`. That image does show the page after the call ("Called 1x", call log), so the runbook item is covered, but there is no separate capture.
- **Format.** The runbook asks for `.png`; the files are `.jpg`.
- **Codex ran with "Full access" on** and first ran a shell command in the operator's project before using the browser (`02-agent-transcript.jpg`). This did not affect the probe, but WebMCP and full shell access were enabled together in that session.
- **One run per host.** No repetition, no other models, no Windows or Linux.
- **The agents could see the page.** Criterion 3 does not depend on that: the page counts a call only when the host invokes `execute`.

## Implications for the tool package (`webmcp-tool-package`)

| Observation | Codex | Kimi | Implication |
|---|---|---|---|
| `navigator.modelContext` (deprecated) | Absent | Present | Use only `document.modelContext`; never detect or register through `navigator.modelContext` |
| `AbortSignal` in the `execute` options | Present (`hadSignal: true`) | Absent (`hadSignal: false`) | Treat the options argument and its `signal` as optional; a long call must not depend on host cancellation |
| `modelContext` members | `registerTool`, `getTools`, `executeTool`, `ontoolchange`, `when`, events | Same | Rely on `registerTool` only |
| Result shape | JSON object relayed | JSON object relayed, some fields masked | Return plain JSON objects (no BigInt, no cycles), as the probe did |
| Tool output as untrusted | Relayed verbatim | A secret-looking value printed as `<untrusted>` until the person asked | Return what the person needs (CIDs, names, counts) as plain, clearly named fields plus a short `summary` string the agent can relay; do not depend on the agent repeating opaque tokens. Whether Kimi masks CIDs is unverified; test it with the first real tool |
| Permissions policy | `tools` allowed | `tools` allowed | The Web UI must not send a `Permissions-Policy` that disables `tools` |
| Fallback (MCP, MCP Apps) | `evidence/FALLBACK.md` | `evidence/FALLBACK.md` | Unchanged; not WebMCP evidence |

## Evidence files

- `evidence/codex/`: `00-page-loaded.jpg`, `01-site-tools-list.txt`, `02-agent-transcript.jpg`, `02-agent-transcript.txt`, `03-page-after-call.jpg`, `04-server-log.txt`, `evidence.json`, `host.txt`
- `evidence/kimi/`: `00-page-loaded.jpg`, `01-site-tools-list.txt`, `02a-agent-transcript-first-answer.jpg`, `02-agent-transcript.jpg`, `02-agent-transcript.txt`, `03-page-after-call.jpg`, `04-server-log.txt`, `evidence.json`, `host.txt`
- `evidence/FALLBACK.md` (task 1.4; MCP and MCP App support, not part of GO)
