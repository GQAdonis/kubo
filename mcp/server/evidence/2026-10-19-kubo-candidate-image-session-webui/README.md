# console-session-auth 6.3 (rebuild): candidate image with the session-mode Web UI

Kubo commit: `1140ff10c97b2c8f07b6eaa3a2b0f75d79dbc613` (`1140ff10c`, branch `feat/ipfs-mcp-server`, pushed). Date: 2026-10-19. Replaces `1cfa42a62` (Web UI `bafybeif77…` from ipfs-webui `21baf2bb`, no session mode), which is not deployed. Procedure: `mcp/server/evidence/2026-10-19-kubo-candidate-image-oidc/` repeated unchanged, plus the Web UI checks below.

Image: `ghcr.io/gqadonis/kubo:1140ff10c`, NOT pushed (operator action). Image ID: `sha256:a50f357ce2f48621ca47bf6d75a3429ecd826666cfc6d92e8a69665e65f64086`. Os=linux, Architecture=amd64. Local digest: none (`RepoDigests=[]`, `1.2-image-inspect-summary.txt`); the registry digest exists only after the push.

Operator push command:

```
docker push ghcr.io/gqadonis/kubo:1140ff10c
```

## Web UI CID and CAR

- Web UI CID: `bafybeihd7ozltm2fciiiufczpfu4iahc4qfkhzkuro2ces75bmirdb3mpa` (CIDv1).
- Source: ipfs-webui `81674a5e157589f3cb291483bddeb54f640491bf`, branch `feat/ipfs-mcp-server` (contains session mode 3.1-3.7), built in a clean detached worktree (`git status --short` empty before and after). The string `81674a5e` is embedded in `build/static/js/main.7c9f1a55.js`.
- Procedure (same as `2026-10-17-promethity-delivery-candidate/1.1-webui-build.md`): Node v24.11.1 (nvm; pinned 24.11.0 is not installed), `npm ci --ignore-scripts` + `npx patch-package` (the `kubo` devDependency postinstall downloads from dist.ipfs.tech and is only used by tests), `npm run build`, then `ipfs add --cid-version 1 -r -Q build` with every other flag at kubo default. Hashed twice with `--only-hash`: identical CID. 270 files, 21,584 KiB. Logs: `0.1-webui-npm-ci.log`, `0.1-webui-build.log`, `0.2-cid-and-car.txt`.
- CAR: `/Users/gqadonis/.local/share/know-me-ops/webui-bafybeihd7ozltm2fciiiufczpfu4iahc4qfkhzkuro2ces75bmirdb3mpa.car`, 21,566,292 bytes, sha256 `7297ec26e950dd6646adb064be8b5373856165dd57be3250f76a4a7b441ddb11`. Made with `ipfs dag export <cid>` from a throwaway repo (`ipfs add` offline, no daemon). Not committed.
- Root check: `ipfs dag import --stats` of the CAR into a second fresh repo printed `Pinned root bafybeihd7ozlt… success`, `Imported 388 blocks (21551384 bytes)`; `ipfs dag stat <cid>` = 388 blocks, 21551384 bytes; `ipfs cat <cid>/index.html` returned the page.

## Kubo change (task 2)

`core/corehttp/webui.go`: `WebUIPath` set to the new CID; both earlier fork CIDs (`bafybeif77…` from `21baf2bb`, `bafybeiblix77…` from `2be6b78c`) now follow it in `WebUIPaths`, ahead of v4.13.0. `docs/changelogs/v0.44.md` fork-only item updated to the new CID and commit. `go fmt` clean; `go test ./core/corehttp/...` ok; `TestWebUI` (`test/cli/webui_test.go`, includes the redirect-equals-`corehttp.WebUIPath` test) 6 PASS, 1 SKIP (`NoFetch=true serves the build once it is local`, skips by design; not examined further). Nothing else pins the old CID in Go code or tests.

## Results

Go checks ran in a fresh `git clone` (not a worktree) checked out detached at `1140ff10c`, 0 uncommitted files before and after.

| Check | Result | File |
|---|---|---|
| 1.1 `make mod_tidy` produces no diff | PASS (0 changed files) | `1.1-0-mod-tidy.txt` |
| 1.1 `make build` | PASS (ldflags `CurrentCommit=1140ff10c`) | `1.1-1-make-build.txt` |
| 1.1 `go test ./mcp/...` | PASS (app, contract 9.3s, server 61.1s, confirm, embeddings ok) | `1.1-2-go-test-mcp.txt` |
| 1.1 `go test ./core/corehttp/...` | PASS | `1.1-2b-go-test-corehttp.txt` |
| 1.1 `TestDefaultMatchesExpectedProfile` | PASS (1) | `1.1-3-TestDefaultMatchesExpectedProfile.txt` |
| 1.1 full `-run 'TestGateway'` set | PASS: 75 `--- PASS`, 0 FAIL, 0 SKIP (same set and count as run 2 and the lead addendum) | `1.1-3-TestGateway-all.txt` |
| 1.1 `TestTelemetry` | PASS (9 PASS, 0 FAIL) | `1.1-3-TestTelemetry.txt` |
| 1.1 `TestWebUI` | PASS (6 PASS, 1 SKIP) | `1.1-3-TestWebUI.txt` |
| 1.2 buildx linux/amd64 `--load`, exit 0, tag `ghcr.io/gqadonis/kubo:1140ff10c` | PASS | `1.2-docker-build.log`, `1.2-image-inspect-summary.txt`, `1.2-image-inspect.json`, `1.2-image-ls.txt` |
| 1.3 `ipfs version --commit` in the image | PASS: `0.44.0-dev-1140ff1` (7-char prefix of `1140ff10c`) | `1.3-version.txt`, `1.3-a-no-config.txt` |
| 1.3 `ipfs mcp --help` lists `--settings-dir` | PASS | `1.3-a-no-config.txt` |
| a: no config, no tokens, no LLM key: starts; `/agui` 403; `/agent/settings`, `/index/search`, `/index/status` 404 | PASS | `1.3-a-no-config.txt` |
| b: `ui` principal, credential env unset: `ipfs mcp` exits 1 with "the config names 1 principal(s) but none has a credential ..." | PASS | `1.3-b-credential-unset.txt` |
| c: throwaway `ui` token: `/agui` 503 `backend_not_configured`; `/agent/settings`, `/index/*` 404; `/auth/session` 404; tools `ipfs_cat`, `ipfs_get`, `ipfs_ls`, `ipfs_node_info`, `ipfs_node_status`, no `ipfs_index_*` | PASS | `1.3-c-token-no-embeddings.txt` |
| e: cutover `auth.oidc` config accepted; log "auth.oidc is on: session tokens from https://oauth.know-me.tools are accepted on routes agui,settings,index,session" | PASS | `1.3-e-mcp-log.txt` |
| e: no token, garbage bearer, forged RS256 JWT (unknown kid and the live kid; correct iss, aud, sub; random signature) on `/agui`, `/mcp`, `/auth/session`, `/agent/settings` | PASS: 401 on each; `/index/search` 404 (probe 12) | `1.3-e-oidc-probes.txt` |
| e: the server reaches the live JWKS from the container | PASS (see note) | `1.3-e-jwks-egress-tcpdump.txt` |
| Web UI: daemon's `GET /webui/` on the RPC listener redirects (302) to `/ipfs/bafybeihd7ozlt…mpa` = `WebUIPath` | PASS | `1.3-webui-redirect-import.txt` |
| Web UI: `ipfs dag import --pin-roots` of the CAR into the container's throwaway node, then `GET /ipfs/<cid>/` serves index.html | PASS: gateway 8181 and RPC listener 5101 both `200 text/html`, `Etag` = the CID, body starts `<!doctype html>`; `ipfs resolve <cid>/index.html` = `bafkreibwbtdxk…` | `1.3-webui-redirect-import.txt` |

The `auth.oidc` config is `cfg-oidc.json` (identical to the previous run); the Web UI checks used scenario a's container (offline, API 5101, gateway 8181, `IPFS_PATH=/tmp/ipfs-smoke`).

Note on the JWKS egress: the startup prefetch holds a keep-alive connection to 23.239.29.33:443 (`oauth.know-me.tools`). The first probe set did not show a new connection; a second forged request with a new unknown `kid` at 13:24:44 produced an outbound request and a 1665-byte reply on that connection (`1.3-e-jwks-egress-tcpdump.txt`). The capture started after the startup DNS lookup, so DNS is not shown in this run.

## Not verified

- A valid ID token was not obtained, so acceptance of a real mapped token is not shown against the live issuer; the forged-token 401 bodies were not inspected, so signature rejection versus another claim failure is not distinguished. Valid-token behavior is covered by the unit tests in `go test ./mcp/...`.
- Web UI build reproducibility: the build ran once; the CID is that of this specific `build/` directory, and the CAR (sha256 above) is the artifact to import, not a rebuild. Node 24.11.1 was used, not the pinned 24.11.0.
- Whether this session-mode build actually behaves correctly in a browser against the console was not tested here (only that it is served).
- The `ghcr.io` registry digest, the `ipfs-0` import and the pod rollout are operator steps.
- The image is linux/amd64 built on an arm64 host; containers ran under emulation.

## Notes

- Containers started and removed by ID (`container-ids.txt`; the first `b` is the first b run whose exit message I did not capture, rerun as the second `b`). An alpine tcpdump sidecar shared scenario e's network namespace. Nothing pruned. Containers publish no ports; nothing touched 4001/5001/8080/5050 on the host.
- Forged JWTs were written to the session scratchpad and never printed or committed; `forge.py` is here. The scenario c token exists only inside its (removed) container.
- Scripts copied from the previous run: `run-scenario.sh` (image tag changed), `smoke-entry.sh`, `raw-run2.sh`, `raw-oidc.sh`, `tools.sh`, `probe.sh`, `forge.py`, `cfg-*.json`.

## Lead addendum: push and provenance (2026-10-19, LOW-13)

Operator ran `docker push ghcr.io/gqadonis/kubo:1140ff10c`; registry digest `sha256:3467dea860aeb097054bdf10391abcada49b48755b3a7bd9c0f585253f952cb6` (size 2407). Link to the tested image:

```
$ docker image inspect ghcr.io/gqadonis/kubo:1140ff10c --format '{{.Id}} {{json .RepoDigests}}'
sha256:a50f357ce2f48621ca47bf6d75a3429ecd826666cfc6d92e8a69665e65f64086 ["ghcr.io/gqadonis/kubo@sha256:3467dea860aeb097054bdf10391abcada49b48755b3a7bd9c0f585253f952cb6"]
$ docker buildx imagetools inspect --raw ghcr.io/gqadonis/kubo@sha256:3467dea8… | jq -r .config.digest,.mediaType
sha256:a50f357ce2f48621ca47bf6d75a3429ecd826666cfc6d92e8a69665e65f64086
application/vnd.docker.distribution.manifest.v2+json
```

The local image ID of the tested build (`sha256:a50f357c…`) is the manifest's `config.digest`, and the tag's RepoDigest is the pushed digest, so the deployed digest is the tested image.

Web UI CAR imported into `ipfs-0` before #50 (operator-directed): `Pinned root bafybeihd7ozltm2fciiiufczpfu4iahc4qfkhzkuro2ces75bmirdb3mpa success`, `Imported 388 blocks (21551384 bytes)`, `pin ls`: recursive.
