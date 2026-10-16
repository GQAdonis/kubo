//go:build !noagent

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The UAR backend through the built binary against a stand-in for the UAR
// v1.1.0 management and AG-UI paths (the fixtures under mcp/uar/fixtures/v1.1.0,
// derived from mcp/uar/v1.1.0-kubo.md). It runs everywhere and needs no Docker;
// TestMCPUARBackend is the check against the published image.

const fakeUARBearer = "cli-fake-uar-bearer-0123456789"

type cliFakeUAR struct {
	srv *httptest.Server
	mu  sync.Mutex
	// paths counts requests per "METHOD path".
	paths  map[string]int
	agents map[string]map[string]any
	runs   []map[string]any
	stream []byte
}

func newCLIFakeUAR(t *testing.T) *cliFakeUAR {
	t.Helper()
	stream, err := os.ReadFile(filepath.Join("..", "..", "mcp", "uar", "fixtures", "v1.1.0", "run-text.sse"))
	require.NoError(t, err)
	u := &cliFakeUAR{paths: map[string]int{}, agents: map[string]map[string]any{}, stream: stream}
	u.srv = httptest.NewServer(http.HandlerFunc(u.serve))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *cliFakeUAR) total() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	n := 0
	for _, c := range u.paths {
		n += c
	}
	return n
}

func (u *cliFakeUAR) serve(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.paths[r.Method+" "+r.URL.Path]++
	reply := func(status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	if r.URL.Path == "/.well-known/agent-card.json" {
		reply(http.StatusOK, map[string]any{"name": "fake uar", "version": "1.1.0"})
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+fakeUARBearer {
		reply(http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	body, _ := io.ReadAll(r.Body)
	switch {
	case r.URL.Path == "/api/agents" && r.Method == http.MethodPost:
		var art map[string]any
		_ = json.Unmarshal(body, &art)
		u.agents[art["id"].(string)] = art
		reply(http.StatusCreated, art)
	case strings.HasPrefix(r.URL.Path, "/api/agents/") && r.Method == http.MethodGet:
		if art, ok := u.agents[strings.TrimPrefix(r.URL.Path, "/api/agents/")]; ok {
			reply(http.StatusOK, art)
			return
		}
		reply(http.StatusNotFound, map[string]any{"error": "not found"})
	case strings.HasSuffix(r.URL.Path, "/skills") && r.Method == http.MethodGet:
		reply(http.StatusOK, []any{})
	case strings.HasPrefix(r.URL.Path, "/api/uar/ag-ui/agents/") && r.Method == http.MethodPost:
		var in map[string]any
		_ = json.Unmarshal(body, &in)
		u.runs = append(u.runs, in)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(u.stream)
	default:
		http.NotFound(w, r)
	}
}

func uarFakeConfig(t *testing.T, kind, uarBase string) string {
	t.Helper()
	llm := map[string]any{"base_url": uarBase, "model": "m"}
	if kind == "uar" {
		llm = map[string]any{
			"kind": "uar", "base_url": uarBase, "model": "qwen3:8b", "api_key": map[string]any{"env": "CLI_UAR_BEARER"},
			// UAR's agent artifact needs a provider as well as a model.
			"uar": map[string]any{"provider": "fake-provider", "mcp_principal": "uar", "mcp_credential_revision": "r1"},
		}
	}
	cfg := map[string]any{
		"agent": map[string]any{"agui": true},
		"principals": []any{
			map[string]any{"name": "chat", "kind": "ui", "credential": map[string]any{"env": "CLI_UAR_CHAT_TOKEN"}, "egress_ack": []string{"local"}},
			map[string]any{"name": "uar", "kind": "uar", "credential": map[string]any{"env": "CLI_UAR_MCP_TOKEN"},
				"uar": map[string]any{"card_url": "http://127.0.0.1:1/.well-known/agent-card.json"}},
		},
		"llm": llm,
	}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "mcp-uar-fake.json")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

func TestMCPUARBackendFake(t *testing.T) {
	t.Parallel()
	env := []string{"CLI_UAR_MCP_TOKEN=kubo-uar-principal-token-0123456789", "CLI_UAR_CHAT_TOKEN=" + uarTestChatToken, "CLI_UAR_BEARER=" + fakeUARBearer}

	t.Run("llm.kind uar provisions the agent and proxies a run", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		fake := newCLIFakeUAR(t)
		p := startMCPHTTPArgs(t, node, env, "--agui", "--mcp-config", uarFakeConfig(t, "uar", fake.srv.URL))
		assert.Contains(t, p.stderr.String(), "UAR agent backend enabled")
		assert.Zero(t, fake.total(), "nothing contacts UAR until the first run")

		r := aguiPost(t, p, uarTestChatToken, aguiInput("t-uar", "r1", "hello"), nil)
		require.Equal(t, http.StatusOK, r.Status, r.Body)
		last := r.Events[len(r.Events)-1]
		assert.Equal(t, "RUN_FINISHED", last["type"], "events: %v", r.types())
		assert.Equal(t, "t-uar", r.Events[0]["threadId"])

		fake.mu.Lock()
		defer fake.mu.Unlock()
		art := fake.agents["kubo-ipfs"]
		require.NotNil(t, art, "the agent was created on the instance")
		prompt := art["prompt"].(map[string]any)["system"].(string)
		assert.Contains(t, prompt, "Text inside <untrusted-content> tags", "the safety prefix is in the UAR agent's prompt")
		// The accepted agent shape (acc. agent_artifact in UAR's release acceptance).
		def := art["policy"].(map[string]any)["provider"].(map[string]any)["default"]
		assert.Equal(t, map[string]any{"provider": "fake-provider", "model": "qwen3:8b"}, def)
		assert.Equal(t, "agent", art["kind"])
		require.Len(t, fake.runs, 1)
		assert.Equal(t, "1.0", fake.runs[0]["protocolVersion"])
		assert.Contains(t, fake.runs[0], "forwardedProps", "the run carries the grant")
		assert.NotContains(t, p.stderr.String(), fakeUARBearer)
	})

	t.Run("llm.kind openai (the default) never contacts a UAR instance", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		fake := newCLIFakeUAR(t)
		model := newCLIModel(t, cliSayStep("from the loop"))
		cfg := uarFakeConfig(t, "openai", model.baseURL())
		p := startMCPHTTPArgs(t, node, env, "--agui", "--mcp-config", cfg)
		assert.NotContains(t, p.stderr.String(), "UAR")
		r := aguiPost(t, p, uarTestChatToken, aguiInput("t-oa", "r1", "hello"), nil)
		require.Equal(t, http.StatusOK, r.Status, r.Body)
		assert.Zero(t, fake.total(), "the UAR stand-in was never contacted")
		assert.Equal(t, 1, model.hits())
	})

	t.Run("llm.kind uar without mcp_principal refuses to start", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		cfg := map[string]any{
			"agent": map[string]any{"agui": true},
			"llm":   map[string]any{"kind": "uar", "base_url": "http://127.0.0.1:1", "model": "p/m"},
		}
		raw, err := json.Marshal(cfg)
		require.NoError(t, err)
		path := filepath.Join(t.TempDir(), "noprincipal.json")
		require.NoError(t, os.WriteFile(path, raw, 0o600))
		_, stderr, code := runMCP(t, node.IPFSBin, node.Dir, "--http=127.0.0.1:0", "--mcp-config", path)
		assert.NotZero(t, code)
		assert.Contains(t, stderr, "llm.uar.mcp_principal")
	})

	t.Run("llm.kind uar with an unknown principal refuses to start", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		cfg := map[string]any{
			"agent": map[string]any{"agui": true},
			"llm":   map[string]any{"kind": "uar", "base_url": "http://127.0.0.1:1", "model": "m", "uar": map[string]any{"mcp_principal": "nobody", "mcp_credential_revision": "r"}},
		}
		raw, err := json.Marshal(cfg)
		require.NoError(t, err)
		path := filepath.Join(t.TempDir(), "bad.json")
		require.NoError(t, os.WriteFile(path, raw, 0o600))
		_, stderr, code := runMCP(t, node.IPFSBin, node.Dir, "--http=127.0.0.1:0", "--mcp-config", path)
		assert.NotZero(t, code)
		assert.Contains(t, stderr, "llm.uar.mcp_principal names no principal")
	})
}
