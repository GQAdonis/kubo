# console-session-auth 6.3: candidate image with the OIDC verifier

Head commit: `1cfa42a621ca12be1a602e6b447ba090826921d0` (`1cfa42a62`, branch `feat/ipfs-mcp-server`). Date: 2026-10-19. Procedure: the kubo-candidate-image run 1 and run 2 (`mcp/server/evidence/2026-10-16-kubo-candidate-image/`), plus scenario e (the cutover's `auth.oidc` block).
Image: `ghcr.io/gqadonis/kubo:1cfa42a62`, NOT pushed (operator action). Image ID: `sha256:fbc6875b34d8b2248637a61a14981b09e46d5ed4282f34332308b6f4f7999dab`. Os=linux, Architecture=amd64. Local digest: none (`RepoDigests=[]`, `1.2-image-ls.txt`); the registry digest exists only after the push.
Replaces the running image `ghcr.io/gqadonis/kubo@sha256:38d453a5...` (commit `3d8a128c5`, no `mcp/server/oidc_config.go`, `jwks.go`, `auth_chain.go`).

Operator push command:

```
docker push ghcr.io/gqadonis/kubo:1cfa42a62
```

Then record the digest it prints (`1cfa42a62: digest: sha256:... size: ...`) for the image-only cluster PR.

## Results

| Check | Result | File |
|---|---|---|
| 1.1 `make mod_tidy` produces no diff (clean worktree of `1cfa42a62`) | PASS (0 changed files) | `1.1-0-mod-tidy.txt` |
| 1.1 `make build` | PASS (ldflags `CurrentCommit=1cfa42a62`) | `1.1-1-make-build.txt` |
| 1.1 `go test ./mcp/...` | PASS (app, contract, server 43.8s, confirm, embeddings ok) | `1.1-2-go-test-mcp.txt` |
| 1.1 `TestDefaultMatchesExpectedProfile` | PASS (1) | `1.1-3-TestDefaultMatchesExpectedProfile.txt` |
| 1.1 `TestGateway` | PASS (59 `--- PASS` lines, 0 FAIL, 0 SKIP; run 2 counted 75 with the same wording, so the count is not comparable, see below) | `1.1-3-TestGateway.txt` |
| 1.1 `TestTelemetry` | PASS (9 PASS, 0 FAIL) | `1.1-3-TestTelemetry.txt` |
| 1.2 buildx build linux/amd64 `--load`, exit 0 | PASS | `1.2-docker-build.log` |
| 1.2 inspect shows linux/amd64 and the tag | PASS | `1.2-image-inspect-summary.txt`, `1.2-image-inspect.json` |
| 1.3 `ipfs version --commit` | PASS: `0.44.0-dev-1cfa42a` (7-char prefix of `1cfa42a62`) | `1.3-version.txt`, `1.3-a-no-config.txt` |
| 1.3 `ipfs mcp --help` lists `--settings-dir` | PASS | `1.3-a-no-config.txt` |
| a: no config, no tokens, no LLM key: starts; `/agui` 403; `/agent/settings`, `/index/search`, `/index/status` 404 | PASS | `1.3-a-no-config.txt` |
| b: `ui` principal, credential env unset | PASS under the R8-05 / S5 amendment: `ipfs mcp` exits 1 with "the config names 1 principal(s) but none has a credential ..." (no longer serves 403) | `1.3-b-credential-unset.txt` |
| c: throwaway `ui` token: `/agui` 503 `backend_not_configured`; `/agent/settings`, `/index/*` 404; tools `ipfs_cat`, `ipfs_get`, `ipfs_ls`, `ipfs_node_info`, `ipfs_node_status`, no `ipfs_index_*` | PASS | `1.3-c-token-no-embeddings.txt` |
| c: `GET /auth/session` 404 with `auth.oidc` absent | PASS | `1.3-c-token-no-embeddings.txt` |
| e: the cutover `auth.oidc` config is accepted; server starts and logs "auth.oidc is on: session tokens from https://oauth.know-me.tools are accepted on routes agui,settings,index,session (keys at .../jwks.json)" | PASS | `1.3-e-mcp-log.txt` |
| e: no token on `/agui`, `/mcp`, `/auth/session`, `/agent/settings` | PASS: 401 on each | `1.3-e-oidc-probes.txt` (probes 1, 6, 7, 10) |
| e: garbage bearer (`garbage`) on `/agui`, `/auth/session` | PASS: 401 | probes 2, 8 |
| e: forged structurally valid RS256 JWT (random signature, unknown `kid` and the live `kid`; correct iss, aud, sub) on `/agui`, `/mcp`, `/auth/session`, `/agent/settings` | PASS: 401 on each | probes 3, 4, 5, 9, 11 |
| e: `/auth/session` and `/agent/settings` are mounted (401, not 404, unlike scenario c) | PASS | probes 7 to 11 vs c |
| e: container has egress and the server fetches the live JWKS on an unknown-kid request | PASS: tcpdump in the container's network namespace shows DNS for `oauth.know-me.tools` (A 23.239.29.33) and a TLS session to 23.239.29.33:443 right after the forged unknown-`kid` request (12:50:42); the earlier 12:50:34 packets are my own busybox `wget` probe | `1.3-e-jwks-egress-tcpdump.txt` |

## The `auth.oidc` config used (scenario e, `cfg-oidc.json`)

Schema read from `mcp/server/oidc_config.go` and the 6.1 text; defaults left unset (algorithms RS256, skew 30s, cache 15m, routes `agui,settings,index,session`, `require_origin` true). Both principals list the single subject from 6.1 (one subject in `ui` and `admin` is allowed because duplicates are only refused within one kind).

```json
{
  "auth": {
    "oidc": {
      "enabled": true,
      "issuer": "https://oauth.know-me.tools",
      "audiences": ["ipfs-console"],
      "jwks_url": "https://oauth.know-me.tools/.well-known/jwks.json"
    }
  },
  "principals": [
    {"name": "console-ui", "kind": "ui", "subjects": ["461e5d70-d7a6-48ec-a20e-06b5e0236881"], "groups": ["*"]},
    {"name": "console-admin", "kind": "admin", "subjects": ["461e5d70-d7a6-48ec-a20e-06b5e0236881"]}
  ],
  "agent": {"agui": true, "origin_allowlist": ["https://console.prometheusags.ai"]}
}
```

Passed as `ipfs mcp --api=/ip4/127.0.0.1/tcp/5101 --http=127.0.0.1:5151 --agui --mcp-config=/cfg.json`. No static credential and no LLM backend were configured in scenario e.

## Not verified

- A valid ID token was NOT obtained (as instructed), so acceptance of a real mapped token (`sub` mapping, `aud`, `exp`, route scope, step-up) is not shown against the live issuer. The probes show every unauthenticated, garbage and forged-signature input is refused with 401 and that the JWKS is fetched; "forged with the live kid" is refused at 401 but the response body does not name the reason (my probe printed only the status line and JSON bodies; none were returned), so signature rejection versus another claim failure is not distinguished here. Verifier behavior for valid tokens is covered by the unit tests in `go test ./mcp/...` and `TestMCPSessionToken` is not in this run's list (task 2.8 ran earlier).
- The prefetch at startup is not visible in the server log; only the request-driven fetch is shown on the wire.
- `TestGateway` count: 59 `--- PASS` lines here vs 75 in run 2. Both runs have 0 FAIL and `ok`; I did not diff the subtest lists, so the drop is unexplained (a changed test file between `3d8a128c5` and `1cfa42a62` is the likely cause, not checked).
- The `ghcr.io` registry digest, the pod rollout and the cluster pre-flight (the principals' secrets mounted, per the R8-05 note: the sidecar uses `--auth-token-env` with no principals, but 6.1 adds session-only principals that need no secret) are not covered.
- The image was built for linux/amd64 on an arm64 host; containers ran under emulation (`--platform linux/amd64`, warning from the daemon only).

## Notes and deviations

- Go checks ran in a clean git worktree of `1cfa42a62` (`scratchpad/kubo-img`, zero uncommitted files before and after). The first image build from that worktree produced `ipfs version 0.44.0-dev` with no commit suffix, because a worktree's `.git` is a file and the Dockerfile's `mkdir -p .git/objects` / `git rev-parse` path needs a real `.git` directory (`1.2-docker-build-attempt1-worktree-no-commit.log`, `1.3-version-attempt1.txt`; that image, ID `sha256:acfc48df...`, was re-tagged away and is now dangling, not pruned). The valid image was built from a fresh `git clone` of the repo checked out detached at `1cfa42a62` (`scratchpad/kubo-img-ctx`, 0 uncommitted files) with the same Dockerfile and flags: `docker buildx build --platform linux/amd64 --load --progress=plain -t ghcr.io/gqadonis/kubo:1cfa42a62 .`.
- Containers use their own `IPFS_PATH=/tmp/ipfs-smoke`, `--offline`, swarm 4101, API 5101, gateway 8181, `ipfs mcp` 5151, all on 127.0.0.1 inside the container; nothing published to the host, nothing on 4001/5001/8080/5050. The `test/cli` runs used the harness's own repos and ports and a fresh `IPFS_PATH`; no daemon was left running.
- Throwaway ui token (scenario c) lives only in `/tmp/ui.token` inside that container. Forged JWTs carry a random signature, were written to files in the scratchpad and never printed; they are not in this folder (`forge.py` is).
- Probe scripts: `run-scenario.sh` (host driver), `smoke-entry.sh`, `raw-run2.sh`, `tools.sh`, `probe.sh` (copies from run 2), `raw-oidc.sh`, `forge.py`.
- Containers started and removed by ID: `container-ids.txt` (a, b, c, e, tcpdump). A one-off `alpine:latest` (arm64) container with tcpdump shared scenario e's network namespace. No images or volumes pruned. OrbStack was not running at the start; I started it with `orbctl start` (the Docker daemon was down).

## Lead addendum: full TestGateway set (2026-10-19)

The `1.1-3-TestGateway.txt` run above matched only the top-level `TestGateway` (59 PASS lines). Run 2 on 2026-10-16 also ran `TestGatewayHAMTDirectory`, `TestGatewayHAMTRanges`, `TestGatewayLimits` and `TestGatewayOverLibp2p` (75). The lead re-ran the full set against this candidate's binary (`kubo-img` worktree, `ipfs version 0.44.0-dev-1cfa42a62`, own `IPFS_PATH`): `go test ./test/cli/ -run 'TestGateway' -count=1 -v`, exit 0, `ok github.com/ipfs/kubo/test/cli 9.247s`, **75 PASS, 0 FAIL, 0 SKIP** (`1.1-3-TestGateway-all.txt`). Same set and count as run 2.
