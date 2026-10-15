# UAR v1.1.0 pin report

This is the evidence file that `mcp/contract/agentic/uar-preflight.json` names in `pinnedFrom.report`.

## Pinned release

| Field | Value |
|-------|-------|
| Tag | `v1.1.0` |
| Source commit | `d815ef72ac7d2c57a9bd18b23fcc58ffb224f326` |
| Image (multi-platform manifest) | `ghcr.io/prometheus-ags/universal-agent-runtime@sha256:064f5004fb3ba76506abce5cf81285b2a8aa3e54fa5a22e2c2bfc97b1211e43b` |
| amd64 digest | `sha256:94f78a7ed17197fc045baec514ed6ec30d3cf23433e8c26340ccfddb080fac90` |
| arm64 digest | `sha256:b3a3d8bdda9d919fa3489dded935c834256bf89984fa17b07a428acdb7a1c1ae` |
| Certification | operator gate OG-4: certified on arm64 and amd64 (amd64 under emulation) |
| Integration contract | `mcp/uar/v1.1.0-kubo.md` (copied from the tag) |

`minVersion` is `1.1.0`. `settingsPath` is `/api/uar/settings/effective`, the effective-settings endpoint requested in `mcp/uar/requests/2026-10-13-v1.1.md` (item 8 of the 2026-10-12 request, read with an admin bearer).

## What is verified, and what is not

- The tag, digests and certification above come from the operator's OG-4 record. No UAR image was pulled or run to write this file.
- The settings response shape is validated against `mcp/contract/agentic/uar-effective-settings.schema.json` at run time by `check-uar`. Whether the released endpoint serves that shape has not been observed; the real-service run of `agent-backend-uar` 1.5 (`test/cli/mcp_uar_backend_test.go`) is the first check against the published image.
