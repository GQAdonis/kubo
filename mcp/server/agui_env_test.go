//go:build !noagent

package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Tokens of the AG-UI test principals. No test names a port: the listener
// binds 127.0.0.1:0.
const (
	aguiTokChat    = "agui-chat-token-0123456789"
	aguiTokChat2   = "agui-chat2-token-012345678"
	aguiTokNoAck   = "agui-noack-token-01234567"
	aguiTokHuman   = "agui-human-token-0123456"
	aguiTokPeer    = "agui-peer-token-01234567"
	aguiTokReadUI  = "agui-readonly-token-0123"
	aguiNeedsAckID = "chat-noack"
)

// aguiEnv is a running HTTP listener with /agui over an agent that talks to a
// scripted model, a fake daemon with pin and repo_gc commands, a fake clock and
// a fake presence resolver (the native prompt).
type aguiEnv struct {
	t      *testing.T
	model  *scriptedModel
	d      *contentFake
	pins   *pinStore
	gcRuns *int
	s      *Server
	hs     *HTTPServer
	base   string
	clock  *fakeClock
	// presence stands in for the system prompt (desktop resolver).
	presence *fakeResolver
	logs     *bytes.Buffer
}

type aguiEnvOpts struct {
	limits      LoopLimits
	origins     []string
	listenerOrg []string
	maxBody     int64
	noAGUI      bool
	noPresence  bool
}

func aguiIdentity(id, kind string, write, destructive, ack []string) Identity {
	i := NewIdentity(Principal{ID: id, Kind: kind}, []string{AllGroups}, write, destructive)
	i.Configured = true
	i.EgressAck = ack
	return i
}

func newAGUIEnv(t *testing.T, opts aguiEnvOpts, steps ...func(modelRequest) []string) *aguiEnv {
	t.Helper()
	e := &aguiEnv{t: t, clock: newFakeClock(), logs: &bytes.Buffer{}, gcRuns: new(int)}
	e.model = newScriptedModel(t, steps...)
	e.d = catFake(t, nil)
	e.pins = installPins(e.d.fakeDaemon, map[string]fakePin{testCID(t, 1): {Type: "recursive", Name: "wiki"}})
	e.d.stream["repo/gc"] = func([]string, map[string]any) ([]any, error) {
		*e.gcRuns++
		return []any{map[string]any{"Key": map[string]any{"/": testCID(t, 2000)}}}, nil
	}
	var resolvers []Resolver
	if !opts.noPresence {
		e.presence = newFakeResolver(ResolverLinuxPolkit)
		resolvers = append(resolvers, e.presence)
	}
	log := slog.New(slog.NewTextHandler(e.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	e.s = mustNew(t, e.d, Options{
		Tools: []string{"node", "content", "pin", "diag", "destructive"}, Resolvers: resolvers,
		Transport: "http", Logger: log, now: e.clock.Now,
		// Notice a clock that moved within milliseconds, not a second.
		awaitRecheck: 10 * time.Millisecond,
	})
	require.NoError(t, e.s.EnableAgent(&AgentSettings{
		BaseURL: e.model.baseURL(), Model: "test-model", Timeout: 5 * time.Second, Limits: opts.limits,
	}))
	e.s.Agent().now = e.clock.Now
	if !opts.noAGUI {
		require.NoError(t, e.s.EnableAGUI(AGUISettings{OriginAllowlist: opts.origins}))
	}
	v, err := NewStaticVerifier([]StaticCredential{
		{Identity: aguiIdentity("chat", KindUI, []string{"pin"}, []string{"pin", "diag"}, []string{"local"}), Token: aguiTokChat},
		{Identity: aguiIdentity("chat2", KindUI, []string{"pin"}, []string{"pin", "diag"}, []string{"local"}), Token: aguiTokChat2},
		{Identity: aguiIdentity(aguiNeedsAckID, KindUI, []string{"pin"}, nil, nil), Token: aguiTokNoAck},
		{Identity: aguiIdentity("human", KindHumanMCP, []string{"pin"}, nil, []string{"local"}), Token: aguiTokHuman},
		{Identity: aguiIdentity("peer", KindA2A, []string{"pin"}, nil, []string{"local"}), Token: aguiTokPeer},
		{Identity: aguiIdentity("chat-ro", KindUI, nil, nil, []string{"local"}), Token: aguiTokReadUI},
	})
	require.NoError(t, err)
	hopts := HTTPOptions{Listen: "127.0.0.1:0", Verifier: v, AllowedOrigins: opts.listenerOrg, MaxBodyBytes: opts.maxBody, Logger: log}
	e.hs, err = NewHTTPServer(e.s, hopts)
	require.NoError(t, err)
	addr, err := e.hs.Listen()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- e.hs.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-done)
	})
	e.base = "http://" + addr.String()
	return e
}

// aguiBody builds a RunAgentInput. resume may be nil.
func aguiBody(thread, run string, messages []any, resume ...any) map[string]any {
	in := map[string]any{"threadId": thread, "runId": run, "messages": messages, "tools": []any{}, "context": []any{}}
	if len(resume) > 0 {
		in["resume"] = resume
	}
	return in
}

func userMsg(id, text string) map[string]any {
	return map[string]any{"id": id, "role": "user", "content": text}
}

func resumeEntry(interruptID, status string, payload any) map[string]any {
	e := map[string]any{"interruptId": interruptID, "status": status}
	if payload != nil {
		e["payload"] = payload
	}
	return e
}

// open sends the request and returns the response; the caller reads the stream.
func (e *aguiEnv) open(ctx context.Context, token string, body any, hdr map[string]string) *http.Response {
	e.t.Helper()
	var raw []byte
	switch b := body.(type) {
	case string:
		raw = []byte(b)
	default:
		var err error
		raw, err = json.Marshal(body)
		require.NoError(e.t, err)
	}
	method := http.MethodPost
	if m := hdr["method"]; m != "" {
		method = m
	}
	req, err := http.NewRequestWithContext(ctx, method, e.base+RouteAGUI, bytes.NewReader(raw))
	require.NoError(e.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		if k != "method" {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(e.t, err)
	e.t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// post sends the request and reads the whole response.
func (e *aguiEnv) post(token string, body any, hdr map[string]string) *aguiResponse {
	e.t.Helper()
	resp := e.open(e.t.Context(), token, body, hdr)
	return readAGUIResponse(e.t, resp)
}

// aguiResponse is a fully read response: an HTTP error body, or the events of a
// stream.
type aguiResponse struct {
	Status int
	Header http.Header
	Body   string
	Events []aguiEvent
}

func readAGUIResponse(t *testing.T, resp *http.Response) *aguiResponse {
	t.Helper()
	out := &aguiResponse{Status: resp.StatusCode, Header: resp.Header}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		out.Events = newSSE(resp.Body).all()
		return out
	}
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	out.Body = string(b)
	return out
}

// errCode returns error.code of a JSON error body.
func (r *aguiResponse) errCode(t *testing.T) string {
	t.Helper()
	var b struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Body), &b), r.Body)
	return b.Error.Code
}

// types lists the event types.
func (r *aguiResponse) types() []string {
	out := make([]string, 0, len(r.Events))
	for _, e := range r.Events {
		out = append(out, e["type"].(string))
	}
	return out
}

// sse reads server-sent events one at a time.
type sse struct {
	sc *bufio.Scanner
	mu sync.Mutex
}

func newSSE(r io.Reader) *sse {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	return &sse{sc: sc}
}

// next returns the next event, or false at the end of the stream. Comments
// (keepalives) are skipped.
func (s *sse) next() (aguiEvent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.sc.Scan() {
		line := s.sc.Text()
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			var ev aguiEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				return aguiEvent{"type": "BAD_JSON", "raw": data}, true
			}
			return ev, true
		}
	}
	return nil, false
}

func (s *sse) all() []aguiEvent {
	var out []aguiEvent
	for {
		ev, ok := s.next()
		if !ok {
			return out
		}
		out = append(out, ev)
	}
}

// findEvents returns the events of a type.
func findEvents(evs []aguiEvent, typ string) []aguiEvent {
	var out []aguiEvent
	for _, e := range evs {
		if e["type"] == typ {
			out = append(out, e)
		}
	}
	return out
}

// finishedInterrupt returns the single interrupt of a run that ended with one.
func finishedInterrupt(t *testing.T, evs []aguiEvent) map[string]any {
	t.Helper()
	require.NotEmpty(t, evs)
	last := evs[len(evs)-1]
	require.Equal(t, "RUN_FINISHED", last["type"], "%v", last)
	outcome := last["outcome"].(map[string]any)
	require.Equal(t, "interrupt", outcome["type"])
	list := outcome["interrupts"].([]any)
	require.Len(t, list, 1, "a run carries at most one interrupt")
	return list[0].(map[string]any)
}

// resultContent returns the content of the TOOL_CALL_RESULT for a call.
func resultContent(t *testing.T, evs []aguiEvent, callID string) string {
	t.Helper()
	for _, e := range findEvents(evs, "TOOL_CALL_RESULT") {
		if e["toolCallId"] == callID {
			return e["content"].(string)
		}
	}
	t.Fatalf("no TOOL_CALL_RESULT for %s in %v", callID, evs)
	return ""
}
