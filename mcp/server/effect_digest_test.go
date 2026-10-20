package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEffectDigestCanonicalEnvelope(t *testing.T) {
	// These bytes pin the existing canonicalizer, not a second JSON encoder.
	const canonical = `{"arguments":{"count":1,"nested":{"confirm_token":"data","n":9007199254740993},"path":"/example"},"tool":"fixture_write"}`
	wantSum := sha256.Sum256([]byte(canonical))
	want := "sha256:" + hex.EncodeToString(wantSum[:])
	for _, raw := range []string{
		`{"path":"/example","count":1.0,"nested":{"n":9007199254740993,"confirm_token":"data"}}`,
		`{"confirm_token":"binding-only","nested":{"confirm_token":"data","n":9007199254740993},"count":1e0,"path":"/example"}`,
	} {
		_, _, args, err := argsDigest(json.RawMessage(raw))
		require.NoError(t, err)
		got, err := effectDigest("fixture_write", args)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
	_, _, args, err := argsDigest(json.RawMessage(`{"path":"/example","recursive":false}`))
	require.NoError(t, err)
	explicit, err := effectDigest("fixture_write", args)
	require.NoError(t, err)
	delete(args, "recursive")
	omitted, err := effectDigest("fixture_write", args)
	require.NoError(t, err)
	otherTool, err := effectDigest("other_write", args)
	require.NoError(t, err)
	assert.NotEqual(t, explicit, omitted, "explicit defaults stay bound")
	assert.NotEqual(t, omitted, otherTool, "the envelope includes the tool")
}

func TestReviewedDigestMismatchDoesNotResolveOrConsume(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, nil, nil)
	conf := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired).Confirmation
	require.NotEmpty(t, conf.EffectDigest)
	assert.Equal(t, &Destination{Kind: "node", Name: "Configured Kubo node"}, conf.Destination)
	p, _ := e.resolver.last(t)
	assert.Equal(t, conf.EffectDigest, p.EffectDigest)
	validateConfirmationDef(t, "ConfirmationRequired", conf)

	err := e.s.gate.broker.ResolveReviewed(p.ID, ResolverLinuxPolkit, true, &ReviewedEffect{EffectDigest: "sha256:" + strings.Repeat("0", 64)})
	require.ErrorIs(t, err, ErrConfirmationDigestMismatch)
	snap, ok := e.s.gate.broker.get(p.ID)
	require.True(t, ok)
	assert.Equal(t, StatePending, snap.State)
	e.assertNothingRan(t)

	// The stale presentation does not rewrite the immutable operation. A
	// correct review can still approve it, and exactly one repeat may run.
	require.NoError(t, e.s.gate.broker.ResolveReviewed(p.ID, ResolverLinuxPolkit, true, &ReviewedEffect{EffectDigest: conf.EffectDigest}))
	res := e.rm(t, cs, map[string]any{"confirm_token": conf.ConfirmToken})
	require.False(t, res.IsError)
	assert.Equal(t, 1, e.d.count("pin/rm"))
	confirmationOf(t, e.rm(t, cs, map[string]any{"confirm_token": conf.ConfirmToken}), CodeConfirmInvalid)
	assert.Equal(t, 1, e.d.count("pin/rm"))
}

func TestEffectBindingMismatchStopsSharedHandler(t *testing.T) {
	for _, token := range []bool{false, true} {
		t.Run(map[bool]string{false: "tokenless", true: "token"}[token], func(t *testing.T) {
			e := newGateEnv(t)
			cs := e.h.connect(t, nil, nil)
			conf := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired).Confirmation
			p, resolve := e.resolver.last(t)
			require.NoError(t, resolve(true))
			// An additive metadata inconsistency must fail even when all v1
			// signed bindings still match; it must not consume approval.
			e.s.gate.broker.mu.Lock()
			e.s.gate.broker.byID[p.ID].EffectDigest = "sha256:" + strings.Repeat("0", 64)
			e.s.gate.broker.mu.Unlock()
			var extra map[string]any
			if token {
				extra = map[string]any{"confirm_token": conf.ConfirmToken}
			}
			te := confirmationOf(t, e.rm(t, cs, extra), CodeConfirmDigestMismatch)
			assert.False(t, te.Retryable)
			snap, _ := e.s.gate.broker.get(p.ID)
			assert.Equal(t, StateApproved, snap.State)
			e.assertNothingRan(t)
		})
	}
}

func TestEffectBindingRejectsChangedDaemonDestination(t *testing.T) {
	e := newGateEnv(t)
	endpoint := "http://127.0.0.1:15001"
	e.s.gate.endpoint = func() string { return endpoint }
	cs := e.h.connect(t, nil, nil)
	conf := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired).Confirmation
	_, resolve := e.resolver.last(t)
	require.NoError(t, resolve(true))
	endpoint = "http://127.0.0.1:25001"
	confirmationOf(t, e.rm(t, cs, map[string]any{"confirm_token": conf.ConfirmToken}), CodeConfirmDigestMismatch)
	e.assertNothingRan(t)
}

func TestEffectSnapshotCannotMutateRetainedDestination(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, nil, nil)
	conf := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired).Confirmation
	p, _ := e.resolver.last(t)
	p.Destination.Name = "modified view"
	snap, _ := e.s.gate.broker.get(conf.ConfirmationID)
	assert.Equal(t, "Configured Kubo node", snap.Destination.Name)
}

func TestLegacyElicitationReceivesEffectMetadata(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, elicitOptions(func(req *mcp.ElicitRequest) *mcp.ElicitResult {
		meta, ok := req.Params.Meta["ipfs-mcp"].(map[string]any)
		require.True(t, ok)
		assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, meta["effect_digest"])
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	}), nil)
	res := e.rm(t, cs, nil)
	require.False(t, res.IsError, "legacy omission of reviewed metadata still follows the gate")
	assert.Equal(t, 1, e.d.count("pin/rm"))
}
