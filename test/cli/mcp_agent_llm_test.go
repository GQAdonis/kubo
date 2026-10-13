//go:build !noagent

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mcpserver "github.com/ipfs/kubo/mcp/server"
	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Falsifiers G10-1 to G10-4 of mcp-llm-proxy-loop. The loop is driven in this
// process against a real Kubo daemon (isolated repo, harness ports) by the
// package API the AG-UI and A2A endpoints will use; the unconfigured cases run
// the built binary. Every listener binds an OS-chosen loopback port; no test
// uses a port a local service owns by default.

// cliModel is a scripted OpenAI-compatible server for the cases that must be
// deterministic. Each request consumes one step, a function from the decoded
// request to the SSE data payloads of the reply.
type cliModel struct {
	srv *httptest.Server

	mu     sync.Mutex
	steps  []func() []string
	bodies []string
}

func newCLIModel(t *testing.T, steps ...func() []string) *cliModel {
	t.Helper()
	m := &cliModel{steps: steps}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		m.mu.Lock()
		m.bodies = append(m.bodies, string(body))
		var step func() []string
		if len(m.steps) > 0 {
			step, m.steps = m.steps[0], m.steps[1:]
		}
		m.mu.Unlock()
		if step == nil {
			http.Error(w, `{"error":{"message":"no scripted step"}}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, d := range step() {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", d)
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *cliModel) baseURL() string { return m.srv.URL + "/v1" }
func (m *cliModel) host() string    { return strings.TrimPrefix(m.srv.URL, "http://") }

func (m *cliModel) hits() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.bodies)
}

func (m *cliModel) all() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return strings.Join(m.bodies, "\n")
}

func cliJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func cliCallStep(id, name string, args any) func() []string {
	return func() []string {
		raw, _ := json.Marshal(args)
		half := len(raw) / 2
		tc := func(fn map[string]any, withID bool) string {
			call := map[string]any{"index": 0, "function": fn}
			if withID {
				call["id"], call["type"] = id, "function"
			}
			return cliJSON(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{call}}}}})
		}
		return []string{
			tc(map[string]any{"name": name, "arguments": ""}, true),
			tc(map[string]any{"arguments": string(raw[:half])}, false),
			tc(map[string]any{"arguments": string(raw[half:])}, false),
			cliJSON(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"}}}),
		}
	}
}

func cliSayStep(text string) func() []string {
	return func() []string {
		return []string{
			cliJSON(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": text}}}}),
			cliJSON(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}}),
		}
	}
}

// agentLLMNode starts an isolated daemon with the test profile.
func agentLLMNode(t *testing.T) *harness.Node {
	t.Helper()
	node := harness.NewT(t).NewNode().Init("--profile=test").StartDaemon()
	t.Cleanup(func() { node.StopDaemon() })
	return node
}

// agentLLMServer builds an in-process ipfs mcp server for the node, with the
// agent attached when set is not nil. It reaches the node through client/rpc
// only, exactly as the binary does.
func agentLLMServer(t *testing.T, node *harness.Node, set *mcpserver.AgentSettings, tools []string, resolvers ...mcpserver.Resolver) *mcpserver.Server {
	t.Helper()
	daemon, err := mcpserver.Discover(mcpserver.DiscoverOptions{API: node.APIAddr().String()})
	require.NoError(t, err)
	srv, err := mcpserver.New(t.Context(), daemon, mcpserver.Options{Tools: tools, Resolvers: resolvers, Version: "test"})
	require.NoError(t, err)
	require.NoError(t, srv.EnableAgent(set))
	return srv
}

// chatPrincipal is a configured chat (ui) principal.
func chatPrincipal(ack []string, writeGroups ...string) mcpserver.Identity {
	id := mcpserver.NewIdentity(mcpserver.Principal{ID: "chat", Kind: mcpserver.KindUI}, []string{mcpserver.AllGroups}, writeGroups, nil)
	id.Configured = true
	id.EgressAck = ack
	return id
}

// ---- G10-3: no model call, no outbound connection without configuration ----

// tcpConns lists the TCP connections of a process as "local->remote" (or just
// the local address for a listener), using lsof. ok is false when lsof is not
// available.
func tcpConns(t *testing.T, pid int) (conns []string, ok bool) {
	t.Helper()
	lsof, err := exec.LookPath("lsof")
	if err != nil {
		return nil, false
	}
	out, err := exec.Command(lsof, "-nP", "-a", "-p", strconv.Itoa(pid), "-iTCP", "-F", "n").Output()
	if err != nil && len(out) == 0 {
		return nil, true // lsof exits 1 when the process has no sockets
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n") {
			conns = append(conns, strings.TrimPrefix(line, "n"))
		}
	}
	return conns, true
}

// assertOnlyDaemonTraffic fails if the process holds a connection that is
// neither to the daemon's RPC API nor accepted on its own listener.
func assertOnlyDaemonTraffic(t *testing.T, p *mcpHTTPProc, node *harness.Node) {
	t.Helper()
	conns, ok := tcpConns(t, p.cmd.Process.Pid)
	if !ok {
		t.Log("lsof is not installed: the outbound-connection check is skipped; the model server hit count still holds")
		return
	}
	apiHostPort := strings.TrimPrefix(node.APIURL(), "http://")
	listen := strings.TrimPrefix(p.base, "http://")
	t.Logf("daemon API %s, ipfs mcp listener %s, TCP sockets of the ipfs mcp process: %v", apiHostPort, listen, conns)
	for _, c := range conns {
		local, remote, connected := strings.Cut(c, "->")
		if !connected {
			assert.Equal(t, listen, local, "the only listener is the one that was asked for: %s", c)
			continue
		}
		assert.True(t, remote == apiHostPort || local == listen,
			"outbound connection to something other than the daemon API %s: %s", apiHostPort, c)
	}
}

func TestMCPAgentLLM(t *testing.T) {
	t.Run("G10-3 unconfigured: nothing mounted, no outbound connection", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		model := newCLIModel(t)
		p := startMCPHTTPArgs(t, node, nil)
		cs := p.mustClient("", nil)
		assert.NotEmpty(t, mcpListedTools(t, cs))
		res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "ipfs_node_info", Arguments: map[string]any{}})
		require.NoError(t, err)
		require.False(t, res.IsError)

		for _, path := range []string{"/agui", "/a2a", "/.well-known/agent-card.json"} {
			assert.Equal(t, http.StatusNotFound, p.status(mcpHTTPRequest{method: http.MethodGet, path: path}), path)
		}
		assert.NotContains(t, p.stderr.String(), "agent loop enabled")
		assertOnlyDaemonTraffic(t, p, node)
		assert.Zero(t, model.hits(), "no model server was ever contacted")
	})

	t.Run("G10-3 configured but idle: still no model call at startup or on tool use", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		model := newCLIModel(t)
		p := startMCPHTTPArgs(t, node, nil, "--llm-base-url", model.baseURL(), "--llm-model", "test-model")
		cs := p.mustClient("", nil)
		assert.NotEmpty(t, mcpListedTools(t, cs))
		_, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "ipfs_node_info", Arguments: map[string]any{}})
		require.NoError(t, err)

		assert.Contains(t, p.stderr.String(), "agent loop enabled: agent backend "+model.host())
		assert.Zero(t, model.hits(), "configuring a backend does not make a model call; only a run does")
		assertOnlyDaemonTraffic(t, p, node)
	})

	t.Run("a literal key is refused and never echoed", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		const literal = "sk-live-0123456789abcdefSECRET"
		for name, args := range map[string][]string{
			"env flag":  {"--llm-api-key-env", literal},
			"file flag": {"--llm-api-key-file", literal},
		} {
			res := node.RunIPFS(append([]string{"mcp", "--http=127.0.0.1:0", "--llm-base-url", "http://127.0.0.1:1/v1", "--llm-model", "m"}, args...)...)
			assert.NotEqual(t, 0, res.ExitCode(), name)
			assert.NotContains(t, res.Stderr.String()+res.Stdout.String(), literal, name)
			assert.Contains(t, res.Stderr.String(), "--llm-api-key", name)
		}
		cfg := t.TempDir() + "/mcp.json"
		require.NoError(t, os.WriteFile(cfg, []byte(`{"llm":{"base_url":"http://127.0.0.1:1/v1","model":"m","api_key":"`+literal+`"}}`), 0o600))
		res := node.RunIPFS("mcp", "--http=127.0.0.1:0", "--mcp-config", cfg)
		assert.NotEqual(t, 0, res.ExitCode())
		assert.Contains(t, res.Stderr.String(), "api_key")
		assert.NotContains(t, res.Stderr.String(), literal)
	})

	t.Run("half-configured backend is an error, not a silent default", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		res := node.RunIPFS("mcp", "--http=127.0.0.1:0", "--llm-model", "m")
		assert.NotEqual(t, 0, res.ExitCode())
		assert.Contains(t, res.Stderr.String(), "llm.base_url")
	})

	// ---- G10-4: egress_ack is enforced for every contentEgress result ----

	t.Run("G10-4 contentEgress result is withheld from an unacknowledged model host", func(t *testing.T) {
		t.Parallel()
		const canary = "G10-4-CANARY-CONTENT-5d1e"
		node := agentLLMNode(t)
		c := node.IPFSAddStr(canary, "--pin=false")
		model := newCLIModel(t,
			cliCallStep("cat1", "ipfs_cat", map[string]any{"path": "/ipfs/" + c}),
			cliSayStep("I may not read that file."))
		srv := agentLLMServer(t, node, &mcpserver.AgentSettings{BaseURL: model.baseURL(), Model: "m"}, []string{"node", "content"})

		res, err := srv.Agent().Run(t.Context(), chatPrincipal(nil), mcpserver.RunInput{Message: "read " + c}, nil)
		require.NoError(t, err)
		assert.Equal(t, mcpserver.OutcomeSuccess, res.Outcome)
		assert.Equal(t, 2, model.hits())
		assert.NotContains(t, model.all(), canary, "the bytes of the file never reached the model server")
		assert.Contains(t, model.all(), "Content not sent: "+model.host()+" is not in egress_ack.")
	})

	t.Run("G10-4 the same result flows once the host is acknowledged", func(t *testing.T) {
		t.Parallel()
		const canary = "G10-4-CANARY-CONTENT-77aa"
		node := agentLLMNode(t)
		c := node.IPFSAddStr(canary, "--pin=false")
		model := newCLIModel(t,
			cliCallStep("cat1", "ipfs_cat", map[string]any{"path": "/ipfs/" + c}),
			cliSayStep("It says what it says."))
		srv := agentLLMServer(t, node, &mcpserver.AgentSettings{BaseURL: model.baseURL(), Model: "m"}, []string{"node", "content"})

		res, err := srv.Agent().Run(t.Context(), chatPrincipal([]string{model.host()}), mcpserver.RunInput{Message: "read " + c}, nil)
		require.NoError(t, err)
		assert.Equal(t, mcpserver.OutcomeSuccess, res.Outcome)
		assert.Contains(t, model.all(), canary)
		// JSON escapes the angle brackets of the frame.
		assert.Contains(t, model.all(), `untrusted-content tool=`, "network content is framed as data")
	})

	// ---- confirmation against a real daemon ----

	t.Run("a write waits for a person: denial and approval against the real node", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		toPin := node.IPFSAddStr("pin me via the agent loop", "--pin=false")
		pinned := func() bool {
			return strings.Contains(node.RunIPFS("pin", "ls", "--type=recursive").Stdout.String(), toPin)
		}
		require.False(t, pinned())

		model := newCLIModel(t,
			cliCallStep("pin1", "pin_add", map[string]any{"path": "/ipfs/" + toPin}),
			cliSayStep("Not pinned."),
			cliCallStep("pin2", "pin_add", map[string]any{"path": "/ipfs/" + toPin}),
			cliSayStep("Pinned."))
		rs := &cliResolver{}
		srv := agentLLMServer(t, node, &mcpserver.AgentSettings{BaseURL: model.baseURL(), Model: "m"}, []string{"node", "pin"}, rs)
		id := chatPrincipal([]string{"local"}, "pin")

		// First run: the person says no.
		res, err := srv.Agent().Run(t.Context(), id, mcpserver.RunInput{Message: "pin " + toPin}, nil)
		require.NoError(t, err)
		require.Equal(t, mcpserver.OutcomeInterrupt, res.Outcome)
		assert.False(t, pinned(), "nothing is pinned while the call waits")
		res, err = srv.Agent().Run(t.Context(), id, mcpserver.RunInput{SessionID: res.SessionID, Resume: []mcpserver.ResumeEntry{
			{InterruptID: res.Interrupts[0].ID, Status: mcpserver.ResumeResolved, Approved: false}}}, nil)
		require.NoError(t, err)
		assert.Equal(t, mcpserver.OutcomeSuccess, res.Outcome)
		assert.False(t, pinned(), "a denied call never reaches the node")

		// Second run: a human-presence resolver approves.
		res, err = srv.Agent().Run(t.Context(), id, mcpserver.RunInput{SessionID: res.SessionID, Message: "try again"}, nil)
		require.NoError(t, err)
		require.Equal(t, mcpserver.OutcomeInterrupt, res.Outcome)
		assert.False(t, pinned())
		require.NoError(t, rs.approve(res.Interrupts[0].ConfirmationID))
		res, err = srv.Agent().Run(t.Context(), id, mcpserver.RunInput{SessionID: res.SessionID, Resume: []mcpserver.ResumeEntry{
			{InterruptID: res.Interrupts[0].ID, Status: mcpserver.ResumeResolved, Approved: true}}}, nil)
		require.NoError(t, err)
		assert.Equal(t, mcpserver.OutcomeSuccess, res.Outcome)
		assert.True(t, pinned(), "approved by a person, then run exactly once")
	})
}

// cliResolver is a human-presence resolver the test answers by hand.
type cliResolver struct {
	mu      sync.Mutex
	resolve map[string]mcpserver.ResolveFunc
}

func (r *cliResolver) Name() string { return mcpserver.ResolverDesktop }

func (r *cliResolver) Request(_ context.Context, p mcpserver.Pending, resolve mcpserver.ResolveFunc) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.resolve == nil {
		r.resolve = map[string]mcpserver.ResolveFunc{}
	}
	r.resolve[p.ID] = resolve
	return nil
}

func (r *cliResolver) approve(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.resolve[id]
	if !ok {
		return fmt.Errorf("no request for confirmation %s", id)
	}
	return f(true)
}

// ---- G10-1 and G10-2: a tool-call round trip with a local Ollama ----

// freeLoopbackPort asks the OS for a free port. The caller binds it right
// away; the window is small and a collision fails the test loudly.
func freeLoopbackPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	_, port, err := net.SplitHostPort(l.Addr().String())
	require.NoError(t, err)
	return port
}

// recordingTransport keeps every exchange with the model server, for the
// recorded-run fixture. Requests carry no credential in this test.
type recordingTransport struct {
	mu        sync.Mutex
	exchanges []recordedExchange
}

type recordedExchange struct {
	Request  json.RawMessage `json:"request"`
	Status   int             `json:"status"`
	Response string          `json:"response_sse"`
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var reqBody []byte
	if req.Body != nil {
		reqBody, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(reqBody))
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	r.mu.Lock()
	r.exchanges = append(r.exchanges, recordedExchange{Request: reqBody, Status: resp.StatusCode, Response: string(body)})
	r.mu.Unlock()
	return resp, nil
}

// ollamaCandidates are tried in order; the first one the private Ollama has
// pulled is used. IPFS_MCP_OLLAMA_MODEL overrides the list.
var ollamaCandidates = []string{"qwen3:8b", "llama3.2:1b"}

// startOllama starts a private `ollama serve` on a free loopback port and
// returns its OpenAI-compatible base URL and a tool-capable model that is
// already pulled. It never uses the default Ollama port and never talks to an
// Ollama the user may be running.
func startOllama(t *testing.T) (baseURL, model string) {
	t.Helper()
	bin, err := exec.LookPath("ollama")
	if err != nil {
		t.Skip("ollama is not installed: the G10-1/G10-2 recorded run is unmet")
	}
	port := freeLoopbackPort(t)
	require.NotEqual(t, "11434", port)
	cmd := exec.Command(bin, "serve")
	cmd.Env = append(os.Environ(), "OLLAMA_HOST=127.0.0.1:"+port)
	stderr := &syncBuffer{}
	cmd.Stderr, cmd.Stdout = stderr, stderr
	// serve starts a runner process per model; killing only serve would leave
	// the runner behind. The whole group is ours, so the whole group goes.
	ownProcessGroup(cmd)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		killProcessGroup(cmd)
		_ = cmd.Wait()
	})

	api := "http://127.0.0.1:" + port
	deadline := time.Now().Add(60 * time.Second)
	for {
		resp, err := http.Get(api + "/api/version")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("private ollama did not start on %s: %s", api, stderr.String())
		}
		time.Sleep(200 * time.Millisecond)
	}

	resp, err := http.Get(api + "/api/tags")
	require.NoError(t, err)
	defer resp.Body.Close()
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tags))
	have := map[string]bool{}
	for _, m := range tags.Models {
		have[m.Name] = true
	}
	want := ollamaCandidates
	if v := os.Getenv("IPFS_MCP_OLLAMA_MODEL"); v != "" {
		want = []string{v}
	}
	for _, m := range want {
		if have[m] {
			return api + "/v1", m
		}
	}
	t.Skipf("no tool-capable model from %v is pulled: the G10-1/G10-2 recorded run is unmet", want)
	return "", ""
}

func TestMCPAgentLLMOllama(t *testing.T) {
	baseURL, model := startOllama(t)
	node := agentLLMNode(t)
	rec := &recordingTransport{}
	srv := agentLLMServer(t, node, &mcpserver.AgentSettings{
		BaseURL:    baseURL,
		Model:      model,
		Timeout:    8 * time.Minute,
		Limits:     mcpserver.LoopLimits{MaxSteps: 4, MaxTokens: 16384, WallClock: 10 * time.Minute},
		HTTPClient: &http.Client{Transport: rec},
	}, []string{"node"})

	// A small local model may fumble a tool call (arguments that are not the
	// schema, or the call written out as text). That is G10-2 evidence, not a
	// loop failure, so each attempt is judged on its own and the test passes
	// when one round trip completes. Every attempt is logged.
	const attempts = 3
	var reasons []string
	for i := 1; i <= attempts; i++ {
		rec.mu.Lock()
		rec.exchanges = nil
		rec.mu.Unlock()
		ok, why := ollamaAttempt(t, srv, node.PeerID().String(), model)
		t.Logf("attempt %d/%d with %s: %s", i, attempts, model, why)
		if ok {
			reasons = nil
			break
		}
		reasons = append(reasons, why)
	}
	require.Empty(t, reasons, "no tool-call round trip completed in %d attempts with %s (G10-2: this model does not handle tool calls well enough)", attempts, model)

	if path := os.Getenv("IPFS_MCP_RECORD_OLLAMA"); path != "" {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		out, err := json.MarshalIndent(map[string]any{
			"description": "Recorded exchange between the ipfs mcp agent loop and a local Ollama (G10-1/G10-2). The model asked for ipfs_node_info and answered after the result.",
			"model":       model,
			"exchanges":   rec.exchanges,
		}, "", " ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, append(out, '\n'), 0o644))
		t.Logf("recorded %d exchanges to %s", len(rec.exchanges), path)
	}
}

// ollamaAttempt runs one conversation on a fresh session and reports whether
// the tool-call round trip completed: the model asked for ipfs_node_info, the
// loop ran it against the real node and the model answered afterwards.
func ollamaAttempt(t *testing.T, srv *mcpserver.Server, peerID, model string) (bool, string) {
	t.Helper()
	var events []mcpserver.Event
	res, err := srv.Agent().Run(t.Context(), chatPrincipal([]string{"local"}), mcpserver.RunInput{
		Message: "Call the ipfs_node_info tool, then tell me this node's peer ID.",
	}, func(ev mcpserver.Event) { events = append(events, ev) })
	if err != nil {
		return false, "run failed: " + err.Error()
	}
	summary := fmt.Sprintf("outcome %s, %d steps, %d tokens, final: %q", res.Outcome, res.Steps, res.Tokens, res.Final)
	var ran bool
	for _, ev := range events {
		if ev.Type == mcpserver.EventToolResult && ev.Tool == "ipfs_node_info" && !ev.IsError && strings.Contains(ev.Content, peerID) {
			ran = true
		}
	}
	switch {
	case !ran:
		return false, "the tool did not run successfully against the real node; " + summary
	case res.Outcome != mcpserver.OutcomeSuccess || res.Final == "" || res.Steps < 2:
		return false, "the model did not answer after the tool result; " + summary
	case !strings.Contains(res.Final, peerID):
		return true, "round trip complete; the answer does not quote the peer ID verbatim; " + summary
	}
	return true, "round trip complete; " + summary
}

// TestMCPAgentLLMUAR documents the UAR leg of G10-1: it needs a published UAR
// release that serves /v1/chat/completions with client-supplied tools, and none
// is pinned yet (ADR-0007).
func TestMCPAgentLLMUAR(t *testing.T) {
	t.Skip("no published UAR release is pinned (ADR-0007); G10-1 for UAR is unmet and UAR stays an unsupported backend until one is")
}
