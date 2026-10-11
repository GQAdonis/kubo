# R11 quality, compatibility and UI review: `ipfs-mcp-server`

- Date: 2026-10-11
- Reviewer: `reviewer` subagent, in a separate context, with read-only tools
- Scope: everything since `833e04a24`

**Verdict: PASS-WITH-FINDINGS.** The reviewer found no CRITICAL issue and two HIGH issues.

## Compatibility checks

These hold, based on reading the code:

- `ipfs mcp` is registered only in `localCommands`.
- `test/cli/mcp_test.go:566` asserts that `/api/v0/mcp` returns 404.
- Nothing under `core/`, `client/`, `plugin/`, `repo/` or `config/` references MCP.
- `ipfs_add` passes no recipe option unless the caller asks for one.
- The changelog edits are limited to the TOC and Highlights.
- The `nomcp` and `noagent` tags gate every MCP and agent file.

The contract and server both list the same 41 tools, and every tool name used in the skills exists.

## Findings

| id | severity | file:line | finding | consequence | suggested fix |
|---|---|---|---|---|---|
| R11-Q01 | HIGH | `mcp/server/tools_dag.go:328-344,70-75`, `core/commands/dag/put.go:130-138` | `dagPut` returns after reading the first streamed object, then cancels the request before `b.Commit()` runs. `pinned` echoes the request instead of the result. | The tool reports a node as stored and pinned when it may be neither. This explains the `TestMCPDAG` pin failure. | Read the stream to EOF and surface any trailer error. Only then report `pinned`. Add a test where the fake daemon fails after emitting the CID. |
| R11-Q02 | HIGH | `docs/mcp/config-reference.md:96-98`, `mcp/server/config.go:27` | `confirmation.resolvers`, `confirmation.cli` and `confirmation.ttl` are validated but never read. | The off switches do not work. | Reject these keys with an error that points to `confirmation.modes`, or implement them with tests. |
| R11-Q03 | MEDIUM | `test/cli/mcp_confirm_test.go:63-65` | A build failure makes the test skip instead of fail. | A regression shows up as green. | Use `t.Fatalf`. |
| R11-Q04 | MEDIUM | `cmd/ipfs/kubo/mcp_noagent.go:24-29`, `test/cli/mcp_test.go:596-617` | Nothing tests the `noagent` disabled state. | A regression in that path goes unseen. | Add `TestMCPNoAgentBuild`. |
| R11-Q05 | MEDIUM | `mcp/app/embed.go:4` | The comment claims `check-dist` runs in CI, but no CI job references `mcp`. | The committed `dist/` can drift from the sources. | Correct the comment, or add the check to CI (needs operator approval for `.github/`). |
| R11-Q06 | MEDIUM | `openspec/changes/ipfs-agent-skills/tasks.md:11` vs `docs/mcp/progress-report.md:180-181` | Task 1.4 is ticked, but the evidence shows 20/22. | The task overclaims. | Untick it, or record 20/22. |
| R11-Q07 | MEDIUM | `docs/mcp/progress-report.md:19,114` | The last full `TestMCP*` CLI run was R7, before later code changes. | G3 evidence is stale. | Re-run the suite in R11 and record it. |
| R11-Q08 | MEDIUM | `mcp/ui/src/approval/ApprovalPanel.tsx:63-64,103-116` | The `alertdialog` variant can render with no focusable control and `aria-modal="false"`. | Breaks the ARIA pattern and WCAG 2.4.3. | Use `group` or `status` when there are no controls, or focus the heading. |
| R11-Q09 | LOW | `ApprovalPanel.tsx:112` | The "Approve removal" label is used for `pin_update`, which moves a pin. | Misleading wording. | Label per tool. |
| R11-Q10 | LOW | `mcp/contract/contract.go:154-162`, `server_test.go:326-338` | Nothing checks that every catalog tool is implemented. | An unimplemented tool would ship unnoticed. | Add a test. |
| R11-Q11 | LOW | `mcp/server/schema_test.go:63-90` | `$ref` is stubbed in the `ToolError` check. | The confirmation payload is never checked against its schema. | Bundle the schema instead. |
| R11-Q12 | LOW | `mcp/skills/`, the skills spec | The skill tool-name check and plugin-copy sync have no Go test or CI. | A tool rename breaks the skills silently. | Add a test in `mcp/contract`. |
| R11-Q13 | LOW | `openspec/changes/ipfs-agent-skills/specs/.../spec.md:9-14` | The spec says 6 skills; 8 exist. | Archive verification will fail. | Update the spec. |
| R11-Q14 | LOW | `cmd/ipfs/kubo/mcp.go:63-64` | The help text lists the write tools out of date. | Inaccurate CLI docs. | Describe the tool classes generically. |
| R11-Q15 | LOW | `mcp/server/token.go:116-120` | `panic` in library code. | Breaks the AGENTS.md style rule. | Remove the dead branch. |
| R11-Q16 | LOW | `mcp/server/agent.go` (826 lines) | Over the 800-line limit. | Hard to maintain. | Split the file. |
| R11-Q17 | LOW | `index_helpers_test.go`, `embeddings/store_test.go`, `resolver_registry_test.go` | Tests use `context.Background()`. | Breaks the style rule. | Use `t.Context()`. |
| R11-Q18 | LOW | `test/cli/mcp_confirm_test.go:50` | The test binary is left behind in the temp directory. | Leaks disk space. | Clean it up. |
| R11-Q19 | LOW | `mcp/ui/src/a2ui/catalog.ts:3` | Each view is about 305 KB gzip, with two copies of zod. | Slightly over the bundle budget. | Drop `zod-v3`, or record the budget decision. |
| R11-Q20 | LOW | `docs/changelogs/v0.44.md:38-42` | The highlights describe a fork-only chat sidebar. | Misleading if sent upstream. | Keep it fork-only, or remove it before upstreaming. |
| R11-Q21 | LOW | `docs/mcp/progress-report.md:234` | Shows 0/4 for `agentic-uar-integration`, but `tasks.md` has 5 lines. | Count drift. | Reconcile. |

## Not verified

- **No commands were run.** No lint, build, tests, Playwright or tag builds.
- **No diff was computed.**
- **Transitive dependency pins** in `go.mod` were not checked against their upstreams.
- **CLI help width** was not tested.
- **`prometheus-ui-review`** was not available, so the UI review is a code reading only.
- **Not checked:** the forks, the plugin evidence files and the `mcp/chat` end-to-end flow.
