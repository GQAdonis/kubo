//go:build !noagent

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/require"
)

// Tokens of the A2A test principals. No test names a port.
const (
	a2aTokPeer    = "a2a-peer-token-0123456789"
	a2aTokPeer2   = "a2a-peer2-token-012345678"
	a2aTokReader  = "a2a-reader-token-01234567"
	a2aTokHuman   = "a2a-human-token-01234567"
	a2aTokChat    = "a2a-chat-token-012345678"
	a2aTokNoAck   = "a2a-noack-token-01234567"
	a2aPeerWriter = "peer"
)

// a2aEnv is a running listener with /a2a over an agent that talks to a scripted
// model, a fake daemon with pin commands, a fake clock and a fake presence
// resolver (the system prompt).
type a2aEnv struct {
	t        *testing.T
	model    *scriptedModel
	d        *contentFake
	pins     *pinStore
	gcRuns   *int
	s        *Server
	hs       *HTTPServer
	base     string
	clock    *fakeClock
	presence *fakeResolver
	cli      *fakeResolver
	logs     *bytes.Buffer
}

type a2aEnvOpts struct {
	noA2A      bool
	noAgent    bool
	noPresence bool
	// cliOnly installs the headless cli resolver as the only resolver.
	cliOnly    bool
	onlyReader bool
	limits     LoopLimits
}

func a2aIdentity(id, kind string, read, write, destructive, ack []string) Identity {
	i := NewIdentity(Principal{ID: id, Kind: kind}, read, write, destructive)
	i.Configured = true
	i.EgressAck = ack
	return i
}

func newA2AEnv(t *testing.T, opts a2aEnvOpts, steps ...func(modelRequest) []string) *a2aEnv {
	t.Helper()
	e := &a2aEnv{t: t, clock: newFakeClock(), logs: &bytes.Buffer{}, gcRuns: new(int)}
	e.model = newScriptedModel(t, steps...)
	e.d = catFake(t, nil)
	e.pins = installPins(e.d.fakeDaemon, map[string]fakePin{testCID(t, 1): {Type: "recursive", Name: "wiki"}})
	e.d.stream["repo/gc"] = func([]string, map[string]any) ([]any, error) {
		*e.gcRuns++
		return []any{map[string]any{"Key": map[string]any{"/": testCID(t, 2000)}}}, nil
	}
	var resolvers []Resolver
	if opts.cliOnly {
		e.cli = newFakeResolver(ResolverCLI)
		resolvers = append(resolvers, e.cli)
	} else if !opts.noPresence {
		e.presence = newFakeResolver(ResolverDesktop)
		resolvers = append(resolvers, e.presence)
	}
	log := slog.New(slog.NewTextHandler(e.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	e.s = mustNew(t, e.d, Options{
		Tools: []string{"node", "content", "pin", "diag", "destructive"}, Resolvers: resolvers,
		Transport: "http", Logger: log, now: e.clock.Now, Version: "9.9.9",
	})
	if !opts.noAgent {
		require.NoError(t, e.s.EnableAgent(&AgentSettings{
			BaseURL: e.model.baseURL(), Model: "test-model", Timeout: 5 * time.Second, Limits: opts.limits,
		}))
		e.s.Agent().now = e.clock.Now
	}
	if !opts.noA2A {
		require.NoError(t, e.s.EnableA2A())
		require.NoError(t, e.s.EnableAGUI(AGUISettings{}))
	}
	local := []string{"local"}
	creds := []StaticCredential{
		{Identity: a2aIdentity("reader", KindA2A, []string{"node", "pin"}, nil, nil, local), Token: a2aTokReader},
		{Identity: a2aIdentity("human", KindHumanMCP, []string{AllGroups}, []string{"pin"}, nil, local), Token: a2aTokHuman},
		{Identity: a2aIdentity("chat", KindUI, []string{AllGroups}, []string{"pin"}, []string{"pin"}, local), Token: a2aTokChat},
	}
	if !opts.onlyReader {
		creds = append(creds,
			StaticCredential{Identity: a2aIdentity(a2aPeerWriter, KindA2A, []string{AllGroups}, []string{"pin"}, []string{"pin", "diag"}, local), Token: a2aTokPeer},
			StaticCredential{Identity: a2aIdentity("peer2", KindA2A, []string{AllGroups}, []string{"pin"}, nil, local), Token: a2aTokPeer2},
			StaticCredential{Identity: a2aIdentity("noack", KindA2A, []string{AllGroups}, []string{"pin"}, nil, nil), Token: a2aTokNoAck})
	}
	if opts.cliOnly {
		// --confirm=cli needs every principal with write groups to declare that
		// it cannot run shell commands.
		noShell := false
		for i := range creds {
			creds[i].Identity.ShellCapable = &noShell
		}
	}
	v, err := NewStaticVerifier(creds)
	require.NoError(t, err)
	e.hs, err = NewHTTPServer(e.s, HTTPOptions{Listen: "127.0.0.1:0", Verifier: v, Logger: log})
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
	saved := awaitRecheck
	awaitRecheck = 10 * time.Millisecond
	t.Cleanup(func() { awaitRecheck = saved })
	return e
}

// call sends one JSON-RPC request and returns the HTTP status and the decoded
// response.
func (e *a2aEnv) call(token, method string, params any) (int, map[string]any) {
	e.t.Helper()
	resp := e.open(token, method, params, nil)
	defer resp.Body.Close()
	var out map[string]any
	raw := new(bytes.Buffer)
	_, _ = raw.ReadFrom(resp.Body)
	if raw.Len() > 0 && strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		require.NoError(e.t, json.Unmarshal(raw.Bytes(), &out), raw.String())
	}
	return resp.StatusCode, out
}

func (e *a2aEnv) open(token, method string, params any, hdr map[string]string) *http.Response {
	e.t.Helper()
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	require.NoError(e.t, err)
	return e.openRaw(token, body, hdr)
}

func (e *a2aEnv) openRaw(token string, body []byte, hdr map[string]string) *http.Response {
	e.t.Helper()
	req, err := http.NewRequestWithContext(e.t.Context(), http.MethodPost, e.base+RouteA2A, bytes.NewReader(body))
	require.NoError(e.t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(e.t, err)
	e.t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// result returns the JSON-RPC result of a response that must not be an error.
func rpcResult(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	require.Nil(t, resp["error"], "%v", resp["error"])
	r, ok := resp["result"].(map[string]any)
	require.True(t, ok, "%v", resp)
	return r
}

// rpcErrorCode returns error.code and error.data[].error.code of a response.
func rpcErrorCode(t *testing.T, resp map[string]any) (int, string) {
	t.Helper()
	e, ok := resp["error"].(map[string]any)
	require.True(t, ok, "no error in %v", resp)
	var tool string
	for _, d := range e["data"].([]any) {
		if m := d.(map[string]any)["error"]; m != nil {
			tool = m.(map[string]any)["code"].(string)
		}
	}
	return int(e["code"].(float64)), tool
}

func (e *a2aEnv) send(token string, parts ...any) map[string]any {
	e.t.Helper()
	_, resp := e.call(token, "SendMessage", map[string]any{"message": userMessage(parts...)})
	return resp
}

func userMessage(parts ...any) map[string]any {
	return map[string]any{"messageId": "m-" + randomID(6), "role": "ROLE_USER", "parts": parts}
}

func toolPart(tool string, args map[string]any) map[string]any {
	return map[string]any{"data": map[string]any{"tool": tool, "arguments": args}, "mediaType": "application/json"}
}

func textPartJSON(s string) map[string]any { return map[string]any{"text": s} }

func stateOf(task map[string]any) string {
	return task["status"].(map[string]any)["state"].(string)
}

// taskOf returns the task of a SendMessage result.
func taskOf(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	return rpcResult(t, resp)["task"].(map[string]any)
}

// getTask returns the task as GetTask shows it.
func (e *a2aEnv) getTask(token, id string) map[string]any {
	e.t.Helper()
	_, resp := e.call(token, "GetTask", map[string]any{"id": id})
	return rpcResult(e.t, resp)
}

// eventuallyState polls GetTask until the task has the state.
func (e *a2aEnv) eventuallyState(token, id, want string) map[string]any {
	e.t.Helper()
	var last map[string]any
	require.Eventually(e.t, func() bool {
		last = e.getTask(token, id)
		return stateOf(last) == want
	}, 5*time.Second, 10*time.Millisecond, "wanted %s, last %v", want, last)
	return last
}

var a2aSchemaDoc map[string]any

// validateA2A checks v against a definition of the published A2A 1.0 JSON
// schema bundle (a2a/a2a.schema.json, see a2a/SOURCES.md).
func validateA2A(t *testing.T, def string, v any) {
	t.Helper()
	if a2aSchemaDoc == nil {
		raw, err := os.ReadFile("a2a/a2a.schema.json")
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &a2aSchemaDoc))
	}
	root := map[string]any{}
	for k, val := range a2aSchemaDoc {
		root[k] = val
	}
	root["$ref"] = "#/$defs/" + def
	raw, err := json.Marshal(root)
	require.NoError(t, err)
	var s jsonschema.Schema
	require.NoError(t, json.Unmarshal(raw, &s))
	resolved, err := s.Resolve(nil)
	require.NoError(t, err)
	// Validate the JSON form, as a peer receives it.
	b, err := json.Marshal(v)
	require.NoError(t, err)
	var generic any
	require.NoError(t, json.Unmarshal(b, &generic))
	require.NoError(t, resolved.Validate(generic), "%s does not match the A2A 1.0 schema: %s", def, b)
}
