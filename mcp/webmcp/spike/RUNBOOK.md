# WebMCP host probe: operator runbook

Change `webmcp-host-spike`, tasks 1.2 (Codex desktop) and 1.3 (Kimi desktop). You run the probe page in each host and save evidence. The probe makes no network requests and makes no IPFS calls.

Draft spec: https://webmachinelearning.github.io/webmcp/ (Draft Community Group Report, 8 October 2026; read 2026-10-09). The probe calls `document.modelContext.registerTool({name, title, description, inputSchema, execute, annotations})`.

## GO criteria (per host, all four required)

1. `document.modelContext` is present.
2. `echo` registers (the page shows a green "Registered" badge).
3. The host agent invokes `echo` (the page shows "Called" and logs the call).
4. The agent transcript contains the nonce shown on the page.

MCP or MCP App support in a host is fallback evidence only and is not WebMCP GO. Record it in `evidence/FALLBACK.md`, not here.

## 1. Serve the page

Use any free port. `http://127.0.0.1` is a secure context, so `document.modelContext` is exposed there. Do not use a LAN IP or a hostname other than `127.0.0.1` or `localhost`.

```bash
cd mcp/webmcp/spike
PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1])')
echo "http://127.0.0.1:$PORT/"
python3 -m http.server "$PORT" --bind 127.0.0.1
```

Keep the terminal open. The server log lists every request. Expected: `index.html`, `probe.css`, `probe.js` and five `.woff2` files, nothing else.

Sanity check in desktop Chrome first: open the URL. The readout renders. In a Chrome without WebMCP the badge reads "Not registered" and "document.modelContext present: No". That is a correct result for Chrome; it only proves the page works.

## 2. Codex desktop (task 1.2)

The WebMCP-capable surface is the ChatGPT desktop app's built-in browser, which Codex and ChatGPT Work use. It is not Codex CLI or the IDE extension.

Prerequisites, from OpenAI docs (read 2026-10-09):

- Latest ChatGPT desktop app. Source: https://learn.chatgpt.com/docs/webmcp
- Model GPT-5.6 Sol or GPT-6 Sol. GPT-5.6 Luna has WebMCP disabled. Source: same page.
- Settings > Browser > Permissions > "Enable site tools" is on. Source: same page.
- Enterprise workspace: an admin must allow Site tools. Edu workspaces do not have it. Source: same page.
- Availability depends on rollout, so the account may not have it yet.

Steps:

1. Open the ChatGPT desktop app and start or open a Codex chat. Select the model above.
2. Open the built-in browser: select New tab then enter the URL, or press `Cmd+T` (`Ctrl+T` on Windows). Source: https://learn.chatgpt.com/docs/browser. The page loads as a top-level page. The docs show local URLs such as `http://127.0.0.1:4173/` working in this browser.
3. Look in the address bar for a "Site tools" control. If present, open it and choose "Available site tools". Screenshot it. If `echo` is listed, criterion 2 is probably met. The page's own badge is the proof. (Source for the control: https://learn.chatgpt.com/docs/webmcp.)
4. In the chat, enter exactly:
   `Use @Browser. In the current browser tab, call the echo site tool with message "codex probe" and tell me the nonce it returns.`
   If the agent answers without calling the tool, enter: `Call the echo tool, not the page text. Report the nonce from the tool result.`
5. Wait for the agent. Approve any safety or site-access prompt that appears and screenshot it. A denied prompt is a result; record it.
6. Screenshot the page after the call. It must show the nonce, "Registered", "Called 1x" and the log row.
7. Click "Copy evidence JSON" and paste it into `evidence/codex/evidence.json`. If the copy is blocked, select the text in the Evidence JSON box and copy it by hand.

Known behavior, from an independent third-party snapshot (https://docs.mcp-b.ai/reference/webmcp/codex-site-tools, tested 2026-08-27, not an OpenAI statement): top-level imperative tools are discovered; tools in iframes and declarative tools are not; `navigator.modelContext` is undefined; callback options may lack an `AbortSignal`; results should be JSON objects. The probe is built to that.

Save to `mcp/webmcp/spike/evidence/codex/`:

| File | Content |
|---|---|
| `01-site-tools-list.png` | Address-bar Site tools panel (or a note that it is absent) |
| `02-agent-transcript.png` | Chat showing the agent calling `echo` and quoting the nonce |
| `02-agent-transcript.txt` | The same transcript as text, copied from the chat |
| `03-page-after-call.png` | Full probe page: nonce, badges, call log |
| `04-server-log.txt` | The terminal output of the `http.server` |
| `evidence.json` | Evidence JSON from the page |
| `host.txt` | ChatGPT desktop app version (Settings > About or app menu), model name, OS, date, account plan type |

If the Site tools control is missing and the badge says "Not registered", still save files 03, 04, evidence.json and host.txt. That is a valid NO-GO result.

## 3. Kimi desktop (task 1.3)

The Kimi desktop app with an agent and an embedded browser is Kimi Code Desktop. Source: https://www.kimi.com/code/docs/en/kimi-code-desktop/built-in-browser.html (read 2026-10-09). There is also a separate "Kimi Work" desktop agent. Unverified: whether Kimi Work embeds a browser. If Kimi Code Desktop is not what you have, record which product and version you used in `host.txt`.

Documented: the built-in browser lives in the right panel; open it with "+" (New tab) then Browser, or File > New Browser Tab; type a URL in the address bar; the Agent can read page text, list elements, take screenshots, click and type. The docs tell you to start the local service first and confirm the actual URL and port. Unverified: that Kimi Code Desktop exposes `document.modelContext` or any WebMCP API. The docs read on 2026-10-09 do not mention WebMCP. The result of this run is the evidence.

Steps:

1. Open Kimi Code Desktop and open a session. Check the version in Settings > About.
2. In the right panel choose "+" > Browser. Enter the probe URL.
3. Screenshot the page. The badges and "document.modelContext present" row show criteria 1 and 2 directly.
4. In the session Composer, enter exactly:
   `Open http://127.0.0.1:PORT/ in the built-in browser. Call the echo tool that the page registers with message "kimi probe" and tell me the nonce. If the page offers no echo tool, say so and do not read the nonce off the screen.`
   (Replace PORT.) The last sentence matters. An agent that reads the nonce from a screenshot does not satisfy criterion 3. If the agent says it read the nonce from the page text instead of calling a tool, record that as "criterion 3 not met".
5. Approve or deny permission prompts as they come. Screenshot them.
6. Screenshot the page after the exchange (nonce, badges, call log). Copy the evidence JSON.

Save to `mcp/webmcp/spike/evidence/kimi/` with the same file names as the Codex table. `host.txt` records the Kimi Code Desktop version, model, OS, date and plan.

## 4. After both runs

Tell the orchestrator both evidence folders are filled. Do not write `REPORT.md` yourself; the GO/NO-GO report (task 1.5) is written against the saved files.

## Rules for the evidence

- Do not edit the screenshots or transcripts.
- Do not crop out the nonce, the badge row or the URL bar.
- Keep one nonce per run. Reloading the page issues a new nonce; if you reload, redo the call and save the new files.
- Never paste credentials or tokens. Blur an account email in screenshots if you want, and note that you did.
