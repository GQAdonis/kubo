package server

import (
	"cmp"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func historyPending(b *Broker, p Principal) Pending {
	return b.create(newConfirmation{
		principal: p, sid: "fixture-session", tool: "pin_add", argsHash: "fixture-hash",
		summary:      Summary{Tool: "pin_add", Class: "write", Text: "Pin content", Targets: []Target{{Kind: "cid", Value: "safe-fixture"}}},
		effectDigest: "sha256:" + strings.Repeat("a", 64), destination: &Destination{Kind: "node", Name: "local"},
	})
}

func TestDecisionHistoryRetainsDeniedExpiredAndWithdrawnAfterTokenSweep(t *testing.T) {
	clock := newFakeClock()
	b := newBroker(clock.Now, confirmTTL)
	owner := Principal{ID: "shared", Kind: KindUI, sub: "private-owner"}
	denied, expired, withdrawn := historyPending(b, owner), historyPending(b, owner), historyPending(b, owner)
	require.NoError(t, b.Resolve(denied.ID, ResolverAGUIInterrupt, false))
	require.NoError(t, b.Resolve(withdrawn.ID, ResolverAGUIInterrupt, true))
	b.Cancel(withdrawn.ID)
	clock.Advance(3 * confirmTTL)
	snapshot := b.snapshotFor(owner.BindingKey()) // sweeps unattended expiry before deleting tokens
	require.Len(t, snapshot.Items, 3)
	for _, fixture := range []struct{ id, state, by string }{
		{denied.ID, StateDenied, ResolverAGUIInterrupt}, {expired.ID, StateExpired, ""}, {withdrawn.ID, StateDenied, ResolverAGUIInterrupt},
	} {
		p, ok := b.observedFor(owner.BindingKey(), fixture.id)
		require.True(t, ok)
		assert.Equal(t, fixture.state, p.State)
		assert.Equal(t, fixture.by, p.ResolvedBy)
		assert.Nil(t, p.Execution, "no invocation occurred")
		assert.Empty(t, p.Principal.sub)
		_, ok = b.get(fixture.id)
		assert.False(t, ok, "history outlives the token record")
	}
	p, _ := b.observedFor(owner.BindingKey(), expired.ID)
	assert.Equal(t, expired.ExpiresAt, p.ResolvedAt, "late observation cannot rejuvenate expiry age")
	clock.Advance(decisionHistoryTTL)
	assert.Empty(t, b.snapshotFor(owner.BindingKey()).Items)
	assert.Empty(t, b.retained)
}

func TestDecisionHistoryOneGlobalCapacityAndFullSubjectFiltering(t *testing.T) {
	clock := newFakeClock()
	b := newBroker(clock.Now, confirmTTL)
	a := Principal{ID: "shared", Kind: KindUI, sub: "alice"}
	z := Principal{ID: "shared", Kind: KindUI, sub: "bob"}
	first := historyPending(b, a)
	require.NoError(t, b.Resolve(first.ID, ResolverAGUIInterrupt, false))
	before := b.snapshotFor(a.BindingKey())
	for range decisionHistoryLimit - 1 {
		p := historyPending(b, z)
		require.NoError(t, b.Resolve(p.ID, ResolverAGUIInterrupt, false))
	}
	assert.Equal(t, before.Revision, b.snapshotFor(a.BindingKey()).Revision, "other identity's activity is not a global revision")
	foreign, ok := b.observedFor(z.BindingKey(), first.ID)
	assert.False(t, ok)
	assert.Empty(t, foreign.ID)
	assert.Empty(t, b.snapshotFor(a.ID).Items, "scrubbed display principal cannot reconstruct subject ownership")
	last := historyPending(b, z)
	require.NoError(t, b.Resolve(last.ID, ResolverAGUIInterrupt, false))
	assert.Len(t, b.retained, decisionHistoryLimit, "200 total, not 200 per identity")
	assert.Len(t, b.snapshotFor(z.BindingKey()).Items, decisionHistoryLimit)
	assert.Empty(t, b.snapshotFor(a.BindingKey()).Items)
	assert.NotEqual(t, before.Revision, b.snapshotFor(a.BindingKey()).Revision, "eviction invalidates only the losing owner's contents")
	_, _, ok = b.observed(first.ID)
	assert.False(t, ok, "evicted history never resurfaces from token grace")
	assert.Nil(t, b.byID[first.ID].observation, "token grace holds no hidden copy of evicted history")
}

func TestDecisionHistoryRevisionsTrackLifecycleAndSnapshotsAreDetached(t *testing.T) {
	clock := newFakeClock()
	b := newBroker(clock.Now, confirmTTL)
	owner := Principal{ID: "owner", Kind: KindUI, sub: "never-project-me"}
	previous := b.snapshotFor(owner.BindingKey())
	changed := func() decisionSnapshot {
		t.Helper()
		next := b.snapshotFor(owner.BindingKey())
		assert.Equal(t, previous.Epoch, next.Epoch)
		assert.Equal(t, previous.HistoryStartedAt, next.HistoryStartedAt)
		assert.NotEqual(t, previous.Revision, next.Revision)
		assert.Equal(t, next.Revision, b.snapshotFor(owner.BindingKey()).Revision)
		previous = next
		return next
	}
	p := historyPending(b, owner)
	changed()
	require.NoError(t, b.ResolveAtSurface(p.ID, ResolverAGUIInterrupt, true, nil, &ResolvingSurface{Kind: "inbox", Instance: "pane"}))
	changed()
	claim, err := b.claimEffect(p.ID, "", nil)
	require.NoError(t, err)
	changed()
	require.True(t, claim.start())
	changed()
	refs := []ReceiptReference{{Kind: "cid", Value: "returned-id"}}
	claim.finish(ExecutionSucceeded, nil, "operation_completed", refs)
	snap := changed()
	refs[0].Value = "tampered-input"
	snap.Items[0].Summary.Targets[0].Value = "tampered"
	snap.Items[0].Destination.Name = "tampered"
	snap.Items[0].ResolvedSurface.Instance = "tampered"
	snap.Items[0].Execution.Receipt.ResultRefs[0].Value = "tampered"
	*snap.Items[0].Execution.StartedAt = time.Time{}
	again := b.snapshotFor(owner.BindingKey())
	assert.Equal(t, previous.Revision, again.Revision, "client mutation cannot change authoritative freshness")
	assert.Equal(t, "safe-fixture", again.Items[0].Summary.Targets[0].Value)
	assert.Equal(t, "local", again.Items[0].Destination.Name)
	assert.Equal(t, "pane", again.Items[0].ResolvedSurface.Instance)
	assert.Equal(t, "returned-id", again.Items[0].Execution.Receipt.ResultRefs[0].Value)
	assert.False(t, again.Items[0].Execution.StartedAt.IsZero())
	assert.Empty(t, again.Items[0].Principal.sub)
	assert.NotContains(t, string(asJSON(t, again)), owner.sub)
}

func TestDecisionHistorySortsCreatedTimeThenOpaqueIDAndResetsOnRestart(t *testing.T) {
	clock := newFakeClock()
	b := newBroker(clock.Now, confirmTTL)
	owner := Principal{ID: "owner", Kind: KindUI}
	first, second := historyPending(b, owner), historyPending(b, owner)
	clock.Advance(time.Second)
	newest := historyPending(b, owner)
	items := b.snapshotFor(owner.BindingKey()).Items
	require.Len(t, items, 3)
	assert.Equal(t, newest.ID, items[0].ID)
	ids := []string{first.ID, second.ID}
	slices.SortFunc(ids, func(a, z string) int { return cmp.Compare(z, a) })
	assert.Equal(t, ids, []string{items[1].ID, items[2].ID})
	restarted := newBroker(clock.Now, confirmTTL)
	assert.NotEqual(t, b.snapshotFor(owner.BindingKey()).Epoch, restarted.snapshotFor(owner.BindingKey()).Epoch)
	assert.Equal(t, clock.Now(), restarted.snapshotFor(owner.BindingKey()).HistoryStartedAt)
	assert.Empty(t, restarted.snapshotFor(owner.BindingKey()).Items)
	assert.NotEqual(t, restarted.snapshotFor("alice").Revision, restarted.snapshotFor("bob").Revision)
}

func TestDecisionHistoryRunningClaimSurvivesGlobalPressureAndAge(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newBroker(time.Now, confirmTTL)
		owner := Principal{ID: "owner", Kind: KindUI}
		p := historyPending(b, owner)
		require.NoError(t, b.Resolve(p.ID, ResolverAGUIInterrupt, true))
		claim, err := b.claimEffect(p.ID, "", nil)
		require.NoError(t, err)
		require.True(t, claim.start())
		for range decisionHistoryLimit + 1 {
			d := historyPending(b, Principal{ID: "busy", Kind: KindUI})
			require.NoError(t, b.Resolve(d.ID, ResolverAGUIInterrupt, false))
		}
		assert.Len(t, b.retained, decisionHistoryLimit+1, "active claim is additional to the resolved bound")
		time.Sleep(decisionHistoryTTL + time.Second)
		item, ok := b.observedFor(owner.BindingKey(), p.ID)
		require.True(t, ok)
		assert.Equal(t, ExecutionRunning, item.Execution.State)
		assert.Len(t, b.retained, 1)
		claim.finish(ExecutionUnknown, &SafeExecutionError{Code: CodeTimeout, Message: "Outcome unavailable."}, "", nil)
		item, ok = b.observedFor(owner.BindingKey(), p.ID)
		require.True(t, ok)
		assert.Equal(t, ExecutionUnknown, item.Execution.State)
		assert.Nil(t, item.Execution.Receipt)
		time.Sleep(decisionHistoryTTL)
		_, ok = b.observedFor(owner.BindingKey(), p.ID)
		assert.False(t, ok)
	})
}

func TestDecisionHistoryFailureCopiesSafeErrorAndExpiryChangesOwnRevision(t *testing.T) {
	clock := newFakeClock()
	b := newBroker(clock.Now, confirmTTL)
	owner := Principal{ID: "owner", Kind: KindUI}
	p := historyPending(b, owner)
	before := b.snapshotFor(owner.BindingKey())
	clock.Advance(confirmTTL)
	after := b.snapshotFor(owner.BindingKey())
	assert.NotEqual(t, before.Revision, after.Revision)
	require.Len(t, after.Items, 1)
	assert.Equal(t, StateExpired, after.Items[0].State)
	p = historyPending(b, owner)
	require.NoError(t, b.Resolve(p.ID, ResolverAGUIInterrupt, true))
	claim, err := b.claimEffect(p.ID, "", nil)
	require.NoError(t, err)
	require.True(t, claim.start())
	publicError := &SafeExecutionError{Code: CodeInternal, Message: "Operation reported failure."}
	claim.finish(ExecutionFailed, publicError, "", nil)
	publicError.Message = "tampered source"
	item, ok := b.observedFor(owner.BindingKey(), p.ID)
	require.True(t, ok)
	assert.Equal(t, "Operation reported failure.", item.Execution.Error.Message)
	item.Execution.Error.Message = "tampered snapshot"
	item, ok = b.observedFor(owner.BindingKey(), p.ID)
	require.True(t, ok)
	assert.Equal(t, "Operation reported failure.", item.Execution.Error.Message)
	assert.Nil(t, item.Execution.Receipt)
}
