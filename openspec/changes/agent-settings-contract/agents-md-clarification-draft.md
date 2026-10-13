# Draft: D12 AGENTS.md clarification for OG-5

Phase `agent-backend-settings`, change `agent-settings-contract`, task 1.6. This is the draft the operator files as a standalone AGENTS.md PR at gate OG-5 (plan revision 3, section 2). It is a draft only: this change does not edit `AGENTS.md`. Per the Scope and Safety rules, an `AGENTS.md` change is always its own PR, signed off by the kubo-maintainers team, with no code changes in it.

Decision context: D12 (plan revision 3, section 1), round-2 finding S5 (section 9), and ADR-0009 section 9 (`docs/mcp/0009-runtime-settings-and-hot-reload.md`).

## What the clarification is for

The User Agency off-switch rule says a new feature "is complete only when `docs/config.md` documents it and its off switch". `docs/config.md` documents the Kubo repo config. The `ipfs mcp` command reads its own config file (`--mcp-config` or `IPFS_MCP_CONFIG`), not the Kubo repo config, so its keys have no natural home in `docs/config.md`. D12 documents every `ipfs mcp` key, default and off switch in the generated `docs/mcp/config-reference.md` and adds one pointer paragraph in `docs/config.md`. The clarification records that reading in the rule itself so the next feature does not re-litigate it.

## Exact change

One paragraph is touched: the **"Every new feature has an off switch"** bullet in the User Agency section. The change appends one sentence to the end of that paragraph. No sentence is removed or reworded, in that paragraph or anywhere else in the Stability and User Agency sections; no other section of `AGENTS.md` changes.

Current paragraph (verbatim, `AGENTS.md` at the time of writing):

> - **Every new feature has an off switch.** Never design a feature without an opt-out. A new feature is complete only when `docs/config.md` documents it and its off switch, the changelog entry shows how to disable it, and tests confirm kubo works with it disabled. Flipping an existing config default, especially off to on, is a feature launch and gets the same treatment. An off switch is not a default-off requirement: in this fork, the agent features that are the fork's purpose ship enabled by default. They are the local `ipfs mcp` agent endpoint that IPFS Desktop starts on loopback, the Web UI agent chat, the WebMCP tools and the Desktop confirmation window. Each keeps a documented off switch, writes still require confirmation, and no third-party service is enabled without the operator choosing it.

Amended paragraph (the added sentence is the last one; everything before it is unchanged):

> - **Every new feature has an off switch.** Never design a feature without an opt-out. A new feature is complete only when `docs/config.md` documents it and its off switch, the changelog entry shows how to disable it, and tests confirm kubo works with it disabled. Flipping an existing config default, especially off to on, is a feature launch and gets the same treatment. An off switch is not a default-off requirement: in this fork, the agent features that are the fork's purpose ship enabled by default. They are the local `ipfs mcp` agent endpoint that IPFS Desktop starts on loopback, the Web UI agent chat, the WebMCP tools and the Desktop confirmation window. Each keeps a documented off switch, writes still require confirmation, and no third-party service is enabled without the operator choosing it. For `ipfs mcp` features, which read their own config file (`--mcp-config` or `IPFS_MCP_CONFIG`) rather than the Kubo repo config, the `docs/config.md` documentation requirement is satisfied by the generated `docs/mcp/config-reference.md` plus one pointer paragraph in `docs/config.md`; the changelog disable example and the disabled-state tests are required unchanged.

## Suggested PR shape for OG-5

- Title: `docs: clarify the off-switch rule's documentation requirement for ipfs mcp settings`
- The PR touches only `AGENTS.md` and carries no code changes (Scope and Safety).
- Body: state the D12 reading, link ADR-0009 section 9, and note that no Stability or User Agency sentence is removed — the diff is one appended sentence.

## Acceptance check for this draft

- Changes only the off-switch paragraph's reading for `ipfs mcp` settings: the diff is one sentence appended to that one paragraph, about `ipfs mcp` documentation location only.
- Removes no Stability or User Agency sentence: the amended paragraph above contains every sentence of the current paragraph verbatim, and no other paragraph is in scope.
