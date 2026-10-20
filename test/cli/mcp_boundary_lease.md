# Real-backend browser fixture lease

`TestMCPBoundaryLease` is optional setup for UI-108/UI-111. It never proves a scenario passed. Missing collaborators, a wrong release or unavailable capabilities produce a public `BLOCKED` status. The browser owner must still execute and assert the actual read, approval, denial, context and RPC flows.

The final packet first runs `make build`. It may then compile the fixture with `go test -c -o <exclusive-scratch>/mcp-boundary-fixture.test ./test/cli`. Compilation and invocation are held until the phase lead's final verification boundary. Use the compiled test executable directly: the `go test` frontend does not forward arbitrary inherited file descriptors.

Run from the Kubo repository root with this exact argument shape:

```text
<exclusive-scratch>/mcp-boundary-fixture.test -test.run=^TestMCPBoundaryLease$ -test.timeout=20m
```

Set `IPFS_MCP_BOUNDARY_LEASE=1` in the fixture process environment. No credential belongs in argv or the runner's recorded environment. The runner creates anonymous pipes and maps child descriptors as follows:

| Descriptor | Direction | Contract |
| --- | --- | --- |
| stdin / 0 | Runner → fixture | Hold open for the lease. EOF cancels setup/running children and joins exactly the fixture's children. Do not inherit a terminal. |
| 3 | Runner → fixture | One UTF-8 JSON setup object, at most 65,536 bytes; write then close. Unknown fields, trailing JSON and invalid setup fail closed. Credential material is memory only. |
| 4 | Fixture → runner | Newline-delimited public JSON status; keep separate from stdout/stderr and drain throughout the lease. |
| 5 | Fixture → runner | One private JSON object containing `lease_id` and `ui_bearer`, then EOF. Never record, serialize, trace, screenshot or log this pipe. |

FD3 setup has these exact fields. All are required except the explicitly optional collaborator fields:

```json
{
  "version": 1,
  "test_only": true,
  "mode": "builtin",
  "make_build_sha256": "64-lowercase-hex-digest-of-the-make-built-cmd/ipfs/ipfs",
  "allowed_origins": ["http://127.0.0.1:19090"],
  "collaborator": {
    "test_only": true,
    "disposable": true,
    "lease_id": "runner-owned-provider-lease",
    "owner_pid": 12345,
    "endpoint": "http://127.0.0.1:19091/v1",
    "model": "qwen3:8b"
  }
}
```

The shown endpoints/PID/hash are placeholders, never defaults. `mode` is `builtin` or `uar`. `make_build_sha256` must be exactly 64 lowercase hex digits and match the actual binary. `allowed_origins` contains at most eight explicit loopback HTTP(S) origins with nonzero ports. `collaborator.lease_id` is 1–128 ASCII letters/digits/underscore/hyphen; its positive `owner_pid` identifies the external test owner and cannot be the fixture's PID. `endpoint` must be an explicit loopback HTTP(S) URL with nonzero port and no user info, query or fragment. `model` is a nonempty UTF-8 string of at most 256 bytes. Optional `credential` is a bearer held only through FD3/memory/environment of the owned MCP child, at most 16,384 bytes.

For `uar`, `credential`, `provider`, `trusted_host_id`, `release_artifact` and `release_receipt` are additionally required. The provider and trusted-host IDs are 1–128 ASCII letters/digits/underscore/dot/colon/hyphen. `release_artifact` must equal the canonical `agentic/uar-preflight.json` pinned artifact. `release_receipt` has exactly `kind` (`image_inspect` or `source_process`), `lease_id`, `owner_pid`, `endpoint`, `artifact`, `subject`, `observed_at` and `sha256`. The lease/owner/endpoint/artifact must match the live collaborator. `subject` is a bounded 1–128 ASCII letters/digits/underscore/dot/colon/hyphen process/container identity. `observed_at` is RFC3339 within five minutes, with at most five seconds future clock tolerance. `sha256` is the 64 lowercase hex digest of the external owner's actual public image-inspection or source/process receipt artifact. The owner must retain that artifact, tied to the live lease, and the final runner must verify its actual bytes against the hash; merely asserting the expected artifact in configuration does not prove the pin. Missing, stale or mismatched binding yields `BLOCKED` / `pinned_release_unverified`. READY preserves the public receipt binding for final reconciliation. The helper also checks the real agent-card version, which alone cannot prove an image pin. It never inspects arbitrary operator processes or reads receipt paths. The UAR owner must configure a real model/provider and trusted host before leasing. A reachable mock is not an eligible collaborator. The helper never starts, stops, inspects credentials from or claims ownership of that external service.

The fixture starts a new temporary offline Kubo daemon, seeds two explicit unpinned fixture CIDs, and starts the make-built MCP binary on a dynamic loopback port. It generates ephemeral UI/callback credentials. Provider credentials occur only in the owned child's environment; its temporary config contains environment references. All child output is discarded or captured in a bounded, memory-only buffer; failure diagnostics use fixed public reasons and never dump a child log or backend response. The UI principal supports the existing pin write group and local egress acknowledgment. No auto-approval or native-presence bypass is installed. The actual built-in AG-UI approval path remains available. For UAR, the helper registers only its uniquely named test destination/agent; it attempts deletion on exit, and the external disposable lease owner remains responsible for its lifecycle/state cleanup.

FD4 emits one of:

- `{"status":"BLOCKED","reason":"<fixed-safe-reason>"}` before readiness. No credential is required. A skipped Go fixture or exit zero is never scenario PASS.
- `{"status":"READY","version":1,"test_only":true,...}` with public `mode`, `lease_id`, `endpoint` (`/agui`), `decisions_url`, `rpc_url`, two `fixture_cids`, `make_build_sha256`, `config_fingerprint`, exact two `owned_pids`, external `collaborator` lease/owner/attestation, `authentication:"private_fd5"`, `browser_capture:"no_trace_or_network"`, and `native_node_confirmation_available:false`.
- `{"status":"CLOSED","lease_id":"...","owned_children_joined":true}` after the lease closes and both owned children are joined. The maximum lifetime is 15 minutes, including setup. The runner must also await fixture process exit.

Bind the private FD5 `lease_id` to the public READY lease before using the ephemeral bearer in memory. `config_fingerprint` hashes the actual temporary MCP configuration (which contains only secret references); it is not proof of backend behavior. The external owner controls its own provider/UAR lease and must release it separately. No descriptor, credential, skipped test or healthy endpoint satisfies a browser assertion.

Browser sessions carrying the bearer must omit trace, network and error-payload collection entirely. Never put the bearer in URLs or artifacts. RPC inspection uses only the helper's explicit `rpc_url`; no operator/default node is read. Missing genuine native node confirmation stays unavailable; a UAR backend acknowledgment is not a node execution receipt. The helper's public `native_node_confirmation_available:false` must not be promoted into an approval capability or a successful write outcome.
