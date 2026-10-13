//go:build !noagent

package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "llm", name))
	require.NoError(t, err)
	return b
}

func TestAssembleSplitToolCallDeltas(t *testing.T) {
	var streamed strings.Builder
	res, err := assembleStream(strings.NewReader(string(fixture(t, "stream-split-toolcalls.sse"))), func(s string) { streamed.WriteString(s) })
	require.NoError(t, err)

	assert.Equal(t, "Let me check.", res.Content)
	assert.Equal(t, "Let me check.", streamed.String(), "text deltas are delivered as they arrive")
	require.Len(t, res.ToolCalls, 2)
	assert.Equal(t, ChatToolCall{ID: "call_a", Type: "function", Function: ChatFunctionCall{Name: "pin_ls", Arguments: `{"name":"x"}`}}, res.ToolCalls[0],
		"arguments split across chunks, with another call in between, are joined by index")
	assert.Equal(t, ChatToolCall{ID: "call_b", Type: "function", Function: ChatFunctionCall{Name: "ipfs_node_info", Arguments: `{}`}}, res.ToolCalls[1])
	assert.Equal(t, "tool_calls", res.FinishReason)
	assert.True(t, res.UsageReported)
	assert.Equal(t, 15, res.Usage.TotalTokens)
	assert.True(t, json.Valid([]byte(res.ToolCalls[0].Function.Arguments)))
}

func TestAssembleToolCallsWithoutIndex(t *testing.T) {
	res, err := assembleStream(strings.NewReader(string(fixture(t, "stream-noindex.sse"))), nil)
	require.NoError(t, err)
	require.Len(t, res.ToolCalls, 2)
	assert.Equal(t, "call_1", res.ToolCalls[0].ID)
	assert.Equal(t, "ipfs_node_info", res.ToolCalls[0].Function.Name, "a name split over chunks is joined")
	assert.Equal(t, "{}", res.ToolCalls[0].Function.Arguments, "fragments without an index continue the last call")
	assert.Equal(t, "call_2", res.ToolCalls[1].ID, "an id starts a new call")
	assert.Equal(t, "key_list", res.ToolCalls[1].Function.Name)
	assert.Equal(t, "{}", res.ToolCalls[1].Function.Arguments, "no arguments become an empty object")
	assert.False(t, res.UsageReported)
}

func TestAssembleKeepsLastEventWithoutBlankLine(t *testing.T) {
	res, err := assembleStream(strings.NewReader(string(fixture(t, "stream-no-terminator.sse"))), nil)
	require.NoError(t, err)
	assert.Equal(t, "no trailing blank line", res.Content)
}

func TestAssembleStreamErrorEvent(t *testing.T) {
	_, err := assembleStream(strings.NewReader(string(fixture(t, "stream-error.sse"))), nil)
	var le *LLMError
	require.ErrorAs(t, err, &le)
	assert.Contains(t, le.Message, "model overloaded")
}

func TestAssembleRejectsMalformedAndOversizedEvents(t *testing.T) {
	_, err := assembleStream(strings.NewReader("data: {not json}\n\n"), nil)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "not json}", "a malformed payload is not echoed back")

	_, err = assembleStream(strings.NewReader("data: "+strings.Repeat("x", maxSSELine+10)+"\n\n"), nil)
	require.Error(t, err, "one line beyond the cap is refused")
}

// TestReplayRecordedOllamaRun replays the exchange recorded by
// TestMCPAgentLLMOllama (test/cli) against a real local Ollama: the model asks
// for the same tool twice, whole calls in separate chunks with an index each,
// and answers after the results.
func TestReplayRecordedOllamaRuns(t *testing.T) {
	for _, tc := range []struct {
		file, model string
		calls       int
		thinking    bool
	}{
		// llama3.2:1b asked for the tool twice, as two whole calls with an index each.
		{"ollama-llama3.2-1b-tool-roundtrip.json", "llama3.2:1b", 2, false},
		// qwen3:8b thinks first: every chunk of the reasoning arrives in a
		// "reasoning" delta that must not end up in the answer.
		{"ollama-qwen3-8b-tool-roundtrip.json", "qwen3:8b", 1, true},
	} {
		t.Run(tc.model, func(t *testing.T) {
			var rec struct {
				Model     string `json:"model"`
				Exchanges []struct {
					Request  chatRequest `json:"request"`
					Status   int         `json:"status"`
					Response string      `json:"response_sse"`
				} `json:"exchanges"`
			}
			require.NoError(t, json.Unmarshal(fixture(t, tc.file), &rec))
			require.Len(t, rec.Exchanges, 2)
			assert.Equal(t, tc.model, rec.Model)
			assert.True(t, rec.Exchanges[0].Request.Stream)
			if tc.thinking {
				assert.Contains(t, rec.Exchanges[0].Response, `"reasoning"`, "the recording has reasoning deltas")
			}

			res, err := assembleStream(strings.NewReader(rec.Exchanges[0].Response), nil)
			require.NoError(t, err)
			require.Len(t, res.ToolCalls, tc.calls)
			ids := map[string]bool{}
			for _, call := range res.ToolCalls {
				assert.Equal(t, "ipfs_node_info", call.Function.Name)
				assert.Equal(t, "{}", call.Function.Arguments)
				ids[call.ID] = true
			}
			assert.Len(t, ids, tc.calls, "every call keeps its own id")
			assert.Equal(t, "tool_calls", res.FinishReason)
			assert.True(t, res.UsageReported)
			assert.Empty(t, res.Content, "reasoning is not content")

			var said strings.Builder
			second, err := assembleStream(strings.NewReader(rec.Exchanges[1].Response), func(s string) { said.WriteString(s) })
			require.NoError(t, err)
			assert.Empty(t, second.ToolCalls)
			assert.Equal(t, second.Content, said.String())
			assert.Contains(t, second.Content, "peer ID")
		})
	}
}

func TestDecodeNonStreamingCompletion(t *testing.T) {
	res, err := decodeCompletion(strings.NewReader(string(fixture(t, "completion.json"))), nil)
	require.NoError(t, err)
	require.Len(t, res.ToolCalls, 1)
	assert.Equal(t, "call_0", res.ToolCalls[0].ID, "a missing id is generated")
	assert.Equal(t, "{}", res.ToolCalls[0].Function.Arguments)
	assert.True(t, res.UsageReported)
	assert.Equal(t, 10, res.Usage.TotalTokens, "total is derived when the backend omits it")
}

func newClientFor(t *testing.T, h http.HandlerFunc, mod func(*AgentSettings)) (*LLMClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	set := AgentSettings{BaseURL: srv.URL + "/v1", Model: "m1", Timeout: 5 * time.Second}
	if mod != nil {
		mod(&set)
	}
	c, err := NewLLMClient(set)
	require.NoError(t, err)
	return c, srv
}

func TestChatRequestShape(t *testing.T) {
	var gotPath, gotAuth, gotCT string
	var body chatRequest
	c, _ := newClientFor(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotCT = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(fixture(t, "stream-split-toolcalls.sse"))
	}, nil)

	tools := []ChatTool{{Type: "function", Function: ChatToolFunction{Name: "pin_ls", Parameters: json.RawMessage(`{"type":"object"}`)}}}
	res, err := c.Chat(t.Context(), []ChatMessage{{Role: "user", Content: "hi"}}, tools, nil)
	require.NoError(t, err)
	assert.Len(t, res.ToolCalls, 2)
	assert.Equal(t, "/v1/chat/completions", gotPath)
	assert.Equal(t, "application/json", gotCT)
	assert.Empty(t, gotAuth, "no key configured, no Authorization header")
	assert.Equal(t, "m1", body.Model)
	assert.True(t, body.Stream)
	assert.Equal(t, true, body.StreamOptions["include_usage"])
	require.Len(t, body.Tools, 1)
	assert.Equal(t, "pin_ls", body.Tools[0].Function.Name)
}

func TestChatHandlesBackendsThatIgnoreStreaming(t *testing.T) {
	c, _ := newClientFor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(fixture(t, "completion.json"))
	}, nil)
	res, err := c.Chat(t.Context(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, res.ToolCalls, 1)
}

func TestChatBackendErrorKeepsOnlyAShortMessage(t *testing.T) {
	c, _ := newClientFor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"`+strings.Repeat("a", 5000)+`"}}`)
	}, nil)
	_, err := c.Chat(t.Context(), []ChatMessage{{Role: "user", Content: "PROMPT-CANARY"}}, nil, nil)
	var le *LLMError
	require.ErrorAs(t, err, &le)
	assert.Equal(t, 401, le.Status)
	assert.LessOrEqual(t, len(le.Message), maxErrorSnippet)
	assert.NotContains(t, err.Error(), "PROMPT-CANARY")
}

func TestChatDoesNotFollowRedirectsWithTheKey(t *testing.T) {
	t.Setenv("IPFS_MCP_TEST_KEY", "redirect-secret")
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the redirect target was contacted with Authorization %q", r.Header.Get("Authorization"))
	}))
	t.Cleanup(other.Close)
	c, _ := newClientFor(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
	}, func(s *AgentSettings) { s.APIKey = &SecretRef{Env: "IPFS_MCP_TEST_KEY"} })
	_, err := c.Chat(t.Context(), nil, nil, nil)
	var le *LLMError
	require.ErrorAs(t, err, &le)
	assert.Equal(t, http.StatusTemporaryRedirect, le.Status)
}

func TestChatTimeoutBoundsTheWholeStream(t *testing.T) {
	c, _ := newClientFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}, func(s *AgentSettings) { s.Timeout = 100 * time.Millisecond })
	start := time.Now()
	_, err := c.Chat(t.Context(), nil, nil, nil)
	require.Error(t, err)
	assert.Less(t, time.Since(start), 3*time.Second)
}

func TestNewLLMClientRequiresBackend(t *testing.T) {
	_, err := NewLLMClient(AgentSettings{})
	require.Error(t, err)
}
