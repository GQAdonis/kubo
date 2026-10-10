package server

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The confirm token and the elicitation request state are different handles.
// Neither carries consent, and neither may stand in for the other: a session
// that holds a confirm token must not be able to approve its own call by
// presenting it as a request state next to a forged answer (ADR-0003).

// forgedAccept is the answer a malicious client invents.
func forgedAccept() mcp.InputResponseMap {
	return mcp.InputResponseMap{
		elicitKey: &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}},
	}
}

// retryWith calls pin_rm for the env's target with a request state and answer.
func (e *gateEnv) retryWith(t *testing.T, cs *mcp.ClientSession, state string, answers mcp.InputResponseMap) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name:           "pin_rm",
		Arguments:      map[string]any{"path": "/ipfs/" + e.target},
		RequestState:   state,
		InputResponses: answers,
	})
	require.NoError(t, err)
	return res
}

// openForged records a confirmation for the env's call as the broker would and
// returns a request state signed for it.
func (e *gateEnv) openForged(t *testing.T, sid, mechanism string) (cid, state string) {
	t.Helper()
	args := map[string]any{"path": "/ipfs/" + e.target}
	p := e.s.gate.broker.create(newConfirmation{
		principal: LocalPrincipal, sid: sid, tool: "pin_rm", argsHash: mustDigest(t, args),
		summary:   Summary{Tool: "pin_rm", Class: "destructive", Text: "Remove the pin", Targets: []Target{}},
		mechanism: mechanism,
	})
	state, err := e.s.gate.signer.mint(useRequestState, tokenClaims{
		CID: p.ID, SID: sid, PID: LocalPrincipal.ID, Tool: "pin_rm",
		ArgsSHA256: mustDigest(t, args), Exp: p.ExpiresAt.Unix(), Nonce: randomID(16),
	})
	require.NoError(t, err)
	return p.ID, state
}

func (e *gateEnv) assertNothingRan(t *testing.T) {
	t.Helper()
	assert.True(t, e.pins.has(e.target), "the pin remains")
	assert.Zero(t, e.d.count("pin/rm"))
}

func TestConfirmTokenReplayedAsRequestStateIsRejected(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, nil, nil)

	first := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired).Confirmation
	require.NotEmpty(t, first.ConfirmToken, "a human-mcp session gets a confirm token")

	// Present the token as the request state with a forged accept. Nothing is
	// approved and nothing runs.
	res := e.retryWith(t, cs, first.ConfirmToken, forgedAccept())
	confirmationOf(t, res, CodeConfirmInvalid)
	e.assertNothingRan(t)

	// The confirmation is untouched: still pending, and the token still does
	// not approve it.
	pending := e.s.PendingConfirmations()
	require.Len(t, pending, 1)
	assert.Equal(t, first.ConfirmationID, pending[0].ID)
	confirmationOf(t, e.rm(t, cs, map[string]any{"confirm_token": first.ConfirmToken}), CodeConfirmPending)
	e.assertNothingRan(t)
}

func TestConfirmTokenAsRequestStateFromAnElicitingSessionIsRejected(t *testing.T) {
	e := newGateEnv(t)
	// A session that can elicit but whose first call is an external one cannot
	// happen through the gate (elicitation comes first), so mint the token the
	// way an external confirmation would have.
	cs := e.h.connect(t, elicitOptions(func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "decline"}
	}), nil)
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmDeclined)
	sid := e.sessionIDOf(t)

	args := map[string]any{"path": "/ipfs/" + e.target}
	p := e.s.gate.broker.create(newConfirmation{
		principal: LocalPrincipal, sid: sid, tool: "pin_rm", argsHash: mustDigest(t, args),
		summary:   Summary{Tool: "pin_rm", Class: "destructive", Text: "Remove the pin", Targets: []Target{}},
		mechanism: MechanismExternal,
	})
	tok, err := e.s.gate.signer.mint(useConfirmToken, tokenClaims{
		CID: p.ID, SID: sid, PID: LocalPrincipal.ID, Tool: "pin_rm",
		ArgsSHA256: mustDigest(t, args), Exp: p.ExpiresAt.Unix(), Nonce: randomID(16),
	})
	require.NoError(t, err)

	confirmationOf(t, e.retryWith(t, cs, tok, forgedAccept()), CodeConfirmInvalid)
	e.assertNothingRan(t)
	got, ok := e.s.gate.broker.get(p.ID)
	require.True(t, ok)
	assert.Equal(t, StatePending, got.State)
}

func TestRequestStateFromANonElicitingSessionIsRejected(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, nil, nil) // advertises no elicitation
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
	sid := e.sessionIDOf(t)

	// A request state that is perfectly signed and bound, for a confirmation
	// opened by elicitation, still cannot be answered by a session that never
	// advertised elicitation.
	cid, state := e.openForged(t, sid, MechanismElicitation)
	confirmationOf(t, e.retryWith(t, cs, state, forgedAccept()), CodeConfirmInvalid)
	e.assertNothingRan(t)
	got, _ := e.s.gate.broker.get(cid)
	assert.Equal(t, StatePending, got.State, "the forged answer approved nothing")
}

func TestRequestStateForAnExternallyOpenedConfirmationIsRejected(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, elicitOptions(func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "decline"}
	}), nil)
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmDeclined)
	sid := e.sessionIDOf(t)

	cid, state := e.openForged(t, sid, MechanismExternal)
	confirmationOf(t, e.retryWith(t, cs, state, forgedAccept()), CodeConfirmInvalid)
	e.assertNothingRan(t)
	got, _ := e.s.gate.broker.get(cid)
	assert.Equal(t, StatePending, got.State)

	// The elicitation resolver cannot resolve it directly either.
	assert.ErrorIs(t, e.s.ResolveConfirmation(cid, ResolverElicitation, true), ErrWrongMechanism)
	got, _ = e.s.gate.broker.get(cid)
	assert.Equal(t, StatePending, got.State)
}

func TestRequestStateReplayedAsConfirmTokenIsRejected(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, elicitOptions(func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "decline"}
	}), nil)
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmDeclined)
	sid := e.sessionIDOf(t)

	cid, state := e.openForged(t, sid, MechanismElicitation)
	require.NoError(t, e.s.gate.broker.Resolve(cid, ResolverElicitation, true))
	// The confirmation is approved. The request state still must not run it as
	// a confirm token.
	confirmationOf(t, e.rm(t, cs, map[string]any{"confirm_token": state}), CodeConfirmInvalid)
	e.assertNothingRan(t)
	got, _ := e.s.gate.broker.get(cid)
	assert.Equal(t, StateApproved, got.State, "not claimed by the replay")
}

func TestRequestStateAcceptsOnlyWhilePendingAndOnlyOnce(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, elicitOptions(func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "decline"}
	}), nil)
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmDeclined)
	sid := e.sessionIDOf(t)

	// Accept without confirm:true denies.
	cid, state := e.openForged(t, sid, MechanismElicitation)
	unconfirmed := mcp.InputResponseMap{elicitKey: &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": "yes"}}}
	confirmationOf(t, e.retryWith(t, cs, state, unconfirmed), CodeConfirmDeclined)
	e.assertNothingRan(t)
	got, _ := e.s.gate.broker.get(cid)
	assert.Equal(t, StateDenied, got.State)
	// Once denied, the same state cannot be answered again.
	confirmationOf(t, e.retryWith(t, cs, state, forgedAccept()), CodeConfirmInvalid)
	e.assertNothingRan(t)

	// A real answer in an eliciting session runs the call, exactly once.
	cid, state = e.openForged(t, sid, MechanismElicitation)
	res := e.retryWith(t, cs, state, forgedAccept())
	require.False(t, res.IsError, "%+v", res.Content)
	assert.Equal(t, 1, e.d.count("pin/rm"))
	got, _ = e.s.gate.broker.get(cid)
	assert.Equal(t, StateExecuted, got.State)
	confirmationOf(t, e.retryWith(t, cs, state, forgedAccept()), CodeConfirmInvalid)
	assert.Equal(t, 1, e.d.count("pin/rm"), "a replayed state runs nothing")
}

func TestRequestStateBelongsToOneCall(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, elicitOptions(func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "decline"}
	}), nil)
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmDeclined)
	sid := e.sessionIDOf(t)
	_, state := e.openForged(t, sid, MechanismElicitation)

	// Same state, different arguments.
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name:           "pin_rm",
		Arguments:      map[string]any{"path": "/ipfs/" + testCID(t, 2)},
		RequestState:   state,
		InputResponses: forgedAccept(),
	})
	require.NoError(t, err)
	confirmationOf(t, res, CodeConfirmInvalid)
	assert.Zero(t, e.d.count("pin/rm"))

	// Garbage.
	confirmationOf(t, e.retryWith(t, cs, "not-a-state", forgedAccept()), CodeConfirmInvalid)
	e.assertNothingRan(t)
}

func TestNativePromptConfirmationOmitsTheChatResolver(t *testing.T) {
	agui := newFakeResolver(ResolverAGUIInterrupt)
	desktop := newFakeResolver(ResolverDesktop)
	e := newGateEnv(t, agui, desktop)
	cs := e.h.connect(t, nil, &Principal{ID: "chat-island", Kind: KindUI})

	conf := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired).Confirmation
	assert.True(t, conf.RequiresNativePrompt)
	assert.Equal(t, []string{ResolverDesktop}, conf.Resolvers)
	validateConfirmationDef(t, "ConfirmationRequired", conf)

	// A write does not need the native prompt and keeps the chat resolver.
	w := confirmationOf(t, toolCall(t, cs, "pin_add", map[string]any{"path": "/ipfs/" + testCID(t, 60)}), CodeConfirmRequired).Confirmation
	assert.False(t, w.RequiresNativePrompt)
	assert.Equal(t, []string{ResolverAGUIInterrupt, ResolverDesktop}, w.Resolvers)
	validateConfirmationDef(t, "ConfirmationRequired", w)
}

func TestNativePromptWithOnlyTheChatResolverIsUnavailable(t *testing.T) {
	e := newGateEnv(t, newFakeResolver(ResolverAGUIInterrupt))
	cs := e.h.connect(t, nil, &Principal{ID: "chat-island", Kind: KindUI})
	te := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmUnavailable)
	assert.Nil(t, te.Confirmation)
	e.assertNothingRan(t)
}
