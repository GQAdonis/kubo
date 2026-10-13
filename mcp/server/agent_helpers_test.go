//go:build !noagent

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/stretchr/testify/require"
)

// modelRequest is what the loop sent to the (fake) model server.
type modelRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Tools    []ChatTool    `json:"tools"`
	Stream   bool          `json:"stream"`
}

// scriptedModel is an OpenAI-compatible server that answers each request from
// the next scripted step and records every request body. A step returns the
// SSE data payloads of its reply.
type scriptedModel struct {
	t   *testing.T
	srv *httptest.Server

	mu     sync.Mutex
	steps  []func(modelRequest) []string
	bodies [][]byte
	reqs   []modelRequest
	auth   []string
	// block, when set, makes every request wait for the client to give up.
	block bool
}

func newScriptedModel(t *testing.T, steps ...func(modelRequest) []string) *scriptedModel {
	t.Helper()
	m := &scriptedModel{t: t, steps: steps}
	m.srv = httptest.NewServer(http.HandlerFunc(m.serve))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *scriptedModel) baseURL() string { return m.srv.URL + "/v1" }

func (m *scriptedModel) host() string { return strings.TrimPrefix(m.srv.URL, "http://") }

func (m *scriptedModel) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req modelRequest
	_ = json.Unmarshal(body, &req)
	m.mu.Lock()
	m.bodies = append(m.bodies, body)
	m.reqs = append(m.reqs, req)
	m.auth = append(m.auth, r.Header.Get("Authorization"))
	block := m.block
	var step func(modelRequest) []string
	if len(m.steps) > 0 {
		step, m.steps = m.steps[0], m.steps[1:]
	}
	m.mu.Unlock()
	if block {
		<-r.Context().Done()
		return
	}
	if step == nil {
		http.Error(w, `{"error":{"message":"no scripted step left"}}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	for _, data := range step(req) {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

func (m *scriptedModel) requests() []modelRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]modelRequest(nil), m.reqs...)
}

func (m *scriptedModel) allBodies() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return string(bytes.Join(m.bodies, []byte("\n")))
}

func (m *scriptedModel) hits() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.bodies)
}

func (m *scriptedModel) setBlock() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.block = true
}

func chunkJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func usageChunk(total int) string {
	return chunkJSON(map[string]any{"choices": []any{}, "usage": map[string]int{"prompt_tokens": total / 2, "completion_tokens": total - total/2, "total_tokens": total}})
}

// sayStep replies with text, split in two deltas.
func sayStep(text string) func(modelRequest) []string {
	return func(modelRequest) []string {
		half := len(text) / 2
		delta := func(s string) string {
			return chunkJSON(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": s}}}})
		}
		return []string{delta(text[:half]), delta(text[half:]),
			chunkJSON(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}}),
			usageChunk(20)}
	}
}

// scriptedCall is one tool call a step asks for. A string Args is sent as is
// (to script malformed arguments).
type scriptedCall struct {
	ID, Name string
	Args     any
}

// callStep replies with tool calls, their arguments split across deltas the way
// real servers stream them. tokens is the usage it reports.
func callStep(tokens int, calls ...scriptedCall) func(modelRequest) []string {
	return func(modelRequest) []string {
		var out []string
		for i, c := range calls {
			var raw []byte
			if s, ok := c.Args.(string); ok {
				raw = []byte(s)
			} else {
				args := c.Args
				if args == nil {
					args = map[string]any{}
				}
				raw, _ = json.Marshal(args)
			}
			half := len(raw) / 2
			tc := func(fn map[string]any, id string) string {
				call := map[string]any{"index": i, "function": fn}
				if id != "" {
					call["id"], call["type"] = id, "function"
				}
				return chunkJSON(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{call}}}}})
			}
			out = append(out,
				tc(map[string]any{"name": c.Name, "arguments": ""}, c.ID),
				tc(map[string]any{"arguments": string(raw[:half])}, ""),
				tc(map[string]any{"arguments": string(raw[half:])}, ""))
		}
		out = append(out,
			chunkJSON(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"}}}),
			usageChunk(tokens))
		return out
	}
}

// agentEnv is a server with an agent talking to a scripted model.
type agentEnv struct {
	t        *testing.T
	model    *scriptedModel
	d        *contentFake
	pins     *pinStore
	s        *Server
	a        *Agent
	clock    *fakeClock
	resolver *fakeResolver
	logs     *bytes.Buffer
}

type agentEnvOpts struct {
	limits    LoopLimits
	resolvers []Resolver
	noResolve bool
	files     map[string][]byte
	key       *SecretRef
}

func newAgentEnv(t *testing.T, opts agentEnvOpts, steps ...func(modelRequest) []string) *agentEnv {
	t.Helper()
	e := &agentEnv{t: t, clock: newFakeClock(), logs: &bytes.Buffer{}}
	e.model = newScriptedModel(t, steps...)
	e.d = catFake(t, opts.files)
	e.pins = installPins(e.d.fakeDaemon, map[string]fakePin{testCID(t, 1): {Type: "recursive"}})
	resolvers := opts.resolvers
	if len(resolvers) == 0 && !opts.noResolve {
		e.resolver = newFakeResolver(ResolverLinuxPolkit)
		resolvers = []Resolver{e.resolver}
	}
	log := slog.New(slog.NewTextHandler(e.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	e.s = mustNew(t, e.d, Options{
		Tools: []string{"node", "content", "pin", "destructive"}, Resolvers: resolvers, Logger: log, now: e.clock.Now,
	})
	require.NoError(t, e.s.EnableAgent(&AgentSettings{
		BaseURL: e.model.baseURL(), Model: "test-model", APIKey: opts.key, Timeout: 5 * time.Second, Limits: opts.limits,
	}))
	e.a = e.s.Agent()
	require.NotNil(t, e.a)
	e.a.now = e.clock.Now
	return e
}

// uiID is a configured chat principal.
func uiID(ack []string, writeGroups ...string) Identity {
	id := NewIdentity(Principal{ID: "chat", Kind: KindUI}, []string{AllGroups}, writeGroups, nil)
	id.Configured = true
	id.EgressAck = ack
	return id
}

func (e *agentEnv) run(id Identity, msg string) (*RunResult, []Event) {
	e.t.Helper()
	var evs []Event
	res, err := e.a.Run(e.t.Context(), id, RunInput{Message: msg}, func(ev Event) { evs = append(evs, ev) })
	require.NoError(e.t, err)
	return res, evs
}

func (e *agentEnv) resume(id Identity, session string, entries ...ResumeEntry) (*RunResult, []Event, error) {
	e.t.Helper()
	var evs []Event
	res, err := e.a.Run(e.t.Context(), id, RunInput{SessionID: session, Resume: entries}, func(ev Event) { evs = append(evs, ev) })
	return res, evs, err
}

// toolNamesOf lists the names of the tools sent in a model request.
func toolNamesOf(r modelRequest) []string {
	names := make([]string, 0, len(r.Tools))
	for _, t := range r.Tools {
		names = append(names, t.Function.Name)
	}
	return names
}

// toolMessageFor returns the content of the tool message answering callID.
func toolMessageFor(t *testing.T, r modelRequest, callID string) string {
	t.Helper()
	for _, m := range r.Messages {
		if m.Role == "tool" && m.ToolCallID == callID {
			return m.Content
		}
	}
	t.Fatalf("no tool message for %s in %+v", callID, r.Messages)
	return ""
}

func catalogTool(t *testing.T, name string) contract.Tool {
	t.Helper()
	cat, err := contract.Load()
	require.NoError(t, err)
	tool, ok := cat.Tool(name)
	require.True(t, ok)
	return tool
}
