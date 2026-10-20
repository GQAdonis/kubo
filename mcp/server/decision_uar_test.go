//go:build !noagent

package server

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecisionReadsProjectOwnedUARIdleAuthorityWithoutNodeReceipt(t *testing.T) {
	e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
	b := uarHistoryBackend(t, e.clock)
	b.srv = e.s
	b.threads = map[string]*uarThread{}
	for _, owner := range []string{"chat", "other"} {
		th := &uarThread{principal: owner, owner: owner, client: "same-thread", lastRun: "same-run", touched: e.clock.Now()}
		pending, err := b.parseInterrupts(th, []any{map[string]any{"id": "backend-id", "reason": uarReasonToolApproval, "message": "PRIVATE BACKEND PROSE"}})
		require.NoError(t, err)
		th.pending, th.shown = pending, pending[0].ShownID
		b.threads[uarThreadKey(owner, th.client)] = th
	}
	e.s.agentMu.Lock()
	e.s.uarExt = b
	e.s.agentMu.Unlock()
	w := e.request(t, "GET", RouteDecisions, tokChat, "", nil)
	page := readDecisionList(t, w)
	require.Len(t, page.Items, 1)
	assert.Contains(t, string(page.Items[0]), `"source":"uar"`)
	assert.Contains(t, string(page.Items[0]), `"source_kind":"backend_approval"`)
	assert.NotContains(t, string(page.Items[0]), "PRIVATE BACKEND PROSE")
	assert.NotContains(t, string(page.Items[0]), "backend-id")
	assert.NotContains(t, string(page.Items[0]), "same-thread")
	assert.NotContains(t, string(page.Items[0]), "receipt")
	assert.NotContains(t, string(page.Items[0]), "effect_digest")
	assert.NotContains(t, string(page.Items[0]), `"approve":true`)
	other := b.threads[uarThreadKey("other", "same-thread")].pending[0].confirmationID()
	assert.Equal(t, 404, e.request(t, "GET", RouteDecisions+"/"+other, tokChat, "", nil).Code)
	th := b.threads[uarThreadKey("chat", "same-thread")]
	id := th.pending[0].confirmationID()
	// A thread holding a run slot is not read concurrently with its writer.
	b.mu.Lock()
	th.running = true
	b.mu.Unlock()
	assert.Empty(t, readDecisionList(t, e.request(t, "GET", RouteDecisions, tokChat, "", nil)).Items)
	th.pending[0].Answer = th.pending[0].answerFor(true, false)
	th.pending[0].Answer.ResolvedBy = ResolverAGUIInterrupt
	b.settle(th, relayResult{responded: true})
	b.mu.Lock()
	th.running = false
	b.mu.Unlock()
	w = e.request(t, "GET", RouteDecisions+"/"+id, tokChat, "", nil)
	require.Equal(t, 200, w.Code)
	validateDecisionRead(t, "ItemResponse", w.Body.Bytes())
	assert.Contains(t, w.Body.String(), `"approval_state":"executed"`)
	assert.NotContains(t, w.Body.String(), `"execution"`)
	assert.NotContains(t, w.Body.String(), `"receipt"`)
	assert.Len(t, readDecisionList(t, e.request(t, "GET", RouteDecisions, tokChat, "", nil)).Items, 1, "one settled observation, no duplicated live history")
}

func TestDecisionUARLiveProjectionHasOnePerThreadAndGlobalBound(t *testing.T) {
	clock := newFakeClock()
	b := uarHistoryBackend(t, clock)
	b.threads = map[string]*uarThread{}
	b.srv.uarExt = b
	for i := range 70 {
		th := &uarThread{principal: "chat", owner: "chat", client: fmt.Sprintf("thread-%03d", i), lastRun: "run", touched: clock.Now()}
		pending, err := b.parseInterrupts(th, []any{
			map[string]any{"id": "first", "reason": uarReasonToolApproval},
			map[string]any{"id": "second", "reason": uarReasonToolApproval},
		})
		require.NoError(t, err)
		th.pending, th.shown = pending, pending[0].ShownID
		b.threads[th.client] = th
	}
	snapshot := b.srv.decisionSnapshotFor("chat")
	assert.Len(t, snapshot.Items, 64, "projection bound holds even if an authority fixture exceeds its normal thread quota")
	assert.Empty(t, b.srv.gate.broker.retained, "live state is derived, not copied into history")
	assert.Equal(t, snapshot.Revision, b.srv.decisionSnapshotFor("chat").Revision)
	assert.Empty(t, b.srv.decisionSnapshotFor("foreign").Items)
	assert.Len(t, b.threads, 70, "observation budget cannot deny or cancel underlying approvals")
}
