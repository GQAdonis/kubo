//go:build !noagent

package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecisionChangesUARSourceTransitionsMatchOwnedSnapshot(t *testing.T) {
	clock := newFakeClock()
	b := uarHistoryBackend(t, clock)
	b.threads = map[string]*uarThread{}
	b.srv.uarExt = b
	broker := b.srv.gate.broker
	owner := Principal{ID: "shared", Kind: KindUI, sub: "alice"}
	other := Principal{ID: "shared", Kind: KindUI, sub: "bob"}
	sub, err := broker.subscribeDecisions(owner)
	require.NoError(t, err)
	defer sub.close()
	foreign, err := broker.subscribeDecisions(other)
	require.NoError(t, err)
	defer foreign.close()
	th, release, err := b.acquireThread(owner.ID, owner.BindingKey(), "thread", true)
	require.NoError(t, err)
	th.lastRun, th.activityID = "run", "act_fixture"
	th.pending, err = b.parseInterrupts(th, []any{map[string]any{"id": "backend", "reason": uarReasonToolApproval, "expiresAt": clock.Now().Add(time.Hour).Format(time.RFC3339)}})
	require.NoError(t, err)
	th.shown = th.pending[0].ShownID
	id := th.pending[0].confirmationID()
	release()
	created := nextDecisionChange(t, sub)
	assert.Equal(t, id, created.ID)
	assert.Equal(t, StatePending, created.State)
	assert.Equal(t, b.srv.decisionSnapshotFor(owner.BindingKey()).Revision, created.Revision)
	noDecisionChanges(t, foreign)
	_, release, err = b.acquireThread(owner.ID, owner.BindingKey(), "thread", false)
	require.NoError(t, err)
	hidden := nextDecisionChange(t, sub)
	assert.Equal(t, StatePending, hidden.State, "temporary source removal is an invalidation, not fabricated running/success")
	assert.Empty(t, b.srv.decisionSnapshotFor(owner.BindingKey()).Items)
	require.Nil(t, th.pending[0].Answer)
	th.pending[0].Answer = th.pending[0].answerFor(true, false)
	th.pending[0].Answer.ResolvedBy = ResolverAGUIInterrupt
	b.settle(th, relayResult{responded: true})
	release()
	settled := nextDecisionChange(t, sub)
	assert.Equal(t, StateExecuted, settled.State, "acknowledged backend approval consumed, no operation outcome invented")
	snapshot := b.srv.decisionSnapshotFor(owner.BindingKey())
	assert.Equal(t, snapshot.Revision, settled.Revision)
	require.Len(t, snapshot.Items, 1)
	assert.Nil(t, snapshot.Items[0].Execution)
	assert.Equal(t, SourceKindBackendApproval, snapshot.Items[0].SourceKind)
	noDecisionChanges(t, foreign)
	noDecisionChanges(t, sub)
}

func TestDecisionChangesUARProjectionExpiryDoesNotResolveUnderlyingApproval(t *testing.T) {
	clock := newFakeClock()
	b := uarHistoryBackend(t, clock)
	b.threads = map[string]*uarThread{}
	b.srv.uarExt = b
	owner := Principal{ID: "chat", Kind: KindUI}
	sub, err := b.srv.gate.broker.subscribeDecisions(owner)
	require.NoError(t, err)
	defer sub.close()
	th, release, err := b.acquireThread(owner.ID, owner.BindingKey(), "thread", true)
	require.NoError(t, err)
	expiry := clock.Now().Add(time.Minute)
	th.pending, err = b.parseInterrupts(th, []any{map[string]any{"id": "backend", "reason": uarReasonToolApproval, "expiresAt": expiry.Format(time.RFC3339)}})
	require.NoError(t, err)
	th.shown = th.pending[0].ShownID
	release()
	nextDecisionChange(t, sub)
	deadline, _ := b.srv.gate.broker.decisionObservationSchedule()
	assert.Equal(t, expiry, deadline)
	clock.Advance(time.Minute)
	b.srv.gate.broker.sweepDecisionObservations()
	removed := nextDecisionChange(t, sub)
	assert.Equal(t, StatePending, removed.State)
	assert.Empty(t, b.srv.decisionSnapshotFor(owner.BindingKey()).Items)
	assert.Equal(t, b.srv.decisionSnapshotFor(owner.BindingKey()).Revision, removed.Revision)
	require.Len(t, th.pending, 1)
	assert.Nil(t, th.pending[0].Answer, "observer cleanup cannot approve or deny a backend prompt")
	assert.Empty(t, b.srv.gate.broker.retained)
}

func TestDecisionChangesUARGenerationReplacementAndMarkerBound(t *testing.T) {
	clock := newFakeClock()
	b := newBroker(clock.Now, confirmTTL)
	owner := Principal{ID: "chat", Kind: KindUI}
	sub, err := b.subscribeDecisions(owner)
	require.NoError(t, err)
	defer sub.close()
	b.setDecisionSourceGeneration("current")
	var sources []liveDecisionSource
	for i := range 70 {
		sources = append(sources, liveDecisionSource{id: "cf_uar_" + randomID(16), owner: owner.BindingKey(), thread: fmt.Sprintf("private-thread-%d", i), state: StatePending, deadline: clock.Now().Add(time.Hour)})
	}
	// Duplicate source entries for one thread cannot double its observation.
	sources = append([]liveDecisionSource{sources[0], {id: "cf_uar_" + randomID(16), owner: owner.BindingKey(), thread: sources[0].thread, state: StatePending, deadline: clock.Now().Add(time.Hour)}}, sources[1:]...)
	b.replaceDecisionSources("current", sources)
	assert.Len(t, b.decisionSources, maxDecisionLiveSources)
	assert.Len(t, sub.events, 1, "overflow remains bounded during source import")
	<-sub.events
	b.mu.Lock()
	before := b.decisionRevisionLocked(owner.BindingKey())
	b.mu.Unlock()
	b.replaceDecisionSources("current", sources)
	noDecisionChanges(t, sub)
	b.mu.Lock()
	assert.Equal(t, before, b.decisionRevisionLocked(owner.BindingKey()))
	b.mu.Unlock()
	sources[0].factHash[0] = 1
	b.replaceDecisionSources("current", sources)
	changed := nextDecisionChange(t, sub)
	assert.Equal(t, sources[0].id, changed.ID, "changed safe source facts invalidate even when ID/state stay the same")
	assert.NotEqual(t, before, changed.Revision)
	b.setDecisionSourceGeneration("replacement")
	assert.Empty(t, b.decisionSources)
	b.replaceDecisionSources("current", sources)
	assert.Empty(t, b.decisionSources, "an old backend's late release cannot restore replaced projections")
	b.setHistoryEnabled(false)
	assert.Empty(t, b.decisionSubs)
	assert.Empty(t, b.decisionSources)
}
