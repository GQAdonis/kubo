# Sources: SurrealDB queries and flags sent by `ipfs mcp` embeddings

Verified 2026-10-09 against the SurrealDB docs and against `surreal` 3.3.0 on macOS arm64 (a private instance on an OS-chosen port, torn down after each probe). Every statement this package sends is listed with its source. The unit tests replay responses recorded from that run (`testdata/`), and `test/cli/mcp_embeddings_store_test.go` runs them against the real binary.

## Client decision (G12-1)

- The client is the Go standard library over SurrealDB's HTTP API. No SurrealDB module is added to any `go.mod` (operator answer R2).
- Build probe of `github.com/surrealdb/surrealdb.go` v1.7.0 in a scratch module outside the repository: `CGO_ENABLED=0 go build` succeeds. A program that only imports it grew from 5,097,906 to 10,402,754 bytes (+5.3 MB, an upper bound for Kubo because Kubo already links `net/http` and more). New modules: `fxamacker/cbor/v2` v2.9.2, `gofrs/uuid` v4.4.0+incompatible, `gorilla/websocket` v1.5.3, `x448/float16` v0.8.4. That exceeds the 3 MB limit in isolation; the operator declined the module anyway.

## HTTP API

| Use | Request | Source |
|---|---|---|
| Queries with bound variables | `POST /rpc`, `Content-Type: application/json`, `Accept: application/json`, body `{"id":1,"method":"query","params":[<surql>, {<vars>}]}`, reply `{"id":1,"result":[{status,result}...]}` or `{"error":{code,message}}` | https://surrealdb.com/docs/reference/rest-api/http-protocol (the `query` example with `params`); confirmed on 3.3.0 |
| Namespace and database | headers `Surreal-NS`, `Surreal-DB` | same page, "Headers" |
| Authentication | HTTP Basic (`user:password`) or `Authorization: Bearer <token>` | same page; confirmed: wrong password gives HTTP 401 |
| Readiness | `GET /health` answers 200 | confirmed on 3.3.0 |
| Per-statement failure | HTTP 200 with `"status":"ERR"` and the message in `result` | observed on 3.3.0; checked for every statement |

## Statements

| Statement | Source |
|---|---|
| `DEFINE NAMESPACE IF NOT EXISTS`, `USE NS`, `DEFINE DATABASE IF NOT EXISTS` | https://surrealdb.com/docs/reference/query-language/statements/define ; confirmed |
| `DEFINE TABLE IF NOT EXISTS ipfs_chunk SCHEMAFULL`, `DEFINE FIELD IF NOT EXISTS ... TYPE string/int/array<float, N>` | https://surrealdb.com/docs/reference/query-language/statements/define ; confirmed |
| `DEFINE INDEX IF NOT EXISTS ... FIELDS cid` | https://surrealdb.com/docs/reference/query-language/statements/define/indexes |
| `DEFINE INDEX IF NOT EXISTS ipfs_chunk_vector ON ipfs_chunk FIELDS vector HNSW DIMENSION N DIST COSINE TYPE F32` | https://surrealdb.com/docs/reference/query-language/statements/define/indexes : `HNSW DIMENSION @dimension [ TYPE @type ] [ DIST @distance ] [ EFC @efc ] [ M @m ]` |
| `INFO FOR TABLE ipfs_chunk` (reads `indexes.ipfs_chunk_vector`, whose text holds `DIMENSION n`) | confirmed on 3.3.0 (`testdata/rpc_info_table_dim4.json`) |
| `BEGIN TRANSACTION; DELETE ipfs_chunk WHERE cid = $cid; FOR $r IN $rows { UPSERT type::record('ipfs_chunk', $r.id) CONTENT {...}; }; COMMIT TRANSACTION;` | UPSERT: https://surrealdb.com/docs/reference/query-language/statements/upsert (`UPSERT @targets CONTENT @value`); `FOR`, `type::record` and transactions confirmed on 3.3.0 |
| `SELECT cid, path, mime, chunk_index, model, vector::distance::knn() AS distance FROM ipfs_chunk WHERE vector <\|K,EF\|> $v ORDER BY distance` | KNN operator `<\|K,EF\|>`: indexes page above; `vector::distance::knn()`: https://surrealdb.com/docs/reference/query-language/functions/database-functions/vector#vectordistanceknn ; confirmed (cosine distance 0.0 for identical, 1.0 for orthogonal) |
| `DELETE ipfs_chunk WHERE cid = $cid RETURN cid` | https://surrealdb.com/docs/reference/query-language/statements/delete ; confirmed |
| `SELECT count() AS n FROM ipfs_chunk GROUP ALL` (an empty table returns `[]`) | confirmed on 3.3.0 (`testdata/rpc_count_empty.json`) |
| `SELECT cid FROM ipfs_chunk WHERE cid > $after GROUP BY cid ORDER BY cid LIMIT N` (distinct CIDs, paged by a bound cursor) | https://surrealdb.com/docs/reference/query-language/statements/select (`GROUP BY`, `ORDER BY`, `LIMIT`); confirmed on 3.3.0 by `TestMCPEmbeddingIndexLifecycle` (reconcile pages) |
| `SELECT count() AS n FROM ipfs_chunk WHERE cid = $cid GROUP ALL` | same page; confirmed on 3.3.0 (no match returns `[]`) |
| `SELECT count() AS n FROM (SELECT cid FROM ipfs_chunk GROUP BY cid) GROUP ALL` (distinct CIDs) | subquery in `FROM`: same page; confirmed on 3.3.0 |
| `SELECT cid, path, mime, chunk_index, model, 1 - vector::similarity::cosine(vector, $v) AS distance FROM ipfs_chunk WHERE cid = $cid ORDER BY distance LIMIT N` | https://surrealdb.com/docs/reference/query-language/functions/database-functions/vector#vectorsimilaritycosine ; confirmed on 3.3.0 by `TestMCPEmbeddingIndexAutoOffStillIndexesExplicitly` (a per-CID search; cosine distance is 1 minus the similarity, the same scale as `vector::distance::knn()` for the cosine index) |
| `DEFINE USER OVERWRITE root ON ROOT PASSWORD "<hex>" ROLES OWNER` (rotation) | https://surrealdb.com/docs/reference/query-language/statements/define/user ; the password must be a literal, a bound parameter is a parse error on 3.3.0, so it is lower-case hex generated by the supervisor |

## `surreal start` (from `surreal start --help`, 3.3.0)

- `surreal start --no-banner --deny-guests --bind 127.0.0.1:<port> surrealkv://<absolute path>`.
- `SURREAL_USER` and `SURREAL_PASS` set the initial root user "only if no other root user exists", which is why a restart on an existing data dir signs in with the stored credentials and rotates the password.
- `surreal version` prints `3.3.0 for macos on aarch64`; the first word is checked against `embedded.min_version`.
- Anonymous requests are refused: `Anonymous access not allowed` (observed with `--deny-guests`).
