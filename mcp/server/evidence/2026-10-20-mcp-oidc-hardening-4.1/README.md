# mcp-oidc-hardening 4.1 integration gate (2026-10-20)

Tree: `feat/ipfs-mcp-server` at `a17a2c626` plus the uncommitted mcp-oidc-hardening changes. Full output: `gate.log` in this folder.

| Step | Command | Result |
|---|---|---|
| go.mod unchanged | `git diff --quiet HEAD -- go.mod go.sum docs/examples/kubo-as-a-library/go.mod test/dependencies/go.mod` | PASS |
| Formatting | `gofmt -l` on changed and new `.go` files | PASS (no output) |
| Build | `make build` | PASS |
| Lint | `gmake -O test_go_lint` (macOS `make` 3.81 has no `-O`; the first attempt failed on that flag) | PASS for this change. One finding, pre-existing and outside the diff: `fuse/node/mount_darwin.go:29` ST1005, identical to `master` (upstream a5179f0dd, #11272), macOS-only |
| Unit tests | `go test ./mcp/server/... ./mcp/contract/... ./cmd/ipfs/kubo/ -count=1` | PASS |
| CLI tests | `go test ./test/cli/... -count=1 -run "TestMCPSessionToken\|TestMCPHTTP\|TestMCPPrincipal\|TestDagJose\|TestDefaultMatchesExpectedProfile\|TestTelemetry"` against the `make build` binary | PASS |
| Config reference | `python3 docs/mcp/config_reference.py --check --ipfs cmd/ipfs/ipfs` | PASS (134 keys, --help compared: yes) |
| Stability | no change under `core/commands`, `core/corehttp`, `client/rpc`, `core/coreiface` | PASS |

Not run: `-race`, the full `make test` suite, gateway conformance (no gateway code changed). Unverified until `lke-agent-settings` 1.8: the `azp` rule against a live Hydra ID token (design D1).
