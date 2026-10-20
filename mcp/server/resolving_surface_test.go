package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvingSurfaceKindsAreAdapterBound(t *testing.T) {
	for resolver, kind := range map[string]string{
		ResolverElicitation: "mcp_host", ResolverAGUIInterrupt: "chat", ResolverDesktop: "desktop",
		ResolverMacOSLocalAuth: "native_prompt", ResolverLinuxPolkit: "native_prompt", ResolverCLI: "cli",
	} {
		surface := ResolverSurface(resolver)
		require.NotNil(t, surface)
		assert.Equal(t, kind, surface.Kind)
		assert.Empty(t, surface.Instance, "no session secret or invented pane ID")
		require.NoError(t, validateResolverSurface(resolver, surface))
		validateConfirmationDef(t, "ResolvingSurface", surface)
	}
	assert.Nil(t, ResolverSurface("client-selected-mechanism"))
	assert.ErrorIs(t, validateResolverSurface(ResolverAGUIInterrupt, &ResolvingSurface{Kind: "native_prompt"}), ErrInvalidResolvingSurface)
	assert.ErrorIs(t, validateResolverSurface(ResolverDesktop, &ResolvingSurface{Kind: "chat", Instance: "pane"}), ErrInvalidResolvingSurface)
}

func TestNodeConfirmationSourceCannotCollideWithSyntheticBackendApproval(t *testing.T) {
	b := newBroker(time.Now, confirmTTL)
	// A UAR MCP principal can request an actual node operation. That broker
	// record is still a node confirmation, unlike a UAR-owned backend prompt.
	p := openPending(b, "pin_add")
	assert.Equal(t, KindUAR, p.Principal.Kind)
	assert.Equal(t, DecisionSourceKubo, p.Source)
	assert.Equal(t, SourceKindNodeConfirmation, p.SourceKind)
	assert.True(t, strings.HasPrefix(p.ID, "cf_n_"))
	assert.False(t, strings.HasPrefix(p.ID, "cf_uar_"))
}

func TestResolvingSurfaceCannotSupplyNativePresence(t *testing.T) {
	b := newBroker(time.Now, confirmTTL)
	p := b.create(newConfirmation{principal: Principal{ID: "chat", Kind: KindUI}, sid: "thread", tool: "pin_rm", argsHash: "hash", nativeReq: true})
	err := b.ResolveAtSurface(p.ID, ResolverAGUIInterrupt, true, nil, &ResolvingSurface{Kind: "native_prompt"})
	require.ErrorIs(t, err, ErrNativePromptRequired)
	err = b.ResolveAtSurface(p.ID, ResolverAGUIInterrupt, true, nil, &ResolvingSurface{Kind: "inbox", Instance: "pane"})
	require.ErrorIs(t, err, ErrNativePromptRequired)
	err = b.ResolveAtSurface(p.ID, ResolverLinuxPolkit, true, nil, &ResolvingSurface{Kind: "desktop"})
	require.ErrorIs(t, err, ErrInvalidResolvingSurface)
	snap, _ := b.get(p.ID)
	assert.Equal(t, StatePending, snap.State)
	assert.Nil(t, snap.ResolvedSurface)
	require.NoError(t, b.Resolve(p.ID, ResolverLinuxPolkit, true))
	snap, _ = b.get(p.ID)
	assert.Equal(t, ResolverLinuxPolkit, snap.ResolvedBy)
	assert.Equal(t, &ResolvingSurface{Kind: "native_prompt"}, snap.ResolvedSurface)
}

func TestResolvingSurfaceNeverReplacesActualTerminalOutcome(t *testing.T) {
	for _, state := range []string{ExecutionSucceeded, ExecutionFailed, ExecutionUnknown} {
		t.Run(state, func(t *testing.T) {
			clock := newFakeClock()
			b := newBroker(clock.Now, confirmTTL)
			p := b.create(newConfirmation{
				principal: Principal{ID: "chat", Kind: KindUI, sub: "owner-subject"}, sid: "thread", tool: "pin_add", argsHash: "hash",
				summary: Summary{Tool: "pin_add", Text: "Pin fixture", Class: "write", Targets: []Target{}}, effectDigest: "sha256:" + strings.Repeat("a", 64),
			})
			viewer := &ResolvingSurface{Kind: "chat", Instance: "pane_a"}
			answer := &ResolvingSurface{Kind: "chat", Instance: "pane_b"}
			require.NoError(t, b.ResolveAtSurface(p.ID, ResolverAGUIInterrupt, true, nil, answer))
			answer.Instance = "mutated-caller"
			snap, _ := b.get(p.ID)
			assert.NotEqual(t, viewer.Instance, snap.ResolvedSurface.Instance, "same-kind panes are distinguishable")
			assert.Equal(t, "pane_b", snap.ResolvedSurface.Instance)
			assert.Nil(t, snap.Execution, "answered elsewhere is no operation verdict")
			claim, err := b.claimEffect(p.ID, "", nil)
			require.NoError(t, err)
			require.True(t, claim.start())
			var publicError *SafeExecutionError
			if state != ExecutionSucceeded {
				publicError = &SafeExecutionError{Code: CodeRPCError, Message: "Actual fixture outcome"}
			}
			claim.finish(state, publicError, "operation_completed", nil)
			// A delayed pane response, expiry refresh and system cancellation
			// cannot demote the actual execution result or rewrite attribution.
			require.ErrorIs(t, b.ResolveAtSurface(p.ID, ResolverAGUIInterrupt, false, nil, &ResolvingSurface{Kind: "inbox", Instance: "late_pane"}), ErrConfirmationResolved)
			b.Cancel(p.ID)
			clock.Advance(3 * confirmTTL)
			b.mu.Lock()
			b.sweep()
			b.mu.Unlock()
			observed, owner, ok := b.observed(p.ID)
			require.True(t, ok)
			assert.Equal(t, (Principal{ID: "chat", sub: "owner-subject"}).BindingKey(), owner)
			assert.Equal(t, DecisionSourceKubo, observed.Source)
			assert.Equal(t, SourceKindNodeConfirmation, observed.SourceKind)
			assert.Equal(t, state, observed.Execution.State)
			assert.Equal(t, ResolverAGUIInterrupt, observed.ResolvedBy)
			assert.Equal(t, "pane_b", observed.ResolvedSurface.Instance)
			validateConfirmationDef(t, "ExecutionRecord", observed.Execution)
			if state == ExecutionSucceeded {
				require.NotNil(t, observed.Execution.Receipt)
			} else {
				assert.Nil(t, observed.Execution.Receipt)
			}
			observed.ResolvedSurface.Instance = "mutated-reader"
			again, _, _ := b.observed(p.ID)
			assert.Equal(t, "pane_b", again.ResolvedSurface.Instance)
		})
	}
}

func TestForwardedSurfaceRejectsExplicitSpoofAndPreservesGenericBranches(t *testing.T) {
	for _, surface := range []string{`null`, `{}`, `{"kind":"chat"}`, `{"kind":"native_prompt","instance":"pane"}`, `{"kind":"desktop","instance":"pane"}`, `{"kind":"chat","instance":"bad/path"}`, `{"kind":"chat","instance":"pane","principal":"other"}`} {
		_, err := forwardedSurface(json.RawMessage(`{"ipfs":{"surface":` + surface + `}}`))
		require.ErrorIs(t, err, ErrInvalidResolvingSurface, surface)
	}
	for _, raw := range []string{`null`, `[]`, `42`, `{"ipfs":null}`, `{"ipfs":[]}`, `{"ignored":"value"}`, `{"ipfs":{"a2ui":"independent","unknown":true}}`} {
		surface, err := forwardedSurface(json.RawMessage(raw))
		require.NoError(t, err, raw)
		assert.Nil(t, surface)
	}
	surface, err := forwardedSurface(json.RawMessage(`{"unrelated":true,"ipfs":{"surface":{"kind":"inbox","instance":"pane_2"},"a2ui":false}}`))
	require.NoError(t, err)
	assert.Equal(t, &ResolvingSurface{Kind: "inbox", Instance: "pane_2"}, surface)
}

func TestResolvingSurfaceAutomaticTransitionsPreserveRealProvenance(t *testing.T) {
	for _, withdraw := range []bool{false, true} {
		clock := newFakeClock()
		b := newBroker(clock.Now, confirmTTL)
		p := openPending(b, "pin_add")
		require.NoError(t, b.ResolveAtSurface(p.ID, ResolverAGUIInterrupt, true, nil, &ResolvingSurface{Kind: "inbox", Instance: "approved_here"}))
		if withdraw {
			b.Cancel(p.ID)
		} else {
			clock.Advance(confirmTTL + time.Second)
		}
		snap, _ := b.get(p.ID)
		if withdraw {
			assert.Equal(t, StateDenied, snap.State)
		} else {
			assert.Equal(t, StateExpired, snap.State)
		}
		assert.Equal(t, ResolverAGUIInterrupt, snap.ResolvedBy, "this describes the real prior approver, not the automatic transition")
		assert.Equal(t, "approved_here", snap.ResolvedSurface.Instance)
		assert.Nil(t, snap.Execution)
		require.Error(t, b.ResolveAtSurface(p.ID, ResolverAGUIInterrupt, true, nil, &ResolvingSurface{Kind: "chat", Instance: "stale_reconnect"}))
		assert.Error(t, b.claim(p.ID, ""), "provenance never makes a terminal confirmation actionable")
	}
	clock := newFakeClock()
	b := newBroker(clock.Now, confirmTTL)
	p := openPending(b, "pin_add")
	clock.Advance(confirmTTL + time.Second)
	snap, _ := b.get(p.ID)
	assert.Empty(t, snap.ResolvedBy, "an unanswered expiry has no invented approver")
	assert.Nil(t, snap.ResolvedSurface)
}
