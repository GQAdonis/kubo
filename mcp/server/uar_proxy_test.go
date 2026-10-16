//go:build !noagent

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	uarTestCredEnv  = "IPFS_MCP_TEST_UAR_CRED"
	uarTestMCPEnv   = "IPFS_MCP_TEST_UAR_MCP_TOKEN"
	uarTestMCPToken = "kubo-uar-principal-token-0123456789"
)

// uarProxyEnv is a running /agui listener whose backend is a UAR agent on a fake
// v1.1.0 instance.
type uarProxyEnv struct {
	t    *testing.T
	fake *fakeUARRelease
	s    *Server
	base string
	set  *AgentSettings
	logs *bytes.Buffer
}

type uarProxyOpts struct {
	// llmExtra is merged into the llm.uar section (JSON object body).
	skillsDir string
	prompt    string
	version   string
	// noMCP leaves llm.uar.mcp_principal unset.
	noMCP bool
}

// repoSkillsDir is mcp/skills of this checkout (absolute).
func repoSkillsDir(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "skills"))
	require.NoError(t, err)
	return abs
}

func uarConfigJSON(base string, o uarProxyOpts) string {
	uar := map[string]any{"agent_id": "kubo-ipfs", "provider": "liter-llm", "mcp_name": "kubo-ipfs", "mcp_grant_ttl": "5m"}
	if !o.noMCP {
		uar["mcp_principal"] = "uar"
		uar["mcp_credential_revision"] = "rev-7"
	}
	if o.skillsDir != "" {
		uar["skills_dir"] = o.skillsDir
	}
	llm := map[string]any{"kind": "uar", "base_url": base, "model": "qwen3:8b", "api_key": map[string]any{"env": uarTestCredEnv}, "uar": uar}
	doc := map[string]any{
		"principals": []any{map[string]any{
			"name": "uar", "kind": "uar", "credential": map[string]any{"env": uarTestMCPEnv},
			"uar": map[string]any{"card_url": base + uarCardPath},
		}},
		"llm": llm,
	}
	b, _ := json.Marshal(doc)
	return string(b)
}

func newUARProxyEnv(t *testing.T, script func(int, map[string]any) (int, []byte), o uarProxyOpts) *uarProxyEnv {
	t.Helper()
	t.Setenv(uarTestCredEnv, fakeUARToken)
	t.Setenv(uarTestMCPEnv, uarTestMCPToken)
	e := &uarProxyEnv{t: t, logs: &bytes.Buffer{}}
	e.fake = newFakeUARRelease(t, script)
	if o.version != "" {
		e.fake.edit(func(u *fakeUARRelease) { u.version = o.version })
	}
	cfg := mustParse(t, uarConfigJSON(e.fake.srv.URL, o))
	set, err := cfg.ResolveAgent(LLMOverrides{})
	require.NoError(t, err)
	require.True(t, set.IsUAR())
	set.SystemPrompt = o.prompt
	e.set = set

	d := catFake(t, nil)
	log := slog.New(slog.NewTextHandler(e.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	e.s = mustNew(t, d, Options{
		Tools: []string{"node", "content", "pin"}, Resolvers: []Resolver{newFakeResolver(ResolverLinuxPolkit)},
		Transport: "http", Logger: log,
	})
	require.NoError(t, e.s.EnableAgent(set))
	require.NoError(t, e.s.EnableAGUI(AGUISettings{}))
	v, err := NewStaticVerifier([]StaticCredential{
		{Identity: aguiIdentity("chat", KindUI, []string{"pin"}, nil, []string{"local"}), Token: aguiTokChat},
		{Identity: aguiIdentity("chat2", KindUI, []string{"pin"}, nil, []string{"local"}), Token: aguiTokChat2},
		{Identity: aguiIdentity("peer", KindA2A, nil, nil, []string{"local"}), Token: aguiTokPeer},
	})
	require.NoError(t, err)
	hs, err := NewHTTPServer(e.s, HTTPOptions{Listen: "127.0.0.1:0", Verifier: v, Logger: log})
	require.NoError(t, err)
	addr, err := hs.Listen()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- hs.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-done)
	})
	e.base = "http://" + addr.String()
	return e
}

// post sends a RunAgentInput as the chat principal and reads the response.
func (e *uarProxyEnv) post(token string, body any) *aguiResponse {
	e.t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(e.t, err)
	req, err := http.NewRequestWithContext(e.t.Context(), http.MethodPost, e.base+RouteAGUI, bytes.NewReader(raw))
	require.NoError(e.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(e.t, err)
	e.t.Cleanup(func() { _ = resp.Body.Close() })
	return readAGUIResponse(e.t, resp)
}

func (e *uarProxyEnv) say(thread, run, text string) *aguiResponse {
	e.t.Helper()
	return e.post(aguiTokChat, aguiBody(thread, run, []any{userMsg("m-"+run, text)}))
}

func (e *uarProxyEnv) answer(thread, run, interruptID string, approved bool) *aguiResponse {
	e.t.Helper()
	return e.post(aguiTokChat, aguiBody(thread, run, nil, resumeEntry(interruptID, "resolved", map[string]any{"approved": approved})))
}

var confirmationIDPattern = regexp.MustCompile(`^cf_[A-Za-z0-9_-]{16,64}$`)

// requireKuboInterrupt checks an interrupt against the shape the island's
// contract fixes (fixtures/agui.schema.json, mcp/ui AguiInterruptSchema): the
// reason tool_call, a message of at least five characters, and a strict
// ipfs-mcp metadata object.
func requireKuboInterrupt(t *testing.T, in map[string]any, tool string) {
	t.Helper()
	assert.Equal(t, "tool_call", in["reason"])
	assert.GreaterOrEqual(t, len(in["message"].(string)), 5)
	assert.NotEmpty(t, in["id"])
	assert.NotEmpty(t, in["toolCallId"])
	rs := in["responseSchema"].(map[string]any)
	assert.Equal(t, []any{"approved"}, rs["required"])
	meta := in["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)
	assert.ElementsMatch(t, []string{"confirmation_id", "tool", "class", "requires_native_prompt"}, keysOf(meta))
	assert.Regexp(t, confirmationIDPattern, meta["confirmation_id"], "the island's schema")
	assert.True(t, strings.HasPrefix(meta["confirmation_id"].(string), "cf_uar_"), "an id no broker confirmation has")
	assert.True(t, strings.HasPrefix(in["message"].(string), uarApprovalLabel), "the card says it is UAR's approval")
	uar := in["metadata"].(map[string]any)["uar"].(map[string]any)
	assert.Equal(t, "uar", uar["source"])
	assert.Equal(t, false, uar["node_confirmation"])
	assert.Equal(t, tool, meta["tool"])
	assert.Equal(t, "write", meta["class"])
	assert.Equal(t, false, meta["requires_native_prompt"])
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func resumeEntries(t *testing.T, run uarRun) []map[string]any {
	t.Helper()
	raw, _ := run.Body["resume"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.(map[string]any))
	}
	return out
}

// ---- provisioning ----

func TestUARProvisionsAgentOnFirstRun(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-text.sse"), uarProxyOpts{skillsDir: repoSkillsDir(t), prompt: "Answer in one line."})
	assert.Zero(t, e.fake.runCount(), "building the backend contacts nothing")
	assert.Empty(t, e.fake.callLog())

	r := e.say("t1", "r1", "hello")
	require.Equal(t, http.StatusOK, r.Status, r.Body)

	// The agent on the instance holds kubo's prompt, prefix included, the MCP
	// binding that names the kubo server, and the model.
	art := e.fake.agents["kubo-ipfs"]
	require.NotNil(t, art)
	assert.Equal(t, composeSystemPrompt("Answer in one line."), art["prompt"].(map[string]any)["system"])
	assert.True(t, strings.HasPrefix(art["prompt"].(map[string]any)["system"].(string), systemPromptSafetyPrefix))
	// The artifact has the shape the acceptance posted (acc. 419-436): the kubo
	// tools are the only tools the agent may use, by their UAR names.
	assert.Equal(t, "agent", art["kind"])
	assert.Equal(t, "1.0.0", art["version"])
	pol := art["policy"].(map[string]any)
	def := pol["provider"].(map[string]any)["default"].(map[string]any)
	assert.Equal(t, map[string]any{"provider": "liter-llm", "model": "qwen3:8b"}, def)
	allow := pol["tools"].(map[string]any)["allow"].([]any)
	require.NotEmpty(t, allow)
	for _, a := range allow {
		assert.True(t, strings.HasPrefix(a.(string), "kubo-ipfs__"), "tool %v is a tool of the kubo MCP server", a)
	}
	assert.Contains(t, allow, "kubo-ipfs__pin_add")
	assert.NotNil(t, art["extensions"], "extensions is a required object, empty")
	assert.Empty(t, art["extensions"])

	// Skill bindings equal the directory names under mcp/skills.
	entries, err := os.ReadDir(repoSkillsDir(t))
	require.NoError(t, err)
	var want []string
	for _, ent := range entries {
		if _, err := os.Stat(filepath.Join(repoSkillsDir(t), ent.Name(), "SKILL.md")); err == nil {
			want = append(want, ent.Name())
		}
	}
	require.NotEmpty(t, want)
	assert.ElementsMatch(t, want, e.fake.bindings["kubo-ipfs"])
	assert.Len(t, e.fake.skills, len(want))

	// Every request carried kubo's UAR credential.
	for _, run := range []uarRun{e.fake.run(0)} {
		assert.Equal(t, "Bearer "+fakeUARToken, run.Auth)
	}
	prefer := pol["skills"].(map[string]any)["prefer"].([]any)
	assert.Len(t, prefer, len(want), "every uploaded skill is preferred")
	assert.NotContains(t, e.fake.callLog(), "PUT /api/uar/agents/kubo-ipfs/skills", "bindings are added one by one, never replaced")
}

func TestUARProvisioningIsIdempotentAndLeavesForeignBindings(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-text.sse"), uarProxyOpts{skillsDir: repoSkillsDir(t)})
	require.Equal(t, http.StatusOK, e.say("t1", "r1", "hello").Status)
	// The operator bound a skill kubo did not upload, and the agent drifted.
	e.fake.edit(func(u *fakeUARRelease) {
		u.bindings["kubo-ipfs"] = append(u.bindings["kubo-ipfs"], "operator-skill")
		u.agents["kubo-ipfs"]["prompt"] = map[string]any{"system": "tampered"}
	})
	// A new backend (a settings apply) provisions again: the agent is replaced
	// in full without If-Match, the skills already exist (409, same digest), and
	// the operator's binding stays.
	require.NoError(t, e.s.ApplyAgentSettings(e.set))
	require.Equal(t, http.StatusOK, e.say("t2", "r2", "hello again").Status)

	art := e.fake.agents["kubo-ipfs"]
	assert.Equal(t, composeSystemPrompt(""), art["prompt"].(map[string]any)["system"])
	assert.Contains(t, e.fake.bindings["kubo-ipfs"], "operator-skill", "a binding kubo did not make is left alone")
	assert.Contains(t, e.fake.callLog(), "PUT /api/agents/kubo-ipfs")
	for _, c := range e.fake.callLog() {
		assert.False(t, strings.HasPrefix(c, "DELETE /api/uar/agents/"), "nothing is unbound: %s", c)
	}
}

// R8-07: a skill kubo bound earlier and no longer ships is unbound; nothing else is.
func TestUARUnbindsOnlyWhatKuboBound(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"alpha", "beta"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, n), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, n, "SKILL.md"), []byte("---\nname: "+n+"\ndescription: d\n---\nbody "+n+"\n"), 0o600))
	}
	e := newUARProxyEnv(t, replay(t, "run-text.sse"), uarProxyOpts{skillsDir: dir})
	b := e.s.UARBackend()
	require.Equal(t, http.StatusOK, e.say("t1", "r1", "hello").Status)
	e.fake.edit(func(u *fakeUARRelease) { u.bindings["kubo-ipfs"] = append(u.bindings["kubo-ipfs"], "operator-skill") })
	require.ElementsMatch(t, []string{"alpha", "beta"}, b.ownedSkills)

	// The same backend provisions again with one skill fewer.
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "beta")))
	b.forgetProvisioned()
	require.Equal(t, http.StatusOK, e.say("t1", "r2", "hello").Status)
	assert.ElementsMatch(t, []string{"alpha", "operator-skill"}, e.fake.bindings["kubo-ipfs"])
	assert.Equal(t, []string{"alpha"}, b.ownedSkills)
	assert.Contains(t, e.fake.callLog(), "DELETE /api/uar/agents/kubo-ipfs/skills/beta")
	assert.NotContains(t, e.fake.callLog(), "DELETE /api/uar/agents/kubo-ipfs/skills/operator-skill")
}

func TestUARWithoutMCPPrincipalHasNoBindingOrGrant(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-text.sse"), uarProxyOpts{noMCP: true})
	require.Equal(t, http.StatusOK, e.say("t1", "r1", "hello").Status)
	assert.Empty(t, e.fake.agents["kubo-ipfs"]["policy"].(map[string]any)["tools"].(map[string]any)["allow"])
	assert.NotContains(t, e.fake.run(0).Body, "forwardedProps")
	assert.Contains(t, e.logs.String(), "no kubo tools")
}

func TestUARRefusesAnOldOrUnpublishedRelease(t *testing.T) {
	for _, v := range []string{"1.0.0", "1.1.0-rc1", "dev"} {
		t.Run(v, func(t *testing.T) {
			e := newUARProxyEnv(t, replay(t, "run-text.sse"), uarProxyOpts{version: v})
			r := e.say("t1", "r1", "hello")
			assert.Equal(t, http.StatusBadGateway, r.Status, r.Body)
			assert.Empty(t, e.fake.agents, "nothing is created on an instance below the pinned release")
			assert.Zero(t, e.fake.runCount())
		})
	}
}

func TestUARProvisioningFailureIsRetriedOnTheNextRun(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-text.sse"), uarProxyOpts{version: "1.0.0"})
	assert.Equal(t, http.StatusBadGateway, e.say("t1", "r1", "hello").Status)
	e.fake.edit(func(u *fakeUARRelease) { u.version = "1.1.0" })
	assert.Equal(t, http.StatusOK, e.say("t1", "r2", "hello").Status)
}

// ---- runs ----

func TestUARRunIsProxiedWithTheGrantAndIDsRewritten(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-text.sse"), uarProxyOpts{})
	r := e.say("thread-a", "run-a", "pin something")
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	assert.Equal(t, []string{"RUN_STARTED", "TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT", "TEXT_MESSAGE_END", "RUN_FINISHED"}, r.types())
	assert.Equal(t, "thread-a", r.Events[0]["threadId"], "the island sees its own thread id, not UAR's")
	assert.Equal(t, "run-a", r.Events[0]["runId"])
	last := r.Events[len(r.Events)-1]
	assert.Equal(t, "run-a", last["runId"])
	assert.Equal(t, map[string]any{"type": "success"}, last["outcome"])

	run := e.fake.run(0)
	assert.Equal(t, "kubo-ipfs", run.AgentID)
	assert.NotEqual(t, "thread-a", run.Body["threadId"], "UAR sees a digest of principal and thread")
	gen := e.s.UARBackend().gen
	assert.Equal(t, uarThreadID(gen, 1, "chat", "thread-a"), run.Body["threadId"])
	// The accepted request shape (fixture initial_request): protocolVersion, no
	// state, no client tools.
	assert.Equal(t, "1.0", run.Body["protocolVersion"])
	assert.NotContains(t, run.Body, "state")
	assert.Equal(t, "run-a", run.Body["runId"])
	msgs := run.Body["messages"].([]any)
	require.Len(t, msgs, 1)
	assert.Equal(t, "pin something", msgs[0].(map[string]any)["content"])
	assert.Empty(t, run.Body["tools"], "client tools are never offered to UAR")

	mcps := run.Body["forwardedProps"].(map[string]any)["uar"].(map[string]any)["mcp_servers"].([]any)
	require.Len(t, mcps, 1)
	srv := mcps[0].(map[string]any)
	assert.Equal(t, "kubo-ipfs", srv["name"])
	grant := srv["grant"].(map[string]any)
	assert.Equal(t, "rev-7", grant["credential_revision"])
	assert.Equal(t, []any{"ipfs"}, grant["scopes"])
	assert.Equal(t, map[string]any{"Authorization": "Bearer " + uarTestMCPToken}, grant["headers"])
	assert.InDelta(t, time.Now().Add(5*time.Minute).Unix(), grant["expires_at_unix"], 10)
}

func TestUARThreadsAreSeparatedPerPrincipal(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-text.sse"), uarProxyOpts{})
	require.Equal(t, http.StatusOK, e.say("same", "r1", "hi").Status)
	r := e.post(aguiTokChat2, aguiBody("same", "r2", []any{userMsg("m", "hi")}))
	require.Equal(t, http.StatusOK, r.Status)
	assert.NotEqual(t, e.fake.run(0).Body["threadId"], e.fake.run(1).Body["threadId"])
}

func TestUARFailuresDoNotLeakTheInstancesWords(t *testing.T) {
	cases := map[string]struct {
		script func(int, map[string]any) (int, []byte)
		status int
		runErr bool
		inBody string
	}{
		"unauthorized": {script: func(int, map[string]any) (int, []byte) { return http.StatusUnauthorized, nil }, status: http.StatusBadGateway, inBody: "refused the credential"},
		"server error": {script: func(int, map[string]any) (int, []byte) { return http.StatusInternalServerError, nil }, status: http.StatusBadGateway, inBody: "HTTP 500"},
		"stream cut": {script: func(int, map[string]any) (int, []byte) {
			return http.StatusOK, []byte("data: {\"type\":\"RUN_STARTED\",\"threadId\":\"x\",\"runId\":\"y\"}\n\n")
		}, status: http.StatusOK, runErr: true},
		"not json": {script: func(int, map[string]any) (int, []byte) { return http.StatusOK, []byte("data: {broken\n\n") }, status: http.StatusBadGateway},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := newUARProxyEnv(t, c.script, uarProxyOpts{})
			r := e.say("t1", "r1", "hello")
			assert.Equal(t, c.status, r.Status, r.Body)
			if c.runErr {
				last := r.Events[len(r.Events)-1]
				assert.Equal(t, "RUN_ERROR", last["type"])
				assert.Equal(t, "backend_error", last["code"])
			} else {
				assert.Equal(t, "internal", r.errCode(t))
				assert.Contains(t, r.Body, c.inBody)
			}
			assert.NotContains(t, r.Body, "scripted")
		})
	}
}

func TestUARRunErrorIsRelayedWithTheBackendErrorCode(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-error.sse"), uarProxyOpts{})
	r := e.say("t1", "r1", "hello")
	require.Equal(t, http.StatusOK, r.Status)
	last := r.Events[len(r.Events)-1]
	assert.Equal(t, "RUN_ERROR", last["type"])
	assert.Equal(t, "backend_error", last["code"])
	assert.Equal(t, "no model configured", last["message"])
}

func TestUARAgentLostOnTheInstanceIsProvisionedAgain(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-text.sse"), uarProxyOpts{})
	require.Equal(t, http.StatusOK, e.say("t1", "r1", "hello").Status)
	e.fake.edit(func(u *fakeUARRelease) { delete(u.agents, "kubo-ipfs") })
	assert.Equal(t, http.StatusBadGateway, e.say("t1", "r2", "hello").Status, "the run that finds the agent gone fails")
	assert.Equal(t, http.StatusOK, e.say("t1", "r3", "hello").Status, "the next run provisions it again")
	assert.Contains(t, e.fake.agents, "kubo-ipfs")
}

// ---- interrupts ----

func TestUARToolApprovalApproved(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-tool-approval-interrupt.sse", "run-resumed-executed.sse"), uarProxyOpts{})
	r := e.say("t1", "r1", "pin bafy-example")
	require.Equal(t, http.StatusOK, r.Status)
	in := finishedInterrupt(t, r.Events)
	requireKuboInterrupt(t, in, "uar:pin_add")
	assert.Equal(t, "int-ta-1", in["id"])
	assert.Equal(t, "tc-1", in["toolCallId"])
	assert.Equal(t, "2030-01-01T00:00:00Z", in["expiresAt"])
	assert.NotContains(t, r.types(), "CUSTOM", "no A2UI advertisement on the proxied stream")

	r2 := e.answer("t1", "r2", "int-ta-1", true)
	require.Equal(t, http.StatusOK, r2.Status, r2.Body)
	assert.Equal(t, "RUN_STARTED", r2.types()[0])
	assert.Equal(t, "r2", r2.Events[0]["runId"], "the island sees its own run id")
	assert.Contains(t, r2.types(), "TOOL_CALL_RESULT")
	assert.Equal(t, map[string]any{"type": "success"}, r2.Events[len(r2.Events)-1]["outcome"])

	// The resume is the contract's: same thread, new run, the preceding run as
	// parent, empty messages, one entry with the documented payload.
	require.Equal(t, 2, e.fake.runCount())
	first, second := e.fake.run(0), e.fake.run(1)
	assert.Equal(t, first.Body["threadId"], second.Body["threadId"])
	assert.Equal(t, "r2", second.Body["runId"])
	assert.Equal(t, "r1", second.Body["parentRunId"])
	assert.Empty(t, second.Body["messages"])
	entries := resumeEntries(t, second)
	require.Len(t, entries, 1)
	assert.Equal(t, map[string]any{"interruptId": "int-ta-1", "status": "resolved", "payload": map[string]any{"approved": true}}, entries[0])
	// The renewed grant travels in forwardedProps.uar.
	assert.NotNil(t, second.Body["forwardedProps"].(map[string]any)["uar"])
}

func TestUARToolApprovalDeniedEndsTheRunWithNothingExecuted(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-tool-approval-interrupt.sse", "run-resumed-denied.sse"), uarProxyOpts{})
	in := finishedInterrupt(t, e.say("t1", "r1", "pin bafy-example").Events)

	r := e.answer("t1", "r2", in["id"].(string), false)
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	assert.NotContains(t, r.types(), "TOOL_CALL_RESULT", "nothing ran")
	assert.Equal(t, map[string]any{"type": "success"}, r.Events[len(r.Events)-1]["outcome"])
	entries := resumeEntries(t, e.fake.run(1))
	require.Len(t, entries, 1)
	assert.Equal(t, map[string]any{"approved": false}, entries[0]["payload"])
	assert.Equal(t, "resolved", entries[0]["status"])

	// The thread is free again.
	assert.Equal(t, http.StatusOK, e.say("t1", "r3", "something else").Status)
}

func TestUARCancelledResumeIsADenial(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-tool-approval-interrupt.sse", "run-resumed-denied.sse"), uarProxyOpts{})
	in := finishedInterrupt(t, e.say("t1", "r1", "pin").Events)
	r := e.post(aguiTokChat, aguiBody("t1", "r2", nil, resumeEntry(in["id"].(string), "cancelled", nil)))
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	entries := resumeEntries(t, e.fake.run(1))
	assert.Equal(t, map[string]any{"interruptId": "int-ta-1", "status": "cancelled"}, entries[0])
}

func TestUARElicitationApprovedAndDenied(t *testing.T) {
	t.Run("approved", func(t *testing.T) {
		e := newUARProxyEnv(t, replay(t, "run-elicitation-interrupt.sse", "run-resumed-executed.sse"), uarProxyOpts{})
		in := finishedInterrupt(t, e.say("t1", "r1", "pin").Events)
		requireKuboInterrupt(t, in, "uar:kubo-ipfs")
		assert.Equal(t, uarApprovalLabel+"Pin bafy-example to this node?", in["message"], "UAR's text, labelled")

		r := e.answer("t1", "r2", in["id"].(string), true)
		require.Equal(t, http.StatusOK, r.Status, r.Body)
		entries := resumeEntries(t, e.fake.run(1))
		assert.Equal(t, map[string]any{
			"interruptId": "int-el-1", "status": "resolved",
			"payload": map[string]any{"action": "accept", "content": map[string]any{"confirm": true}},
		}, entries[0])
	})
	t.Run("denied", func(t *testing.T) {
		e := newUARProxyEnv(t, replay(t, "run-elicitation-interrupt.sse", "run-resumed-denied.sse"), uarProxyOpts{})
		in := finishedInterrupt(t, e.say("t1", "r1", "pin").Events)
		r := e.answer("t1", "r2", in["id"].(string), false)
		require.Equal(t, http.StatusOK, r.Status, r.Body)
		entries := resumeEntries(t, e.fake.run(1))
		assert.Equal(t, map[string]any{"action": "decline"}, entries[0]["payload"])
		assert.NotContains(t, r.types(), "TOOL_CALL_RESULT")
	})
	t.Run("cancelled", func(t *testing.T) {
		e := newUARProxyEnv(t, replay(t, "run-elicitation-interrupt.sse", "run-resumed-denied.sse"), uarProxyOpts{})
		in := finishedInterrupt(t, e.say("t1", "r1", "pin").Events)
		r := e.post(aguiTokChat, aguiBody("t1", "r2", nil, resumeEntry(in["id"].(string), "cancelled", nil)))
		require.Equal(t, http.StatusOK, r.Status, r.Body)
		assert.Equal(t, "cancelled", resumeEntries(t, e.fake.run(1))[0]["status"])
	})
}

func TestUARTwoInterruptsAreAskedOneAtATimeAndResumedTogether(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-both-interrupts.sse", "run-resumed-executed.sse"), uarProxyOpts{})
	first := finishedInterrupt(t, e.say("t1", "r1", "pin").Events)
	assert.Equal(t, "int-ta-1", first["id"], "UAR's order")

	// Approving the first asks the second without contacting UAR: UAR rejects a
	// resume that does not cover every pending interrupt.
	r2 := e.answer("t1", "r2", "int-ta-1", true)
	require.Equal(t, http.StatusOK, r2.Status, r2.Body)
	second := finishedInterrupt(t, r2.Events)
	assert.Equal(t, "int-el-1", second["id"])
	assert.Equal(t, 1, e.fake.runCount(), "no UAR request yet")

	r3 := e.answer("t1", "r3", "int-el-1", true)
	require.Equal(t, http.StatusOK, r3.Status, r3.Body)
	require.Equal(t, 2, e.fake.runCount())
	entries := resumeEntries(t, e.fake.run(1))
	require.Len(t, entries, 2)
	assert.Equal(t, "int-ta-1", entries[0]["interruptId"])
	assert.Equal(t, map[string]any{"approved": true}, entries[0]["payload"])
	assert.Equal(t, "int-el-1", entries[1]["interruptId"])
	assert.Equal(t, "accept", entries[1]["payload"].(map[string]any)["action"])
	assert.Equal(t, "r1", e.fake.run(1).Body["parentRunId"], "the parent is the run that raised the interrupts")
}

func TestUARDenyDeniesEveryPendingInterrupt(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-both-interrupts.sse", "run-resumed-denied.sse"), uarProxyOpts{})
	finishedInterrupt(t, e.say("t1", "r1", "pin").Events)
	r := e.answer("t1", "r2", "int-ta-1", false)
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	assert.NotContains(t, r.types(), "TOOL_CALL_RESULT")
	entries := resumeEntries(t, e.fake.run(1))
	require.Len(t, entries, 2)
	assert.Equal(t, map[string]any{"approved": false}, entries[0]["payload"])
	assert.Equal(t, map[string]any{"action": "decline"}, entries[1]["payload"], "the second is never shown and never approved")
}

func TestUARInterruptsThatNeedNoPersonAreNeverApproved(t *testing.T) {
	cases := map[string]struct {
		fixture string
		status  string
		payload map[string]any
	}{
		"url elicitation is declined":          {"run-url-elicitation-interrupt.sse", "resolved", map[string]any{"action": "decline"}},
		"a form kubo cannot render is dropped": {"run-form-elicitation-interrupt.sse", "cancelled", nil},
		"an unreachable connection is dropped": {"run-unresolvable-elicitation-interrupt.sse", "cancelled", nil},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := newUARProxyEnv(t, replay(t, c.fixture, "run-resumed-denied.sse"), uarProxyOpts{})
			r := e.say("t1", "r1", "go")
			require.Equal(t, http.StatusOK, r.Status, r.Body)
			// The island never saw an interrupt: the run continued in the same request.
			assert.Equal(t, map[string]any{"type": "success"}, r.Events[len(r.Events)-1]["outcome"])
			assert.Equal(t, 1, len(findEvents(r.Events, "RUN_STARTED")), "one RUN_STARTED across both UAR requests")
			require.Equal(t, 2, e.fake.runCount())
			entries := resumeEntries(t, e.fake.run(1))
			require.Len(t, entries, 1)
			assert.Equal(t, c.status, entries[0]["status"])
			if c.payload != nil {
				assert.Equal(t, c.payload, entries[0]["payload"])
			} else {
				assert.NotContains(t, entries[0], "payload")
			}
			assert.Equal(t, "r1-h1", e.fake.run(1).Body["runId"])
		})
	}
}

func TestUARPeerThatKeepsRaisingInterruptsIsCutOff(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-url-elicitation-interrupt.sse"), uarProxyOpts{})
	r := e.say("t1", "r1", "go")
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	last := r.Events[len(r.Events)-1]
	assert.Equal(t, "RUN_ERROR", last["type"])
	assert.Equal(t, maxUARHops, e.fake.runCount())
}

func TestUARResumeValidation(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-tool-approval-interrupt.sse", "run-resumed-executed.sse"), uarProxyOpts{})

	r := e.post(aguiTokChat, aguiBody("none", "r0", nil, resumeEntry("int-ta-1", "resolved", map[string]any{"approved": true})))
	assert.Equal(t, http.StatusNotFound, r.Status, "an unknown thread")

	finishedInterrupt(t, e.say("t1", "r1", "pin").Events)
	assert.Equal(t, http.StatusUnprocessableEntity, e.answer("t1", "r2", "int-other", true).Status, "an interrupt that is not open")
	assert.Equal(t, http.StatusNotFound, e.post(aguiTokChat2, aguiBody("t1", "r2", nil, resumeEntry("int-ta-1", "resolved", map[string]any{"approved": true}))).Status,
		"another principal cannot answer it")
	assert.Equal(t, http.StatusUnprocessableEntity,
		e.post(aguiTokChat, aguiBody("t1", "r2", nil, resumeEntry("int-ta-1", "resolved", map[string]any{"wait": true}))).Status, "wait is for the native prompt")
	assert.Equal(t, http.StatusUnprocessableEntity,
		e.post(aguiTokChat, aguiBody("t1", "r2", nil, resumeEntry("int-ta-1", "resolved", map[string]any{"approved": "true"}))).Status, "approval is a boolean")
	assert.Equal(t, http.StatusConflict, e.say("t1", "r2", "new message while a confirmation is open").Status)
	assert.Equal(t, 1, e.fake.runCount(), "none of these reached UAR")

	require.Equal(t, http.StatusOK, e.answer("t1", "r3", "int-ta-1", true).Status)
	assert.Equal(t, http.StatusUnprocessableEntity, e.answer("t1", "r4", "int-ta-1", true).Status, "an interrupt is answered once")
}

// answerFor is the one place an answer to a UAR interrupt is made: deny and
// cancel never carry an approval.
func TestUARAnswersNeverApproveWithoutAnApproval(t *testing.T) {
	p := &uarPending{Reason: uarReasonToolApproval}
	assert.Equal(t, map[string]any{"approved": false}, p.answerFor(false, false).Payload)
	assert.Equal(t, ResumeCancelled, p.answerFor(true, true).Status, "cancel wins over approve")
	e := &uarPending{Reason: uarReasonElicitation, ApproveKey: "confirm"}
	assert.Equal(t, map[string]any{"action": "decline"}, e.answerFor(false, false).Payload)
}

func TestClassifyUARInterrupt(t *testing.T) {
	form := func(schema map[string]any) map[string]any {
		return map[string]any{"id": "i", "reason": uarReasonElicitation, "message": "m", "metadata": map[string]any{"uar": map[string]any{
			"serverName": "s", "resolvable": true, "request": map[string]any{"mode": "form", "message": "ask", "requestedSchema": schema},
		}}}
	}
	boolSchema := map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}, "required": []any{"ok"}}
	for name, c := range map[string]struct {
		in        map[string]any
		needsUser bool
		key       string
	}{
		"boolean form":          {form(boolSchema), true, "ok"},
		"optional boolean":      {form(map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}}), false, ""},
		"two fields":            {form(map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "boolean"}, "b": map[string]any{"type": "boolean"}}, "required": []any{"a", "b"}}), false, ""},
		"string field":          {form(map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}}, "required": []any{"a"}}), false, ""},
		"no schema":             {form(nil), false, ""},
		"unknown reason":        {map[string]any{"id": "i", "reason": "something_new"}, false, ""},
		"tool approval":         {map[string]any{"id": "i", "reason": uarReasonToolApproval, "message": "m", "toolCallId": "c"}, true, ""},
		"tool approval no call": {map[string]any{"id": "i", "reason": uarReasonToolApproval}, true, ""},
	} {
		t.Run(name, func(t *testing.T) {
			p, ok := classifyUARInterrupt(c.in, nil)
			require.True(t, ok)
			assert.Equal(t, c.needsUser, p.NeedsUser)
			assert.Equal(t, c.key, p.ApproveKey)
			if !c.needsUser {
				require.NotNil(t, p.Answer)
				assert.NotEqual(t, "accept", fmt.Sprint(p.Answer.Payload["action"]), "kubo never accepts for the person")
				assert.NotEqual(t, true, p.Answer.Payload["approved"])
			}
		})
	}
	_, ok := classifyUARInterrupt(map[string]any{"reason": uarReasonToolApproval}, nil)
	assert.False(t, ok, "no id")
	long := strings.Repeat("x", 200)
	p, ok := classifyUARInterrupt(map[string]any{"id": long, "reason": uarReasonToolApproval}, nil)
	require.True(t, ok)
	assert.Equal(t, long, p.ID)
	assert.True(t, len(p.ShownID) <= maxAGUIIDLen && slices.Contains([]bool{true}, strings.HasPrefix(p.ShownID, "uar-")))
}

// ---- backend selection ----

func TestUARBackendReplacesTheLoopAndRestoresIt(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-text.sse"), uarProxyOpts{})
	assert.Nil(t, e.s.Agent(), "no OpenAI-compatible loop while the backend is a UAR agent")
	require.NotNil(t, e.s.UARBackend())

	model := newScriptedModel(t, sayStep("from the loop"))
	require.NoError(t, e.s.ApplyAgentSettings(&AgentSettings{BaseURL: model.baseURL(), Model: "m", Timeout: 5 * time.Second}))
	assert.Nil(t, e.s.UARBackend())
	require.NotNil(t, e.s.Agent())
	r := e.say("t9", "r9", "hello")
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	assert.Equal(t, 0, e.fake.runCount(), "the swapped-away UAR instance is not contacted")
	assert.Equal(t, 1, model.hits())
}
