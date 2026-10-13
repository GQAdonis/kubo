//go:build !noagent

package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplyEventAnnouncesEveryCallOnceBeforeAnyRuns(t *testing.T) {
	gc := scriptedCall{ID: "tc-gc", Name: "repo_gc"}
	pin := scriptedCall{ID: "tc-pin", Name: "pin_add", Args: map[string]any{"path": "/ipfs/" + testCID(t, 5)}}
	e := newAgentEnv(t, agentEnvOpts{}, callStep(10, pin, gc), sayStep("done"))
	id := uiID(nil, "pin")
	res, evs := e.run(id, "pin it, then collect garbage")
	require.Equal(t, OutcomeInterrupt, res.Outcome)

	var kinds []EventType
	for _, ev := range evs {
		kinds = append(kinds, ev.Type)
	}
	assert.Equal(t, []EventType{EventRunStarted, EventReply, EventToolCall, EventToolCall, EventInterrupt, EventRunFinished}, kinds,
		"both calls are announced when the reply completes, before the first one is confirmed")
	reply := evs[1]
	require.Len(t, reply.Calls, 2)
	assert.Equal(t, CallInfo{ID: "tc-pin", Tool: "pin_add", Arguments: reply.Calls[0].Arguments}, reply.Calls[0])
	assert.Contains(t, reply.Calls[0].Arguments, testCID(t, 5))
	assert.Equal(t, "tc-gc", reply.Calls[1].ID, "in the order the model asked")

	// Approve and resume: the queued call is not announced again.
	_, resolve := e.resolver.last(t)
	require.NoError(t, resolve(true))
	_, evs2, err := e.resume(id, res.SessionID, ResumeEntry{InterruptID: res.Interrupts[0].ID, Status: ResumeResolved, Approved: true})
	require.NoError(t, err)
	for _, ev := range evs2 {
		assert.NotEqual(t, EventToolCall, ev.Type, "a call is announced by the run that received the reply, never by a later one")
		assert.NotEqual(t, EventReply, ev.Type)
	}
}

func TestAReplyWithoutCallsHasNoReplyEvent(t *testing.T) {
	e := newAgentEnv(t, agentEnvOpts{}, sayStep("just text"))
	_, evs := e.run(uiID(nil), "hi")
	for _, ev := range evs {
		assert.NotEqual(t, EventReply, ev.Type)
	}
}

func TestOpenInterruptIsOwnerChecked(t *testing.T) {
	e := newAgentEnv(t, agentEnvOpts{}, callStep(10, pinAddCall(t)), sayStep("ok"))
	owner := uiID(nil, "pin")
	res, _ := e.run(owner, "pin it")
	require.Equal(t, OutcomeInterrupt, res.Outcome)

	got, err := e.a.OpenInterrupt(owner, res.SessionID)
	require.NoError(t, err)
	assert.Equal(t, res.Interrupts[0].ID, got.ID)
	assert.Equal(t, res.Interrupts[0].ConfirmationID, got.ConfirmationID)
	got.Resolvers = append(got.Resolvers, "tamper")
	again, _ := e.a.OpenInterrupt(owner, res.SessionID)
	assert.NotContains(t, again.Resolvers, "tamper", "the caller gets a copy")

	other := uiID(nil, "pin")
	other.Principal = Principal{ID: "someone-else", Kind: KindUI}
	_, err = e.a.OpenInterrupt(other, res.SessionID)
	require.ErrorIs(t, err, ErrSessionNotFound, "another principal's thread looks like no thread")
	_, err = e.a.OpenInterrupt(owner, "no-such-thread")
	require.ErrorIs(t, err, ErrSessionNotFound)
	_, err = e.a.OpenInterrupt(owner, "")
	require.ErrorIs(t, err, ErrSessionNotFound)

	// Idle sessions expire like everywhere else.
	e.clock.Advance(sessionIdleTTL + time.Minute)
	_, err = e.a.OpenInterrupt(owner, res.SessionID)
	require.ErrorIs(t, err, ErrSessionNotFound)
}

func TestOpenInterruptWithoutAnOpenOneOrWhileRunning(t *testing.T) {
	e := newAgentEnv(t, agentEnvOpts{}, sayStep("hello"))
	id := uiID(nil)
	res, _ := e.run(id, "hi")
	_, err := e.a.OpenInterrupt(id, res.SessionID)
	require.ErrorIs(t, err, ErrNoOpenInterrupt)

	e.a.mu.Lock()
	e.a.sessions[res.SessionID].busy = true
	e.a.mu.Unlock()
	_, err = e.a.OpenInterrupt(id, res.SessionID)
	require.ErrorIs(t, err, ErrSessionBusy)
}
