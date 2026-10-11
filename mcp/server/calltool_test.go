//go:build !noagent

package server

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func callEnv(t *testing.T, resolvers ...Resolver) (*Server, *pinStore) {
	t.Helper()
	d := catFake(t, nil)
	pins := installPins(d.fakeDaemon, map[string]fakePin{testCID(t, 1): {Type: "recursive"}})
	s := mustNew(t, d, Options{Tools: []string{"node", "content", "pin", "destructive"}, Resolvers: resolvers})
	return s, pins
}

func peerID(write, destructive []string, ack ...string) Identity {
	id := a2aIdentity("peer", KindA2A, []string{AllGroups}, write, destructive, ack)
	noShell := false
	id.ShellCapable = &noShell // --confirm=cli withdraws writes from a principal that may have a shell
	return id
}

func args(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestCallToolReadRunsThroughTheHandler(t *testing.T) {
	s, _ := callEnv(t)
	res, err := s.CallTool(t.Context(), peerID(nil, nil, "local"), "k", "pin_ls", args(t, map[string]any{}))
	require.NoError(t, err)
	require.False(t, res.IsError)
	assert.NotNil(t, res.StructuredContent)
	assert.NotEmpty(t, res.Content, "a text fallback accompanies structuredContent")
}

func TestCallToolNeedsASessionKey(t *testing.T) {
	s, _ := callEnv(t)
	_, err := s.CallTool(t.Context(), peerID(nil, nil), "", "pin_ls", nil)
	assert.ErrorIs(t, err, ErrNoSessionKey)
}

func TestCallToolRefusals(t *testing.T) {
	s, pins := callEnv(t)
	cases := map[string]struct {
		id   Identity
		tool string
		args any
		code string
	}{
		"unknown tool":       {peerID(nil, nil, "local"), "rm_rf", map[string]any{}, CodeToolUnavailable},
		"write not granted":  {peerID(nil, nil, "local"), "pin_add", map[string]any{"path": "/ipfs/" + testCID(t, 5)}, CodePermissionDenied},
		"other group":        {peerID([]string{"mfs"}, nil, "local"), "pin_add", map[string]any{"path": "/ipfs/" + testCID(t, 5)}, CodePermissionDenied},
		"destructive":        {peerID([]string{"pin"}, nil, "local"), "pin_rm", map[string]any{"path": "/ipfs/" + testCID(t, 1)}, CodePermissionDenied},
		"invalid arguments":  {peerID(nil, nil, "local"), "pin_ls", map[string]any{"type": 5}, CodeInvalidArgument},
		"egress unconfirmed": {peerID(nil, nil), "ipfs_cat", map[string]any{"path": "/ipfs/" + testCID(t, 1)}, CodeEgressNotAcknowledged},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := s.CallTool(t.Context(), c.id, "k", c.tool, args(t, c.args))
			require.NoError(t, err)
			require.True(t, res.IsError)
			assert.Equal(t, c.code, errorOf(res).Code)
		})
	}
	assert.True(t, pins.has(testCID(t, 1)))
	assert.False(t, pins.has(testCID(t, 5)))
}

func TestCallToolNonObjectArguments(t *testing.T) {
	s, _ := callEnv(t)
	for _, raw := range []string{`[1]`, `"x"`, `5`} {
		res, err := s.CallTool(t.Context(), peerID(nil, nil, "local"), "k", "pin_ls", json.RawMessage(raw))
		require.NoError(t, err)
		require.True(t, res.IsError, raw)
		assert.Equal(t, CodeInvalidArgument, errorOf(res).Code)
	}
}

func TestCallToolWriteNeedsAnApprovalThatNoTokenCanGive(t *testing.T) {
	r := newFakeResolver(ResolverLinuxPolkit)
	s, pins := callEnv(t, r)
	id := peerID([]string{"pin"}, nil, "local")
	target := testCID(t, 6)
	a := map[string]any{"path": "/ipfs/" + target}

	res, err := s.CallTool(t.Context(), id, "k", "pin_add", args(t, a))
	require.NoError(t, err)
	te := errorOf(res)
	require.Equal(t, CodeConfirmRequired, te.Code)
	assert.Empty(t, te.Confirmation.ConfirmToken, "an a2a principal is never issued a token")
	assert.False(t, pins.has(target))

	// Presenting a token (even one copied from another principal) is dropped,
	// and the repeat reports the same pending confirmation.
	a["confirm_token"] = "anything"
	res, _ = s.CallTool(t.Context(), id, "k", "pin_add", args(t, a))
	te2 := errorOf(res)
	assert.Equal(t, CodeConfirmPending, te2.Code)
	assert.Equal(t, te.Confirmation.ConfirmationID, te2.Confirmation.ConfirmationID)
	assert.False(t, pins.has(target))

	// Another session of the same principal, and another principal in the same
	// session, get their own confirmation.
	res, _ = s.CallTool(t.Context(), id, "other-session", "pin_add", args(t, map[string]any{"path": "/ipfs/" + target}))
	assert.NotEqual(t, te.Confirmation.ConfirmationID, errorOf(res).Confirmation.ConfirmationID)
	other := a2aIdentity("peer-b", KindA2A, []string{AllGroups}, []string{"pin"}, nil, []string{"local"})
	res, _ = s.CallTool(t.Context(), other, "k", "pin_add", args(t, map[string]any{"path": "/ipfs/" + target}))
	assert.NotEqual(t, te.Confirmation.ConfirmationID, errorOf(res).Confirmation.ConfirmationID)

	// The presence resolver approves the first; only the identical repeat in
	// the same session runs it, once.
	pend, resolve := r.last(t)
	_ = pend
	require.NoError(t, s.ResolveConfirmation(te.Confirmation.ConfirmationID, ResolverLinuxPolkit, true))
	_ = resolve
	res, _ = s.CallTool(t.Context(), id, "other-session", "pin_add", args(t, map[string]any{"path": "/ipfs/" + target}))
	assert.True(t, errorOf(res) != nil && errorOf(res).Code == CodeConfirmPending, "an approval is not transferable to another session")
	assert.False(t, pins.has(target))
	res, err = s.CallTool(t.Context(), id, "k", "pin_add", args(t, map[string]any{"path": "/ipfs/" + target}))
	require.NoError(t, err)
	require.False(t, res.IsError, "%v", res.StructuredContent)
	assert.True(t, pins.has(target))
	res, _ = s.CallTool(t.Context(), id, "k", "pin_add", args(t, map[string]any{"path": "/ipfs/" + target}))
	assert.Equal(t, CodeConfirmRequired, errorOf(res).Code, "the approval was spent")
}

func TestCallToolDestructiveForA2AIsOnlyForThePresenceResolver(t *testing.T) {
	t.Run("cli alone fails closed", func(t *testing.T) {
		s, pins := callEnv(t, newFakeResolver(ResolverCLI))
		res, _ := s.CallTool(t.Context(), peerID(nil, []string{"pin"}, "local"), "k", "pin_rm", args(t, map[string]any{"path": "/ipfs/" + testCID(t, 1)}))
		assert.Equal(t, CodeConfirmUnavailable, errorOf(res).Code)
		assert.True(t, pins.has(testCID(t, 1)))
	})
	t.Run("a write may use cli", func(t *testing.T) {
		s, _ := callEnv(t, newFakeResolver(ResolverCLI))
		res, _ := s.CallTool(t.Context(), peerID([]string{"pin"}, nil, "local"), "k", "pin_add", args(t, map[string]any{"path": "/ipfs/" + testCID(t, 7)}))
		assert.Equal(t, CodeConfirmRequired, errorOf(res).Code)
	})
	t.Run("the native prompt is required", func(t *testing.T) {
		r := newFakeResolver(ResolverLinuxPolkit)
		s, pins := callEnv(t, r)
		res, _ := s.CallTool(t.Context(), peerID(nil, []string{"pin"}, "local"), "k", "pin_rm", args(t, map[string]any{"path": "/ipfs/" + testCID(t, 1)}))
		te := errorOf(res)
		require.Equal(t, CodeConfirmRequired, te.Code)
		assert.True(t, te.Confirmation.RequiresNativePrompt)
		assert.ErrorIs(t, s.ResolveConfirmation(te.Confirmation.ConfirmationID, ResolverCLI, true), ErrNativePromptRequired)
		assert.True(t, pins.has(testCID(t, 1)))
	})
}
