# Evidence: mcp-content-extractor 1.1, xberg v1.3.10 behavior

Date: 2026-10-14. Host: Darwin 25.6.0, arm64. Nothing here is committed
as a binary; the binary lives in a scratch directory outside the repository.

## Binary and checksum

- Asset: `xberg-cli-aarch64-apple-darwin.tar.gz` from release `v1.3.10` of `xberg-io/xberg`.
- SHA-256 of the downloaded file: `378cc1a7e76addb1302944fd155daead565dba1aa913cec56d19ef2a5878b45a`.
- Release digest published by GitHub for that asset (`gh api repos/xberg-io/xberg/releases/tags/v1.3.10 --jq '.assets[] | select(.name|test("aarch64-apple-darwin"))'`):
  `sha256:378cc1a7e76addb1302944fd155daead565dba1aa913cec56d19ef2a5878b45a`. They match.
- `xberg --version` prints `xberg 1.3.10`. The tarball also carries `libaom`, `libheif`, `libde265`,
  `libx265`, `libvmaf` and `libsharpyuv` dylibs next to the executable (loaded through the executable's directory).

## Method

Every claim below was run against the real binary on loopback ports 18733, 18735 and 18736, with a
scratch `XBERG_CACHE_DIR` and `HF_HOME`, then stopped by pid. Fixtures: a hand-written one-page PDF
("The zanzibar quokka migrates quietly across the lattice"), a DOCX made with `textutil`, and a CSV.

## Findings

### Serve flags, bind host, cache directory

`xberg serve --help`: `-H/--host`, `-p/--port`, `-c/--config` (TOML, YAML or JSON), `--log-level`.
Precedence: CLI flags, then `XBERG_HOST`/`XBERG_PORT`, then the config file, then the default `127.0.0.1:8000`.
There is no cache-directory flag; the cache directory is the environment variable `XBERG_CACHE_DIR`
(confirmed: after a run with `XBERG_CACHE_DIR=<dir>`, `<dir>/ocr/*.msgpack` exists). Hugging Face
artifacts follow `HF_HUB_CACHE`, then `HF_HOME` (stated in `xberg cache warm --help` and the
`/cache/warm` OpenAPI description), not `XBERG_CACHE_DIR`.

Binds only the requested host (`xberg serve --host 127.0.0.1 --port 18733`):

```
$ netstat -an -p tcp | grep 18733
tcp4  0  0  127.0.0.1.18733  *.*  LISTEN
$ curl -m3 http://10.0.0.20:18733/health      # the host's LAN address
lan:000                                        # connection refused
$ curl http://127.0.0.1:18733/health
{"status":"healthy","version":"1.3.10","plugins":{...}}
```

`GET /info` returns `{"version":"1.3.10","rust_backend":true}`. With `--config <file containing {}>`
the server starts normally, and an explicit `--config` skips the discovery of `xberg.toml` in the
working directory and its parents (documented in `xberg serve --help`), which the supervisor relies on.

CORS: by default the server answers every origin (`access-control-allow-origin: *`, and the startup log
warns "CORS configured to allow all origins (default)"). With `XBERG_CORS_ORIGINS=http://127.0.0.1:18736`
a request carrying `Origin: http://evil.example` gets no `access-control-allow-origin` header. The embedded
supervisor therefore sets `XBERG_CORS_ORIGINS` to its own loopback origin.

### Request shape of `POST /extract`

Multipart fields accepted (from the server's own 400 message): `file, files, urls, inputs, config,
output_format, pdf_password, format`. Anything else is a 400. `output_format=markdown` as a multipart
field and `config={"output_format":"markdown"}` both produce Markdown (the CSV becomes a pipe table);
with neither, the content is plain text (`'name city note\nAda Lisbon ...'`).

```
$ curl -F files=@sample.pdf  -F output_format=markdown http://127.0.0.1:18733/extract
  -> 200, results[0].content = "The zanzibar quokka migrates quietly across the lattice\n", mime_type application/pdf
$ curl -F files=@sample.docx -F output_format=markdown ...
  -> 200, "The zanzibar quokka migrates quietly across the lattice.\n\nSecond paragraph about orchids.\n"
$ curl -F files=@sample.csv  -F output_format=markdown ...
  -> 200, "| name | city | note |\n| --- | --- | --- |\n| Ada | Lisbon | velociraptor harmonics |\n| Bob | Oslo | plain |\n"
```

Response envelope (`ExtractionResult` in `xberg api schema`): `results[]` (each has `content`, `mime_type`,
`metadata`, `tables`, and optional `images`, which is absent unless image extraction is configured),
`errors[]` (`index`, `code`, `error_type`, `source`, `message`) and `summary`.

Failure shape: a file xberg cannot read is still **HTTP 200** with empty `results` and one `errors[]`
item, so a client must look at `errors` and `results`, not the status line:

```
bad.pdf -> {"results":[],"errors":[{"index":0,"code":1001,"error_type":"parsing","source":"bad.pdf","message":"Parsing error: ... Invalid cross-reference table"}],"summary":{...}}
bad.mp4 -> {"results":[],"errors":[{"index":0,"code":1010,"error_type":"unsupported_format","message":"Unsupported format: video/mp4"}],...}
```

Audio and video: `GET /formats` lists 127 formats and none is audio or video; `xberg extract --help` has no
transcription option; the OpenAPI document has no transcription field. **xberg v1.3.10 cannot transcribe.**
`extractor.transcribe: true` therefore yields `extraction_failed` for video today. The client sends the file
and reports xberg's `unsupported_format`; it does not pretend.

### Model downloads: the offline switch and what `/extract` returns

xberg downloads models lazily through `hf-hub` (log: `Resolving via hf-hub repo=xberg-io/layout-models`).
The switch that stops this is the environment variable `HF_HUB_OFFLINE=1` (the binary's strings also
carry `HUGGINGFACE_HUB_OFFLINE`; only `HF_HUB_OFFLINE` was exercised). Server started with
`HF_HUB_OFFLINE=1 XBERG_CACHE_DIR=<scratch> HF_HOME=<empty scratch>`; the HF cache stayed empty.

PaddleOCR image (needs a model that is not cached):

```
$ curl -F files=@sample.png -F output_format=markdown -F 'config={"ocr":{"backend":"paddle-ocr"},"force_ocr":true}' .../extract
HTTP 200, results [], errors [{"index":0,"code":1008,"error_type":"plugin","source":"sample.png",
  "message":"Plugin error in 'paddle-ocr': Hugging Face offline mode is enabled and 'v6/rec/small/model.onnx'
  from xberg-io/paddleocr-onnx-models@bc5ec866... is not available in the local cache"}]
```

Layout detection on a text PDF (needs the `rtdetr` model): HTTP 200 with the text extracted natively and
a `processing_warnings` entry "layout detection failed (... Hugging Face offline mode is enabled and
'rtdetr/model.onnx' ... is not available in the local cache)". So a missing model is either an `errors[]`
item (the OCR plugin) or a degraded-but-successful result with a warning (layout); `/extract` never hangs
trying to download.

Not exercised: a run **without** `HF_HUB_OFFLINE` that actually downloads a model. That would fetch from the
Hugging Face Hub, which the contract keeps behind `allow_model_downloads` (default false); no download was made.
Tesseract OCR needs no Hugging Face model (system `tessdata`).

### Endpoints that read a server-side path or fetch a URL

- `files` (multipart bytes) is the only input that reads nothing server-side.
- `urls` (JSON array of strings) makes the server fetch a URL: `urls=["http://127.0.0.1:18734/sample.csv"]`
  returned an `errors[]` item `ssrf_policy_violation ... denied by SSRF policy: loopback`, so xberg runs an
  SSRF policy, but it would fetch a public URL.
- `inputs` (JSON array of `{kind, uri|bytes|...}`) accepts local paths and `file://` URIs. Default answer:
  `400 "Local path and file:// URI extraction are disabled for the HTTP API. Set XBERG_API_ALLOW_LOCAL_URI_INPUTS=1 to
  enable server-side local URI access."` Same for `urls=["file:///..."]`.
- `PUT /process` (OpenWebUI) and `POST /v1/convert/file` (docling-compatible) take raw bytes or `files`.
- `POST /extract-async` + `GET /jobs/{id}` mirror `/extract`. `/cache/warm` downloads models (a Hugging Face call).
  `DELETE /cache/clear` clears the xberg cache.

Consequence for the client and the supervisor: the client sends **only** `files` and `output_format` (and a
`config` object only for OCR); it never sends `urls` or `inputs`. The supervisor starts xberg with a minimal
environment, so `XBERG_API_ALLOW_LOCAL_URI_INPUTS` cannot be inherited from the operator's shell. A remote
xberg that an operator exposes is outside this control, as the contract already says (no authentication).

### Not verified

- A transcription run (not supported by this version, see above).
- A model download without `HF_HUB_OFFLINE` (deliberately not run).
- Windows and Linux binaries (only the macOS arm64 asset was fetched).
