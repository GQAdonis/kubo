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

// swapModelOnly is a settings apply that changes the model name and nothing
// that identifies the backend: same endpoint, same key.
func (e *agentEnv) swapModelOnly(t *testing.T, key *SecretRef) {
	t.Helper()
	snap, err := newSnapshot(AgentSettings{BaseURL: e.model.baseURL(), Model: "other-model", APIKey: key, Timeout: 5 * time.Second})
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

// A swap that changes only the model (or the prompt) keeps the open interrupt
// on the backend its run started with: same endpoint, same key.
func TestAgentSwapResumeUsesOriginalBackend(t *testing.T) {
	t.Setenv("IPFS_MCP_TEST_SWAP_KEY", "old-key")
	key := &SecretRef{Env: "IPFS_MCP_TEST_SWAP_KEY"}
	e := newAgentEnv(t, agentEnvOpts{key: key},
		callStep(10, pinAddCall(t)), sayStep("Pinned on the old backend."))
	id := uiID(nil, "pin")

	res, _ := e.run(id, "pin it")
	require.Equal(t, OutcomeInterrupt, res.Outcome)
	in := res.Interrupts[0]
	old := e.a.snap.Load()

	e.swapModelOnly(t, key)
	assert.Equal(t, secret("old-key"), old.llm.key, "the open interrupt retains the old snapshot for its resume")

	_, resolve := e.resolver.last(t)
	require.NoError(t, resolve(true))
	res2, _, err := e.resume(id, res.SessionID, ResumeEntry{InterruptID: in.ID, Status: ResumeResolved, Approved: true})
	require.NoError(t, err)
	assert.Equal(t, OutcomeSuccess, res2.Outcome)
	assert.Equal(t, "Pinned on the old backend.", res2.Final)
	assert.Equal(t, 2, e.model.hits(), "the resumed run asked the original snapshot again")
	assert.Equal(t, 1, e.d.count("pin/add"), "the write ran exactly once")
	assert.Empty(t, string(old.llm.key), "no run uses the old snapshot anymore: its key is dropped")
}

// L5: a swap that moves the endpoint or clears or replaces the key revokes the
// retained snapshot of an open interrupt: the key is dropped at once and the
// resume fails with ErrBackendChanged, never reaching either backend.
func TestAgentSwapRevokesOpenInterrupt(t *testing.T) {
	oldKey := &SecretRef{Env: "IPFS_MCP_TEST_SWAP_KEY"}
	cases := map[string]func(t *testing.T, e *agentEnv, newModel *scriptedModel){
		"endpoint moved": func(t *testing.T, e *agentEnv, newModel *scriptedModel) {
			snap, err := newSnapshot(AgentSettings{BaseURL: newModel.baseURL(), Model: "m", APIKey: oldKey, Timeout: 5 * time.Second})
			require.NoError(t, err)
			e.a.swapSnapshot(snap)
		},
		"key cleared": func(t *testing.T, e *agentEnv, _ *scriptedModel) {
			snap, err := newSnapshot(AgentSettings{BaseURL: e.model.baseURL(), Model: "m", Timeout: 5 * time.Second})
			require.NoError(t, err)
			e.a.swapSnapshot(snap)
		},
		"key replaced": func(t *testing.T, e *agentEnv, _ *scriptedModel) {
			t.Setenv("IPFS_MCP_TEST_SWAP_KEY_2", "new-key")
			snap, err := newSnapshot(AgentSettings{BaseURL: e.model.baseURL(), Model: "m", APIKey: &SecretRef{Env: "IPFS_MCP_TEST_SWAP_KEY_2"}, Timeout: 5 * time.Second})
			require.NoError(t, err)
			e.a.swapSnapshot(snap)
		},
	}
	for name, swap := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("IPFS_MCP_TEST_SWAP_KEY", "old-key")
			e := newAgentEnv(t, agentEnvOpts{key: oldKey}, callStep(10, pinAddCall(t)), sayStep("must not run"))
			id := uiID(nil, "pin")
			res, _ := e.run(id, "pin it")
			require.Equal(t, OutcomeInterrupt, res.Outcome)
			in := res.Interrupts[0]
			old := e.a.snap.Load()
			require.Equal(t, secret("old-key"), old.llm.key)

			newModel := newScriptedModel(t, sayStep("new backend"))
			swap(t, e, newModel)
			assert.Empty(t, string(old.llm.key), "the old key leaves the retained snapshot at the swap")

			_, resolve := e.resolver.last(t)
			require.NoError(t, resolve(true))
			_, _, err := e.resume(id, res.SessionID, ResumeEntry{InterruptID: in.ID, Status: ResumeResolved, Approved: true})
			require.ErrorIs(t, err, ErrBackendChanged)
			assert.Equal(t, 1, e.model.hits(), "the resume never reached the old backend")
			assert.Zero(t, newModel.hits(), "nor the new one")
			assert.Zero(t, e.d.count("pin/add"), "the confirmed write did not run on a backend the operator moved away from")

			// The error is stable: asking again gives the same answer, and a new
			// message starts a fresh run on the current backend.
			_, _, err = e.resume(id, res.SessionID, ResumeEntry{InterruptID: in.ID, Status: ResumeResolved, Approved: true})
			require.ErrorIs(t, err, ErrBackendChanged)

			// The confirmation was approved before the revoked resume; the session
			// still accepts a new message at once, on the current backend (N2).
			res2, err := e.a.Run(t.Context(), id, RunInput{SessionID: res.SessionID, Message: "carry on"}, nil)
			require.NoError(t, err)
			assert.Equal(t, OutcomeSuccess, res2.Outcome)
			assert.Zero(t, e.d.count("pin/add"), "the approved write never ran")
		})
	}
}

// A run that was in flight when the endpoint moved and then ends on an
// interrupt must not retain its stale snapshot either.
func TestAgentSwapRevokesInterruptOfRunInFlight(t *testing.T) {
	t.Setenv("IPFS_MCP_TEST_SWAP_KEY", "old-key")
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	e := newAgentEnv(t, agentEnvOpts{key: &SecretRef{Env: "IPFS_MCP_TEST_SWAP_KEY"}},
		gateStep(started, release, callStep(10, pinAddCall(t))), sayStep("must not run"))
	id := uiID(nil, "pin")

	type outcome struct {
		res *RunResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := e.a.Run(t.Context(), id, RunInput{Message: "pin it"}, nil)
		done <- outcome{res, err}
	}()
	<-started
	old := e.a.snap.Load()
	e.swapTo(t, newScriptedModel(t, sayStep("new backend")))
	unblock()

	out := <-done
	require.NoError(t, out.err)
	require.Equal(t, OutcomeInterrupt, out.res.Outcome)
	assert.Empty(t, string(old.llm.key), "the interrupt of a stale run keeps no key")
	_, _, err := e.resume(id, out.res.SessionID, ResumeEntry{InterruptID: out.res.Interrupts[0].ID, Status: ResumeResolved, Approved: true})
	require.ErrorIs(t, err, ErrBackendChanged)
}

// A revoked interrupt's confirmation is denied, so the person is not left with
// a prompt that can no longer run, and a new message continues the session.
func TestAgentSwapRevokedResumeDeniesConfirmation(t *testing.T) {
	e := newAgentEnv(t, agentEnvOpts{}, callStep(10, pinAddCall(t)), sayStep("must not run"))
	id := uiID(nil, "pin")
	res, _ := e.run(id, "pin it")
	require.Equal(t, OutcomeInterrupt, res.Outcome)
	in := res.Interrupts[0]

	newModel := newScriptedModel(t, sayStep("fresh start on the new backend"))
	e.swapTo(t, newModel)
	_, _, err := e.resume(id, res.SessionID, ResumeEntry{InterruptID: in.ID, Status: ResumeResolved, Approved: true})
	require.ErrorIs(t, err, ErrBackendChanged)
	require.ErrorIs(t, e.s.ResolveConfirmation(in.ConfirmationID, ResolverLinuxPolkit, true), ErrConfirmationResolved,
		"the revoked resume already recorded the denial")

	res2, err := e.a.Run(t.Context(), id, RunInput{SessionID: res.SessionID, Message: "carry on"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "fresh start on the new backend", res2.Final)
	assert.Zero(t, e.d.count("pin/add"))
}

// The confirmation of a revoked resume may already be approved. The session
// accepts a new message at once, and the approved call never runs, even if the
// model repeats it (N2).
func TestAgentSwapApprovedRevokedResumeAcceptsNewMessage(t *testing.T) {
	e := newAgentEnv(t, agentEnvOpts{}, callStep(10, pinAddCall(t)), sayStep("must not run"))
	id := uiID(nil, "pin")
	res, _ := e.run(id, "pin it")
	require.Equal(t, OutcomeInterrupt, res.Outcome)
	in := res.Interrupts[0]

	newModel := newScriptedModel(t, callStep(10, pinAddCall(t)), sayStep("done on the new backend"))
	e.swapTo(t, newModel)
	_, resolve := e.resolver.last(t)
	require.NoError(t, resolve(true))
	_, _, err := e.resume(id, res.SessionID, ResumeEntry{InterruptID: in.ID, Status: ResumeResolved, Approved: true})
	require.ErrorIs(t, err, ErrBackendChanged)
	assert.Zero(t, e.d.count("pin/add"))

	res2, err := e.a.Run(t.Context(), id, RunInput{SessionID: res.SessionID, Message: "carry on"}, nil)
	require.NoError(t, err, "a new message is accepted at once, not after the confirmation expires")
	require.Equal(t, OutcomeInterrupt, res2.Outcome, "the repeated call opens a new confirmation")
	assert.NotEqual(t, in.ConfirmationID, res2.Interrupts[0].ConfirmationID, "the revoked approval was not reused")
	assert.Zero(t, e.d.count("pin/add"), "the approved call never ran")
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

	e.swapModelOnly(t, nil)

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

		e.swapModelOnly(t, &SecretRef{Env: "IPFS_MCP_TEST_SWAP_KEY"})
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
