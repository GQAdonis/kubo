//go:build !noagent

package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/ipfs/kubo/mcp/contract"
	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of mcp-agui-endpoint. They run the built ipfs binary
// against a daemon with an isolated IPFS_PATH, a model server on an OS-chosen
// loopback port and `ipfs mcp --http=127.0.0.1:0`. The native confirmation
// prompt is off (mcpEnv sets IPFS_MCP_CONFIRM=none), so no test raises a system
// dialog: writes are answered by the chat, and destructive calls, which need the
// system prompt, are refused.

const (
	aguiChatToken  = "cli-agui-chat-token-0123456"
	aguiChat2Token = "cli-agui-chat2-token-012345"
	aguiOriginWeb  = "webui://localhost"
)

// aguiConfig is the --mcp-config of the AG-UI tests: two chat principals (one
// may remove pins, which only the system prompt could approve), a human-mcp
// reader and an A2A peer. Credentials are env references. An empty llmBase
// leaves the llm section out, so no backend is configured.
func aguiConfig(t *testing.T, llmBase string, agui bool) string {
	t.Helper()
	cfg := map[string]any{
		"agent": map[string]any{
			"agui":             agui,
			"origin_allowlist": []string{aguiOriginWeb},
		},
		"principals": []any{
			map[string]any{"name": "chat", "kind": "ui", "credential": map[string]any{"env": "CLI_AGUI_CHAT_TOKEN"},
				"write_groups": []string{"pin"}, "destructive_groups": []string{"pin"}, "egress_ack": []string{"local"}},
			map[string]any{"name": "chat2", "kind": "ui", "credential": map[string]any{"env": "CLI_AGUI_CHAT2_TOKEN"},
				"write_groups": []string{"pin"}, "egress_ack": []string{"local"}},
			map[string]any{"name": "reader", "kind": "human-mcp", "credential": map[string]any{"env": "CLI_MCP_READER_TOKEN"}},
			map[string]any{"name": "peer", "kind": "a2a", "credential": map[string]any{"env": "CLI_MCP_PEER_TOKEN"}, "groups": []string{"node", "pin"}},
		},
	}
	if llmBase != "" {
		cfg["llm"] = map[string]any{"base_url": llmBase, "model": "m"}
	}
	if !agui {
		delete(cfg["agent"].(map[string]any), "agui")
	}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "mcp-agui.json")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

func aguiEnvVars() []string {
	return []string{"CLI_AGUI_CHAT_TOKEN=" + aguiChatToken, "CLI_AGUI_CHAT2_TOKEN=" + aguiChat2Token}
}

// aguiEvent is one decoded AG-UI event.
type aguiEvent = map[string]any

// aguiResult is a response to POST /agui.
type aguiResult struct {
	Status int
	Header http.Header
	Body   string
	Events []aguiEvent
}

func (r *aguiResult) types() []string {
	out := make([]string, 0, len(r.Events))
	for _, e := range r.Events {
		out = append(out, fmt.Sprint(e["type"]))
	}
	return out
}

func aguiRequest(ctx context.Context, t *testing.T, p *mcpHTTPProc, token string, body any, header map[string]string) *http.Response {
	t.Helper()
	var raw []byte
	if s, ok := body.(string); ok {
		raw = []byte(s)
	} else {
		var err error
		raw, err = json.Marshal(body)
		require.NoError(t, err)
	}
	method := http.MethodPost
	if m := header["method"]; m != "" {
		method = m
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base+"/agui", bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range header {
		if k != "method" {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func readAGUI(t *testing.T, resp *http.Response) *aguiResult {
	t.Helper()
	out := &aguiResult{Status: resp.StatusCode, Header: resp.Header}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		out.Body = string(b)
		return out
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
			var ev aguiEvent
			require.NoError(t, json.Unmarshal([]byte(data), &ev), data)
			out.Events = append(out.Events, ev)
		}
	}
	require.NoError(t, sc.Err())
	return out
}

func aguiPost(t *testing.T, p *mcpHTTPProc, token string, body any, header map[string]string) *aguiResult {
	t.Helper()
	return readAGUI(t, aguiRequest(t.Context(), t, p, token, body, header))
}

func aguiInput(thread, run, text string) map[string]any {
	return map[string]any{
		"threadId": thread, "runId": run, "tools": []any{}, "context": []any{},
		"messages": []any{map[string]any{"id": "msg-u-" + run, "role": "user", "content": text}},
	}
}

func aguiResume(thread, run, interruptID, status string, payload any) map[string]any {
	entry := map[string]any{"interruptId": interruptID, "status": status}
	if payload != nil {
		entry["payload"] = payload
	}
	return map[string]any{"threadId": thread, "runId": run, "tools": []any{}, "context": []any{}, "messages": []any{}, "resume": []any{entry}}
}

func aguiInterrupt(t *testing.T, r *aguiResult) map[string]any {
	t.Helper()
	require.NotEmpty(t, r.Events, "status %d body %s", r.Status, r.Body)
	last := r.Events[len(r.Events)-1]
	require.Equal(t, "RUN_FINISHED", last["type"], "%v", r.types())
	out := last["outcome"].(map[string]any)
	require.Equal(t, "interrupt", out["type"])
	list := out["interrupts"].([]any)
	require.Len(t, list, 1)
	return list[0].(map[string]any)
}

// validateAGUIFixtureSchema checks the exchange against the contract's AG-UI
// fixture schema, the same rules the recorded fixtures obey, and against the
// published @ag-ui/core schemas when AGUI_CORE_DIR names them (see
// mcp/server/agui/SOURCES.md).
func validateAGUIFixtureSchema(t *testing.T, input map[string]any, events []aguiEvent) {
	t.Helper()
	raw, err := fs.ReadFile(contract.FS(), "fixtures/agui.schema.json")
	require.NoError(t, err)
	var s jsonschema.Schema
	require.NoError(t, json.Unmarshal(raw, &s))
	resolved, err := s.Resolve(nil)
	require.NoError(t, err)
	doc := map[string]any{
		"description": "Events recorded from ipfs mcp --http --agui in test/cli.", "provenance": "recorded",
		"principalKind": "ui", "input": input, "events": events,
	}
	// Normalise through JSON so numbers and slices have the shapes the schema expects.
	b, err := json.Marshal(doc)
	require.NoError(t, err)
	var generic any
	require.NoError(t, json.Unmarshal(b, &generic))
	require.NoError(t, resolved.Validate(generic), "events do not match fixtures/agui.schema.json: %s", b)

	if dir := os.Getenv("AGUI_CORE_DIR"); dir != "" {
		node, err := exec.LookPath("node")
		require.NoError(t, err)
		cmd := exec.CommandContext(t.Context(), node, "../../mcp/server/agui/validate.cjs")
		cmd.Env = append(os.Environ(), "AGUI_CORE_DIR="+dir)
		in, _ := json.Marshal(map[string]any{"inputs": []any{input}, "events": events})
		cmd.Stdin = bytes.NewReader(in)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "@ag-ui/core rejected the events: %s", out)
	}
}

func hasPin(node *harness.Node, cid string) bool {
	return strings.Contains(node.RunIPFS("pin", "ls", "--type=recursive", "-q").Stdout.String(), cid)
}

func aguiResultContent(t *testing.T, r *aguiResult, callID string) string {
	t.Helper()
	for _, e := range r.Events {
		if e["type"] == "TOOL_CALL_RESULT" && e["toolCallId"] == callID {
			return fmt.Sprint(e["content"])
		}
	}
	t.Fatalf("no TOOL_CALL_RESULT for %s in %v", callID, r.types())
	return ""
}

func TestMCPAGUI(t *testing.T) {
	t.Run("disabled: a backend without --agui serves no /agui and makes no model call", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		model := newCLIModel(t, cliSayStep("must never be asked"))
		p := startMCPHTTPArgs(t, node, aguiEnvVars(), "--mcp-config", aguiConfig(t, model.baseURL(), false))
		for _, tok := range []string{aguiChatToken, ""} {
			r := aguiPost(t, p, tok, aguiInput("t-off", "r1", "hello"), nil)
			assert.Equal(t, http.StatusNotFound, r.Status, "token %q", tok)
		}
		assert.Equal(t, http.StatusNotFound, p.status(mcpHTTPRequest{method: http.MethodOptions, path: "/agui", header: map[string]string{
			"Origin": aguiOriginWeb, "Access-Control-Request-Method": "POST"}}))
		assert.NotContains(t, p.stderr.String(), "AG-UI endpoint enabled")
		assert.Zero(t, model.hits())
		assertOnlyDaemonTraffic(t, p, node)
	})

	t.Run("help documents the disabled and no-backend states", func(t *testing.T) {
		t.Parallel()
		help, _, code := runMCP(t, harness.NewT(t).IPFSBin, t.TempDir(), "--help")
		assert.Zero(t, code)
		assert.Contains(t, help, "path answers 404")
		assert.Contains(t, help, "503 backend_not_configured")
	})

	t.Run("enabled without a backend: /agui answers 503 backend_not_configured", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		p := startMCPHTTPArgs(t, node, aguiEnvVars(), "--mcp-config", aguiConfig(t, "", true))
		r := aguiPost(t, p, aguiChatToken, aguiInput("t-nobackend", "r1", "hello"), nil)
		assert.Equal(t, http.StatusServiceUnavailable, r.Status, r.Body)
		assert.Contains(t, r.Body, "backend_not_configured")
		assert.Empty(t, r.Events, "no SSE stream was started")
		assert.Equal(t, http.StatusForbidden, p.status(mcpHTTPRequest{path: "/agui", token: mcpHTTPReaderToken, body: "{}"}),
			"the mount still scopes /agui to the ui principal")
		assert.Equal(t, http.StatusUnauthorized, p.status(mcpHTTPRequest{path: "/agui", body: "{}"}))
		assert.Contains(t, p.stderr.String(), "AG-UI endpoint enabled at /agui")
		assert.Contains(t, p.stderr.String(), "/agui answers 503 backend_not_configured")
		assertOnlyDaemonTraffic(t, p, node)
	})

	t.Run("disabled: --agui needs the HTTP listener", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		model := newCLIModel(t)
		// Stdio serves until stdin closes; runMCP gives it none, so it exits.
		_, stderr, code := runMCP(t, node.IPFSBin, node.Dir, "--agui", "--llm-base-url", model.baseURL(), "--llm-model", "m")
		assert.Equal(t, 0, code, stderr)
		assert.Contains(t, stderr, "--agui is ignored")
		assert.Zero(t, model.hits())
	})

	t.Run("a read-only run streams events that match the contract schema", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		model := newCLIModel(t,
			cliCallStep("tc-info", "ipfs_node_info", map[string]any{}),
			cliSayStep("This node is "+node.PeerID().String()))
		p := startMCPHTTPArgs(t, node, aguiEnvVars(), "--mcp-config", aguiConfig(t, model.baseURL(), true), "--agui")
		assert.Contains(t, p.stderr.String(), "AG-UI endpoint enabled at /agui")

		in := aguiInput("thread-read", "run-1", "Who are you?")
		r := aguiPost(t, p, aguiChatToken, in, nil)
		require.Equal(t, http.StatusOK, r.Status, r.Body)
		assert.Equal(t, "text/event-stream", r.Header.Get("Content-Type"))
		assert.Equal(t, "RUN_STARTED", r.types()[0])
		assert.Equal(t, map[string]any{"type": "success"}, r.Events[len(r.Events)-1]["outcome"])
		assert.Contains(t, aguiResultContent(t, r, "tc-info"), node.PeerID().String(), "the tool ran against the real node")
		validateAGUIFixtureSchema(t, in, r.Events)
		assert.Equal(t, 2, model.hits())
	})

	t.Run("an unconfirmed write leaves the node unchanged; the chat's approval runs it once", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		toPin := node.IPFSAddStr("pin me from the chat", "--pin=false")
		model := newCLIModel(t,
			cliCallStep("tc-pin", "pin_add", map[string]any{"path": "/ipfs/" + toPin, "recursive": true}),
			cliSayStep("Pinned."),
			cliCallStep("tc-pin2", "pin_add", map[string]any{"path": "/ipfs/" + toPin, "recursive": true}),
			cliSayStep("Understood, nothing pinned."))
		p := startMCPHTTPArgs(t, node, aguiEnvVars(), "--mcp-config", aguiConfig(t, model.baseURL(), true))

		// Thread A: the run ends with an interrupt and nothing is pinned.
		in := aguiInput("thread-a", "run-1", "pin "+toPin)
		first := aguiPost(t, p, aguiChatToken, in, nil)
		require.Equal(t, http.StatusOK, first.Status, first.Body)
		intr := aguiInterrupt(t, first)
		validateAGUIFixtureSchema(t, in, first.Events)
		assert.False(t, hasPin(node, toPin), "an interrupted write changes nothing")
		meta := intr["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)
		assert.Equal(t, "pin_add", meta["tool"])
		assert.Equal(t, false, meta["requires_native_prompt"])
		assert.Equal(t, 1, model.hits(), "the model is not asked again while the call waits")

		// Another chat principal cannot answer it.
		other := aguiPost(t, p, aguiChat2Token, aguiResume("thread-a", "run-2", intr["id"].(string), "resolved", map[string]any{"approved": true}), nil)
		assert.Equal(t, http.StatusNotFound, other.Status)
		assert.False(t, hasPin(node, toPin))

		// A token for another kind of principal cannot reach the route at all.
		for _, tok := range []string{mcpHTTPReaderToken, mcpHTTPPeerToken} {
			r := aguiPost(t, p, tok, aguiResume("thread-a", "run-2", intr["id"].(string), "resolved", map[string]any{"approved": true}), nil)
			assert.Equal(t, http.StatusForbidden, r.Status)
		}
		assert.False(t, hasPin(node, toPin))

		// The owner approves in the chat: the pin is added, once.
		resume := aguiResume("thread-a", "run-2", intr["id"].(string), "resolved", map[string]any{"approved": true})
		done := aguiPost(t, p, aguiChatToken, resume, nil)
		require.Equal(t, http.StatusOK, done.Status, done.Body)
		assert.NotContains(t, done.types(), "TOOL_CALL_START", "a resumed call is not announced again")
		assert.Contains(t, aguiResultContent(t, done, "tc-pin"), toPin)
		assert.True(t, hasPin(node, toPin))
		assert.Equal(t, http.StatusUnprocessableEntity, aguiPost(t, p, aguiChatToken, resume, nil).Status, "the interrupt is spent")

		// Thread B: the person says no.
		require.Equal(t, 0, node.RunIPFS("pin", "rm", toPin).ExitCode())
		in2 := aguiInput("thread-b", "run-1", "pin it again")
		second := aguiPost(t, p, aguiChatToken, in2, nil)
		intr2 := aguiInterrupt(t, second)
		denied := aguiPost(t, p, aguiChatToken, aguiResume("thread-b", "run-2", intr2["id"].(string), "resolved", map[string]any{"approved": false}), nil)
		require.Equal(t, http.StatusOK, denied.Status, denied.Body)
		assert.Contains(t, aguiResultContent(t, denied, "tc-pin2"), "confirmation_declined")
		assert.False(t, hasPin(node, toPin), "a denied write never reaches the node")
	})

	t.Run("a destructive call is never approved by the chat", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		pinned := node.IPFSAddStr("keep me", "--pin=true")
		require.True(t, hasPin(node, pinned))
		model := newCLIModel(t,
			cliCallStep("tc-rm", "pin_rm", map[string]any{"path": "/ipfs/" + pinned}),
			cliSayStep("I could not remove it."))
		p := startMCPHTTPArgs(t, node, aguiEnvVars(), "--mcp-config", aguiConfig(t, model.baseURL(), true))

		// With the system prompt off (IPFS_MCP_CONFIRM=none) no native prompt
		// exists, so the call is refused and the chat is never offered an approve
		// control.
		r := aguiPost(t, p, aguiChatToken, aguiInput("thread-rm", "run-1", "unpin it"), nil)
		require.Equal(t, http.StatusOK, r.Status, r.Body)
		assert.Contains(t, aguiResultContent(t, r, "tc-rm"), "confirmation_unavailable")
		assert.True(t, hasPin(node, pinned), "the pin remains")
		last := r.Events[len(r.Events)-1]
		assert.Equal(t, map[string]any{"type": "success"}, last["outcome"], "no interrupt: nothing could approve it")
	})

	t.Run("G9-4 a token works on its own path only", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		model := newCLIModel(t, cliSayStep("hello"))
		p := startMCPHTTPArgs(t, node, aguiEnvVars(), "--mcp-config", aguiConfig(t, model.baseURL(), true))
		body := aguiInput("thread-matrix", "run-1", "hi")
		raw, _ := json.Marshal(body)

		matrix := []struct {
			name, token string
			mcp, agui   int
		}{
			{"no token", "", http.StatusUnauthorized, http.StatusUnauthorized},
			{"unknown token", "cli-unknown-token-0123456789", http.StatusUnauthorized, http.StatusUnauthorized},
			{"ui principal", aguiChatToken, http.StatusOK, http.StatusOK},
			{"human-mcp principal", mcpHTTPReaderToken, http.StatusOK, http.StatusForbidden},
			{"a2a principal", mcpHTTPPeerToken, http.StatusForbidden, http.StatusForbidden},
		}
		for _, m := range matrix {
			assert.Equal(t, m.mcp, p.status(mcpHTTPRequest{path: "/mcp", token: m.token, body: mcpHTTPInitialize}), "/mcp with "+m.name)
			assert.Equal(t, m.agui, p.status(mcpHTTPRequest{path: "/agui", token: m.token, body: string(raw)}), "/agui with "+m.name)
		}
		assert.Equal(t, http.StatusNotFound, p.status(mcpHTTPRequest{path: "/a2a", token: mcpHTTPPeerToken, body: "{}"}), "a2a is not mounted by this change")

		// The chat token, on /mcp, cannot reach tools outside its groups.
		cs := p.mustClient(aguiChatToken, nil)
		res := callMCP(t, cs, "pin_add", map[string]any{"path": "/ipfs/bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku"})
		assert.Equal(t, "confirmation_unavailable", mcpToolErrorCode(t, res), "a write on /mcp still needs a confirmation that nobody gave")
		assert.False(t, hasPin(node, "bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku"))
		assert.Equal(t, 1, model.hits(), "only the chat principal's /agui request reached the model")
	})

	t.Run("origins: only agent.origin_allowlist, with exactly the contract's CORS headers", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		model := newCLIModel(t, cliSayStep("fine"))
		p := startMCPHTTPArgs(t, node, aguiEnvVars(), "--mcp-config", aguiConfig(t, model.baseURL(), true))
		body := aguiInput("thread-cors", "run-1", "hi")

		pre := aguiPost(t, p, "", "", map[string]string{"method": http.MethodOptions, "Origin": aguiOriginWeb, "Access-Control-Request-Method": "POST"})
		assert.Equal(t, http.StatusNoContent, pre.Status)
		assert.Equal(t, aguiOriginWeb, pre.Header.Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "POST, OPTIONS", pre.Header.Get("Access-Control-Allow-Methods"))
		assert.Equal(t, "authorization, content-type", pre.Header.Get("Access-Control-Allow-Headers"))
		assert.Contains(t, pre.Header.Values("Vary"), "Origin")
		assert.Empty(t, pre.Header.Get("Access-Control-Allow-Credentials"))

		for _, origin := range []string{"https://evil.example", "http://app.example", "null"} {
			r := aguiPost(t, p, aguiChatToken, body, map[string]string{"Origin": origin})
			assert.Equal(t, http.StatusForbidden, r.Status, origin)
			assert.Empty(t, r.Header.Get("Access-Control-Allow-Origin"), origin)
		}
		assert.Zero(t, model.hits(), "a foreign origin starts no run")

		ok := aguiPost(t, p, aguiChatToken, body, map[string]string{"Origin": aguiOriginWeb})
		assert.Equal(t, http.StatusOK, ok.Status)
		assert.Equal(t, aguiOriginWeb, ok.Header.Get("Access-Control-Allow-Origin"))
		assert.Equal(t, 1, model.hits())
	})
}

func findAGUI(evs []aguiEvent, typ string) []aguiEvent {
	var out []aguiEvent
	for _, e := range evs {
		if e["type"] == typ {
			out = append(out, e)
		}
	}
	return out
}

// TestMCPAGUIOllama drives /agui with a private Ollama (a free loopback port,
// never the default one): a read-only question answered through a real tool
// call against the real node. A small local model may fumble a tool call, so
// a few attempts are made and the test passes when one round trip completes.
func TestMCPAGUIOllama(t *testing.T) {
	baseURL, model := startOllama(t)
	node := agentLLMNode(t)
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	raw, err := json.Marshal(map[string]any{
		"llm":   map[string]any{"base_url": baseURL, "model": model, "timeout": "8m"},
		"agent": map[string]any{"agui": true, "loop": map[string]any{"max_steps": 4, "wall_clock": "10m"}},
		"principals": []any{map[string]any{"name": "chat", "kind": "ui", "credential": map[string]any{"env": "CLI_AGUI_CHAT_TOKEN"},
			"egress_ack": []string{"local"}}},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfg, raw, 0o600))
	p := startMCPHTTPArgs(t, node, aguiEnvVars(), "--mcp-config", cfg)

	const attempts = 3
	var why []string
	for i := 1; i <= attempts; i++ {
		in := aguiInput(fmt.Sprintf("thread-ollama-%d", i), "run-1", "Call the ipfs_node_info tool, then tell me this node's peer ID.")
		ctx, cancel := context.WithTimeout(t.Context(), 9*time.Minute)
		r := readAGUI(t, aguiRequest(ctx, t, p, aguiChatToken, in, nil))
		cancel()
		validateAGUIFixtureSchema(t, in, r.Events)
		ran := false
		for _, e := range r.Events {
			if e["type"] == "TOOL_CALL_RESULT" && strings.Contains(fmt.Sprint(e["content"]), node.PeerID().String()) {
				ran = true
			}
		}
		last := r.Events[len(r.Events)-1]
		t.Logf("attempt %d/%d with %s: %v", i, attempts, model, r.types())
		if ran && last["type"] == "RUN_FINISHED" && len(findAGUI(r.Events, "TEXT_MESSAGE_CONTENT")) > 0 {
			return
		}
		why = append(why, fmt.Sprintf("attempt %d: tool ran against the node=%v, last event %v", i, ran, last["type"]))
	}
	require.Empty(t, why, "no tool-call round trip over /agui completed with %s", model)
}
