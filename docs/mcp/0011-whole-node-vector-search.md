# ADR-0011: Whole-node vector search: xberg extraction and model descriptions feeding the G12 index, operator-started bulk indexing, and a Web UI search route

## Status

**Accepted**, 2026-10-14, by the operator. Both answers are recorded verbatim in `.kbd-orchestrator/phases/agent-backend-settings/execution.md`.

- OG-16 (plan revision 4: decisions V1-V9 and recommendations O-5 to O-8 and O-10), verbatim: "approve the vector search plan revision".
- OG-17, OG-18 and OG-19 (recommendations O-1 to O-4 and O-9), verbatim: "go with your recommendations for OG-17, OG-18 and OG-19". The recommendations, now decided: Desktop describes images with a local vision model and video description is off; on LKE the describer is off until the operator has read Xiaomi's data terms, then hosted MiMo for images, and video only by a further explicit operator choice; no GPU on LKE, embeddings on CPU in-cluster; liter-llm is not deployed at first (if added later, the upstream image pinned by digest unless a fork-only feature is needed); Desktop uses operator-installed `xberg` and `surreal` binaries, not bundled; Hugging Face downloads only for the LKE xberg warm Job; artifact pins (the xberg image digest and macOS CLI checksum, the SurrealDB and embedder image digests) are resolved and recorded by the owning task at the time it runs, never floating tags.

This ADR extends ADR-0006 (embeddings in SurrealDB) and does not reopen it: `ipfs mcp` stays the only component that embeds and stores, the store stays SurrealDB through the standard library, the embedder stays an operator-configured OpenAI-compatible endpoint with no default, and a daemon-wide indexing hook stays refused.

## Amendment 2026-10-14: xberg source is the Know-Me-Tools fork

Operator decision, verbatim: "we will use a fork of xberg (https://github.com/Know-Me-Tools/xberg) using the git address: git@github.com:Know-Me-Tools/xberg.git to ensure that we can adapt xberg for our needs." This supersedes the ADR's "upstream `xberg-io/xberg`, not the Prometheus-AGS fork" for the source of xberg; the extraction design, pins by version and digest, and every off switch are unchanged.

- **Source:** `Know-Me-Tools/xberg` (git `git@github.com:Know-Me-Tools/xberg.git`). Reason: the operator wants to adapt xberg for our needs (a candidate first adaptation is an auth option or Unix-socket binding, which would close the accepted loopback residual).
- **Identical today:** the fork's `main` and tag `v1.3.10` are the same commit as upstream (`d926382a`; full SHA pinned in `versions.toml`). Until the fork diverges, the upstream `v1.3.10` release binaries and the upstream `ghcr.io/xberg-io/xberg:v1.3.10` image are builds of the same commit, so the binary verified in `openspec/changes/mcp-content-extractor/evidence/2026-10-14-task-1.1/` stays valid. That evidence is a historical record and is not rewritten.
- **Open operator item:** the fork has no GitHub releases and no container image, so binaries and a `ghcr` image built from the fork are not yet published, and where they will be published is undecided. The fork also inherits upstream's enabled publish workflows; whether to disable or adapt them is for the operator, and nothing here touches the fork repository.
- **LKE image reference:** once a fork-built image exists, the `ipfs-xberg` image becomes that fork-built image pinned by digest (never a floating tag). Until then it stays the upstream `v1.3.10` image of the same commit, pinned by digest at the time `lke-vector-search` runs.
- **Desktop binary:** the operator-installed `xberg` is the verified upstream `v1.3.10` macOS arm64 CLI until a fork release exists; after the fork diverges, the fork build replaces it and its checksum is re-verified by the owning task.

## Date

2026-10-14

## Context

- Operator direction of 2026-10-14, recorded verbatim in `.kbd-orchestrator/phases/agent-backend-settings/execution.md` ("Operator direction 2026-10-14"): vector search over the files on the node, on the LKE cloud node and on IPFS Desktop (macOS arm64); text and Markdown, PDF and Office documents, source code and JSON/CSV, images, and image and video search through Xiaomi MiMo-V2.6 descriptions ("embeddings off inference"); document processing with upstream `xberg-io/xberg` at its latest release, not the Prometheus-AGS fork; a configurable embedder host; `GQAdonis/liter-llm` "if necessary" for model access; search from agent chat and MCP clients and from a search box in the Web UI.
- Research of the same day: `.kbd-orchestrator/phases/agent-backend-settings/research-vector-search.md`. Facts used below come from it unless marked otherwise.
- What G12 already delivers (`docs/mcp/embeddings.md`, ADR-0006): chunking, an OpenAI-compatible `/embeddings` call, SurrealDB in embedded mode (a supervised operator-supplied `surreal` on loopback) or remote mode (a URL), four tools (`ipfs_index_status`, `ipfs_index_search`, `ipfs_index_cid`, `ipfs_index_reconcile`), pin-aware deletion, `egress_ack` checks before any request, offline reads for search hits. Gaps: only adds made through `ipfs mcp` are indexed automatically, only `text/plain` and `text/markdown` up to 1 MiB, search only through agents, nothing configured on LKE.
- xberg `v1.3.10` (2026-10-13, MIT): 107 formats including PDF with OCR, Office, ODF, JSON, CSV, YAML, code via tree-sitter, image OCR, audio and video transcription of the audio track only. It does not describe visual content. It runs as a REST server (`xberg serve`, `POST /extract`), an MCP server, a CLI, or a library (the Go binding uses cgo and downloads `libxberg_ffi` at setup). Release assets checked on 2026-10-14 with `gh release view v1.3.10 -R xberg-io/xberg`: `xberg-cli-aarch64-apple-darwin.tar.gz` (51 MB) exists, so a native macOS arm64 CLI is available; the Go archive for macOS arm64 is 337 MB. Container images `ghcr.io/xberg-io/xberg:<ver>` (about 1.5-2.1 GB) and `:<ver>-core` are linux/amd64 and linux/arm64 only. **No authentication is documented** for the REST server. Layout, PaddleOCR and NER models are downloaded at runtime from the Hugging Face Hub unless pre-warmed.
- MiMo-V2.6 (Xiaomi, 2026-09-22): hosted OpenAI-compatible API at `https://api.xiaomimimo.com/v1` (`mimo-v2.6-flash`, `mimo-v2.6-pro`) with `image_url` parts and a `video_url` part carrying `fps` and `media_resolution`; no embeddings endpoint. Self-hosting Flash needs 4-8 datacenter GPUs; there is no official Mac or CPU path.
- liter-llm (`GQAdonis/liter-llm`, fork of `xberg-io/liter-llm`): an OpenAI-compatible gateway with `/v1/chat/completions` and `/v1/embeddings`. Its request type has no `video_url` content part (`crates/liter-llm/src/types/common.rs:106`), and its built-in `xiaomi_mimo` provider is chat-only with `"vision": false` (inferred from source, not run). The fork has no published release or image; upstream's latest is `v2.2.3` with image `ghcr.io/xberg-io/liter-llm`. Its default listener is `0.0.0.0:4000`.
- `ipfs mcp` is a local command (`cmd/ipfs/kubo/mcp_register.go`: `localCommands["mcp"]`, `NoRemote: true`), not part of the `/api/v0` command tree, and it reaches the daemon only through `client/rpc` (F4, ADR-0001).
- Binding rules: `AGENTS.md` Stability (no `/api/v0`, gateway, CID or wire change) and User Agency (no silent CID injection, no appropriation of the operator's resources, every endpoint configurable with an off switch, every feature documented with a disabled-state test). D12 of plan revision 3: `ipfs mcp` settings live in the `ipfs mcp` config file, the contract config schema and `docs/mcp/config-reference.md`, never in Kubo's repo config.

## Decision

### V1. xberg runs as a service and is never linked into the `ipfs` binary

- `ipfs mcp` talks to xberg's REST API (`POST /extract`, `output_format: markdown`) with `net/http` and `mime/multipart`. No xberg module, no cgo and no native library enter any of Kubo's three `go.mod` files.
- Two modes, mirroring ADR-0006's store modes:
  - **embedded:** `ipfs mcp` supervises an operator-supplied `xberg` binary given by absolute path (no `PATH` lookup, no download by Kubo), started as `xberg serve` on `127.0.0.1` with an ephemeral port, a cache directory the operator names, and a minimum-version check. It stops when `ipfs mcp` exits and orphans are cleaned up the way `surreal` orphans are. Model downloads are off unless `extractor.embedded.allow_model_downloads` is true; with downloads off the operator pre-warms the cache. Which xberg flag or environment variable enforces this is unverified and is the first task of `mcp-content-extractor`.
  - **remote:** an operator-configured `base_url` (for example a ClusterIP Service on LKE). There is no default URL. Because xberg documents no authentication, a remote xberg must sit on a private network or behind an authenticating proxy; the optional `credential` exists for that proxy.
- Bytes reach xberg from `cat` of the indexed CID, capped at `extractor.max_bytes` before the request is built.

### V2. Two new stages in `ipfs mcp`, one embedding path

```
            cat (offline for bulk and search)          egress_ack checked for every host below,
CID ──► bytes ──► kind (name + sniff, contract map)    before any request is built
                    │
                    ├─ text, markdown ───────────────────────────────┐ (native, as in G12)
                    ├─ document, code, data ──► extractor (xberg) ───┤ source: extracted | ocr
                    ├─ image ──► extractor (OCR, if on) ─────────────┤ source: ocr
                    │        └─► describer (chat model) ─────────────┤ source: description
                    └─ video ──► extractor (transcript, if on) ──────┤ source: transcript
                             └─► describer (video, if enabled) ──────┤ source: description
                                                                     ▼
                                       existing chunker ──► embedder ──► SurrealDB (keyed by CID)
```

- The **extractor** (xberg) turns documents, code, data, image text (OCR) and video audio (transcription) into Markdown.
- The **describer** is any OpenAI-compatible `/chat/completions` endpoint with image (and, if enabled, video) input. It returns a text description; that text is chunked and embedded by the same embedder as everything else, so images and videos are searched in the same vector space as documents ("embeddings off inference"). MiMo-V2.6 hosted is one such endpoint; a local vision model is another.
- Media go to the describer as base64 `data:` URLs inside the request, never as a gateway URL. A URL would publish the CID to the model host and make it fetch the content from the node, which Desktop cannot serve publicly anyway.
- The fixed describer instruction is part of the contract, not a config key. The description is untrusted model output and is labelled as such everywhere it is shown.
- Which stage handles which MIME type is one contract file (`mcp/contract/derivations/content-kinds.json`), read by `ipfs mcp` and the Web UI alike. `embeddings.policy.mime_allowlist` stays the single allowlist; its default stays `["text/plain", "text/markdown"]`, so configuring an extractor or describer does nothing until the operator adds the types they want.
- **Hit text.** G12 recovers a hit's text by reading the content again and cutting it the same way. That does not work for derived text: extraction is expensive and descriptions are not deterministic. Entries with `source` other than `text` therefore store their chunk text in SurrealDB beside the vector (an additive field). Plain-text entries keep the re-read behaviour.
- Hits gain `kind`, `source`, `mime_type` and, for descriptions, `described_by` (the model). These are additive optional output fields of `ipfs_index_search`; its input gains optional `kinds` and `sources` filters.

### V3. liter-llm is an optional gateway, not a dependency

- `ipfs mcp` needs only OpenAI-compatible endpoints for the embedder and the describer. liter-llm is a supported deployment choice that puts provider keys in one operator-owned TOML and gives `ipfs mcp` one host to acknowledge. Nothing in `ipfs mcp` is liter-llm-specific.
- **Video does not pass through liter-llm today**: its content parts are `text`, `image_url`, `document` and `input_audio`, so MiMo's `video_url`, `fps` and `media_resolution` would be rejected or dropped. Video description goes directly to the describer endpoint. Images through liter-llm to MiMo also need its `xiaomi_mimo` provider's vision flag enabled (unverified whether it accepts images today).
- When liter-llm is deployed it listens on loopback or a ClusterIP only, never `0.0.0.0` on a reachable interface, and always with a master key.
- Decided at OG-19: no gateway is deployed at first; one is added only when a second provider is configured, and then as the upstream image `ghcr.io/xberg-io/liter-llm` pinned by digest, unless a fork-only feature is needed (an image built from the `GQAdonis/liter-llm` fork, which publishes no artifact).

### V4. Bulk indexing of pins and MFS: operator-started, capped, resumable, never in the background by default

- Three new tools: `ipfs_index_bulk_start` (write class, confirmed on every transport per ADR-0003), `ipfs_index_bulk_status` (read) and `ipfs_index_bulk_cancel` (write class, confirmed).
- **Scope of "all files".** A run walks UnixFS files reachable from recursive and direct pins (`pin/ls`) and from MFS under `bulk.mfs_root` (`files/ls`), descending directories with `ls`. It does not index unpinned blocks in the blockstore (the gateway cache on LKE holds content strangers asked for, not the operator's content, and pin-aware deletion would delete it again), raw blocks, or non-UnixFS DAGs (`not_a_file`).
- **Every read is offline.** `cat` and `ls` carry `offline: true`, as search already does. A file with missing blocks is skipped with `not_local`; a run never fetches from the network. This matters: with the default `Provide.Strategy` a fetched block would be stored and announced, which would be a CID entering the provide queue without a local cause.
- **Nothing is added, pinned or announced.** Indexing issues no `add`, `block/put`, `dag/put`, `pin/add`, `files/write` or `routing/provide` call. Images that xberg extracts from documents are discarded, never added.
- **Caps** (`embeddings.bulk`): files per run, bytes per run, files per minute, one run at a time.
- **Resumable, never automatic.** Run state (cursor, counts, the starting principal, caps) is stored in SurrealDB. If `ipfs mcp` stops, the run is `paused` on the next start and stays paused until a principal resumes it through `ipfs_index_bulk_start` with `resume: true`, which asks for confirmation again. There is no scheduler and no start-up trigger. Files already indexed are skipped (`already_indexed`), so a later run indexes only what is new.
- **Who may start it.** The calling principal's `egress_ack` must cover every host the run may contact (embedder, remote store, remote extractor, describer); the check runs at start and before each file. The operator identity (stdio and `--auth-token-*`) has no `egress_ack`, so, as in G12, it cannot index. The confirmation prompt names the sources, the caps and every egress host.
- This is consistent with ADR-0006's refusal of a daemon-wide hook: each run has a local cause (an explicit, confirmed operator request), reads only what the operator pins or keeps in MFS, and changes nothing on the node.
- The Web UI shows bulk status read-only. Starting a run goes through the agent chat or any MCP client, where the confirmation already exists; no new write surface is added to the Web UI.

### V5. A Web UI search route on the `ipfs mcp` HTTP surface, principal-scoped

- Two routes on the `ipfs mcp --http` listener: `POST /index/search` (request body = `ipfs_index_search` input schema, response = its output schema) and `GET /index/status` (`ipfs_index_status` output plus the bulk run summary). The contract names both routes in a route descriptor that references the tool schemas by path; there is no second copy of the shapes.
- Authorization is the tool's: principal kinds `ui` and `human-mcp` with the `index` group, the same `egress_ack` check, the listener's `origin_allowlist` CORS rule and body limits. An `admin` principal is refused (ADR-0009 section 3). Responses carry hit text as data; the Web UI renders it as plain text, never as HTML or Markdown.
- The routes mount only with `--http`, an `embeddings` section and `embeddings.search_route.enabled` not `false`; otherwise they answer 404 and nothing is read or dialed.
- On LKE the console routes for `/index/*` are bearer-only and outside the basic-auth SecurityPolicy, exactly like `/agui` (ADR-0009 section 4). On Desktop the Web UI reaches them through the same endpoint and token source as the agent chat.
- No `/api/v0` endpoint is added or changed. `ipfs mcp` uses only existing RPC calls (`pin/ls`, `files/ls`, `files/stat`, `ls`, `cat`); `--print-rpc-allowed-paths` lists them so a scoped `API.Authorizations` entry keeps working.

### V6. Configuration: the `ipfs mcp` config file, each key with a default and an off switch (D12)

All keys are additive under the existing `embeddings` section of the contract config schema (`mcp/contract/config/ipfs-mcp-config.schema.json`), rendered into `docs/mcp/config-reference.md` by the existing generator. No Kubo repo config key is added and `docs/config.md` gains no entry beyond its existing pointer.

| Key | Default | Off switch |
|-----|---------|-----------|
| `embeddings.extractor` | `null` | Remove the block: no xberg process or request; only `text/plain` and `text/markdown` are indexed, as in G12 |
| `embeddings.extractor.mode` | none (required: `embedded` or `remote`) | Remove the block |
| `embeddings.extractor.embedded.xberg_binary` | none (absolute path, required in embedded mode) | Remove the embedded block: no process starts |
| `embeddings.extractor.embedded.cache_dir` | none (absolute path, required in embedded mode) | Remove the embedded block |
| `embeddings.extractor.embedded.min_version` | `"1.3.10"` | Always checked; an older binary is refused |
| `embeddings.extractor.embedded.allow_model_downloads` | `false` | `false` (default): xberg runs with downloads disabled; missing models make the file `extraction_failed` |
| `embeddings.extractor.remote.base_url` | none (required in remote mode) | Remove the remote block: no connection |
| `embeddings.extractor.remote.credential` | `null` | `null`: no credential sent (for an authenticating proxy only) |
| `embeddings.extractor.timeout_seconds` | `120` | Always applies |
| `embeddings.extractor.max_bytes` | `52428800` (50 MiB; at most 100 MiB) | Larger files are skipped `too_large` |
| `embeddings.extractor.ocr` | `false` | `false`: no OCR of images or scanned PDFs |
| `embeddings.extractor.transcribe` | `false` | `false`: no transcription of audio tracks |
| `embeddings.describer` | `null` | Remove the block: no description request; images and videos are skipped `describer_not_configured` unless OCR or transcription covers them |
| `embeddings.describer.base_url`, `.model` | none (required) | Remove the block |
| `embeddings.describer.api_key` | `null` | `null`: requests carry no key |
| `embeddings.describer.kinds` | `["image"]` | Empty list: nothing is described; video only when listed |
| `embeddings.describer.max_output_tokens` | `512` | Always applies |
| `embeddings.describer.image_max_bytes` | `20971520` (20 MiB) | Larger images are not described |
| `embeddings.describer.video_max_bytes` | `52428800` (50 MiB) | Larger videos are not described |
| `embeddings.describer.video_fps` | `1` (0.1 to 10) | Always applies to video |
| `embeddings.describer.max_requests_per_hour` | `120` | `0`: the describer is disabled |
| `embeddings.describer.timeout_seconds` | `180` | Always applies |
| `embeddings.bulk.enabled` | `true` (inert without an `embeddings` section) | `false`: the three bulk tools are not listed |
| `embeddings.bulk.sources` | `["pins", "mfs"]` | Empty list: a run indexes nothing |
| `embeddings.bulk.mfs_root` | `"/"` | Narrow it to limit the MFS walk |
| `embeddings.bulk.max_files_per_run` | `1000` | Always applies |
| `embeddings.bulk.max_bytes_per_run` | `1073741824` (1 GiB) | Always applies |
| `embeddings.bulk.files_per_minute` | `30` | Always applies |
| `embeddings.search_route.enabled` | `true` (inert without `--http` and an `embeddings` section) | `false`: `/index/search` and `/index/status` answer 404 |

The whole feature remains off without an `embeddings` section, as in ADR-0006. Embedding settings are read at start; changing them restarts `ipfs mcp` (Desktop restarts its sidecar). Adding them to the hot-reload settings API of ADR-0009 is deferred (O-6, decided at OG-16).

### V7. Egress: `egress_ack` covers every new host unless it is loopback

- The host set checked against the calling principal's `egress_ack` grows from {embedder, remote store} to {embedder, remote store, remote extractor, describer}. A loopback host is covered by `local`; the embedded extractor, like the embedded store, needs no acknowledgement.
- A gateway such as liter-llm is acknowledged by its own host. The operator must understand that acknowledging a gateway acknowledges every provider behind it; the docs say so next to the key.
- The check runs per stage and before any request is built. A refused stage sends nothing and is recorded on the file (`egress_not_acknowledged` with the hosts); the other stages of the same file still run. A bulk run is refused at start if any configured stage's host is not acknowledged.
- A hosted describer is never a default. `docs/mcp/embeddings.md` and the changelog name the fact that describing a file sends its bytes to that host.

### V8. Deployment

**LKE (know-me-cluster, namespace `ipfs`, change `lke-vector-search`):**

```
 console host (bearer, outside basic auth)
        │  /index/search, /index/status, /agui, /mcp
        ▼
 ┌──────────── pod ipfs ────────────┐       ClusterIP only, NetworkPolicy: ingress from pod ipfs only
 │ kubo daemon ◄─RPC─ ipfs-mcp ─────┼──► surrealdb   (StatefulSet, PVC, root credential: out-of-band Secret)
 └──────────────────────────────────┘──► xberg       (Deployment :8000, model cache PVC, egress denied)
                                    ──► embedder    (in-cluster CPU model, no GPU: OG-18)
                                    ──► liter-llm   (not deployed at first: OG-19; if added, master key Secret)
                                    ──► api.xiaomimimo.com (off until OG-17's terms condition is met; then in egress_ack)
```

- SurrealDB in remote mode: the Kubo image does not contain `surreal` (BSL 1.1; ADR-0006 keeps it out of Kubo's artifacts), so embedded mode is not used in the pod.
- xberg from `ghcr.io/xberg-io/xberg:v1.3.10` (or `-core`) pinned by a digest that `lke-vector-search` resolves and records at the time it runs (OG-19: never a floating tag), never exposed on a Gateway listener; models pre-warmed by a one-off Job that is the only workload allowed to reach Hugging Face (OG-19 approves that egress for this Job only).
- Secrets (SurrealDB credential, liter-llm master key, Xiaomi key) are created out of band, never in git, mounted with `optional: true`; an absent Secret disables the dependent stage, never the pod (ADR-0009 CRITICAL-2 pattern).
- The `ui` principal's `egress_ack` lists exactly the in-cluster hosts; the Xiaomi host is added only after the operator has read Xiaomi's data terms and confirms the hosted describer (OG-17), and then for images only — video on LKE takes a further explicit operator choice.

**IPFS Desktop (macOS arm64, change `desktop-search-sidecar`):**

- Search is off by default. When the operator turns it on, Desktop asks for absolute paths to an operator-installed `surreal` and `xberg` (the `xberg-cli-aarch64-apple-darwin` release asset) and an embedder URL, and its `ipfs mcp` sidecar writes an `embeddings` block in embedded mode for both, with data and cache directories under Desktop's userData. Desktop downloads and bundles neither binary (O-4, decided at OG-19: operator-installed by absolute path).
- The embedder is local by default in the docs (for example Ollama with `bge-m3`). Per OG-17, Desktop describes images with a local vision model through any OpenAI-compatible server, and video description is off. **Video description has no Mac path** in any case: it needs a hosted describer or datacenter GPUs.
- A non-loopback embedder or describer host is added to the Desktop `ui` principal's `egress_ack` only after a confirmation that names the host, with Touch ID, as D4 does for the chat backend.

### V9. Stability and User Agency checklist

- No `/api/v0`, gateway, default CID recipe or wire change; no Kubo repo config key.
- No default endpoint: extractor, describer, embedder and store URLs have no default.
- No silent CID injection: indexing reads offline and adds, pins and announces nothing (falsifier V-1 below).
- Resource use is opt-in and capped: nothing indexes without an `embeddings` section, a configured stage, an allowlisted MIME type and either an `ipfs mcp` add with `policy.auto` or an operator-started, confirmed run.
- Every new feature has an off switch (V6), documentation (`docs/mcp/embeddings.md`, `docs/mcp/config-reference.md`, the changelog with a disable example) and a disabled-state test in each implementing change.

## Alternatives considered

- **Link the xberg Go binding into `ipfs`.** Rejected: cgo in a codebase that has none, a build-time download of a native library, about 337 MB per platform, and a new module in `go.mod` for every Kubo build.
- **Let agents call xberg's own MCP server directly.** Rejected: results carry no CID, no `egress_ack`, no pin-aware deletion, and the Web UI would have no index to search.
- **Use xberg's `embed` as the embedder.** Not chosen: its REST exposure is unverified and it would couple embedding to extraction. The embedder stays a separate OpenAI-compatible endpoint; this can be revisited if xberg's embedding API proves out.
- **Multimodal vector embeddings for images (an image encoder).** Rejected for now: a second vector space with its own dimensions and index, and not what the operator asked for (descriptions from MiMo).
- **Send gateway URLs instead of bytes to the describer.** Rejected: it discloses the CID, requires a publicly reachable gateway (Desktop has none) and lets the model host fetch anything the URL allows.
- **Route video through liter-llm.** Not possible with today's content-part types; reconsidered if the fork or upstream adds `video_url`.
- **A background watcher or daemon-wide hook that indexes everything the node stores.** Refused (ADR-0006, User Agency). No flagged variant.
- **A scheduled bulk run, on by default.** Rejected: background resource use by default. An operator can schedule calls to the tool themselves.
- **Index unpinned blocks (the gateway cache).** Rejected: not the operator's content, and pin-aware deletion would remove it again.
- **A new `/api/v0` search endpoint or `Embeddings.*` keys in Kubo's config.** Refused: Stability rules and D12.
- **The Web UI as a full MCP client on `/mcp` for search.** Not chosen: a session-based JSON-RPC client is heavier than a search box needs; the dedicated route reuses the tool's schemas and authorization, so it adds no second contract.
- **A "Start indexing" button in the Web UI.** Deferred: it would be a new write surface on the console; the agent chat already carries confirmed writes.
- **The Prometheus-AGS xberg fork.** Superseded by the operator on 2026-10-14: its only tag is stale Kreuzberg-era code.
- **Self-host MiMo-V2.6 on LKE.** Rejected at OG-18 (no GPU on LKE): Flash needs tensor parallel 4 to 8 on datacenter GPUs.

## The uncomfortable thing

- **Xiaomi hosted egress.** The describer that does what the operator asked for, MiMo-V2.6 on video, is in practice Xiaomi's hosted API. Every described image or video leaves the node as bytes to a third party whose retention and training terms this ADR has not checked. The design makes that an explicit, per-host acknowledgement and never a default, but if the operator turns it on for "all files", a bulk run sends every allowlisted image and video the node pins to Xiaomi, capped only by `max_requests_per_hour` and the run caps.
- **There is no Mac path for video description.** On Desktop, video search either uses hosted egress or covers only the audio transcript. The operator's "image and video search" goal is only half met on Desktop without a hosted describer.
- **Indexing all files costs real resources.** OCR, layout models and Whisper are CPU-heavy; the xberg image is 1-2 GB; the Desktop `xberg` CLI is 51 MB compressed plus models; a bulk run over a large pinset spends hours of CPU and, with remote stages, bandwidth and provider spend. The caps make each run bounded and visible; they do not make it cheap.
- **xberg downloads models at runtime.** Unless downloads are disabled and the cache pre-warmed, xberg contacts the Hugging Face Hub, an endpoint the operator did not configure. Whether xberg v1.3.10 honours an offline switch is unverified, which is why `allow_model_downloads` defaults to `false` and the first extractor task verifies it. If no switch works, the embedded mode on Desktop cannot promise "no unconfigured endpoint" and the docs must say so.
- **xberg's API is unauthenticated.** On Desktop the embedded xberg listens on loopback, so any local process can submit files to it and spend the operator's CPU. On LKE it is reachable from any pod the NetworkPolicy lets through; if the cluster does not enforce NetworkPolicy (unverified), any pod in the cluster can use it.
- **The index now holds copies of derived text.** Extracted document text and model descriptions sit in SurrealDB (V2), on a PVC on LKE and under userData on Desktop. Deletion still follows pins, but between an out-of-band unpin and the next reconciliation that text stays readable to anyone with the store.
- **Descriptions are model output searched as if they were content.** A wrong or injected description ranks like a fact. The Web UI and tool output label it as "AI description by <model>", and agents treat it as untrusted, but a user can still be misled.
- **Supply chain.** Two new third-party runtimes (xberg, optionally liter-llm) and their model weights run next to the node. Pinning by version and digest limits drift; it does not audit them.

## Consequences

### Positive

- One index and one embedding path for every file type; agents, MCP clients and the Web UI search the same data.
- Every new endpoint is operator-configured, acknowledged per host and switchable off; nothing changes for operators who do not add the new blocks.
- No Kubo build, `go.mod` or RPC change; xberg and liter-llm can be upgraded or removed without rebuilding Kubo.

### Negative

- More moving parts on both nodes (xberg, SurrealDB, an embedder, optionally a gateway and a hosted describer).
- Derived text is stored, which G12 avoided.
- Embedding settings are not hot-reloaded; changing them restarts `ipfs mcp`.
- The Web UI cannot start a bulk run; the operator uses the agent chat or an MCP client.

## Falsifiers

- **V-1 (no CID injection):** after a bulk run over a node with pins and MFS, the pin set, the blockstore key set and the provide queue are unchanged, and the recorded RPC calls contain no `add`, `block/put`, `dag/put`, `pin/add`, `files/write` or `routing/provide`. A run over a pin with missing blocks fetches nothing (`not_local`).
- **V-2 (egress):** with the describer host absent from `egress_ack`, a run sends zero requests to it and records `egress_not_acknowledged` for that stage.
- **V-3 (off switches):** with no `extractor` and no `describer` block, indexing behaves exactly as G12 (existing tests unchanged); with `search_route.enabled: false` both routes answer 404; with `bulk.enabled: false` the bulk tools are not listed.
- **V-4 (no auto-resume):** a run interrupted by stopping `ipfs mcp` is `paused` after restart and indexes nothing until resumed with confirmation.
- **V-5 (route scope):** an `admin` bearer on `/index/search` gets 403; a `ui` principal without the `index` group gets 403.

## References

- ADR-0001 (placement, F4), ADR-0003 (confirmation), ADR-0004 (principals, `egress_ack`), ADR-0006 (embeddings in SurrealDB), ADR-0009 (runtime settings, console routes, absent Secrets).
- `.kbd-orchestrator/phases/agent-backend-settings/research-vector-search.md`
- `.kbd-orchestrator/phases/agent-backend-settings/execution.md`, "Operator direction 2026-10-14"
- `.kbd-orchestrator/phases/agent-backend-settings/plan-revision-4-proposal.md`
- https://github.com/xberg-io/xberg/releases/tag/v1.3.10 (upstream release; the fork's `v1.3.10` is the same commit)
- https://github.com/Know-Me-Tools/xberg (the fork, git `git@github.com:Know-Me-Tools/xberg.git`), amendment 2026-10-14
- https://github.com/GQAdonis/liter-llm
