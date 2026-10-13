# Goals

- Runtime settings UI for the agent backend in BOTH IPFS Desktop (macOS arm64) and the Web UI served from the cloud node (https://ipfs.prometheusags.ai), replacing config-file-only setup; saving applies without a manual restart
- Backend option 1, Universal Agent Runtime (UAR): point at a UAR instance, create an IPFS agent on it (own system prompt, the kubo MCP tools, the IPFS agent skills) and chat through that agent; code at ~/Projects/prometheus/universal-agent-runtime (our code; prior deferral tracked in Prometheus-AGS/universal-agent-runtime#379)
- Backend option 2, OpenAI-compatible endpoint (e.g. liter-llm): a Go proxy in this kubo codebase, started like ipfs mcp, that serves AG-UI plus the A2UI control registry and forwards to the OpenAI-compatible endpoint (build on the existing ipfs mcp --agui bounded agent loop rather than duplicating it)
- Agent features on by default per AGENTS.md 5f019f0df (Desktop starts the local endpoint, chat on, WebMCP on, Desktop confirmation window on); every feature keeps an off switch; writes still require confirmation; no third-party backend chosen for the operator
