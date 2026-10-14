# Research: vector search over all node files (2026-10-14)

Read-only research for the operator's 2026-10-14 scope change (`execution.md`, "Operator direction 2026-10-14"). Gathered by a read-only Claude research subagent from GitHub, release metadata, model cards and vendor docs; the lead checked repository facts with `gh`. Items marked unverified were not run or not found in a source. This note feeds the architect's plan revision; it decides nothing.

## Operator requirements

- Vector search over files on the cloud node (LKE) and IPFS Desktop (macOS arm64).
- File types: text and Markdown, PDF and Office, source code and JSON/CSV, images, and image and video search through Xiaomi MiMo-V2.6 descriptions ("embeddings off inference").
- Document processing: xberg, upstream `xberg-io/xberg` latest release (the Prometheus-AGS fork is not used).
- Embedder host: configurable.
- Model access: `GQAdonis/liter-llm` "if necessary".
- Search from agent chat and MCP clients, and from a search box in the Web UI.

## What already exists (G12)

- `ipfs mcp` chunks text, embeds through an OpenAI-compatible `/embeddings` endpoint and stores vectors in SurrealDB (embedded or remote); tools `ipfs_index_status`, `ipfs_index_search`, `ipfs_index_cid`, `ipfs_index_reconcile` (`docs/mcp/embeddings.md`, ADR-0006).
- Gaps: only adds through `ipfs mcp` are auto-indexed (no bulk index of pins or MFS); `text/plain` and `text/markdown` only, 1 MiB cap; search only through agents; nothing configured on LKE; indexing needs an HTTP principal with `egress_ack`.

## xberg (upstream `xberg-io/xberg`)

- Latest release `v1.3.10` (2026-10-13). License MIT. Renamed from Kreuzberg (`kreuzberg-dev/kreuzberg` redirects).
- Formats: 107 formats including PDF with OCR, Office (doc/docx/xls/xlsx/ppt/pptx), ODF, iWork, JSON, CSV, YAML, Markdown; images via OCR (Tesseract, PaddleOCR, Candle, or a VLM backend); code via tree-sitter (371 languages) with syntax-aware chunking; audio and video **transcription of the audio track only** (Whisper ONNX). It does not describe visual content.
- Deployment: library bindings for 15 languages plus C FFI (the Go binding uses cgo and downloads a prebuilt `libxberg_ffi` at setup); CLI with `serve`, `mcp`, `embed`, `chunk`; REST server `xberg serve` (port 8000); MCP server; images `ghcr.io/xberg-io/xberg:<ver>` (about 1.5-2.1 GB) and `:<ver>-core` (about 1.0-1.3 GB), linux/amd64 and linux/arm64 only; Helm chart `charts/xberg`; macOS through the CLI binary.
- REST: `POST /extract` (multipart `files`, optional `config` JSON, `output_format` plain, markdown, djot, html, json or doctags) returns `{results:[{content, mime_type, metadata, tables, detected_languages, chunks, images}], errors, summary}`; also `/extract-async`, `/jobs/{id}`, `/health`, `/detect`, `/cache/warm`, `/openapi.json`. Body limit 100 MB by default. **No authentication is documented**: the listener must stay on loopback or ClusterIP.
- Network: layout, PaddleOCR and NER models are downloaded at runtime from the Hugging Face Hub at pinned revisions (`/cache/warm` pre-fetches into `XBERG_CACHE_DIR`/`HF_HOME`). No phone-home code found; telemetry is local OTel/Prometheus behind features that are off by default. An offline flag is unverified.

## MiMo-V2.6 (Xiaomi)

- Released 2026-09-22. Pro (1.02T total, 42B active, MoE) and Flash (309B total, 15B active, MoE): text, image, video and audio in, text out, 1M context. Distill-Qwen-9B (9.4B, image-text-to-text). Pro-UltraSpeed is hosted only. Open weights under MIT (the Qwen base license of the 9B distill was not checked).
- Hosted API: OpenAI-compatible, `https://api.xiaomimimo.com/v1`, models `mimo-v2.6-flash` and `mimo-v2.6-pro`. Image part `{"type":"image_url","image_url":{"url": https or data URL}}` (JPEG, PNG, GIF, WebP, BMP, 50 MB). Video part `{"type":"video_url","video_url":{"url":...},"fps":2,"media_resolution":"default"}` (`fps` 0.1-10; MP4, MOV, AVI, WMV; 300 MB by URL or 50 MB base64; no maximum duration documented). Price per 1M tokens (input/output): Flash $0.14/$0.28, Pro $0.435/$0.87. No embeddings endpoint documented.
- Self-hosting: Flash on vLLM with tensor parallel 4 or SGLang with tp 8; Pro on SGLang tp 16. Multi-GPU datacenter hardware; nothing official for a Mac or CPU-only. Whether the 9B distill handles video is unverified.

## liter-llm (`GQAdonis/liter-llm`)

- Fork of `xberg-io/liter-llm`: 67 commits ahead, 77 behind; its `v1.1.1` tag dates from 2026-03-29; upstream latest release `v2.2.3` (2026-10-10). No release or image published under the fork. License MIT.
- `liter-llm api --config liter-llm-proxy.toml` runs an OpenAI-compatible proxy: `/v1/chat/completions`, `/v1/embeddings`, `/v1/models`, `/v1/audio/transcriptions`, `/v1/rerank`, `/v1/ocr`, `/health`. TOML config with hot reload. Default listener `0.0.0.0:4000`; optional `master_key` and virtual keys (whether it serves unauthenticated with no key is unverified).
- **Video limit**: the request type's content parts are `text`, `image_url`, `document`, `input_audio` only (`crates/liter-llm/src/types/common.rs:106`); MiMo's `video_url` part and its `fps` and `media_resolution` fields would very likely be rejected or dropped (inferred from types, not run). The built-in `xiaomi_mimo` provider is chat-only and marked `"vision": false` (images may be refused; unverified).
- Providers include Ollama (chat, embeddings) and vLLM (chat, embeddings). Upstream image `ghcr.io/xberg-io/liter-llm` (linux/amd64, linux/arm64, about 35 MB); macOS through `brew install xberg-io/tap/liter-llm`.

## Recommended shape (for the architect to decide)

1. **xberg runs as a service, never linked.** LKE: a ClusterIP Service or sidecar from `ghcr.io/xberg-io/xberg:v1.3.10` pinned by digest, models pre-warmed into a volume. Desktop: the `xberg` CLI (`xberg serve` on loopback, random port) supervised by Desktop or `ipfs mcp`. Linking the cgo binding into the `ipfs` binary is refused: it adds a native dependency and a build-time download to Kubo's `go.mod`.
2. **`ipfs mcp` gains an extractor**: bytes from `cat` go to `POST /extract` (markdown output), then the existing chunk, embed and SurrealDB path. One embedding path.
3. **A describer for images and video**: an OpenAI-compatible chat endpoint (MiMo hosted, or self-hosted vLLM or SGLang) returns a description; the description and xberg's audio transcript are embedded and indexed with the file's CID.
4. **liter-llm as the model gateway** for embeddings and image description, on loopback or ClusterIP with a mandatory master key: one endpoint for `ipfs mcp`, provider keys in one operator-owned TOML. Video goes direct to the describer endpoint until the fork gains a `video_url` content part (or upstream does), and `xiaomi_mimo` vision is enabled.
5. **Bulk indexing** of pins and MFS: operator-started, paged, rate- and size-capped, resumable; never automatic in the background.
6. **Web UI search page** calling a search route on the `ipfs mcp` HTTP surface (principal-scoped), rendering hits with CID, path, score and snippet.
7. **Config** lives in the `ipfs mcp` config file and `docs/mcp/config-reference.md` (D12), never Kubo's repo config: `embeddings.extractor.{base_url, timeout, max_bytes, mime_allowlist}`, `embeddings.describer.{base_url, model, api_key, video_fps, max_video_bytes, max_duration}`, `embeddings.bulk.{rate, max_bytes}`, each with a default and an off switch, and the extractor and describer hosts covered by `egress_ack` unless loopback.

## Risks (the uncomfortable part)

- **Data egress to a third party.** Describing images and video with Xiaomi's hosted API sends file bytes to Xiaomi. It must never be a default; it requires an explicit `egress_ack` for the host and is named in the changelog. Self-hosting needs 4-8 datacenter GPUs; there is no Mac path for video description.
- **Resource use.** OCR, layout and Whisper models are CPU-heavy, the xberg image is 1-2 GB, and indexing "all files" spends the operator's CPU and bandwidth. Bulk indexing is opt-in, capped and visible.
- **Hidden network calls.** xberg's runtime Hugging Face downloads are an endpoint the operator did not configure unless models are pre-warmed or downloads are gated.
- **Unauthenticated services.** xberg has no documented auth; liter-llm defaults to `0.0.0.0`. Both stay on loopback or ClusterIP, liter-llm with a master key.
- **Supply chain.** Pin xberg and liter-llm by version and image digest; the liter-llm fork has no published artifact and diverges from upstream.
- **Video through liter-llm does not work today** (typed content parts).
