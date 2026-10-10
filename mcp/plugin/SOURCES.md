# Manifest sources

Every field used in the plugin and marketplace manifests under `mcp/plugin`, `.claude-plugin` and `.agents/plugins` is listed here with the primary source that defines it. Access date for all sources: 2026-10-10. Where the vendor documents a field only by example, that is stated.

Primary sources (fetched on 2026-10-10 with Firecrawl, `curl`, or the vendor CLI itself):

| Id | Source |
|---|---|
| CC-PLUGIN | Claude Code, Plugins reference: https://code.claude.com/docs/en/plugins-reference |
| CC-MKT | Claude Code, Create a marketplace: https://code.claude.com/docs/en/plugin-marketplaces |
| CC-MKT-REF | Claude Code, Marketplace reference: https://code.claude.com/docs/en/plugins/marketplace-reference |
| CC-MCP | Claude Code, MCP (section "Plugin-provided MCP servers", and the `type` rules): https://code.claude.com/docs/en/mcp.md |
| CX-PKG | OpenAI, Package your plugin: https://developers.openai.com/plugins/build/plugins |
| CX-SCHEMA-PLUGIN | Agent Plugins 1.0.0 manifest schema: https://agent-plugins.org/schemas/1.0.0/plugin.schema.json |
| CX-SCHEMA-MCP | Agent Plugins 1.0.0 MCP schema: https://agent-plugins.org/schemas/1.0.0/mcp.schema.json |
| CX-CURATED | OpenAI curated Codex marketplace (real values for `category` and `policy`): https://raw.githubusercontent.com/openai/plugins/main/.agents/plugins/marketplace.json |
| CX-MCP | OpenAI, Codex MCP configuration: https://developers.openai.com/codex/mcp.md |
| OC-CONFIG | OpenCode, Config: https://opencode.ai/docs/config/ |
| OC-SCHEMA | OpenCode JSON Schema (the `$schema` URL of every OpenCode config): https://opencode.ai/config.json |
| OC-MCP | OpenCode, MCP servers: https://opencode.ai/docs/mcp-servers/ |
| OC-SKILLS | OpenCode, Agent Skills: https://opencode.ai/docs/skills/ |
| OC-PLUGINS | OpenCode, Plugins: https://opencode.ai/docs/plugins/ |
| OC-PLUGIN-TYPES | `@opencode-ai/plugin` 1.4.3 type declarations (`Hooks.config`), read from the locally installed package `dist/index.d.ts` |

## Claude Code

`mcp/plugin/ipfs/.claude-plugin/plugin.json`

| Field | Source |
|---|---|
| `name` (required, kebab-case) | CC-PLUGIN, Fields table and "name" |
| `displayName` | CC-PLUGIN, Fields table |
| `version` | CC-PLUGIN, Fields table |
| `description` | CC-PLUGIN, Fields table |
| `author` (object with `name`) | CC-PLUGIN, Fields table |
| `license` (SPDX identifier) | CC-PLUGIN, Fields table. `Apache-2.0 OR MIT` is an SPDX expression; `claude plugin validate` accepts it |
| `keywords` | CC-PLUGIN, Fields table |

Skills are found in the default `skills/` directory (CC-PLUGIN, "Standard layout"; one `<name>/SKILL.md` per skill), so no `skills` key is set.

`mcp/plugin/ipfs/.mcp.json`

| Field | Source |
|---|---|
| File location (`.mcp.json` at the plugin root) and top-level `mcpServers` map | CC-MCP "Plugin-provided MCP servers"; CC-PLUGIN "mcpServers" and "Standard layout" |
| `type: "stdio"`, `command`, `args` | CC-MCP (stdio entries; `type` rules); CC-PLUGIN example `mcpServers` |

`mcp/plugin/ipfs/alternatives/claude-code-http.mcp.json` (not loaded by the plugin)

| Field | Source |
|---|---|
| `type: "http"`, `url`, `headers` | CC-MCP (HTTP servers; `streamable-http` is accepted as an alias) |
| `${IPFS_MCP_TOKEN}` expansion in `headers` | CC-MCP "environment variable expansion"; plugin placeholders table says `url` and `headers` are substituted for http servers |

`.claude-plugin/marketplace.json`

| Field | Source |
|---|---|
| `name`, `owner` (`name`), `plugins` (required) | CC-MKT-REF, Top-level fields |
| `description` | CC-MKT-REF, Top-level fields (validate warns when missing) |
| `plugins[].name`, `plugins[].source` (relative path string starting `./`, resolved from the marketplace root) | CC-MKT "Rules for plugin entries"; CC-MKT-REF Plugin entries |
| `plugins[].description`, `category`, `tags` | CC-MKT-REF, Plugin entries |

Entry `name` equals the manifest `name` (`ipfs`), as CC-MKT requires.

## Codex (Agent Plugins)

`mcp/plugin/ipfs/plugin.json`

| Field | Source |
|---|---|
| `$schema` (const `https://agent-plugins.org/schemas/1.0.0/plugin.schema.json`), `name` (pattern `^(?!.*(?:--\|\.\.))[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`, max 64), `version`, `description`, `author` (`name`, `email`, `url`), `license`, `keywords`, `extensions` | CX-SCHEMA-PLUGIN (`additionalProperties: false`, so no other root keys); CX-PKG "Plugin structure" |
| Skills discovered from root `skills/`, no manifest field | CX-PKG "Create a plugin manually" |
| `extensions."com.openai".interface` with `displayName`, `shortDescription`, `longDescription`, `category`, `capabilities`, `defaultPrompt` | CX-PKG "Add OpenAI-specific metadata" (shown by example; values `Read` and `Write` for `capabilities` come from that example) |
| `interface.category` value `Developer Tools` | CX-CURATED (27 of 65 curated entries use it) |

`mcp/plugin/ipfs/mcp.json`

| Field | Source |
|---|---|
| `$schema` (const `https://agent-plugins.org/schemas/1.0.0/mcp.schema.json`), `mcpServers` | CX-SCHEMA-MCP (both required) |
| `type: "stdio"`, `command`, `args` | CX-SCHEMA-MCP, `stdioServer` |

`mcp/plugin/ipfs/alternatives/codex-http.config.toml` (not loaded by the plugin)

| Field | Source |
|---|---|
| `[mcp_servers.<name>]`, `url`, `bearer_token_env_var` | CX-MCP, "Streamable HTTP servers" |

`.agents/plugins/marketplace.json`

| Field | Source |
|---|---|
| `name`, `interface.displayName`, `plugins[]` | CX-PKG "Marketplace metadata" |
| `plugins[].name`, `source.source: "local"`, `source.path` (`./`-prefixed, relative to the marketplace root, inside it) | CX-PKG "Marketplace metadata" and "Install a local plugin manually" |
| `policy.installation: "AVAILABLE"`, `policy.authentication: "ON_INSTALL"`, `category` (always present per CX-PKG) | CX-PKG; values confirmed against CX-CURATED (`AVAILABLE`; `ON_INSTALL` or `ON_USE`; `Developer Tools`) |

Codex also reads `.claude-plugin/marketplace.json` as a legacy-compatible marketplace (CX-PKG "How local marketplaces work"); this repo ships both files so either harness finds its own.

## OpenCode

OpenCode has no marketplace manifest. Its documented distribution routes are an npm package named in the `plugin` config array, a file in `.opencode/plugins/` or `~/.config/opencode/plugins/`, and skills in `.opencode/skills/` (OC-PLUGINS "Use a plugin", OC-SKILLS "Place files"). The OpenCode package under `mcp/plugin/ipfs/opencode/` therefore is a plugin module plus a config snippet; no `marketplace.json` exists for it and none is invented here. The `package.json` is marked `"private": true`; publishing to npm is an operator action.

`mcp/plugin/ipfs/opencode/opencode.json` (config snippet) and the config produced by `ipfs-mcp.js`

| Field | Source |
|---|---|
| `$schema` `https://opencode.ai/config.json` | OC-CONFIG "Format" |
| `mcp.<name>` with `type: "local"`, `command` (array), `enabled` | OC-MCP "Local" options table |
| `mcp.<name>` with `type: "remote"`, `url`, `headers`, `oauth: false` (alternatives/opencode-http.json) | OC-MCP "Remote" options table and "Disabling OAuth" |
| `{env:VAR}` substitution in `headers` | OC-CONFIG "Env vars"; OC-MCP Context7 example |
| `skills.paths` (array of additional skill folders) | OC-SCHEMA (`skills.paths`: "Additional paths to skill folders"). Not on the OC-CONFIG or OC-SKILLS pages; the live schema and the 1.18.35 binary's behavior (`opencode debug skill`) are the evidence |
| `plugin` array entry given as a `file://` URL | OC-CONFIG "Plugins" documents npm names; the `file://` form is not documented. Verified only by running OpenCode 1.18.35 (see evidence). Treat as version-sensitive |

`mcp/plugin/ipfs/opencode/ipfs-mcp.js`

| Item | Source |
|---|---|
| A plugin is a JS/TS module exporting a function that returns a hooks object | OC-PLUGINS "Create a plugin" |
| `config` hook that receives and mutates the config | OC-PLUGIN-TYPES `Hooks.config?: (input: Config) => Promise<void>`. Not described on OC-PLUGINS. Verified by running OpenCode 1.18.35 |

`mcp/plugin/ipfs/opencode/package.json` uses ordinary npm fields (`name`, `version`, `type`, `main`, `license`, `private`, `keywords`); OC-PLUGINS says npm packages are installed with Bun at startup and does not define extra required fields.

## Not verified from a primary source

- OpenCode `plugin` entries as `file://` URLs and `skills.paths` are evidenced by the schema and by running the binary, not by prose documentation (OpenCode also advertises a "v2"; this was checked on 1.18.35 only).
- Codex forwards only an allowlisted environment to stdio servers (`env_vars`, CX-MCP). The Agent Plugins stdio schema (CX-SCHEMA-MCP) offers `env` with static strings only, so a plugin cannot forward `IPFS_PATH`. Observed behavior is in `README.md`.
- The `claude plugin validate` advice line asking for a README install line comes from the validator and is addressed in `mcp/plugin/README.md`; the root `README.md` of the Kubo fork is outside this change's ownership.
