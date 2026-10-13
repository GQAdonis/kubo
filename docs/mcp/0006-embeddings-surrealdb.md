# ADR-0006: `ipfs mcp` embeds documents and stores vectors in SurrealDB, embedded through a supervised operator-supplied `surreal` process or remote by URL

## Status

Accepted, 2026-10-09, per the operator's binding answers in `.kbd-orchestrator/phases/ipfs-mcp-server/operator-answers.md`, "Round 0 answers (team-ownership-grant 1.1, 2026-10-09)":

- R1 (E6) approved: "`ipfs mcp` performs embeddings and stores them in SurrealDB: embedded mode supervises the operator-supplied `surreal` binary (3.3.0 installed) on loopback; remote mode uses an operator-configured URL."
- R2: "standard-library HTTP client. No `surrealdb.go` and no new Go modules for SurrealDB in the root `go.mod`."
- R5: `bge-m3` (1024 dimensions) through local Ollama for real-service embedding tests.

And the orchestrator defaults in "Answers to the G9-G12 addendum" that the operator may overrule: Q8 (opt-in embedding hook, pin-aware deletion, operator-triggered reconciliation), Q9 (SurrealDB BSL 1.1 accepted for self-hosted use), Q10 (both embedded and remote SurrealDB; a remote embedder by configuration, off by default, behind `egress_ack`).

This ADR replaces the addendum's ADR-0006 draft, which left the component choice open, with plan revision 2's re-score (`plan.md` D6).

## Date

2026-10-09

## Context

- Goal G12: optional, configuration-driven embedding of content added through the agent surfaces (chunking, model and dimension, content types, trigger), stored in SurrealDB with vector search, in both embedded and remote SurrealDB modes; off unless configured; embedding providers operator-configured, never a hardcoded endpoint; no content leaves the machine without explicit operator configuration.
- `AGENTS.md`: no silent CID injection; no appropriation of the operator's resources; every endpoint configurable with an off switch; a new module in `go.mod` gets code-level scrutiny.
- Kubo has no cgo: `grep -rln 'import "C"' --include=*.go .` outside tests returns nothing (re-run 2026-10-09).
- **Embedding SurrealDB in a Go process requires cgo** [external, https://surrealdb.com/docs/reference/golang/embedding, fetched 2026-10-09]. The page says the Go bindings in `surrealdb.c` "let a Go program run SurrealDB in-process instead of connecting to a server", and that the module "links against a C static library (libsurrealdb_c.a) at compile time. You must build surrealdb.c and set CGO_LDFLAGS to its location before go build or go test will work." The Go package is `github.com/surrealdb/surrealdb.c.go`.
- The pure-Go SDK `surrealdb.go` (v1.7.0, Apache-2.0) is a network client [external, `proxy.golang.org`, 2026-10-09]. The operator declined it (R2).
- The SurrealDB engine is licensed Business Source License 1.1 with a change license of Apache 2.0 [external, https://github.com/surrealdb/surrealdb/blob/main/LICENSE, fetched 2026-10-09].
- `surreal` 3.3.0 is installed on the operator machine (`surreal version` reports `3.3.0 for macos on aarch64`, 2026-10-09).
- UAR has no `/v1/embeddings` route, and its `/api/ingest` uses one hardcoded knowledge base (`plan.md` D6). Kubo has no pin-removal event stream.

### Re-score under the goal-fit gate

Plan revision 2 re-scored G12 with the addendum's weights (goal fit 3, Kubo surface 2, rules fit 3, license/build 2, maintenance 2, search 1; maximum 65), not crediting undelivered UAR work, and excluding any option that lacks either SurrealDB mode or covers only UAR-routed content (goal fit at most 2):

| Option | Goal fit | Surface | Rules | License/build | Maint. | Search | Weighted |
|---|---|---|---|---|---|---|---|
| E1: UAR embeds and stores (topology B only) | 2: gated on a UAR release; topology-A content not covered; knowledge base not keyed by CID | | | | | | excluded |
| E2: cgo `surrealdb.c.go` linked into `ipfs` | 4 | 1 | 3 | 1 | 1 | 4 | 31 |
| Remote-only from `ipfs mcp` | 2: no embedded mode | | | | | | excluded |
| **E6: `ipfs mcp` embeds and stores in both topologies; embedded = managed local `surreal`; remote = URL** | **4** (WebMCP adds not automatic) | **2** | **4** | **4** | **2** | **4** | **44** |

The addendum's revision-2 choice (E1 at 54) had the same defect as UAR-hosted G9: it credited UAR changes that had not been delivered. Under the gate it is excluded (`assessment-addendum.md`, "Orchestrator adjudication" item 1 and the cross-model finding 4, upheld).

## Decision

E6: `ipfs mcp` is the component that embeds content and stores vectors, in both topologies. In topology B, UAR's writes pass through `ipfs mcp`, so UAR-side embedding is not needed.

### Off by default

With no `embeddings` section in the `ipfs mcp` config file (`--mcp-config`), no `surreal` process is spawned, no store or embedder is contacted, and the index tools are not listed. The `embeddings` keys are in the contract config schema (`mcp/contract/config/`) with a `default` and an `offSwitch` each, and are documented in `docs/mcp/config-reference.md`. No Kubo config key is added.

### Embedded mode: a supervised operator-supplied `surreal` process

- `ipfs mcp` runs `surreal start --bind 127.0.0.1:<ephemeral> surrealkv://<operator data dir>` from an **absolute binary path the operator configures**. There is no `PATH` lookup. Kubo never downloads, bundles or ships the `surreal` binary, so no BSL-licensed code is in Kubo's artifacts.
- Credentials are random per start and written to a 0600 file. The bind is loopback only, on an ephemeral port (no reserved default port, `plan.md` D7).
- A minimum-version check runs before use.
- The process stops when `ipfs mcp` exits, and crash cleanup removes orphans (falsifier G12-3).
- "Embedded" means supervised and private to `ipfs mcp`, not linked in-process.

### Remote mode: an operator URL

- The operator configures the URL, namespace and database. Credentials come from an environment variable or a file, never a literal.
- There is no default URL.

### Client: the Go standard library

`ipfs mcp` talks to SurrealDB over its HTTP API with `net/http` (Round 0 R2). No SurrealDB module is added to the root `go.mod`. `surrealdb.go` stays declined, and `surrealdb.c.go` is excluded because it needs cgo and a native library.

### Embedder

- An operator-configured OpenAI-compatible `/embeddings` endpoint, with the model and dimension configured. No default.
- Its host is checked against `egress_ack`. A loopback embedder (for example Ollama with `bge-m3`, 1024 dimensions, used by the real-service tests) is the documented local setup; a remote embedder works only with a matching acknowledgement (Q10).
- Chunking, the MIME allowlist and the size cap are configurable.

### Trigger

- `embeddings.auto` is off by default. When on, content is indexed after a successful `ipfs_add` from any `ipfs mcp` transport (stdio, HTTP, AG-UI chat, A2A), subject to the MIME allowlist and size cap.
- `ipfs_index_cid` indexes one CID on explicit request (a `write`-class tool, confirmed per ADR-0003).
- Storage is keyed by CID.
- A daemon-wide hook that embeds everything the node adds is **refused**: it would index content without a per-item local cause (`AGENTS.md`, User Agency). There is no flagged variant.

### Deletion

- A CID's vectors are deleted only when `pin ls --type=all <cid>` shows no direct, recursive or indirect pin.
- After `repo_gc`, only CIDs that GC reports as removed are deleted.
- `ipfs_index_reconcile` handles content unpinned outside `ipfs mcp`, on operator request.

### Ownership in the plan

`mcp-embeddings-surrealdb` builds the store, both modes and the embedder. `mcp-embedding-index-lifecycle` builds the trigger, index tools, pin-aware deletion and reconciliation (`plan.md` D6).

## Consequences

### Positive

- Both SurrealDB modes work with no cgo, no new Go module and no BSL code in Kubo's artifacts.
- Content added through every `ipfs mcp` transport can be indexed, in both topologies, without depending on a UAR release.
- Nothing runs and nothing is contacted unless the operator configures it, and egress to an embedder is acknowledged by host.

### Negative

- `ipfs mcp` supervises a third-party process in embedded mode (`plan.md` trade-off T8): lifecycle, crash cleanup and version drift are now Kubo's responsibility.
- A hand-written HTTP client must track SurrealDB's HTTP API across releases, without an SDK's maintenance.
- Content added through WebMCP in the browser calls the RPC API directly, not `ipfs mcp`, so it is not indexed automatically.
- Content unpinned outside `ipfs mcp` stays indexed until reconciliation runs.
- Operators who deploy SurrealDB themselves inherit BSL 1.1, which restricts offering SurrealDB as a database service. If a hosted offering is planned, the operator must say so (Q9 default).

## Rejected alternatives

| Alternative | Rejected because (evidence) |
|---|---|
| E2: link SurrealDB into `ipfs` through `surrealdb.c.go` | Requires cgo and the static library `libsurrealdb_c.a` with `CGO_LDFLAGS` (SurrealDB Go embedding docs, fetched 2026-10-09). Kubo has no cgo, and BSL code would ship inside `ipfs`. Scored 31/65. |
| `surrealdb.go` v1.7.0 as the client | Adds root `go.mod` modules (`fxamacker/cbor/v2`, `gorilla/websocket`, `lxzan/gws`, `gofrs/uuid`); the operator chose the standard-library client (R2). |
| E1: UAR embeds and stores | Goal fit 2 under the gate: only in topology B, gated on a UAR release, topology-A content not covered, and UAR's knowledge base is not keyed by CID (`/api/ingest` uses one hardcoded knowledge base). |
| Remote-only from `ipfs mcp` | Goal fit 2: G12 requires an embedded mode (operator Q10). |
| E4: another vector store inside Kubo | Goal fit 2: G12 names SurrealDB. |
| E5: a daemon-wide embedding hook | Refused under User Agency: indexes all node content without a per-item local cause. |
| In-memory vector search fallback | Not accepted as SurrealDB vector search; if HNSW cannot be used, G12 is deferred instead (G12-2). |

## Where this is weak

- **E6 scores 44/65, low in absolute terms.** It wins because every option that scored higher was excluded by the goal-fit gate, not because it is cheap: Kubo surface and maintenance are both 2.
- **The SurrealDB 3.x HTTP API details are unverified here.** The exact endpoints, headers and authentication flow for the standard-library client, and whether an HNSW index can be defined and queried through them, are pinned only by the real-service tests in `mcp-embeddings-surrealdb`.
- **Process supervision is new to Kubo.** No precedent exists in the repository for supervising a child database process.
- **The `surreal` binary path is trusted.** An operator who points it at a malicious binary runs that binary as their user; Kubo can check the path is absolute and the version, not the binary's integrity.
- **Coverage is incomplete by design.** WebMCP adds and out-of-agent unpins are handled only by explicit indexing and reconciliation.
- **Egress control is acknowledgement, not verification** (ADR-0004).

## Falsifiers

| ID | Assumption | Observable falsifier | Consequence |
|---|---|---|---|
| G12-1 | The standard-library client covers what the store needs | The real-service test against `surreal` 3.3.0 cannot authenticate, write, query or delete through the HTTP API with `net/http` | Return to the operator: approve `surrealdb.go` after a measured build probe (`CGO_ENABLED=0 make build`, binary delta under 3 MB), or defer G12 |
| G12-2 | SurrealDB vector search (HNSW) works for the configured dimension | The test cannot define or query an HNSW index for 1024-dimension `bge-m3` vectors | G12 deferred; no in-memory fallback |
| G12-3 | The managed process is supervised safely | An orphaned `surreal` process remains after `ipfs mcp` is killed, or it binds a non-loopback address | Embedded mode unsupported until fixed |
| G12-4 | Deletion stays consistent with the pin state | A CID pinned twice (recursive root and direct) loses its vectors when one pin is removed | Block |
| G12-5 | Disabled means disabled | With no `embeddings` section, a `surreal` process is spawned, a store or embedder is contacted (network capture), or an index tool is listed | Block |

## References

- SurrealDB Go embedding documentation, https://surrealdb.com/docs/reference/golang/embedding, fetched 2026-10-09
- SurrealDB license, https://github.com/surrealdb/surrealdb/blob/main/LICENSE, fetched 2026-10-09
- `.kbd-orchestrator/phases/ipfs-mcp-server/operator-answers.md`: G9-G12 answers Q8-Q10, Round 0 R1, R2, R5
- `.kbd-orchestrator/phases/ipfs-mcp-server/assessment-addendum.md`: Summary 5, G12 options and scores, DECISION (G12), ADR DRAFTS (ADR-0006), Orchestrator adjudication
- `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`: D6 (re-score and decision E6), D7, sections 3 and 4
- ADR-0003 (confirmation of `ipfs_index_cid`), ADR-0004 (`egress_ack`, principals)
