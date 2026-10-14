//go:build !noagent

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// swapTo points the agent at a different backend the way a settings apply
// will: a new snapshot built from the same settings shape, swapped atomically.
func (e *agentEnv) swapTo(t *testing.T, m *scriptedModel) {
	t.Helper()
	snap, err := newSnapshot(AgentSettings{BaseURL: m.baseURL(), Model: "swapped-model", Timeout: 5 * time.Second})
	require.NoError(t, err)
	e.a.swapSnapshot(snap)
}

// gateStep answers with payload, but only once release is closed; started is
// closed when the request arrives, so the test can swap the backend while the
// run is in flight.
func gateStep(started chan<- struct{}, release <-chan struct{}, payload func(modelRequest) []string) func(modelRequest) []string {
	var once sync.Once
	return func(r modelRequest) []string {
		once.Do(func() { close(started) })
		<-release
		return payload(r)
	}
}

func TestAgentSwapResumeUsesOriginalBackend(t *testing.T) {
	t.Setenv("IPFS_MCP_TEST_SWAP_KEY", "old-key")
	e := newAgentEnv(t, agentEnvOpts{key: &SecretRef{Env: "IPFS_MCP_TEST_SWAP_KEY"}},
		callStep(10, pinAddCall(t)), sayStep("Pinned on the old backend."))
	id := uiID(nil, "pin")

	res, _ := e.run(id, "pin it")
	require.Equal(t, OutcomeInterrupt, res.Outcome)
	in := res.Interrupts[0]
	old := e.a.snap.Load()

	newModel := newScriptedModel(t, sayStep("on the new backend"))
	e.swapTo(t, newModel)
	assert.Equal(t, secret("old-key"), old.llm.key, "the open interrupt retains the old backend for its resume")

	_, resolve := e.resolver.last(t)
	require.NoError(t, resolve(true))
	res2, _, err := e.resume(id, res.SessionID, ResumeEntry{InterruptID: in.ID, Status: ResumeResolved, Approved: true})
	require.NoError(t, err)
	assert.Equal(t, OutcomeSuccess, res2.Outcome)
	assert.Equal(t, "Pinned on the old backend.", res2.Final)
	assert.Equal(t, 2, e.model.hits(), "the resumed run asked the original backend again")
	assert.Zero(t, newModel.hits(), "the resumed run never touched the new backend")
	assert.Equal(t, 1, e.d.count("pin/add"), "the write ran exactly once")
	assert.Empty(t, string(old.llm.key), "no run uses the old snapshot anymore: its key is dropped")
}

func TestAgentSwapMidStream(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	e := newAgentEnv(t, agentEnvOpts{}, gateStep(started, release, sayStep("finished on the old backend")))
	id := uiID(nil)

	type outcome struct {
		res *RunResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := e.a.Run(t.Context(), id, RunInput{Message: "hi"}, nil)
		done <- outcome{res, err}
	}()
	<-started // the run is in flight on the old backend

	newModel := newScriptedModel(t, sayStep("new backend"))
	e.swapTo(t, newModel)
	unblock()

	out := <-done
	require.NoError(t, out.err)
	assert.Equal(t, OutcomeSuccess, out.res.Outcome)
	assert.Equal(t, "finished on the old backend", out.res.Final, "the in-flight run completed on the backend it started with")
	assert.Equal(t, 1, e.model.hits())
	assert.Zero(t, newModel.hits())

	res2, _ := e.run(id, "again")
	assert.Equal(t, "new backend", res2.Final, "the next run uses the new backend")
	assert.Equal(t, 1, newModel.hits())
	assert.Equal(t, 1, e.model.hits())
}

func TestAgentSwapKeepsPendingConfirmation(t *testing.T) {
	e := newAgentEnv(t, agentEnvOpts{}, callStep(10, pinAddCall(t)), sayStep("Pinned."))
	id := uiID(nil, "pin")

	res, _ := e.run(id, "pin it")
	require.Equal(t, OutcomeInterrupt, res.Outcome)
	in := res.Interrupts[0]

	e.swapTo(t, newScriptedModel(t, sayStep("new backend")))

	// The confirmation opened before the swap still resolves, exactly once.
	require.NoError(t, e.s.ResolveConfirmation(in.ConfirmationID, ResolverLinuxPolkit, true))
	require.ErrorIs(t, e.s.ResolveConfirmation(in.ConfirmationID, ResolverLinuxPolkit, true), ErrConfirmationResolved)

	res2, _, err := e.resume(id, res.SessionID, ResumeEntry{InterruptID: in.ID, Status: ResumeResolved, Approved: true})
	require.NoError(t, err)
	assert.Equal(t, OutcomeSuccess, res2.Outcome)
	assert.Equal(t, 1, e.d.count("pin/add"), "the confirmed write ran exactly once")
	assert.True(t, e.pins.has(testCID(t, 5)))
}

func TestAgentSwapDropsPreviousKey(t *testing.T) {
	t.Run("no run in flight", func(t *testing.T) {
		t.Setenv("IPFS_MCP_TEST_SWAP_KEY", "k1")
		e := newAgentEnv(t, agentEnvOpts{key: &SecretRef{Env: "IPFS_MCP_TEST_SWAP_KEY"}})
		old := e.a.snap.Load()
		require.Equal(t, secret("k1"), old.llm.key)

		e.swapTo(t, newScriptedModel(t))
		assert.Empty(t, string(old.llm.key), "no run uses the swapped-out snapshot: its key is dropped at once")
	})

	t.Run("evicted session releases the retained snapshot", func(t *testing.T) {
		t.Setenv("IPFS_MCP_TEST_SWAP_KEY", "k1")
		e := newAgentEnv(t, agentEnvOpts{key: &SecretRef{Env: "IPFS_MCP_TEST_SWAP_KEY"}}, callStep(10, pinAddCall(t)))
		id := uiID(nil, "pin")
		res, _ := e.run(id, "pin it")
		require.Equal(t, OutcomeInterrupt, res.Outcome)
		old := e.a.snap.Load()

		e.swapTo(t, newScriptedModel(t))
		assert.Equal(t, secret("k1"), old.llm.key, "the open interrupt still retains the snapshot")

		// Idle eviction drops the session; nothing is left that could resume
		// onto the old backend, so its key is dropped.
		e.clock.Advance(sessionIdleTTL + time.Minute)
		_, _, err := e.resume(id, "other-session", ResumeEntry{InterruptID: "int-x", Status: ResumeResolved})
		require.ErrorIs(t, err, ErrSessionNotFound)
		assert.Empty(t, string(old.llm.key))
	})
}

// aguiRunOver posts one AG-UI run to the listener of an a2aEnv, which mounts
// both agent transports over one Agent, and reads the whole response.
func aguiRunOver(t *testing.T, e *a2aEnv, thread, runID string) *aguiResponse {
	t.Helper()
	body, err := json.Marshal(aguiBody(thread, runID, []any{userMsg("m-"+runID, "hello")}))
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, e.base+RouteAGUI, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+a2aTokChat)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	return readAGUIResponse(t, resp)
}

// aguiText joins the text deltas of a run's reply.
func aguiText(r *aguiResponse) string {
	var b strings.Builder
	for _, ev := range findEvents(r.Events, "TEXT_MESSAGE_CONTENT") {
		b.WriteString(ev["delta"].(string))
	}
	return b.String()
}

// TestAgentSwapSharedByAGUIAndA2A is the task 1.1 transport test: AG-UI and
// A2A read the same snapshot pointer, so a swap made through the apply entry
// point is visible to both at once and neither keeps its own backend.
func TestAgentSwapSharedByAGUIAndA2A(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{}, sayStep("old backend for the chat"), sayStep("old backend for the peer"))

	first := aguiRunOver(t, e, "thread-swap", "run-1")
	require.Equal(t, http.StatusOK, first.Status, first.Body)
	assert.Equal(t, "old backend for the chat", aguiText(first))
	task := taskOf(t, e.send(a2aTokPeer, textPartJSON("hello over a2a")))
	require.Equal(t, a2aCompleted, stateOf(task))
	art := task["artifacts"].([]any)[0].(map[string]any)
	assert.Equal(t, "old backend for the peer", art["parts"].([]any)[0].(map[string]any)["text"])
	require.Equal(t, 2, e.model.hits())

	// Apply a new backend the way the settings API does (task 1.5).
	newModel := newScriptedModel(t, sayStep("new backend for the chat"), sayStep("new backend for the peer"))
	require.NoError(t, e.s.ApplyAgentSettings(&AgentSettings{BaseURL: newModel.baseURL(), Model: "swapped-model", Timeout: 5 * time.Second}))

	second := aguiRunOver(t, e, "thread-swap", "run-2")
	require.Equal(t, http.StatusOK, second.Status, second.Body)
	assert.Equal(t, "new backend for the chat", aguiText(second))
	task2 := taskOf(t, e.send(a2aTokPeer, textPartJSON("hello again")))
	require.Equal(t, a2aCompleted, stateOf(task2))
	art2 := task2["artifacts"].([]any)[0].(map[string]any)
	assert.Equal(t, "new backend for the peer", art2["parts"].([]any)[0].(map[string]any)["text"])

	assert.Equal(t, 2, e.model.hits(), "no run after the swap touched the old backend")
	assert.Equal(t, 2, newModel.hits(), "both transports read the swapped snapshot")
}

// TestAgentSwapKeepsSessionsAndRateState is the rest of task 1.1: sessions and
// rate buckets live on the Agent, not on the snapshot, so a swap loses neither
// a conversation nor a principal's rate state.
func TestAgentSwapKeepsSessionsAndRateState(t *testing.T) {
	e := newAgentEnv(t, agentEnvOpts{}, sayStep("one"))
	e.a.runBurst, e.a.runsPerMinute = 1, 60
	id := uiID(nil)

	res, _ := e.run(id, "first")
	require.Equal(t, OutcomeSuccess, res.Outcome)
	e.a.mu.Lock()
	sessionsBefore := len(e.a.sessions)
	e.a.mu.Unlock()
	require.Equal(t, 1, sessionsBefore)

	newModel := newScriptedModel(t, sayStep("two"))
	e.swapTo(t, newModel)

	e.a.mu.Lock()
	assert.Len(t, e.a.sessions, sessionsBefore, "a backend swap keeps the session table")
	e.a.mu.Unlock()

	// The burst is spent and no time has passed: the same principal is still
	// rate-limited, and the refused run never reaches the new backend.
	_, err := e.a.Run(t.Context(), id, RunInput{Message: "second"}, nil)
	require.ErrorIs(t, err, ErrRateLimited)
	assert.Zero(t, newModel.hits())
}
