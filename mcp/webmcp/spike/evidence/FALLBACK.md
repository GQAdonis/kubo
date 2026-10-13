# Fallback evidence: MCP and MCP Apps in Codex desktop and Kimi desktop

Change `webmcp-host-spike`, task 1.4. Recorded 2026-10-09. This file is separate from the WebMCP GO criteria. MCP or MCP App support here is NOT WebMCP GO.

Labels: **documented** means an official vendor doc states it. **community** means GitHub issue reports or third-party pages. **unverified** means no source found; the operator must test it.

## Codex desktop (ChatGPT desktop app, Codex host)

| Question | Finding | Label | Source (read 2026-10-09) |
|---|---|---|---|
| MCP stdio servers | Supported, with environment variables, `cwd`, per-tool allow and deny lists, and approval modes | documented | https://learn.chatgpt.com/docs/extend/mcp |
| MCP Streamable HTTP servers | Supported, with bearer token, OAuth (CIMD and DCR), static and helper headers | documented | same |
| Where configured | ChatGPT desktop app Settings > MCP servers, `~/.codex/config.toml`, or project `.codex/config.toml` (trusted projects). The desktop app, CLI and IDE extension share this configuration | documented | same |
| Plugin-bundled MCP servers | Supported through the plugin manifest | documented | same |
| Server `instructions` field | Read at initialization; first 512 characters should be self-contained | documented | same |
| Renders MCP Apps (`ui://`, `text/html;profile=mcp-app`) | Not stated in the MCP page. History from GitHub issue openai/codex#21019: on 2026-05-04 Desktop 26.429 did not render them (gated by feature flag `enable_mcp_apps`, under development, PR #19884); one commenter reported it worked in 26.519.81530, broke in 26.527 to 26.609, and was fixed in 26.616.51431 (2026-06-21); the issue was closed as completed on 2026-08-05 by an OpenAI maintainer. A comment on 2026-05-29 said apps displayed "w/o proper CSP support" | community | https://github.com/openai/codex/issues/21019 |
| Current MCP Apps behavior and CSP enforcement in today's build | No official doc found. Test with the Kubo MCP App before relying on it | unverified | none |
| `_meta` keys Codex reads for the UI | `_meta.ui.resourceUri`, `ui/resourceUri`, `openai/outputTemplate` and `mcp_app_resource_uri` were seen in session logs | community | issue #21019 comments |
| MCP Apps in ChatGPT proper (not the Codex host) | ChatGPT developer mode supports MCP-powered apps | documented | https://help.openai.com/en/articles/12584461-developer-mode-and-mcp-apps-in-chatgpt (search excerpt only; page not read in full) |

Codex also documents WebMCP "site tools" in the built-in browser (https://learn.chatgpt.com/docs/webmcp). That is a WebMCP claim. It is not counted here, and it is not GO until the probe run in `evidence/codex/` passes the four criteria.

Implication for the fallback: stdio and Streamable HTTP are safe to target for the Kubo MCP server on Codex. Ship a text `content` result with every UI tool, since MCP Apps rendering has regressed between builds.

## Kimi desktop (Kimi Code Desktop)

| Question | Finding | Label | Source (read 2026-10-09) |
|---|---|---|---|
| MCP stdio servers | Supported (`command` entries) | documented | https://www.kimi.com/code/docs/en/kimi-code-cli/customization/mcp.html |
| MCP HTTP servers | Supported (`url` entries; headers, `bearerTokenEnvVar`, OAuth through `/mcp-config login <server>`) | documented | same |
| Legacy SSE | Supported with `transport: "sse"` | documented | same |
| Desktop uses the same MCP config as the CLI | Yes: `~/.kimi-code/mcp.json` (user) and `.kimi-code/mcp.json` (project); new sessions pick up changes, open sessions do not | documented | https://www.kimi.com/code/docs/en/kimi-code-desktop/settings-and-extensions.html |
| MCP tool naming | `mcp__<server>__<tool>`; deferred tool loading via `"deferred": true` | documented | MCP doc above |
| Built-in browser | Right panel; the agent can read, click, type, screenshot | documented | https://www.kimi.com/code/docs/en/kimi-code-desktop/built-in-browser.html |
| Renders MCP Apps (`ui://`) | No mention in the Desktop docs, the MCP doc or the changelog page (the changelog page returned no readable text; unchecked) | unverified | searched 2026-10-09; no source found |
| Kimi WebMCP / `document.modelContext` | No mention in the Desktop docs. Not counted as evidence either way | unverified | built-in-browser doc |
| Separate product "Kimi Work" (desktop agent with WebBridge browser automation) | Exists per a third-party blog; MCP and MCP Apps support not checked | unverified | https://explainx.ai/blog/kimi-work-desktop-agent-webbridge-swarm-july-2026 (search excerpt only) |

Implication for the fallback: stdio and HTTP MCP work on Kimi Code Desktop. Treat MCP App rendering there as unavailable until tested. A Kubo tool must return a complete text result.

## What the operator should test (extra, about 5 minutes per host)

After the WebMCP probe, if the Kubo MCP server exists by then, add it to each host and call one UI tool. Save a screenshot to `evidence/codex/` or `evidence/kimi/` named `fallback-mcp-app.png`, or a note that no UI rendered. This turns the unverified rows into evidence. It does not change the WebMCP verdict.
