# @gqadonis/ipfs-webmcp

Framework-free WebMCP tool definitions and `kubo-rpc-client` executors, generated from `mcp/contract`. No React. Used by the Web UI fork (`src/webmcp`); see ADR-0002 (`docs/mcp/0002-webmcp-web-ui.md`).

## What it exports

- `createWebMcpTools({ client, confirm?, includeWrites?, timeoutMs?, isActive?, writesAllowed?, dnsLinkAllowed? })` returns the objects to hand to `document.modelContext.registerTool`. Each has exactly `name, title, description, inputSchema, annotations, execute`.
- `registerWebMcpTools(modelContext, tools)` registers with `registerTool(tool, { signal })` and returns `{ registered, failed, unregister }`. `getModelContext()` reads only `document.modelContext`.
- `TOOL_DEFINITIONS`, `EXCLUDED_TOOLS`, `CONTRACT_VERSION`: the generated data.

## Rules the package enforces

- **Generated, not authored.** `pnpm gen` reads `catalog.json`, `derivations/webmcp.json` and `schemas/tools/*.input.json`. The annotations are `readOnlyHint`, `untrustedContentHint`, `consequentialHint`, `debugging` (the WebMCP draft's four), derived by the contract rules. `pnpm gen:check` and `tests/contract.test.ts` fail on drift.
- **No destructive tools.** The contract's `exposed` rule drops them. They are listed in `EXCLUDED_TOOLS` with reason `destructive`. Also excluded, with reasons: the `index` group (server-side state), `ipfs_node_status`, `pin_verify` and `dag_stat` (no `kubo-rpc-client` 6.1.0 method), and `car_import` (the client cannot stop Kubo announcing CAR roots). The `exposed` rule is an allow-list (`in:confirmation:none,write`); an unknown class makes the generator throw.
- **Writes need the host.** No write tool is created without a `confirm` hook and `includeWrites: true`. Every write call builds a `ConfirmationRequest` (tool, effect, targets, details) and sends no RPC request unless the hook resolves exactly `true`. A throwing hook is a decline. The two-step `confirm_token` of the MCP server is not offered; the hook replaces it.
- **Live gate.** Every `execute`, and again after an approval, checks `isActive`, `writesAllowed` and (for DNSLink names) `dnsLinkAllowed`. A host that ignored unregistration gets `declined` (`inactive`, `writes_off`, `dnslink_off`) and no RPC request. DNSLink names (`/ipns/<domain>`, `ipns_resolve` of a domain) are declined unless `dnsLinkAllowed()` is true; libp2p-key names always pass.
- **Safe to show.** `describeWrite` passes every agent-supplied value through `sanitize` (bidi, zero-width, control and other invisible code points become `<U+XXXX>`), cuts long values in the middle (`full` keeps the rest) and adds byte length and SHA-256 of the content.
- **Prototype-safe input.** The validator uses own-property checks, returns a null-prototype copy and rejects `__proto__`, `constructor` and `prototype` keys at any depth; `dag_put` data is checked the same way before the hook.
- **RPC error text is untrusted.** `message` is fixed text; `detail` is the redacted error (URLs and credentials removed).
- **Input is validated** against the generated schema (including defaults) before the hook or the client are touched.
- **The AbortSignal is optional.** `execute(input, options?)` works with no options (Kimi) and with a signal (Codex).
- **Results are JSON objects.** Trusted results are `{ ok, status, tool, summary, result }`. Results that carry file, network or node content are `{ ok, status, tool, summary, untrusted_content: true, data_notice, data }`, so relayable facts stay in plain fields and untrusted content is separate. Declines and errors are `{ ok: false, status: 'declined' | 'error', code, message }`.

## Commands

```
pnpm install
pnpm gen          # regenerate src/generated/tools.ts
pnpm check        # gen:check, typecheck, unit tests, build, integration tests
pnpm test         # unit tests (mock client, no network)
pnpm test:integration   # real daemon
```

The integration tests start `ipfs` (`IPFS_BIN`, else `cmd/ipfs/ipfs` from `make build`, else `PATH`) with `init --profile=test` in a temporary `IPFS_PATH`. The profile binds API, gateway and swarm to 127.0.0.1 on OS-assigned ports and turns off bootstrap, mDNS and autoconf; the harness refuses to continue if the daemon binds 4000, 4001, 5001 or 8080. It stops the daemon by PID and removes the repo.
