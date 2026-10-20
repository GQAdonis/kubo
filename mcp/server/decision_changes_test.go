//go:build !noagent

package server

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nextDecisionChange(t *testing.T, sub *decisionSubscription) decisionChanged {
	t.Helper()
	select {
	case event, ok := <-sub.events:
		require.True(t, ok)
		require.NotNil(t, event.change)
		require.Nil(t, event.snapshot)
		data := asJSON(t, event.change)
		validateDecisionRead(t, "DecisionChanged", data)
		require.LessOrEqual(t, len(data), maxDecisionEventBytes)
		var fields map[string]any
		require.NoError(t, json.Unmarshal(data, &fields))
		assert.ElementsMatch(t, []string{"id", "state", "revision"}, keysOf(fields))
		return *event.change
	default:
		t.Fatal("expected a decision invalidation")
		return decisionChanged{}
	}
}

func noDecisionChanges(t *testing.T, sub *decisionSubscription) {
	t.Helper()
	select {
	case event := <-sub.events:
		t.Fatalf("unexpected invalidation: %+v", event)
	default:
	}
}

func TestDecisionChangesOwnerIsolationAndActualExecutionStates(t *testing.T) {
	clock := newFakeClock()
	b := newBroker(clock.Now, confirmTTL)
	owner := Principal{ID: "shared", Kind: KindUI, sub: "private-alice"}
	other := Principal{ID: owner.ID, Kind: owner.Kind, sub: "private-bob"}
	sub, err := b.subscribeDecisions(owner)
	require.NoError(t, err)
	defer sub.close()
	foreign, err := b.subscribeDecisions(other)
	require.NoError(t, err)
	defer foreign.close()
	validateDecisionRead(t, "SnapshotRequired", asJSON(t, sub.initial))
	assert.Equal(t, "initial", sub.initial.Reason)
	assert.Equal(t, b.snapshotFor(owner.BindingKey()).Revision, sub.initial.Revision)
	p := historyPending(b, owner)
	assertChange := func(state string) {
		t.Helper()
		change := nextDecisionChange(t, sub)
		assert.Equal(t, p.ID, change.ID)
		assert.Equal(t, state, change.State)
		assert.Equal(t, b.snapshotFor(owner.BindingKey()).Revision, change.Revision)
		assert.NotContains(t, string(asJSON(t, change)), owner.sub)
		assert.NotContains(t, string(asJSON(t, change)), owner.BindingKey())
		noDecisionChanges(t, foreign)
	}
	assertChange(StatePending)
	require.NoError(t, b.Resolve(p.ID, ResolverAGUIInterrupt, true))
	assertChange(StateApproved)
	claim, err := b.claimEffect(p.ID, "", nil)
	require.NoError(t, err)
	assertChange(ExecutionNotStarted)
	require.True(t, claim.start())
	assertChange(ExecutionRunning)
	claim.finish(ExecutionSucceeded, nil, "operation_completed", nil)
	assertChange(ExecutionSucceeded)
	claim.finish(ExecutionFailed, &SafeExecutionError{Code: CodeInternal, Message: "Late delivery failure."}, "", nil)
	noDecisionChanges(t, sub)
	assert.Equal(t, ExecutionSucceeded, b.retained[p.ID].execution.State)
}

func TestDecisionChangesKnownFailureAndUnknownNeverPublishSuccess(t *testing.T) {
	for _, state := range []string{ExecutionFailed, ExecutionUnknown} {
		b := newBroker(time.Now, confirmTTL)
		owner := Principal{ID: "owner", Kind: KindUI}
		p := historyPending(b, owner)
		require.NoError(t, b.Resolve(p.ID, ResolverAGUIInterrupt, true))
		claim, err := b.claimEffect(p.ID, "", nil)
		require.NoError(t, err)
		require.True(t, claim.start())
		sub, err := b.subscribeDecisions(owner)
		require.NoError(t, err)
		claim.finish(state, &SafeExecutionError{Code: CodeRPCError, Message: "Outcome reported."}, "", nil)
		assert.Equal(t, state, nextDecisionChange(t, sub).State)
		assert.Nil(t, b.retained[p.ID].execution.Receipt)
		_, err = b.claimEffect(p.ID, "", nil)
		assert.Error(t, err)
		noDecisionChanges(t, sub)
		sub.close()
	}
}

func TestDecisionChangesSweepExpiryWithdrawalAndRetentionRemoval(t *testing.T) {
	clock := newFakeClock()
	b := newBroker(clock.Now, confirmTTL)
	owner := Principal{ID: "owner", Kind: KindUI}
	sub, err := b.subscribeDecisions(owner)
	require.NoError(t, err)
	defer sub.close()
	p := historyPending(b, owner)
	nextDecisionChange(t, sub)
	deadline, wake := b.decisionObservationSchedule()
	assert.Equal(t, p.ExpiresAt, deadline)
	clock.Advance(confirmTTL)
	b.sweepDecisionObservations() // scheduler hook; no active tool request
	change := nextDecisionChange(t, sub)
	assert.Equal(t, StateExpired, change.State)
	assert.Equal(t, b.snapshotFor(owner.BindingKey()).Revision, change.Revision)
	select {
	case <-wake:
	default:
		t.Fatal("expiry must wake reconciliation scheduling")
	}
	withdrawn := historyPending(b, owner)
	nextDecisionChange(t, sub)
	b.Cancel(withdrawn.ID)
	assert.Equal(t, StateDenied, nextDecisionChange(t, sub).State)
	clock.Advance(decisionHistoryTTL)
	b.sweepDecisionObservations()
	removed := map[string]string{}
	for len(sub.events) > 0 {
		event := nextDecisionChange(t, sub)
		removed[event.ID] = event.State
	}
	assert.Equal(t, StateExpired, removed[p.ID], "removal hints preserve last known state, not invented failure")
	assert.Equal(t, StateDenied, removed[withdrawn.ID])
	assert.Empty(t, b.snapshotFor(owner.BindingKey()).Items)
}

func TestDecisionChangesGlobalPressureOnlyInvalidatesOwnerActuallyEvicted(t *testing.T) {
	clock := newFakeClock()
	b := newBroker(clock.Now, confirmTTL)
	owner := Principal{ID: "quiet", Kind: KindUI}
	p := historyPending(b, owner)
	require.NoError(t, b.Resolve(p.ID, ResolverAGUIInterrupt, false))
	sub, err := b.subscribeDecisions(owner)
	require.NoError(t, err)
	defer sub.close()
	for range decisionHistoryLimit - 1 {
		d := historyPending(b, Principal{ID: "busy", Kind: KindUI})
		require.NoError(t, b.Resolve(d.ID, ResolverAGUIInterrupt, false))
	}
	noDecisionChanges(t, sub)
	d := historyPending(b, Principal{ID: "busy", Kind: KindUI})
	require.NoError(t, b.Resolve(d.ID, ResolverAGUIInterrupt, false))
	change := nextDecisionChange(t, sub)
	assert.Equal(t, p.ID, change.ID)
	assert.Equal(t, StateDenied, change.State)
	assert.Equal(t, b.snapshotFor(owner.BindingKey()).Revision, change.Revision)
	noDecisionChanges(t, sub)
}

func TestDecisionChangesOverflowCoalescesAndInitialCannotBeOverwritten(t *testing.T) {
	b := newBroker(time.Now, confirmTTL)
	owner := Principal{ID: "owner", Kind: KindUI}
	sub, err := b.subscribeDecisions(owner)
	require.NoError(t, err)
	defer sub.close()
	initial := sub.initial
	for range 100 {
		historyPending(b, owner)
	}
	assert.Equal(t, initial, sub.initial, "first transport marker remains initial even before a slow consumer starts")
	require.Len(t, sub.events, 1)
	event := <-sub.events
	require.Nil(t, event.change)
	require.NotNil(t, event.snapshot)
	validateDecisionRead(t, "SnapshotRequired", asJSON(t, event.snapshot))
	assert.Equal(t, "overflow", event.snapshot.Reason)
	assert.Equal(t, b.snapshotFor(owner.BindingKey()).Revision, event.snapshot.Revision)
	historyPending(b, owner)
	assert.Equal(t, StatePending, nextDecisionChange(t, sub).State, "a drained overflow queue can accept fresh hints")
}

func TestDecisionChangesQueuesAreDetachedAndQuotaResourcesRelease(t *testing.T) {
	b := newBroker(time.Now, confirmTTL)
	owner := Principal{ID: "owner", Kind: KindUI}
	first, err := b.subscribeDecisions(owner)
	require.NoError(t, err)
	second, err := b.subscribeDecisions(owner)
	require.NoError(t, err)
	p := historyPending(b, owner)
	event := <-first.events
	event.change.ID = "mutated-local-copy"
	assert.Equal(t, p.ID, nextDecisionChange(t, second).ID)
	third, err := b.subscribeDecisions(owner)
	require.NoError(t, err)
	fourth, err := b.subscribeDecisions(owner)
	require.NoError(t, err)
	_, err = b.subscribeDecisions(owner)
	assert.ErrorIs(t, err, errDecisionSubscriptionsFull, "full-identity cap is four")
	first.close()
	first.close()
	second.close()
	third.close()
	fourth.close()
	var subs []*decisionSubscription
	for principal := range 16 {
		for connection := range maxDecisionStreamsPerOwner {
			p := Principal{ID: fmt.Sprintf("principal-%d", principal), Kind: KindUI}
			sub, err := b.subscribeDecisions(p)
			require.NoError(t, err, "connection %d", connection)
			subs = append(subs, sub)
		}
	}
	_, err = b.subscribeDecisions(Principal{ID: "extra", Kind: KindUI})
	assert.ErrorIs(t, err, errDecisionSubscriptionsFull)
	for _, sub := range subs {
		sub.close()
	}
	assert.Empty(t, b.decisionSubs)
	for subject := range 4 {
		for range maxDecisionStreamsPerOwner {
			sub, err := b.subscribeDecisions(Principal{ID: "shared", Kind: KindUI, sub: fmt.Sprintf("subject-%d", subject)})
			require.NoError(t, err)
			subs = append(subs, sub)
		}
	}
	_, err = b.subscribeDecisions(Principal{ID: "shared", Kind: KindUI, sub: "fifth-subject"})
	assert.ErrorIs(t, err, errDecisionSubscriptionsFull, "principal cap combines subjects without sharing their data")
	for _, sub := range subs {
		sub.close()
	}
}

func TestDecisionChangesDisableClosesAndDrainsWithoutTouchingApproval(t *testing.T) {
	b := newBroker(time.Now, confirmTTL)
	owner := Principal{ID: "owner", Kind: KindUI}
	sub, err := b.subscribeDecisions(owner)
	require.NoError(t, err)
	p := historyPending(b, owner)
	b.setHistoryEnabled(false)
	_, open := <-sub.events
	assert.False(t, open, "disable releases queued hints and closes the subscriber")
	assert.Empty(t, b.decisionSubs)
	assert.Empty(t, b.decisionSources)
	pending, ok := b.get(p.ID)
	require.True(t, ok)
	assert.Equal(t, StatePending, pending.State, "observer disable cannot deny an underlying request")
	_, err = b.subscribeDecisions(owner)
	assert.ErrorIs(t, err, errDecisionObserverDisabled)
	b.setHistoryEnabled(true)
	newSub, err := b.subscribeDecisions(owner)
	require.NoError(t, err)
	assert.NotEqual(t, sub.initial.Epoch, newSub.initial.Epoch)
	newSub.close()
}

func TestDecisionChangesConcurrentMutationConsumptionAndCloseDoNotBlock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newBroker(time.Now, confirmTTL)
		owner := Principal{ID: "owner", Kind: KindUI}
		var received atomic.Int64
		var workers sync.WaitGroup
		for range 4 {
			sub, err := b.subscribeDecisions(owner)
			require.NoError(t, err)
			workers.Go(func() {
				for range sub.events {
					received.Add(1)
				}
			})
			workers.Go(func() { time.Sleep(time.Millisecond); sub.close() })
		}
		for range 16 {
			workers.Go(func() {
				for range 10 {
					p := historyPending(b, owner)
					b.Cancel(p.ID)
				}
			})
		}
		workers.Wait()
		assert.Empty(t, b.decisionSubs)
		assert.Greater(t, received.Load(), int64(0))
	})
}
