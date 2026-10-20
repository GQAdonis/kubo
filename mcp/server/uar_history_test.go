//go:build !noagent

package server

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func uarHistoryBackend(t *testing.T, clock *fakeClock) *uarBackend {
	t.Helper()
	t.Setenv("KUBO_MCP_HISTORY_FIXTURE_TOKEN", "history-fixture-only")
	return &uarBackend{
		srv: &Server{gate: &gate{broker: newBroker(clock.Now, confirmTTL)}}, now: clock.Now,
		cli: newUARClient("http://127.0.0.1:1", nil, nil), set: &AgentSettings{UAR: &UARSettings{MCPCredential: &SecretRef{Env: "KUBO_MCP_HISTORY_FIXTURE_TOKEN"}}},
	}
}

func TestUARHistorySyntheticIDsSeparateOwnedThreadsAndPreserveBackendResumeID(t *testing.T) {
	clock := newFakeClock()
	b := uarHistoryBackend(t, clock)
	list := []any{map[string]any{"id": "same-backend-id", "reason": uarReasonToolApproval, "toolCallId": "tc"}}
	var ids []string
	for _, pair := range []struct{ subject, thread string }{{"alice", "one"}, {"alice", "two"}, {"bob", "one"}} {
		owner := Principal{ID: "shared", Kind: KindUI, sub: pair.subject}
		th := &uarThread{principal: owner.ID, owner: owner.BindingKey(), client: pair.thread, lastRun: "same-run", tools: map[string]string{"tc": "pin_add"}}
		pending, err := b.parseInterrupts(th, list)
		require.NoError(t, err)
		th.pending = pending
		id := pending[0].confirmationID()
		assert.True(t, strings.HasPrefix(id, "cf_uar_"))
		assert.NotContains(t, ids, id)
		ids = append(ids, id)
		clock.Advance(time.Second)
		refreshed, err := b.parseInterrupts(th, list)
		require.NoError(t, err)
		assert.Equal(t, id, refreshed[0].confirmationID())
		assert.Equal(t, pending[0].createdAt, refreshed[0].createdAt)
		assert.Equal(t, "same-backend-id", refreshed[0].ID)
		th.pending = refreshed
		th.pending[0].Answer = &uarAnswer{Status: ResumeResolved, Payload: map[string]any{"approved": true}, approved: true, ResolvedBy: ResolverAGUIInterrupt}
		// The transport payload remains the original UAR interruption; no
		// synthetic decision ID, owner or attribution is sent to the backend.
		entry := b.resumeBody(th, "resume")["resume"].([]any)[0].(map[string]any)
		assert.Equal(t, "same-backend-id", entry["interruptId"])
		assert.NotContains(t, entry, "resolved_by")
		b.settle(th, relayResult{responded: true})
		p, ok := b.srv.gate.broker.observedFor(owner.BindingKey(), id)
		require.True(t, ok)
		assert.Equal(t, SourceKindBackendApproval, p.SourceKind)
		assert.Equal(t, StateExecuted, p.State)
		assert.Nil(t, p.Execution, "accepted backend approval is not tool-success evidence")
	}
	assert.Len(t, b.srv.gate.broker.retained, 3)
	assert.Len(t, b.srv.gate.broker.snapshotFor((Principal{ID: "shared", sub: "alice"}).BindingKey()).Items, 2)
	assert.Len(t, b.srv.gate.broker.snapshotFor((Principal{ID: "shared", sub: "bob"}).BindingKey()).Items, 1)
}

func TestUARHistorySettlesExactlyOnceWithoutRejuvenatingOrSynthesizingOutcome(t *testing.T) {
	clock := newFakeClock()
	b := uarHistoryBackend(t, clock)
	owner := Principal{ID: "chat", Kind: KindUI, sub: "private"}
	th := &uarThread{principal: owner.ID, owner: owner.BindingKey(), tools: map[string]string{"tc": "pin_add"}, lastRun: "run"}
	parsed, err := b.parseInterrupts(th, []any{map[string]any{"id": "backend", "reason": uarReasonToolApproval, "toolCallId": "tc", "message": "PRIVATE BACKEND PROSE"}})
	require.NoError(t, err)
	th.pending, th.shown = parsed, parsed[0].ShownID
	parsed[0].Answer = &uarAnswer{Status: ResumeResolved, Payload: map[string]any{"approved": false}, ResolvedBy: ResolverAGUIInterrupt, ResolvedSurface: &ResolvingSurface{Kind: "inbox", Instance: "pane"}}
	id := parsed[0].confirmationID()
	b.settle(th, relayResult{responded: true})
	first := b.srv.gate.broker.snapshotFor(owner.BindingKey())
	require.Len(t, first.Items, 1)
	assert.Equal(t, StateDenied, first.Items[0].State)
	assert.NotContains(t, string(asJSON(t, first)), "PRIVATE BACKEND PROSE")
	assert.NotContains(t, string(asJSON(t, first)), owner.sub)
	clock.Advance(time.Hour)
	th.pending = parsed // repeated ingestion of the same settled source object
	b.retainSettled(th, false)
	assert.Equal(t, first.Revision, b.srv.gate.broker.snapshotFor(owner.BindingKey()).Revision)
	assert.Len(t, b.srv.gate.broker.retained, 1)
	clock.Advance(decisionHistoryTTL)
	_, ok := b.srv.gate.broker.observedFor(owner.BindingKey(), id)
	assert.False(t, ok)
	b.retainSettled(th, false)
	assert.Empty(t, b.srv.gate.broker.retained, "source deduplication survives ring eviction")
}

func TestUARHistoryUnacknowledgedResumeRemainsOpenWithoutTerminalRecord(t *testing.T) {
	clock := newFakeClock()
	b := uarHistoryBackend(t, clock)
	p, ok := classifyUARInterrupt(map[string]any{"id": "backend", "reason": uarReasonToolApproval}, nil)
	require.True(t, ok)
	p.createdAt = clock.Now()
	p.Answer = &uarAnswer{Status: ResumeResolved, Payload: map[string]any{"approved": true}, approved: true, ResolvedBy: ResolverAGUIInterrupt}
	th := &uarThread{principal: "chat", owner: "chat", pending: []*uarPending{p}, shown: p.ShownID}
	b.settle(th, relayResult{err: &uarBackendError{"Transport failed."}})
	assert.Nil(t, p.Answer, "existing retry/reopen behavior stays authoritative")
	assert.Empty(t, b.srv.gate.broker.retained)
	b.retainSettled(th, true)
	item, ok := b.srv.gate.broker.observedFor("chat", p.confirmationID())
	require.True(t, ok)
	assert.Equal(t, StateDenied, item.State)
	assert.Empty(t, item.ResolvedBy, "automatic withdrawal invents no human answer")
	assert.Nil(t, item.ResolvedSurface)
	assert.Nil(t, item.Execution)
}

func TestUARHistorySharesNodeGlobalCapacityWithoutNodeAuthority(t *testing.T) {
	clock := newFakeClock()
	b := uarHistoryBackend(t, clock)
	broker := b.srv.gate.broker
	node := historyPending(broker, Principal{ID: "node-owner", Kind: KindUI})
	require.NoError(t, broker.Resolve(node.ID, ResolverAGUIInterrupt, false))
	for range decisionHistoryLimit {
		parsed, err := b.parseInterrupts(&uarThread{}, []any{map[string]any{"id": "same-backend-id", "reason": uarReasonToolApproval}})
		require.NoError(t, err)
		parsed[0].Answer = &uarAnswer{Status: ResumeResolved, Payload: map[string]any{"approved": true}, approved: true}
		th := &uarThread{principal: "backend-owner", owner: "backend-owner", pending: parsed}
		b.settle(th, relayResult{responded: true})
	}
	assert.Len(t, broker.retained, decisionHistoryLimit)
	assert.Empty(t, broker.snapshotFor("node-owner").Items)
	for _, p := range broker.snapshotFor("backend-owner").Items {
		assert.Equal(t, SourceKindBackendApproval, p.SourceKind)
		assert.Nil(t, p.Execution)
		require.ErrorIs(t, broker.Resolve(p.ID, ResolverAGUIInterrupt, true), ErrUnknownConfirmation, "history is not broker authority")
	}
}
