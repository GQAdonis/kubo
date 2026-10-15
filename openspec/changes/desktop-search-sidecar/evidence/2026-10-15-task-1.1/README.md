# Evidence: desktop-search-sidecar 1.1, xberg v1.3.10 under Desktop on macOS arm64

Date: 2026-10-15. Host: macOS 26.7.2 (25G320), arm64, Electron 44.3.0 (the Desktop dev tree's
`node_modules/electron`). Nothing was installed system-wide; the binary was unpacked into scratch
directories outside every repository. Builds on `mcp-content-extractor` `evidence/2026-10-14-task-1.1/`,
which holds the serve/extract/offline findings; this file adds what Desktop needs.

## Artifact and checksum

- Asset: `xberg-cli-aarch64-apple-darwin.tar.gz`, release `v1.3.10` of `xberg-io/xberg`, downloaded again today
  with `gh release download v1.3.10 -R xberg-io/xberg -p xberg-cli-aarch64-apple-darwin.tar.gz`.
- `shasum -a 256` of the download: `378cc1a7e76addb1302944fd155daead565dba1aa913cec56d19ef2a5878b45a`.
- Release digest from `gh api repos/xberg-io/xberg/releases/tags/v1.3.10`: `sha256:378cc1a7e76addb1302944fd155daead565dba1aa913cec56d19ef2a5878b45a`. They match, and match yesterday's record.
- SHA-256 of the unpacked `xberg` executable: `a39086a4df63c6fe1366e7e910ebaba72cc1144eae0a7164c4ecb32030e407f7` (same for the freshly unpacked copy and the copy used on 2026-10-14).
- Tag `v1.3.10` resolves to commit `d926382a0c124dcac4562c7c61a5086d71a3aa68`. The `Know-Me-Tools/xberg` fork (parent `xberg-io/xberg`, default branch `main`) contains that same commit (`gh api repos/Know-Me-Tools/xberg/commits/d926382a...` returns it) and has no release yet (`releases` is `[]`), so the upstream v1.3.10 artifact of the identical commit is what was verified (OG-19 allows this).
- `xberg --version` prints `xberg 1.3.10`.
- The tarball carries the executable plus `libaom`, `libde265`, `libheif`, `libsharpyuv`, `libvmaf`, `libx265` dylibs, `LICENSE` and `THIRD_PARTY_LICENSES.md`. The dylibs must stay next to the executable; the operator points Desktop at the executable and leaves the directory intact.

## Gatekeeper and quarantine, launched as a child of Electron

Method: `ELECTRON_RUN_AS_NODE=1 <Electron> spawn-child.cjs <xberg>` runs `child_process.spawnSync(<xberg>, ['--version'])` from the Electron binary, which is how the Desktop supervisor spawns its sidecar.

| Case | Result |
|---|---|
| As downloaded and unpacked (only `com.apple.provenance` set, no quarantine) | exit 0, stdout `xberg 1.3.10`, empty stderr |
| After `xattr -w com.apple.quarantine "0083;...;Safari;..."` on the executable (the flag a browser download sets) | exit 0, stdout `xberg 1.3.10`, empty stderr; no dialog, no kill |

- Signature: `codesign -dv` reports `Signature=adhoc`, `TeamIdentifier=not set`; `spctl -a -t exec -vv` says `rejected` (it is not notarized, as expected for a CLI tarball). A rejected `spctl` assessment did not stop the child in either case above.
- Limit of this evidence: the quarantine attribute was written by hand with `xattr`, so macOS has no matching quarantine-database event for it. A tarball a person downloads in a browser and unpacks with Archive Utility carries a real one, and on some macOS releases the first run of such a file is blocked ("cannot be opened because the developer cannot be verified") until it is allowed in System Settings, Privacy and Security, or `xattr -dr com.apple.quarantine <dir>` is run on the unpacked directory. That path was not exercised; it is **open**, with that workaround, and the Desktop Search section must tell the operator to clear the quarantine flag on the directory they installed the binaries in. Desktop does not clear it (it never downloads or modifies the binaries).

## `serve` flags, bind, cache and offline switch as a Desktop child will use them

- `xberg serve --help`: `-H/--host`, `-p/--port`, `--log-level`, `-c/--config` (`.toml`, `.yaml`, `.yml` or `.json` only; `--config /dev/null` fails with "Config file must have .toml, .yaml, .yml, or .json extension", so a supervisor must pass a real `*.json` file such as `{}`). Precedence: CLI flags, then `XBERG_HOST`/`XBERG_PORT`, then the config file, then `127.0.0.1:8000`.
- Run with a minimal environment (`env -i HOME PATH=/usr/bin:/bin HF_HUB_OFFLINE=1 XBERG_CACHE_DIR=<dir> HF_HOME=<dir> XBERG_CORS_ORIGINS=http://127.0.0.1:18747`) as `xberg serve --host 127.0.0.1 --port 18747 --config xberg-empty.json`:
  - `netstat -an -p tcp` showed only `127.0.0.1.18747 LISTEN`.
  - `GET /health` returned `{"status":"healthy","version":"1.3.10", ...}`.
  - `POST /extract` of a text file with `files` and `output_format=markdown` returned 200 with the content.
  - The log said "CORS configured with 1 explicit allowed origin(s)".
  - `SIGTERM` ended it; it was stopped by its own pid.
  - The scratch `HF_HOME` stayed empty (no model download attempted).
- Default cache path: `xberg cache stats` printed `Directory: <current working directory>/.xberg`. With no `XBERG_CACHE_DIR` the cache lands in the working directory of the process, so the supervisor must set `XBERG_CACHE_DIR` (and `HF_HOME`) under Desktop's userData and a working directory it owns; relying on the default would write `.xberg` wherever the sidecar happened to start. (The stat call did not create the directory.)
- Offline mechanism: `HF_HUB_OFFLINE=1`, as recorded on 2026-10-14 (a missing model is an `errors[]` entry or a `processing_warnings` entry, never a download). Desktop stays offline (OG-19), so image OCR with a model that is not cached and layout detection degrade; plain text, Office, CSV and text PDFs extract natively.

## SurrealDB

`surreal` was not part of this task, but the operator's installed binary is `~/.prometheus/bin/surreal`; `surreal version` prints `3.3.0 for macos on aarch64`. Desktop uses whichever absolute path the operator sets and does not search `PATH`.

## Not verified / open

- Real (quarantine-database) Gatekeeper first-run behavior of a browser-downloaded tarball (see above).
- A run that downloads models (deliberately offline).
- Windows and Linux builds (the Search section is built for macOS arm64 first; the code does not depend on the platform, the binaries the operator installs do).
