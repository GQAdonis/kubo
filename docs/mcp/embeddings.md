# Embeddings and the search index

`ipfs mcp` can chunk text content, send it to an embedding model the operator configures, store the vectors in SurrealDB and answer semantic searches over them. Beyond plain text and Markdown it can route documents, code, structured data, images and videos through an extraction stage (xberg) and a media description stage (any OpenAI-compatible chat model), index everything the operator pins or keeps in MFS with an operator-started bulk run, and answer searches over HTTP for the Web UI. This page covers the two store modes, the content pipeline, bulk indexing, the search routes and the two deployments. The decisions behind the design are [ADR-0006](0006-embeddings-surrealdb.md) (the index itself) and [ADR-0011](0011-whole-node-vector-search.md) (whole-node vector search); the authoritative list of keys, defaults and off switches is the config schema in `mcp/contract/config/ipfs-mcp-config.schema.json`, rendered in the [configuration reference](config-reference.md#embeddings).

The whole feature is off unless the config file named by `--mcp-config` (or `IPFS_MCP_CONFIG`) has an `embeddings` section. Without it no `surreal` or `xberg` process starts, no store, embedder, extractor or describer is contacted, and the `ipfs_index_*` tools are not listed (`ipfs_node_status` reports them as `not_configured`). Removing the section is the off switch. The feature adds no Kubo repo config key and changes nothing in `docs/config.md`, the `/api/v0` RPC API or the gateway.

## Contents

- [Try it](#try-it)
- [What leaves the machine](#what-leaves-the-machine)
- [Store modes](#store-modes)
  - [Embedded mode](#embedded-mode)
  - [Remote mode](#remote-mode)
  - [The embedder](#the-embedder)
- [Content kinds](#content-kinds)
- [The extractor (xberg)](#the-extractor-xberg)
- [The describer](#the-describer)
- [Indexing](#indexing)
  - [Policy](#policy)
  - [Automatic indexing after `ipfs_add`](#automatic-indexing-after-ipfs_add)
  - [Index tools](#index-tools)
  - [Bulk indexing](#bulk-indexing)
  - [Pin-aware deletion](#pin-aware-deletion)
  - [Reconciliation](#reconciliation)
- [Search over HTTP](#search-over-http)
- [Setups](#setups)
  - [LKE (cloud node)](#lke-cloud-node)
  - [IPFS Desktop](#ipfs-desktop)
- [Limitations](#limitations)

## Try it

This walks through embedded mode with a local embedder, from an empty config to a search hit. You need a running daemon, a `surreal` binary (3.0.0 or newer) that you installed yourself, and an OpenAI-compatible embeddings server on this machine. The example uses Ollama with `bge-m3`; any server that answers `POST /v1/embeddings` works, with `dimensions` set to its vector length.

1. Write `ipfs-mcp.json`. The principal `indexer` may add content and use the index tools, and its content may go to loopback hosts only:

   ```json
   {
     "principals": [
       {
         "name": "indexer",
         "kind": "human-mcp",
         "credential": { "env": "IPFS_MCP_INDEXER_TOKEN" },
         "write_groups": ["content", "index"],
         "egress_ack": ["local"]
       }
     ],
     "embeddings": {
       "mode": "embedded",
       "embedded": {
         "surreal_binary": "/absolute/path/to/surreal",
         "data_dir": "/absolute/path/to/ipfs-mcp-index"
       },
       "embedder": {
         "base_url": "http://127.0.0.1:11434/v1",
         "model": "bge-m3",
         "dimensions": 1024
       },
       "policy": { "auto": true }
     }
   }
   ```

2. If the daemon uses `API.Authorizations`, print the RPC paths these tools need and add them to the secret `ipfs mcp` uses:

   ```bash
   ipfs mcp --mcp-config ipfs-mcp.json --print-rpc-allowed-paths
   ```

3. Create the token and start the server over HTTP (the operator identity on stdio cannot index; see [What leaves the machine](#what-leaves-the-machine)):

   ```bash
   export IPFS_MCP_INDEXER_TOKEN="$(openssl rand -hex 32)"
   ipfs mcp --mcp-config ipfs-mcp.json --http
   ```

   It prints, on stderr:

   ```text
   ipfs mcp: embeddings enabled: embedded store on 127.0.0.1:<port>, embedder 127.0.0.1:11434 model bge-m3 (1024 dimensions)
   ipfs mcp: serving MCP over HTTP on http://127.0.0.1:5050/mcp
   ```

4. Connect an MCP client with the token, for example:

   ```bash
   claude mcp add --transport http ipfs-index http://127.0.0.1:5050/mcp \
     --header "Authorization: Bearer $IPFS_MCP_INDEXER_TOKEN"
   ```

5. Ask the agent to add a Markdown note with `ipfs_add` (approve the confirmation), then to search for it. With `policy.auto` the add is indexed at once; `ipfs_index_status` shows `indexed_cids: 1`, and `ipfs_index_search` returns the chunk with its CID, path, score and text. `ipfs_index_reconcile` with `dry_run: true` reports what an unpin elsewhere would remove.

6. Stop `ipfs mcp` (Ctrl-C). It stops the `surreal` process it started; the index stays in `data_dir` for the next start.

To turn it all off again, remove the `embeddings` section: no `surreal` process starts and the `ipfs_index_*` tools disappear.

Steps 1-3 and the tool calls of step 5 were run against a private daemon (`ipfs init --profile=test`, OS-chosen ports, its own `IPFS_PATH`), with `--http` on an OS-chosen loopback port with a deterministic stand-in embedder of 8 dimensions instead of Ollama, and an MCP client that approved the elicitation; step 4's `claude mcp add` was run with a throwaway `HOME`, and `claude mcp list` reported the server connected. The `surreal` process was gone after `ipfs mcp` exited. The agent conversation of step 5 inside Claude Code was not run.

## What leaves the machine

Indexing and searching send text to the embedder, and vectors to the store; with the extended pipeline they can also send file bytes to an extractor and media bytes to a describer. Each happens only on behalf of an identity whose `egress_ack` covers every host involved:

- the embedder host (`embeddings.embedder.base_url`), always;
- the store host (`embeddings.remote.endpoint`), in remote mode. The embedded store listens on loopback and needs no acknowledgement;
- the extractor host (`embeddings.extractor.remote.base_url`), in remote extractor mode. The file's bytes (at most `embeddings.extractor.max_bytes`) go there. The embedded extractor runs on loopback and needs no acknowledgement;
- the describer host (`embeddings.describer.base_url`). Images and videos are uploaded to it as base64 `data:` URLs inside the chat request (at most `describer.image_max_bytes` or `describer.video_max_bytes`), never as a gateway URL: a URL would publish the CID to the model host and let it fetch the content from the node.

`egress_ack` entries are host names, optionally with a port (`embed.internal.example`, `10.0.0.5:8000`), or `local`, which covers any loopback host (`localhost`, `127.0.0.1`, `::1`). The check runs per stage, before any request is built: the automatic trigger, `ipfs_index_cid`, `ipfs_index_search` and every file of a bulk run. A refused stage sends nothing and records `egress_not_acknowledged` with the missing hosts; the file's other stages still run.

**A hosted describer is never a default.** Describing a file sends its bytes to that host. Xiaomi's hosted MiMo API (`api.xiaomimimo.com`) is the describer [ADR-0011](0011-whole-node-vector-search.md) names for video; turning it on for "all files" sends every allowlisted image and video the node pins to that third party, bounded only by `describer.max_requests_per_hour` and the bulk caps. The same applies to any hosted endpoint you configure.

**A gateway host stands for every provider behind it.** If you front models with a gateway such as liter-llm, the host you acknowledge is the gateway's: acknowledging it acknowledges every provider the gateway can reach. No gateway is required; `ipfs mcp` only needs OpenAI-compatible endpoints, and nothing in it is liter-llm-specific.

The operator identity (stdio, and HTTP requests authenticated with `--auth-token-*`) has no `egress_ack`, so it never sends content to an embedder, extractor or describer. This is intended: the safe direction is the default. To index, call the tools as a configured HTTP principal that has an `egress_ack`:

```json
{
  "principals": [
    {
      "name": "indexer",
      "kind": "human-mcp",
      "credential": { "env": "IPFS_MCP_INDEXER_TOKEN" },
      "egress_ack": ["local"]
    }
  ]
}
```

With a local embedder (for example Ollama on `127.0.0.1`), the embedded store, the embedded extractor and no describer, `["local"]` is enough. With a remote embedder, store, extractor or any describer, list those hosts instead.

## Store modes

`embeddings.mode` is `embedded` or `remote`; there is no default. Both modes use the same schema, created on start if it is missing:

- table `ipfs_chunk` with `cid`, `path`, `mime`, `chunk_index`, `model`, `dims` and `vector`;
- an index on `cid`;
- an HNSW cosine index whose dimension is `embeddings.embedder.dimensions`.

For plain text the chunk text is not stored, only its vector and where it came from: a search reads the text back from the node (see [Index tools](#index-tools)). Entries produced by the extractor or the describer (`source` other than `text`) do store their chunk text beside the vector, because extraction is expensive and a description cannot be reproduced by re-reading the file. A database whose vector index was built for another dimension is refused at start with an error that names both dimensions; use another database or `data_dir`, or remove the `ipfs_chunk` table.

`ipfs mcp` talks to SurrealDB over its HTTP API (`POST /rpc`, method `query`) with the Go standard library. No SurrealDB Go module is linked. `mcp/server/embeddings/SOURCES.md` cites the documentation behind every statement it sends.

### Embedded mode

```json
{
  "embeddings": {
    "mode": "embedded",
    "embedded": {
      "surreal_binary": "/opt/surreal/bin/surreal",
      "data_dir": "/var/lib/ipfs-mcp/index"
    },
    "embedder": {
      "base_url": "http://127.0.0.1:11434/v1",
      "model": "bge-m3",
      "dimensions": 1024
    }
  }
}
```

`ipfs mcp` supervises a SurrealDB process started from a binary the operator installed. It never downloads one.

- **Binary.** `embedded.surreal_binary` must be an absolute path to a regular, executable file; it is never looked up on `PATH`. Before starting it, `ipfs mcp` runs `surreal version` and refuses a version older than `embedded.min_version` (default `3.0.0`).
- **Data.** `embedded.data_dir` must be an absolute path. It is created with mode `0700` and holds the store (`db/`, SurrealKV), the credentials and the supervisor state.
- **Command.** `surreal start --no-banner --deny-guests --bind 127.0.0.1:<port> surrealkv://<data_dir>/db`. `--deny-guests` refuses unauthenticated access. The process gets a minimal environment (`PATH`, `HOME`, `TMPDIR`, and the Windows equivalents) plus the root credentials, so none of the operator's other secrets reach it. Its output is kept only to explain a failed start: stdout is the MCP channel of `ipfs mcp`, so the store's output is never forwarded and never logged.
- **Port.** The OS picks a free loopback port. Ports of other local services (`4000`, `4001`, `5001`, `8080`, `1906`-`1908`, `50051`, `8181`, `8081`, `11434`, `23001`, `28000`) are never used. The start waits up to 30 seconds for `/health` and retries with a new port if the process exits early (up to three tries).
- **Namespace and database.** `ipfs_mcp` and `index`, fixed.
- **Credentials, rotated on every start.** Each start generates a new random root password (24 random bytes, hex encoded). It reaches the process through `SURREAL_USER` and `SURREAL_PASS` in its environment, never its argument list, which any local user can read. The current password is kept in `<data_dir>/credentials.json` with mode `0600`. On an existing store, SurrealDB ignores the environment credentials because a root user already exists; `ipfs mcp` signs in with the stored password and replaces it (`DEFINE USER OVERWRITE`). During the rotation the new password is kept in `credentials.pending.json`, so a crash halfway leaves a password that still opens the store. If neither file opens it, the start fails and says so: restore `credentials.json` or remove `<data_dir>/db` to start a new index.
- **Shutdown.** When `ipfs mcp` exits, it stops the store with `SIGTERM`, then kills it after 10 seconds.
- **Crash cleanup (Linux, macOS).** If `ipfs mcp` is killed, a small `sh` watchdog it started notices within a second and stops the store (`SIGTERM`, then `SIGKILL` after 10 seconds). It signals the recorded pid only while that process's command line still names the store's data path, so a recycled pid is left alone. `<data_dir>/supervisor.json` records the process; if the watchdog did not run either, the next start stops the leftover store after the same command-line check.
- **Crash cleanup (Windows).** There is no watchdog, and the next start cannot verify a recorded pid, so it does not stop it. A `surreal` left running by a killed `ipfs mcp` keeps `data_dir` locked until the operator stops it. A normal exit stops the store (without a graceful signal: Windows has no `SIGTERM` for it).

### Remote mode

```json
{
  "embeddings": {
    "mode": "remote",
    "remote": {
      "endpoint": "https://surreal.internal.example:8000",
      "namespace": "ipfs",
      "database": "index",
      "credential": { "env": "SURREAL_CREDENTIALS" }
    },
    "embedder": {
      "base_url": "https://embed.internal.example/v1",
      "model": "bge-m3",
      "dimensions": 1024,
      "api_key": { "file": "/etc/ipfs-mcp/embedder.key" }
    }
  }
}
```

- `remote.endpoint`, `namespace` and `database` have no defaults. The endpoint is `http` or `https` with a host, and must not carry credentials, a query or a fragment. Namespace and database names are 1-128 letters, digits, `_` or `-`.
- `remote.credential` is an env or file reference, never a literal. A value of the form `user:password` (with a non-empty user; the password is everything after the first colon) is sent as HTTP Basic authentication, for a root or system user. Any other value is sent as a bearer token (`Authorization: Bearer`). Without a credential the connection is unauthenticated. The value is never logged.
- On start, `ipfs mcp` defines the namespace and database if they are missing. That needs a root or namespace user; a user scoped to one database cannot, and then both must already exist.
- The store host has its own egress acknowledgement: it must be in the calling identity's `egress_ack` as well as the embedder host, even when the embedder is local. Vectors are derived from content, so sending them to a remote store is egress.

### The embedder

- `embedder.base_url`, `model` and `dimensions` have no defaults; `dimensions` is 1 to 8192.
- `ipfs mcp` calls `POST {base_url}/embeddings` with `{model, input}` (OpenAI-compatible), up to 16 chunks per request. A vector of another length than `dimensions` is an error.
- `embedder.api_key` is an optional env or file reference, sent as a bearer token.
- Redirects are not followed. An error from the embedder is clipped and never includes the request, so no content reaches a log through it.

## Content kinds

Every indexed file is assigned one kind, from its file name and a content sniff, and each kind names the stages that turn it into searchable text. The mapping is one contract file, [`mcp/contract/derivations/content-kinds.json`](../../mcp/contract/derivations/content-kinds.json), read by `ipfs mcp` and the Web UI alike:

| Kind | MIME types (examples) | Stages | `source` of its chunks |
|------|-----------------------|--------|------------------------|
| `text` | `text/plain`, `text/markdown` | native chunking, exactly as before | `text` |
| `document` | PDF, Office, ODF, iWork, RTF, EPUB, HTML | extractor; OCR for pages without a text layer | `extracted`, `ocr` |
| `code` | source code in common languages | extractor (tree-sitter-aware) | `extracted` |
| `data` | JSON, YAML, CSV, TSV, XML, TOML | extractor | `extracted` |
| `image` | PNG, JPEG, GIF, WebP, TIFF, BMP, AVIF, HEIC | OCR when on; describer when configured | `ocr`, `description` |
| `video` | MP4, WebM, QuickTime, Matroska, MPEG, AVI | transcription of the audio track when on; describer when enabled | `transcript`, `description` |

Every stage is gated:

- the extractor stages need an `embeddings.extractor` block; OCR additionally needs `extractor.ocr: true`, transcription `extractor.transcribe: true`;
- the describer stage needs an `embeddings.describer` block whose `kinds` list contains the file's kind (default `["image"]`);
- `embeddings.policy.mime_allowlist` stays the single allowlist of types that are indexed at all. Its default is `["text/plain", "text/markdown"]`, so configuring an extractor or describer indexes nothing until you add the types you want.

A stage that is off skips the file with a matching `skipped_reason` (`kind_not_enabled`, `extractor_unavailable`, `extraction_failed`, `describer_not_configured`) and the file's other stages still run. Whatever a stage produced is chunked and embedded by the same embedder and stored under the same CID, so images and videos are searched in the same vector space as documents. Hits carry `kind`, `source`, `mime_type` and, for descriptions, `described_by`; a description is untrusted model output, labelled "AI description by <model>" wherever it is shown.

## The extractor (xberg)

The extractor is [xberg](https://github.com/Know-Me-Tools/xberg) (MIT; our fork of `xberg-io/xberg`, identical to upstream `v1.3.10` until it diverges), which turns documents, code, structured data, image text (OCR) and video audio tracks (transcription) into Markdown. `ipfs mcp` talks to its REST API (`POST /extract`) with the Go standard library: no xberg code, cgo or native library is linked into the `ipfs` binary, and `ipfs mcp` never downloads xberg. The bytes it sends come from `cat` of the indexed CID, capped at `extractor.max_bytes` before the request is built.

| Key | Default | Meaning and off switch |
|-----|---------|------------------------|
| `embeddings.extractor` | `null` | Absent (default): no xberg process or request; only `text/plain` and `text/markdown` are indexed. Removing the block is the off switch. |
| `embeddings.extractor.mode` | none | Required when the block exists: `embedded` or `remote`. |
| `embeddings.extractor.embedded.xberg_binary` | none | Absolute path of an operator-installed `xberg` binary (required in embedded mode; never a `PATH` lookup). Remove the `embedded` block and no process starts. |
| `embeddings.extractor.embedded.cache_dir` | none | Absolute path of the model cache directory (required in embedded mode). It must be owned by the user running `ipfs mcp` and not writable by group or others (`chmod go-w`), or startup is refused; one `ipfs mcp` holds a lock on it, so give each its own. The ownership check and the lock are Unix-only; on Windows neither runs, so give each `ipfs mcp` its own `cache_dir`. xberg gets `XBERG_CACHE_DIR=<cache_dir>` and `HF_HOME=<cache_dir>/huggingface`. Pre-warm it when downloads are off (see [Pre-warming the cache](#pre-warming-the-cache)). |
| `embeddings.extractor.embedded.min_version` | `"1.3.10"` | Always checked; an older binary is refused. |
| `embeddings.extractor.embedded.allow_model_downloads` | `false` | `false` (default): xberg runs with model downloads disabled and a file that needs a missing model is `extraction_failed`. `true` lets xberg download layout, OCR and transcription models from the Hugging Face Hub, an endpoint you did not otherwise configure. |
| `embeddings.extractor.remote.base_url` | none | The xberg REST endpoint (required in remote mode; there is no default). A non-loopback host must use `https`: plain `http` to it is refused at startup because documents and any credential would travel unencrypted. xberg documents no authentication, so a remote xberg must sit on a private network or behind an authenticating proxy; the host must also be in the caller's `egress_ack`. Remove the `remote` block and no connection is made. |
| `embeddings.extractor.remote.credential` | `null` | Env or file reference sent as a bearer token, for an authenticating proxy only. `null` (default): no credential is sent. |
| `embeddings.extractor.timeout_seconds` | `120` | Per-request limit; always enforced. |
| `embeddings.extractor.max_bytes` | `52428800` (50 MiB; at most 100 MiB) | Larger files are skipped `too_large` before any request is built. |
| `embeddings.extractor.max_text_bytes` | `2097152` (2 MiB; at most 32 MiB) | Upper bound on the Markdown taken from one extraction; longer output is discarded and the file is skipped `too_large` (it is not truncated). Always enforced. It bounds memory and how many chunks one file can produce. |
| `embeddings.extractor.ocr` | `false` | `false` (default): no OCR of images or scanned PDFs. |
| `embeddings.extractor.transcribe` | `false` | `false` (default): no transcription of audio tracks. xberg 1.3.10 does not support transcription, so `true` produces no transcripts with that version; leave it `false`. |

In embedded mode `ipfs mcp` supervises `xberg serve` on `127.0.0.1` with an ephemeral port; it stops when `ipfs mcp` exits and leftovers are cleaned up the way `surreal` leftovers are. xberg runs with a minimal environment: nothing of your shell reaches it except `HF_ENDPOINT` (your own Hugging Face mirror, if you set one), `TESSDATA_PREFIX` and `ORT_DYLIB_PATH`. `DO_NOT_TRACK=1` is always set for it. With `allow_model_downloads: false` it also runs with `HF_HUB_OFFLINE=1`.

### Turning the extractor off

Omit the `embeddings.extractor` block. No xberg process starts, no request is made, and only the types in `embeddings.policy.mime_allowlist` that `ipfs mcp` handles itself (`text/plain` and `text/markdown` by default) are indexed.

### Pre-warming the cache

With `allow_model_downloads: false`, a file that needs a model that is not in the cache fails as `extraction_failed`. Fill the cache once, with your own xberg binary and the same `cache_dir` you configure:

```bash
XBERG_CACHE_DIR=<cache_dir> HF_HOME=<cache_dir>/huggingface xberg cache warm
```

Set `allow_model_downloads: true` instead and xberg fetches models on demand from the Hugging Face Hub, or from the mirror named by `HF_ENDPOINT` in the environment of `ipfs mcp`.

### Skip reasons

- `too_large`: the file is larger than `embeddings.extractor.max_bytes` (content routed to the extractor) or `embeddings.policy.max_bytes` (text), the extractor's output exceeds `embeddings.extractor.max_text_bytes`, or the text would produce more than the 2000-chunk cap. Nothing is stored.
- `kind_not_enabled`: the content is of a listed kind with a binary signature, for example a PDF, and its type is not in `embeddings.policy.mime_allowlist`, or its stage is off.
- `mime_not_allowed`: the content is UTF-8 text with no binary signature (JSON, CSV, XML, HTML) and its type is not in `embeddings.policy.mime_allowlist`. Such text stays on the native path, so adding an extractor does not de-index it.

### Data that leaves the machine

A remote store (`embeddings.mode: "remote"`) now receives the extracted plaintext of documents in the chunks it stores, not only the text of plain files. A remote extractor receives the file bytes. In both cases the host must be in the calling principal's `egress_ack`; without it the call fails with `egress_not_acknowledged`.

### Residual risk of the embedded listener

xberg 1.3.10 has no authentication option and no Unix socket support. Its ephemeral port on `127.0.0.1` is therefore reachable by any local process, and by a web page that finds the port, because a simple cross-origin POST is delivered whatever the CORS policy. A caller that reaches the port can make xberg fetch URLs (`urls` on `/extract`; checked on 2026-10-14, xberg refuses literal private, link-local, unique-local and loopback targets by policy, including `169.254.169.254`, but fetches public URLs, and a public hostname that resolves to a private address was not tested), clear xberg's model cache (`DELETE /cache/clear`, which undoes your pre-warming), and spend your CPU on extraction. xberg checks no `Host` header, so after DNS rebinding a page becomes same-origin and can read the responses. `ipfs mcp` itself reaches xberg only through its own authenticated loopback proxy, restricts xberg's CORS origins to xberg's own address, and cannot close the port. The operator accepted this residual on 2026-10-14 ([evidence](../../openspec/changes/mcp-content-extractor/evidence/2026-10-14-ssrf-policy/README.md)). To avoid it, omit the `embeddings.extractor` block or use a `remote` extractor behind an authenticating proxy. Source: [Know-Me-Tools/xberg](https://github.com/Know-Me-Tools/xberg) (fork of [xberg-io/xberg](https://github.com/xberg-io/xberg)).


### Downgrading

An older `ipfs mcp` that does not know about derived rows treats them as native text rows of the same CID; reindex after a downgrade.

## The describer

The describer is any OpenAI-compatible `/chat/completions` endpoint with image (and, if enabled, video) input: Xiaomi's hosted MiMo API, or a local vision model behind any compatible server. It returns a text description, and that text is chunked and embedded like everything else — images and videos are searched in the same vector space as documents, with no second index. Media go to the describer as base64 `data:` URLs inside the request, never as a gateway URL. The fixed instruction is part of the contract, not a config key, and the description is untrusted model output: tool results name the model in `described_by` and every surface labels it "AI description by <model>".

| Key | Default | Meaning and off switch |
|-----|---------|------------------------|
| `embeddings.describer` | `null` | Absent (default): no description request; images and videos are skipped `describer_not_configured` unless OCR or transcription covers them. Removing the block is the off switch. |
| `embeddings.describer.base_url` | none | The chat endpoint (required; no default). Describing a file sends its bytes to this host; the host must also be in the caller's `egress_ack`. |
| `embeddings.describer.model` | none | The model name (required). |
| `embeddings.describer.api_key` | `null` | Env or file reference, sent as a bearer token. `null` (default): requests carry no key. The value is never logged. |
| `embeddings.describer.kinds` | `["image"]` | Which kinds are described. Empty list: nothing is described. Video is described only when listed. |
| `embeddings.describer.max_output_tokens` | `512` | Always enforced; bounds one description's length. |
| `embeddings.describer.image_max_bytes` | `20971520` (20 MiB) | Larger images are not described. |
| `embeddings.describer.video_max_bytes` | `52428800` (50 MiB) | Larger videos are not described. |
| `embeddings.describer.video_fps` | `1` (0.1 to 10) | Frames per second sampled for the video part; always applies to video. |
| `embeddings.describer.max_requests_per_hour` | `120` | `0`: the describer is disabled even with `base_url` and `model` set. |
| `embeddings.describer.timeout_seconds` | `180` | Per-request limit; always enforced. |

**Video description has no Mac path.** MiMo's self-hosted variant needs datacenter GPUs and there is no official Mac or CPU build, so on Desktop video search covers only the audio transcript (with `extractor.transcribe: true`) unless you point the describer at a hosted endpoint — which sends the video bytes to that host. Video is therefore absent from the default `kinds` list, and the Desktop setup keeps it off.

A video request cannot pass through liter-llm today (its content parts have no `video_url`), so video description goes directly to the describer endpoint.

## Indexing

### Policy

`embeddings.policy` decides what is indexed:

| Key | Default | Meaning |
|-----|---------|---------|
| `auto` | `false` | Index text after a successful `ipfs_add`. Off: only `ipfs_index_cid` indexes. |
| `mime_allowlist` | `["text/plain", "text/markdown"]` | Content types that are indexed. An empty list indexes nothing. |
| `max_bytes` | `1048576` (1 MiB) | Larger content is skipped. At most 100 MiB. |
| `chunk_chars` | `2000` | Longest chunk, in characters (200 to 20000). |

The content type comes from the file name (`.md` and `.markdown` are `text/markdown`, `.txt` is `text/plain`) or is sniffed from the bytes; the type assigns the file a [content kind](#content-kinds), which decides which stages run. Natively indexed content that is not valid UTF-8, contains a NUL byte or is larger than `max_bytes` is never indexed. Content outside the policy is skipped silently by the automatic trigger, and with a `skipped_reason` by `ipfs_index_cid`.

Chunks are cut at a paragraph break, else a line break, else a space in the second half of the piece, else hard at `chunk_chars` characters. Pieces are trimmed and empty ones dropped. Cutting is a pure function of the text and `chunk_chars`, which is how a search recovers a hit's text: it reads the content again and cuts it the same way. **Changing `chunk_chars` after indexing breaks that.** Hits whose chunk can no longer be found are dropped from search results; run `ipfs_index_cid` with `force: true` on the affected CIDs to rebuild their entries.

### Automatic indexing after `ipfs_add`

With `policy.auto` set to `true`, every successful `ipfs_add` from `ipfs mcp` is indexed, whichever transport made it: stdio, Streamable HTTP, the AG-UI chat, A2A, the agent loop, and UAR in topology B.

- The indexed CID is the one `ipfs_add` returned. With a `filename`, that is the wrapping directory, and the entry's `path` is `/ipfs/<cid>/<filename>`.
- Indexing uses the bytes the add already has. It reads nothing from the node, and it adds, pins and announces nothing.
- It runs inline after the add, with a 45-second limit.
- A failure never fails the add. An unreachable embedder or store, a timeout or a missing egress acknowledgement is a line in the add result's `warnings`, and the CID is returned as usual. An add from the operator identity therefore succeeds with a warning that names the embedder host and sends nothing.
- Adds made by WebMCP browser tools would not be indexed automatically; index them with `ipfs_index_cid`. WebMCP itself is not delivered yet.

### Index tools

| Tool | Class | What it does |
|------|-------|--------------|
| `ipfs_index_status` | read | Reports whether indexing is configured, the mode, store reachability, model, dimensions and counts (optionally for one CID). Contacts no embedder. |
| `ipfs_index_search` | read, content egress, untrusted content | Embeds `query`, returns the `top_k` (default 5, at most 50) nearest chunks with CID, path, chunk index, score (1 minus cosine distance) and text, plus `kind`, `source`, `mime_type` and, for descriptions, `described_by`. `cid` restricts the search to one CID; the optional `kinds` and `sources` filters restrict it to those content kinds or chunk sources. |
| `ipfs_index_cid` | write (confirmed) | Indexes one UnixFS file by CID: `files/stat`, then `cat` of at most `max_bytes`, routed through the stages of its [content kind](#content-kinds). `force: true` re-indexes a CID that already has entries. |
| `ipfs_index_reconcile` | write (confirmed), `destructiveHint` | Deletes the entries of indexed CIDs that no pin covers, one page per call. See [Reconciliation](#reconciliation). |
| `ipfs_index_bulk_start` | write (confirmed), content egress | Starts (or with `resume: true` resumes) the one bulk run over pins and MFS. See [Bulk indexing](#bulk-indexing). |
| `ipfs_index_bulk_status` | read | Reports the state and counts of the current or last bulk run. Contacts no daemon endpoint, extractor or embedder. |
| `ipfs_index_bulk_cancel` | write (confirmed) | Cancels the active or paused run; files already indexed stay indexed. Off switch for all three: `embeddings.bulk.enabled: false`. |

`ipfs_index_cid` returns `chunks_indexed` and the `model`, or `chunks_indexed: 0` with a `skipped_reason`:

- `mime_not_allowed`: a type not in `mime_allowlist`, or natively indexed content that is not valid UTF-8 text or contains a NUL byte;
- `too_large`: larger than the applicable cap (`policy.max_bytes`, or `extractor.max_bytes` for extractor-routed kinds);
- `already_indexed`: the CID has entries and `force` was not set;
- `not_a_file`: a directory or another non-file node;
- `empty`: an empty or whitespace-only file;
- `not_local`: an offline read found blocks missing locally; nothing is fetched;
- `kind_not_enabled`: the file's kind has no stage enabled (for example a document with no extractor, or an image with both OCR and the describer off);
- `extractor_unavailable`: no extractor is configured, or it could not be reached;
- `extraction_failed`: the extractor ran but produced nothing usable (for example a model missing from a downloads-disabled cache);
- `describer_not_configured`: no describer covers the file's kind (applies to the description stage; the file's other stages still run).

Errors follow the contract's error model:

- `egress_not_acknowledged`: the calling identity's `egress_ack` does not cover the embedder or the remote store. The error names the missing hosts. Not retryable; use another principal.
- `index_unavailable`: the embedder or the store failed. Retryable.

Search results are untrusted content: the text is whatever was stored or read back, and clients and the agent loop treat it as data, never instructions. Hits with `source: text` are read from the node with `cat` (at most `max_bytes` per document, 20 seconds each); a hit whose text cannot be read or found is dropped, so a search can return fewer results than `top_k`. Hits of every other source serve the chunk text stored beside the vector, and a hit with `source: description` is untrusted model output named by `described_by`.

### Bulk indexing

One operator-started run can index everything the node holds for you: UnixFS files reachable from recursive and direct pins (`pin/ls`) and from the MFS tree under `embeddings.bulk.mfs_root` (`files/ls`), descending directories with `ls`. It does not index unpinned blocks in the blockstore (a gateway cache holds content strangers asked for, not yours, and pin-aware deletion would remove it again), raw blocks or non-UnixFS DAGs (`not_a_file`). A run never starts in the background: there is no scheduler and no start-up trigger.

- **Every read is offline.** `cat` and `ls` carry `offline: true`; a file with missing blocks is skipped `not_local` and the run never fetches from the network, so nothing is stored or announced because of indexing. A run issues no `add`, `block/put`, `dag/put`, `pin/add`, `files/write` or `routing/provide` call, and images the extractor pulls out of documents are discarded, never added.
- **Caps.** `embeddings.bulk` bounds every run: `max_files_per_run` (default 1000), `max_bytes_per_run` (default 1 GiB) and `files_per_minute` (default 30), one run at a time. Per-call caps on `ipfs_index_bulk_start` (`max_files`, `max_bytes`, `files_per_minute`) can only tighten the configured ones. `bulk.sources` (default `["pins", "mfs"]`; an empty list indexes nothing) and `bulk.mfs_root` (default `"/"`) set the walk. Off switch: `embeddings.bulk.enabled: false` removes all three bulk tools from `tools/list`.
- **Confirmation and egress.** `ipfs_index_bulk_start` is a write-class tool, confirmed on every transport; the prompt names the sources, the caps and every host the run may contact. The caller's `egress_ack` must cover every configured stage's host (embedder, remote store, remote extractor, describer), checked at start and again before each file. As with single-file indexing, the operator identity has no `egress_ack` and cannot start a run.
- **Resumable, never automatic.** Run state (cursor, counts, the starting principal, the caps) lives in SurrealDB. If `ipfs mcp` stops, the run comes back `paused` and indexes nothing until a principal resumes it with `ipfs_index_bulk_start` and `resume: true`, which asks for confirmation again. Files already indexed are skipped (`already_indexed`), so a resumed or later run indexes only what is new.

`ipfs_index_bulk_status` reports the run (`running`, `paused`, `completed`, `cancelled` or `failed`), the principal that started it, the effective sources and caps, and per-reason skip counts; `ipfs_index_bulk_cancel` (write class, confirmed) stops it. The Web UI shows bulk status read-only; starting a run goes through the agent chat or any MCP client, where the confirmation already exists.

### Pin-aware deletion

Vectors follow pins. A CID's entries are deleted only when `pin ls --type=all /ipfs/<cid>` shows that no direct, recursive or indirect pin covers it. They are never deleted on a guess: if the pin check or the store fails, the entries stay and the tool's result carries a line in its `warnings` field.

The check runs after these tools, which are destructive and so are enabled only with the `destructive` selector:

- `pin_rm`: for each CID it unpinned;
- `pin_update` with `unpin` (the default): for the old CID;
- `repo_gc`: for every indexed CID garbage collection removed. Every removed block key is compared with the indexed CIDs, not only the 100 keys in `removed_sample`. The comparison is by multihash, because garbage collection reports block keys with the raw codec while the index holds the dag-pb CID.

The unpin or collection itself has already succeeded when the check runs; its warnings never turn it into a failure. A recursive `pin_rm` of a directory does not reach files inside it that were indexed under their own CID; `ipfs_index_reconcile` does.

If `ipfs mcp` runs with a scoped `API.Authorizations` entry, print it with `ipfs mcp --print-rpc-allowed-paths` (with the same `--tools` selection): it includes `pin/ls` for these tools and `cat` for `ipfs_index_search`.

### Reconciliation

Kubo has no pin event stream, and a daemon-wide hook is refused under the User Agency rules, so `ipfs mcp` does not see content unpinned outside it: by the CLI, the Web UI or another client. `ipfs_index_reconcile` closes that gap when the operator runs it.

- It walks the indexed CIDs in order, `limit` per call, and returns `page.next_cursor` while more remain. The cursor is the last CID of the page, so deleting while paging skips nothing.
- Each CID gets the same pin check as above. Entries of CIDs no pin covers are deleted and listed in `deleted`; the rest are counted in `kept`.
- `dry_run: true` lists what would be deleted and deletes nothing.
- A CID whose pin check or delete fails is kept, with a warning in `warnings`.

Content added with `pin: false` and indexed is covered by no pin, so the next reconciliation or garbage collection removes its entries. Pin what should stay searchable.

## Search over HTTP

For the Web UI search box and other clients that are not full MCP sessions, `ipfs mcp` mounts two routes on its `--http` listener (the full descriptor is [`mcp/contract/routes/index.json`](../../mcp/contract/routes/index.json); request and response shapes are exactly the tool schemas, with no second copy):

- `POST /index/search`: the body is the `ipfs_index_search` input (including its `kinds` and `sources` filters), the response its output;
- `GET /index/status`: the `ipfs_index_status` output plus a `bulk_run` summary when a bulk run exists.

Authorization is the tool's: a bearer of kind `ui` or `human-mcp` with the `index` group, the same `egress_ack` check as the tool (`POST /index/search` embeds the query, so the embedder and a remote store must be acknowledged; `GET /index/status` contacts no embedder), and the listener's origin allowlist CORS rule. An `admin` principal is refused (403), exactly as on `/mcp` and `/agui`. Hit text is data: render it as plain text, never as HTML or Markdown.

Off switch: `embeddings.search_route.enabled: false` (or no `--http`, or no `embeddings` section) unmounts both routes — they answer 404 and nothing is read or dialed. The Web UI gates its search nav entry on `GET /index/status` answering at all. No `/api/v0` endpoint is added or changed; `ipfs mcp --print-rpc-allowed-paths` lists the RPC paths indexing uses (`pin/ls`, `files/ls`, `files/stat`, `ls`, `cat`) so a scoped `API.Authorizations` entry keeps working.

## Setups

### LKE (cloud node)

The know-me-cluster deployment of [ADR-0011](0011-whole-node-vector-search.md) V8:

- **Store:** SurrealDB in remote mode (the Kubo image contains no `surreal` binary), as an in-cluster StatefulSet with its root credential in an out-of-band Secret.
- **Extractor:** xberg as a ClusterIP-only Deployment from a fork-built image pinned by digest once one exists (never a floating tag); until then `ghcr.io/xberg-io/xberg:v1.3.10`, a build of the same commit as the fork's `v1.3.10`, with a model-cache PVC pre-warmed by a one-off Job — the only workload allowed to reach the Hugging Face Hub. A NetworkPolicy lets only the `ipfs` pod reach xberg.
- **Embedder:** an in-cluster CPU model; there is no GPU on LKE.
- **Gateway:** liter-llm is not deployed at first; if a second provider is added later, it is the upstream image pinned by digest, on ClusterIP with a master key.
- **Describer:** off until the operator has read Xiaomi's data terms; then hosted MiMo for images only, with `api.xiaomimimo.com` added to the `ui` principal's `egress_ack`. Video on LKE takes a further explicit operator choice.
- **Secrets** (store credential, gateway key, Xiaomi key) are created out of band, never in git, mounted with `optional: true`: an absent Secret disables the dependent stage, never the pod.
- **Routes:** the console exposes `/index/search` and `/index/status` bearer-only and outside the basic-auth policy, exactly like `/agui`.

### IPFS Desktop

Search is off by default on Desktop. When you turn it on, Desktop asks for absolute paths to an operator-installed `surreal` and `xberg` (the `xberg-cli-aarch64-apple-darwin` release asset) and an embedder URL, and its `ipfs mcp` sidecar writes an `embeddings` block in embedded mode for both, with data and cache directories under Desktop's userData. Desktop downloads and bundles neither binary.

The embedder is local by default (for example Ollama with `bge-m3`). Images are described by a local vision model behind any OpenAI-compatible server; video description is off — [there is no Mac path for it](#the-describer). A non-loopback embedder or describer host is added to the Desktop `ui` principal's `egress_ack` only after a confirmation that names the host, with Touch ID.

Embedding settings are read when `ipfs mcp` starts; changing them restarts `ipfs mcp` (Desktop restarts its sidecar). They are not part of the hot-reload settings API.

## Limitations

- **The operator identity cannot index.** Stdio and `--auth-token-*` carry no `egress_ack`, so indexing and search need a configured HTTP principal. This keeps content from reaching an embedder by default, at the cost of one more config entry.
- **Video description has no Mac path.** On Desktop, video search covers only the audio transcript unless you configure a hosted describer, which receives the video's bytes.
- **Derived text is stored.** Extracted text, OCR output, transcripts and descriptions sit in SurrealDB next to their vectors (natively indexed text is not stored). Deletion still follows pins, but between an out-of-band unpin and the next reconciliation that text stays readable to anyone with the store.
- **xberg downloads models at runtime** unless `extractor.embedded.allow_model_downloads` stays `false` and the cache is pre-warmed. With downloads on, xberg contacts the Hugging Face Hub, an endpoint you did not configure.
- **xberg's REST API is unauthenticated.** In embedded mode it listens on loopback, where any local process can submit files and spend your CPU; a remote xberg must sit on a private network or behind an authenticating proxy (`extractor.remote.credential`).
- **Descriptions are model output searched as if they were content.** A wrong or injected description ranks like a fact; it is labelled "AI description by <model>" wherever shown, but treat it as untrusted.
- **Embedding settings are not hot-reloaded.** Changing them restarts `ipfs mcp`.
- **Indexing everything costs real resources.** OCR, layout and transcription models are CPU-heavy; the bulk caps make a run bounded and visible, not cheap.
- **Unpins outside `ipfs mcp` are not seen until reconciliation.** Between an out-of-band unpin and the next `ipfs_index_reconcile`, search can still match entries for content the operator no longer pins.
- **No watchdog on Windows.** A killed `ipfs mcp` leaves `surreal` running and `data_dir` locked until the operator stops it.
- **WebMCP adds are not indexed automatically.** WebMCP is not delivered yet; when it is, its adds bypass `ipfs mcp` and need `ipfs_index_cid`.
- **Changing `chunk_chars` or the embedder model needs a re-index.** Entries made with the old size or model stay until `ipfs_index_cid` with `force` replaces them; a different `dimensions` needs a new database.
- **SurrealDB license.** The SurrealDB server is licensed under the Business Source License 1.1 (change license Apache 2.0), accepted for self-hosted use in ADR-0006. `ipfs mcp` links no SurrealDB code; the operator supplies the binary or the server.
