//go:build !noagent

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sseOf builds an event stream from event JSON documents.
func sseOf(events ...string) []byte {
	var b bytes.Buffer
	for _, e := range events {
		fmt.Fprintf(&b, "data: %s\n\n", e)
	}
	return b.Bytes()
}

// scriptAnswer is the reply to one UAR run; scriptOf answers run n with the n-th, the last repeating.
type scriptAnswer struct {
	status int
	stream []byte
}

func scriptOf(answers ...scriptAnswer) func(int, map[string]any) (int, []byte) {
	return func(n int, _ map[string]any) (int, []byte) {
		a := answers[min(n, len(answers)-1)]
		return a.status, a.stream
	}
}

func ok(stream []byte) scriptAnswer { return scriptAnswer{http.StatusOK, stream} }

func status(code int) scriptAnswer { return scriptAnswer{code, nil} }

// ---- U3: the relayed event types ----

func TestUARRelaysOnlyAllowlistedEventsAndFields(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-hostile-events.sse"), uarProxyOpts{})
	r := e.say("t1", "r1", "hello")
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	assert.Equal(t, []string{
		"RUN_STARTED", "TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT", "TEXT_MESSAGE_END",
		"TOOL_CALL_START", "TOOL_CALL_ARGS", "TOOL_CALL_END", "TOOL_CALL_RESULT", "RUN_FINISHED",
	}, r.types(), "CUSTOM, STATE_*, MESSAGES_SNAPSHOT, RAW and STEP_* are dropped")

	// The island's own thread and run ids on RUN_*; no UAR identity anywhere else.
	assert.Equal(t, "t1", r.Events[0]["threadId"])
	assert.Equal(t, "r1", r.Events[0]["runId"])
	assert.NotContains(t, r.Events[0], "rawEvent")
	last := r.Events[len(r.Events)-1]
	assert.Equal(t, "t1", last["threadId"])
	assert.Equal(t, "r1", last["runId"])
	for _, ev := range r.Events[1 : len(r.Events)-1] {
		for k := range ev {
			assert.NotContains(t, []string{"threadId", "runId", "rawEvent", "extra"}, k, "%s carries %s", ev["type"], k)
		}
	}
	assert.Equal(t, "Visible text.", findEvents(r.Events, "TEXT_MESSAGE_CONTENT")[0]["delta"])
	assert.Equal(t, `{"pins":[]}`, findEvents(r.Events, "TOOL_CALL_RESULT")[0]["content"])
	assert.NotContains(t, r.Body, "uar-thread")
}

func TestUARDropsEachUnlistedEventType(t *testing.T) {
	for _, typ := range []string{
		"CUSTOM", "STATE_SNAPSHOT", "STATE_DELTA", "MESSAGES_SNAPSHOT", "RAW",
		"STEP_STARTED", "STEP_FINISHED", "TEXT_MESSAGE_CHUNK", "TOOL_CALL_CHUNK", "THINKING_START", "SOMETHING_NEW",
	} {
		t.Run(typ, func(t *testing.T) {
			stream := sseOf(
				`{"type":"RUN_STARTED","threadId":"u","runId":"u"}`,
				fmt.Sprintf(`{"type":%q,"name":"a2ui","value":{"x":1},"snapshot":{},"messages":[],"delta":"d"}`, typ),
				`{"type":"RUN_FINISHED","threadId":"u","runId":"u","outcome":{"type":"success"}}`,
			)
			e := newUARProxyEnv(t, scriptOf(ok(stream)), uarProxyOpts{})
			r := e.say("t1", "r1", "hello")
			require.Equal(t, http.StatusOK, r.Status, r.Body)
			assert.Equal(t, []string{"RUN_STARTED", "RUN_FINISHED"}, r.types())
		})
	}
}

// ---- U8: secrets in relayed text ----

func TestUARScrubsTheKeyAndTheGrantFromEveryRelayedString(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-leaks-secrets.sse"), uarProxyOpts{})
	r := e.say("t1", "r1", "hello")
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	assert.NotContains(t, r.Body, fakeUARToken)
	assert.NotContains(t, r.Body, uarTestMCPToken)
	raw, err := json.Marshal(r.Events)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), fakeUARToken)
	assert.NotContains(t, string(raw), uarTestMCPToken)
	assert.Contains(t, findEvents(r.Events, "TEXT_MESSAGE_CONTENT")[0]["delta"], "[redacted]")
	assert.Contains(t, findEvents(r.Events, "TOOL_CALL_START")[0]["toolCallName"], "[redacted]")
	assert.Contains(t, findEvents(r.Events, "TOOL_CALL_RESULT")[0]["content"], "[redacted]")
	last := r.Events[len(r.Events)-1]
	require.Equal(t, "RUN_ERROR", last["type"], "the agent's own RUN_ERROR is relayed, cleaned (nit-1)")
	assert.Contains(t, last["message"], "[redacted]")
	assert.Contains(t, last["message"], "upstream said")
}

func TestUARScrubsTheApprovalCardAndCleansItsNames(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-leaks-secrets-interrupt.sse"), uarProxyOpts{})
	r := e.say("t1", "r1", "hello")
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	assert.NotContains(t, r.Body, fakeUARToken)
	assert.NotContains(t, r.Body, uarTestMCPToken)
	in := finishedInterrupt(t, r.Events)
	assert.Contains(t, in["message"], "[redacted]")
	tool := in["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)["tool"].(string)
	assert.Regexp(t, `^uar:[A-Za-z0-9._-]{1,64}$`, tool, "the peer's tool name is reduced to safe characters")
}

// ---- U9: the approval card is not a node confirmation ----

func TestUARApprovalCardIsLabelledAsUARsAndValidatesAgainstTheIslandSchema(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-tool-approval-interrupt.sse"), uarProxyOpts{})
	in := finishedInterrupt(t, e.say("t1", "r1", "pin").Events)
	requireKuboInterrupt(t, in, "uar:pin_add")
	assert.Equal(t, uarApprovalLabel+"Approve running pin_add?", in["message"])
	id := in["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)["confirmation_id"].(string)
	// No confirmation of the broker has this id, so a resolver cannot resolve one.
	assert.Error(t, e.s.ResolveConfirmation(id, "ui", true))
	assert.Empty(t, e.s.PendingConfirmations())
}

// ---- U7: a same-name skill on the instance ----

func oneSkillDir(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, name), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name, "SKILL.md"), []byte(body), 0o600))
	return dir
}

func TestUARProvisioningFailsClosedOnASameNameSkill(t *testing.T) {
	const src = "---\nname: alpha\ndescription: d\n---\nbody\n"
	// What kubo uploads: the source plus the version key UAR requires.
	md := withUARSkillVersion(src)
	cases := map[string]func(u *fakeUARRelease){
		"different content": func(u *fakeUARRelease) { u.skills["alpha"] = md + "tampered\n" },
		"no digest":         func(u *fakeUARRelease) { u.skills["alpha"] = md; u.noDigest["alpha"] = true },
		"other digest":      func(u *fakeUARRelease) { u.skills["alpha"] = md; u.digestOverride["alpha"] = "sha256:00" },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			e := newUARProxyEnv(t, replay(t, "run-text.sse"), uarProxyOpts{skillsDir: oneSkillDir(t, "alpha", src)})
			e.fake.edit(setup)
			r := e.say("t1", "r1", "hello")
			assert.Equal(t, http.StatusBadGateway, r.Status, r.Body)
			assert.Contains(t, r.Body, `skill \"alpha\"`)
			assert.Empty(t, e.fake.agents, "no agent is written")
			assert.Empty(t, e.fake.bindings["kubo-ipfs"], "nothing is bound")
			assert.Zero(t, e.fake.runCount())
		})
	}
	t.Run("same content is reused", func(t *testing.T) {
		e := newUARProxyEnv(t, replay(t, "run-text.sse"), uarProxyOpts{skillsDir: oneSkillDir(t, "alpha", src)})
		e.fake.edit(func(u *fakeUARRelease) { u.skills["alpha"] = md })
		require.Equal(t, http.StatusOK, e.say("t1", "r1", "hello").Status)
		assert.Equal(t, []string{"alpha"}, e.fake.bindings["kubo-ipfs"])
	})
}

// ---- U10: SKILL.md must be a regular file ----

func TestLoadUARSkillsRefusesNonRegularSkillFiles(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(secret, []byte("not a skill"), 0o600))

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "linked"), 0o755))
	require.NoError(t, os.Symlink(secret, filepath.Join(dir, "linked", "SKILL.md")))
	_, err := loadUARSkills(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a regular file")

	dir = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "odd", "SKILL.md"), 0o755))
	_, err = loadUARSkills(dir)
	require.Error(t, err, "a directory named SKILL.md")

	dir = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "big"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big", "SKILL.md"), bytes.Repeat([]byte("x"), maxSkillBytes+1), 0o600))
	_, err = loadUARSkills(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "larger than")

	dir = oneSkillDir(t, "fine", "---\nname: fine\n---\nx\n")
	got, err := loadUARSkills(dir)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "fine", got[0].Name)
}

// ---- U11: bounds across hops ----

func TestUARRequestByteBudgetCoversAllHops(t *testing.T) {
	pad := func(n int) []byte {
		line := fmt.Sprintf(`{"type":"TEXT_MESSAGE_CONTENT","messageId":"m","delta":%q}`, strings.Repeat("A", 3<<20))
		var evs []string
		for range n {
			evs = append(evs, line)
		}
		return sseOf(evs...)
	}
	interrupt := sseOf(string(mustLine(t, "run-url-elicitation-interrupt.sse", "RUN_FINISHED")))
	t.Run("one hop", func(t *testing.T) {
		e := newUARProxyEnv(t, scriptOf(ok(append(sseOf(`{"type":"RUN_STARTED","threadId":"u","runId":"u"}`), pad(maxUARRequestBytes/(3<<20)+2)...))), uarProxyOpts{})
		r := e.say("t1", "r1", "go")
		require.Equal(t, http.StatusOK, r.Status)
		last := r.Events[len(r.Events)-1]
		assert.Equal(t, "RUN_ERROR", last["type"])
		assert.Equal(t, 1, e.fake.runCount())
	})
	t.Run("across hops", func(t *testing.T) {
		// Each hop alone is well under the budget; together they are not.
		hop := append(sseOf(`{"type":"RUN_STARTED","threadId":"u","runId":"u"}`), pad(4)...)
		hop = append(hop, interrupt...)
		e := newUARProxyEnv(t, scriptOf(ok(hop)), uarProxyOpts{})
		r := e.say("t1", "r1", "go")
		require.Equal(t, http.StatusOK, r.Status)
		last := r.Events[len(r.Events)-1]
		assert.Equal(t, "RUN_ERROR", last["type"])
		assert.Contains(t, last["message"], "more data than one request allows")
		assert.Equal(t, 3, e.fake.runCount(), "the third hop crosses the request budget")
	})
}

// mustLine returns the first data line of a fixture whose event has the type.
func mustLine(t *testing.T, fixture, typ string) []byte {
	t.Helper()
	for _, line := range strings.Split(string(uarReleaseFixture(t, fixture)), "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if ok && strings.Contains(data, `"type":"`+typ+`"`) {
			return []byte(data)
		}
	}
	t.Fatalf("no %s event in %s", typ, fixture)
	return nil
}

// rtFunc is an http.RoundTripper, so a test can run the proxy on a virtual
// clock without sockets.
type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(body)), Request: r,
	}
}

// newUARBackendRT builds a UAR backend whose instance is the round tripper.
func newUARBackendRT(t *testing.T, rt http.RoundTripper, wall time.Duration) (*Server, *uarBackend) {
	t.Helper()
	t.Setenv(uarTestCredEnv, fakeUARToken)
	t.Setenv(uarTestMCPEnv, uarTestMCPToken)
	cfg := mustParse(t, uarConfigJSON("http://127.0.0.1:1", uarProxyOpts{}))
	set, err := cfg.ResolveAgent(LLMOverrides{})
	require.NoError(t, err)
	set.HTTPClient = &http.Client{Transport: rt}
	set.Limits.WallClock = wall
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := mustNew(t, catFake(t, nil), Options{
		Tools: []string{"node", "content", "pin"}, Resolvers: []Resolver{newFakeResolver(ResolverLinuxPolkit)},
		Transport: "http", Logger: log,
	})
	require.NoError(t, s.EnableAgent(set))
	b := s.UARBackend()
	require.NotNil(t, b)
	return s, b
}

func TestUARWallClockIsOneDeadlinePerIslandRequest(t *testing.T) {
	const wall, perHop = 100 * time.Second, 40 * time.Second
	interrupt := append(sseOf(`{"type":"RUN_STARTED","threadId":"u","runId":"u"}`), sseOf(string(mustLine(t, "run-url-elicitation-interrupt.sse", "RUN_FINISHED")))...)
	var requests int
	rt := rtFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		pr, pw := io.Pipe()
		go func() {
			select {
			case <-time.After(perHop):
				_, _ = pw.Write(interrupt)
				_ = pw.Close()
			case <-r.Context().Done():
				_ = pw.CloseWithError(r.Context().Err())
			}
		}()
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: pr, Request: r}, nil
	})
	s, b := newUARBackendRT(t, rt, wall)
	b.provisioned.Store(true)
	h := &aguiHandler{srv: s, log: s.log}
	id := aguiIdentity("chat", KindUI, []string{"pin"}, nil, []string{"local"})
	raw, err := json.Marshal(aguiBody("t1", "r1", []any{userMsg("m1", "go")}))
	require.NoError(t, err)

	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, RouteAGUI, bytes.NewReader(raw))
		b.serve(h, rec, req, id)
		took := time.Since(start)

		// Three hops of 40s would be 120s; the one deadline cuts the third at 100s.
		assert.Equal(t, wall, took)
		assert.Equal(t, 3, requests)
		res := readAGUIResponse(t, rec.Result())
		last := res.Events[len(res.Events)-1]
		assert.Equal(t, "RUN_ERROR", last["type"])
		assert.Equal(t, "The run reached the time limit.", last["message"])
	})
}

func TestUARProvisioningDoesNotHoldEveryChat(t *testing.T) {
	var release chan struct{}
	rt := rtFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Path == uarCardPath:
			<-release // the instance is slow to answer the first chat
			return jsonResponse(r, http.StatusOK, `{"version":"1.1.0"}`), nil
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, uarAgentsPath+"/"):
			return jsonResponse(r, http.StatusNotFound, `{}`), nil
		case r.Method == http.MethodPost && r.URL.Path == uarAgentsPath:
			return jsonResponse(r, http.StatusCreated, `{}`), nil
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/skills"):
			return jsonResponse(r, http.StatusOK, `[]`), nil
		}
		return jsonResponse(r, http.StatusNotFound, `{}`), nil
	})
	_, b := newUARBackendRT(t, rt, time.Minute)

	synctest.Test(t, func(t *testing.T) {
		// Channels made outside the bubble would not let virtual time advance.
		release = make(chan struct{})
		b.provSem = make(chan struct{}, 1)
		first := make(chan error, 1)
		go func() { first <- b.ensureProvisioned(t.Context()) }()
		synctest.Wait() // the first chat holds the slot, blocked in the card request

		start := time.Now()
		err := b.ensureProvisioned(t.Context())
		assert.ErrorIs(t, err, errUARProvisioning, "a second chat fails fast instead of queueing")
		assert.Equal(t, uarProvisionWait, time.Since(start))

		// A chat whose own deadline is sooner ends at that deadline.
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		start = time.Now()
		assert.ErrorIs(t, b.ensureProvisioned(ctx), context.DeadlineExceeded)
		assert.Equal(t, time.Second, time.Since(start))

		close(release)
		require.NoError(t, <-first)
		assert.True(t, b.provisioned.Load())
		start = time.Now()
		require.NoError(t, b.ensureProvisioned(t.Context()))
		assert.Zero(t, time.Since(start), "once provisioned, chats do not touch the slot")
	})
}

// ---- R8-03: open interrupts are forgotten only when UAR settled them ----

func TestUARFailedResumeKeepsTheInterruptOpenAndRetryable(t *testing.T) {
	interrupt := uarReleaseFixture(t, "run-tool-approval-interrupt.sse")
	executed := uarReleaseFixture(t, "run-resumed-executed.sse")
	text := uarReleaseFixture(t, "run-text.sse")
	for name, failure := range map[string]scriptAnswer{
		"bad gateway":  status(http.StatusBadGateway),
		"unauthorized": status(http.StatusUnauthorized),
		"rate limited": status(http.StatusTooManyRequests),
	} {
		t.Run(name, func(t *testing.T) {
			e := newUARProxyEnv(t, scriptOf(ok(interrupt), failure, ok(executed), ok(text)), uarProxyOpts{})
			in := finishedInterrupt(t, e.say("t1", "r1", "pin").Events)

			r := e.answer("t1", "r2", in["id"].(string), true)
			assert.Equal(t, http.StatusBadGateway, r.Status, r.Body)
			require.Equal(t, 2, e.fake.runCount())

			// UAR still holds the interrupt, so a new message must not reach its thread.
			r = e.say("t1", "r3", "something else")
			assert.Equal(t, http.StatusConflict, r.Status, r.Body)
			assert.Equal(t, 2, e.fake.runCount(), "the message never reached UAR")

			// The person answers again; the parent is still the run that raised it.
			r = e.answer("t1", "r4", in["id"].(string), true)
			require.Equal(t, http.StatusOK, r.Status, r.Body)
			retry := e.fake.run(2)
			assert.Equal(t, "r1", retry.Body["parentRunId"])
			assert.Equal(t, e.fake.run(0).Body["threadId"], retry.Body["threadId"])
			require.Len(t, resumeEntries(t, retry), 1)
			assert.Equal(t, map[string]any{"interruptId": "int-ta-1", "status": "resolved", "payload": map[string]any{"approved": true}}, resumeEntries(t, retry)[0])

			// Settled: the thread takes a new message on the same UAR thread.
			require.Equal(t, http.StatusOK, e.say("t1", "r5", "thanks").Status)
			assert.Equal(t, e.fake.run(0).Body["threadId"], e.fake.run(3).Body["threadId"])
		})
	}
}

func TestUARRefusedResumeAbandonsTheThreadOnAFreshUARThread(t *testing.T) {
	interrupt := uarReleaseFixture(t, "run-tool-approval-interrupt.sse")
	text := uarReleaseFixture(t, "run-text.sse")
	for _, code := range []int{http.StatusConflict, http.StatusNotFound, http.StatusGone, http.StatusUnprocessableEntity} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			e := newUARProxyEnv(t, scriptOf(ok(interrupt), status(code), ok(text)), uarProxyOpts{})
			in := finishedInterrupt(t, e.say("t1", "r1", "pin").Events)
			r := e.answer("t1", "r2", in["id"].(string), true)
			assert.Equal(t, http.StatusBadGateway, r.Status, r.Body)

			// UAR said the interrupt cannot be resumed: the thread starts over, on a
			// UAR thread that does not hold it.
			r = e.say("t1", "r3", "start again")
			require.Equal(t, http.StatusOK, r.Status, r.Body)
			assert.NotEqual(t, e.fake.run(0).Body["threadId"], e.fake.run(2).Body["threadId"])
		})
	}
}

func TestUARAFailedHopOfInterruptsThatNeedNoPersonAbandonsTheThread(t *testing.T) {
	e := newUARProxyEnv(t, scriptOf(
		ok(uarReleaseFixture(t, "run-url-elicitation-interrupt.sse")), status(http.StatusBadGateway), ok(uarReleaseFixture(t, "run-text.sse")),
	), uarProxyOpts{})
	r := e.say("t1", "r1", "go")
	require.Equal(t, http.StatusOK, r.Status, "the stream had begun")
	assert.Equal(t, "RUN_ERROR", r.Events[len(r.Events)-1]["type"])
	// Nothing can answer the interrupt that was left, so it must not block the thread.
	r = e.say("t1", "r2", "again")
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	assert.NotEqual(t, e.fake.run(0).Body["threadId"], e.fake.run(2).Body["threadId"])
}

func TestUARThreadIDsDifferAfterABackendRebuildAndAfterEviction(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-text.sse"), uarProxyOpts{})
	require.Equal(t, http.StatusOK, e.say("t1", "r1", "hi").Status)
	require.NoError(t, e.s.ApplyAgentSettings(e.set))
	require.Equal(t, http.StatusOK, e.say("t1", "r2", "hi").Status)
	assert.NotEqual(t, e.fake.run(0).Body["threadId"], e.fake.run(1).Body["threadId"], "a rebuilt backend maps the thread to a fresh UAR thread")

	b := e.s.UARBackend()
	th, release, err := b.acquireThread("chat", "evict-me", true)
	require.NoError(t, err)
	first := th.uarID
	release()
	b.mu.Lock()
	now := b.now()
	b.mu.Unlock()
	b.now = func() time.Time { return now.Add(2 * sessionIdleTTL) }
	_, release, err = b.acquireThread("chat", "other", true) // sweeps the idle thread
	require.NoError(t, err)
	release()
	th, release, err = b.acquireThread("chat", "evict-me", true)
	require.NoError(t, err)
	defer release()
	assert.NotEqual(t, first, th.uarID, "an evicted thread comes back on a fresh UAR thread")
}
