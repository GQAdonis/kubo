# Embeddings and the search index

`ipfs mcp` can chunk text content, send it to an embedding model the operator configures, store the vectors in SurrealDB and answer semantic searches over them. This page covers the two store modes and the indexing lifecycle. The decision behind the design is [ADR-0006](0006-embeddings-surrealdb.md); the authoritative list of keys, defaults and off switches is the config schema in `mcp/contract/config/ipfs-mcp-config.schema.json`, rendered in the [configuration reference](config-reference.md#embeddings).

The whole feature is off unless the config file named by `--mcp-config` (or `IPFS_MCP_CONFIG`) has an `embeddings` section. Without it no `surreal` process starts, no store or embedder is contacted, and the four `ipfs_index_*` tools are not listed (`ipfs_node_status` reports them as `not_configured`). Removing the section is the off switch. The feature adds no Kubo repo config key and changes nothing in `docs/config.md`, the `/api/v0` RPC API or the gateway.

## Contents

- [Try it](#try-it)
- [What leaves the machine](#what-leaves-the-machine)
- [Store modes](#store-modes)
  - [Embedded mode](#embedded-mode)
  - [Remote mode](#remote-mode)
  - [The embedder](#the-embedder)
- [Indexing](#indexing)
  - [Policy](#policy)
  - [Automatic indexing after `ipfs_add`](#automatic-indexing-after-ipfs_add)
  - [Index tools](#index-tools)
  - [Pin-aware deletion](#pin-aware-deletion)
  - [Reconciliation](#reconciliation)
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

Indexing and searching send text to the embedder, and vectors to the store. Both happen only on behalf of an identity whose `egress_ack` covers every host involved:

- the embedder host (`embeddings.embedder.base_url`), always;
- the store host (`embeddings.remote.endpoint`), in remote mode. The embedded store listens on loopback and needs no acknowledgement.

`egress_ack` entries are host names, optionally with a port (`embed.internal.example`, `10.0.0.5:8000`), or `local`, which covers any loopback host (`localhost`, `127.0.0.1`, `::1`). The check runs before any request is built, for the automatic trigger, `ipfs_index_cid` and `ipfs_index_search`. A refusal names the hosts that are not acknowledged and sends nothing.

The operator identity (stdio, and HTTP requests authenticated with `--auth-token-*`) has no `egress_ack`, so it never sends content to an embedder. This is intended: the safe direction is the default. To index, call the tools as a configured HTTP principal that has an `egress_ack`:

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

With a local embedder (for example Ollama on `127.0.0.1`) and the embedded store, `["local"]` is enough. With a remote embedder or a remote store, list those hosts instead.

## Store modes

`embeddings.mode` is `embedded` or `remote`; there is no default. Both modes use the same schema, created on start if it is missing:

- table `ipfs_chunk` with `cid`, `path`, `mime`, `chunk_index`, `model`, `dims` and `vector`;
- an index on `cid`;
- an HNSW cosine index whose dimension is `embeddings.embedder.dimensions`.

The chunk text is not stored, only its vector and where it came from. A search reads the text back from the node (see [Index tools](#index-tools)). A database whose vector index was built for another dimension is refused at start with an error that names both dimensions; use another database or `data_dir`, or remove the `ipfs_chunk` table.

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

## Indexing

### Policy

`embeddings.policy` decides what is indexed:

| Key | Default | Meaning |
|-----|---------|---------|
| `auto` | `false` | Index text after a successful `ipfs_add`. Off: only `ipfs_index_cid` indexes. |
| `mime_allowlist` | `["text/plain", "text/markdown"]` | Content types that are indexed. An empty list indexes nothing. |
| `max_bytes` | `1048576` (1 MiB) | Larger content is skipped. At most 100 MiB. |
| `chunk_chars` | `2000` | Longest chunk, in characters (200 to 20000). |

The content type comes from the file name (`.md` and `.markdown` are `text/markdown`, `.txt` is `text/plain`) or is sniffed from the bytes. Content that is not valid UTF-8, contains a NUL byte or is larger than `max_bytes` is never indexed. Content outside the policy is skipped silently by the automatic trigger, and with a `skipped_reason` by `ipfs_index_cid`.

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
| `ipfs_index_search` | read, content egress, untrusted content | Embeds `query`, returns the `top_k` (default 5, at most 50) nearest chunks with CID, path, chunk index, score (1 minus cosine distance) and text. `cid` restricts the search to one CID. |
| `ipfs_index_cid` | write (confirmed) | Indexes one UnixFS file by CID: `files/stat`, then `cat` of at most `max_bytes`. `force: true` re-indexes a CID that already has entries. |
| `ipfs_index_reconcile` | write (confirmed), `destructiveHint` | Deletes the entries of indexed CIDs that no pin covers, one page per call. See [Reconciliation](#reconciliation). |

`ipfs_index_cid` returns `chunks_indexed` and the `model`, or `chunks_indexed: 0` with a `skipped_reason`:

- `mime_not_allowed`: not UTF-8 text, contains a NUL byte, or a type not in `mime_allowlist`;
- `too_large`: larger than `max_bytes`;
- `already_indexed`: the CID has entries and `force` was not set;
- `not_a_file`: a directory or another non-file node;
- `empty`: an empty or whitespace-only file.

Errors follow the contract's error model:

- `egress_not_acknowledged`: the calling identity's `egress_ack` does not cover the embedder or the remote store. The error names the missing hosts. Not retryable; use another principal.
- `index_unavailable`: the embedder or the store failed. Retryable.

Search results are untrusted content: the text is whatever was stored, and clients and the agent loop treat it as data, never instructions. Hit text is read from the node with `cat` (at most `max_bytes` per document, 20 seconds each); a hit whose text cannot be read or found is dropped, so a search can return fewer results than `top_k`.

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

## Limitations

- **The operator identity cannot index.** Stdio and `--auth-token-*` carry no `egress_ack`, so indexing and search need a configured HTTP principal. This keeps content from reaching an embedder by default, at the cost of one more config entry.
- **Unpins outside `ipfs mcp` are not seen until reconciliation.** Between an out-of-band unpin and the next `ipfs_index_reconcile`, search can still match entries for content the operator no longer pins.
- **No watchdog on Windows.** A killed `ipfs mcp` leaves `surreal` running and `data_dir` locked until the operator stops it.
- **WebMCP adds are not indexed automatically.** WebMCP is not delivered yet; when it is, its adds bypass `ipfs mcp` and need `ipfs_index_cid`.
- **Changing `chunk_chars` or the embedder model needs a re-index.** Entries made with the old size or model stay until `ipfs_index_cid` with `force` replaces them; a different `dimensions` needs a new database.
- **SurrealDB license.** The SurrealDB server is licensed under the Business Source License 1.1 (change license Apache 2.0), accepted for self-hosted use in ADR-0006. `ipfs mcp` links no SurrealDB code; the operator supplies the binary or the server.
