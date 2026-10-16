# agent-backend-uar 1.2 and 1.5: real-service run (2026-10-16)

`go test ./test/cli/ -run 'TestMCPUARBackend$' -v` at feat/ipfs-mcp-server with the skill-version and diagnostics fixes, against:

- UAR `ghcr.io/prometheus-ags/universal-agent-runtime@sha256:064f5004fb3ba76506abce5cf81285b2a8aa3e54fa5a22e2c2bfc97b1211e43b` (v1.1.0, arm64 on this Mac) with the pinned SurrealDB companion, started the way UAR's release acceptance does;
- model: Alibaba Qwen Token Plan `https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1`, `openai/qwen3.8-max`, the provider the know-me.tools chat uses (operator direction); the key was read from `knowme/uar-secrets` `UAR_LLM__API_KEY` into an environment variable inside the single command and never printed; `test-output.txt` contains it zero times (checked by `grep -c -F`).

Result: PASS (95.4 s), `test-output.txt`.

| Check | Result |
|---|---|
| `GET /api/agents/kubo-ipfs` shows kubo's prompt (safety prefix included), the kubo tools as the allowlist, skill bindings equal to `mcp/skills` directory names | pass |
| one chat run through `/agui` completes | pass |
| a write run raises UAR `tool_approval`, translated to the kubo shape, resumes after approval and finishes | pass (pinned afterwards: false; kubo's own confirmation decides the write and was not answered in the test, by design) |
| a denied write ends with nothing executed | pass |

Earlier attempts the same day: run 1 failed on the test's listen-address regex (`[::]`); run 2 got UAR 400 `skill_markdown_invalid` ("missing field `version`") on skill import, fixed by adding `version: 1.0.0` to uploaded frontmatter; two further failures (MCP destination registered after the first run, `host.docker.internal` not a private address) were found and fixed with a dummy key before run 3.
