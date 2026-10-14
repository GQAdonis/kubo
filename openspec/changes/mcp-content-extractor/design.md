# Design: mcp-content-extractor

Phase `agent-backend-settings`. ADR-0011 V1, V2, V7 hold the decisions; this file records the Round 4 correction decisions and the docs follow-ups.

## xberg source (2026-10-14)

The operator chose the fork `Know-Me-Tools/xberg` (git `git@github.com:Know-Me-Tools/xberg.git`) so xberg can be adapted for our needs; see the ADR-0011 amendment of 2026-10-14. Its `main` and `v1.3.10` are the same commit as upstream (`d926382a`), so the behavior verified in `evidence/2026-10-14-task-1.1/` holds until the fork diverges, and the upstream `v1.3.10` release binary is a build of that commit. Fork-built artifacts are not yet published (open operator item).

## Allowlist gating and skip reasons

- An extractor is configured and a type is not allowlisted: content of a listed kind with a binary signature is skipped `kind_not_enabled`; UTF-8 text with no binary signature (JSON, CSV, XML, HTML) stays on the native path and is skipped `mime_not_allowed`. Rejecting such text would de-index content when an extractor is added.
- PDF skips as `kind_not_enabled`; XML and HTML skip as `mime_not_allowed`.
- An already-indexed JSON CID stays `kind: text`, `source: text` until `force` or reconcile runs after `application/json` is allowlisted.

## Name-based detection

`ipfs_index_cid` receives only a CID, so `mimeByName` is unreachable from it. Until the bulk directory walk passes file names (`mcp-whole-node-indexing` task 1.8), code without a shebang, YAML, TOML, Markdown and iWork index as text or skip.

## Size caps

Text uses `embeddings.policy.max_bytes`. Content routed to the extractor uses `embeddings.extractor.max_bytes`, and its extracted output is bounded by `embeddings.extractor.max_text_bytes`. The `ipfs_index_cid` description states this.

## Residual risk of the embedded listener

xberg 1.3.10 has no auth option or Unix socket, so its ephemeral loopback port is reachable by any local process and by web pages (simple cross-origin POSTs; no `Host` check, so readable after DNS rebinding), which can make it fetch public URLs (literal private, link-local and loopback targets are refused by xberg policy, see `evidence/2026-10-14-ssrf-policy/`), clear its model cache and spend CPU; `ipfs mcp` reaches it only through its authenticated proxy, the operator accepted the residual on 2026-10-14, and omitting the extractor block or using a remote extractor avoids it.

## Docs follow-ups

`vector-search-contract` task 1.7 is complete, so these land in `docs/mcp/embeddings.md` and `docs/config.md` as a follow-up:

- The `extractor` options, including `max_text_bytes`.
- The pre-warm recipe: `XBERG_CACHE_DIR=<cache_dir> HF_HOME=<cache_dir>/huggingface xberg cache warm`.
- A remote store now receives extracted plaintext (also in the `egress_not_acknowledged` message).
- Residual risk of the local xberg listener on loopback.
- `transcribe` is not supported by xberg 1.3.10.
