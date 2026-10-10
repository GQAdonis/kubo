package server

import (
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Task 1.5: external resolution, A2A input-required waits, and no
// self-confirmation by principals that cannot elicit.

func TestPrincipalsThatCannotElicitNeverGetATokenAndCannotUseOne(t *testing.T) {
	for _, kind := range []string{KindA2A, KindUAR} {
		t.Run(kind, func(t *testing.T) {
			e := newGateEnv(t)
			agent := &Principal{ID: "agent-1", Kind: kind}
			cs := e.h.connect(t, nil, agent)

			// First call: input-required style result, never a token.
			first := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
			require.NotNil(t, first.Confirmation)
			assert.Empty(t, first.Confirmation.ConfirmToken)
			assert.Equal(t, kind, first.Confirmation.PrincipalKind)
			validateConfirmationDef(t, "ConfirmationRequired", first.Confirmation)
			assert.True(t, e.pins.has(e.target))
			_, resolve := e.resolver.last(t)

			// The agent forges the best token it can: a genuinely signed one for
			// exactly this call. It is still refused, and the pending
			// confirmation keeps waiting for the out-of-band resolver.
			forged, err := e.s.gate.signer.mint(useConfirmToken, tokenClaims{
				CID:        first.Confirmation.ConfirmationID,
				SID:        e.sessionIDOf(t),
				PID:        agent.ID,
				Tool:       "pin_rm",
				ArgsSHA256: mustDigest(t, map[string]any{"path": "/ipfs/" + e.target}),
				Exp:        first.Confirmation.ExpiresAt.Unix(),
				Nonce:      randomID(16),
			})
			require.NoError(t, err)
			rejected := confirmationOf(t, e.rm(t, cs, map[string]any{"confirm_token": forged}), CodeConfirmInvalid)
			assert.Contains(t, rejected.Message, "not accepted")
			assert.True(t, e.pins.has(e.target))
			assert.Zero(t, e.d.count("pin/rm"))

			// Waiting: repeating the call reports it is still pending.
			pending := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmPending)
			assert.Empty(t, pending.Confirmation.ConfirmToken)
			assert.Equal(t, first.Confirmation.ConfirmationID, pending.Confirmation.ConfirmationID, "same confirmation, not a new one")

			// A forged token cannot approve even after the human did: tokens
			// from these principals are never read.
			require.NoError(t, resolve(true))
			confirmationOf(t, e.rm(t, cs, map[string]any{"confirm_token": forged}), CodeConfirmInvalid)
			assert.Zero(t, e.d.count("pin/rm"))

			// The approved call runs exactly once, on a plain repeat.
			res := e.rm(t, cs, nil)
			require.False(t, res.IsError, "%+v", res.Content)
			assert.Equal(t, 1, e.d.count("pin/rm"))
			assert.False(t, e.pins.has(e.target))

			// Another repeat is a new call and needs a new approval.
			again := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
			assert.NotEqual(t, first.Confirmation.ConfirmationID, again.Confirmation.ConfirmationID)
			assert.Equal(t, 1, e.d.count("pin/rm"))
		})
	}
}

func TestDeniedExternalConfirmationForAnAgentStaysDenied(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, nil, &Principal{ID: "a2a-peer", Kind: KindA2A})
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
	_, resolve := e.resolver.last(t)
	require.NoError(t, resolve(false))
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmDeclined)
	assert.Zero(t, e.d.count("pin/rm"))
	assert.True(t, e.pins.has(e.target))
}

func TestConfirmationIsBoundToThePrincipalAndSession(t *testing.T) {
	e := newGateEnv(t)
	a := e.h.connect(t, nil, &Principal{ID: "agent-a", Kind: KindUAR})
	b := e.h.connect(t, nil, &Principal{ID: "agent-b", Kind: KindUAR})

	confirmationOf(t, e.rm(t, a, nil), CodeConfirmRequired)
	_, resolve := e.resolver.last(t)
	require.NoError(t, resolve(true))

	// Another agent making the same call gets its own confirmation; it cannot
	// ride on agent A's approval.
	other := confirmationOf(t, e.rm(t, b, nil), CodeConfirmRequired)
	assert.NotEmpty(t, other.Confirmation.ConfirmationID)
	assert.Zero(t, e.d.count("pin/rm"))

	require.False(t, e.rm(t, a, nil).IsError)
	assert.Equal(t, 1, e.d.count("pin/rm"))
}

func TestUIPrincipalDestructiveCallNeedsANativePrompt(t *testing.T) {
	agui := newFakeResolver(ResolverAGUIInterrupt)
	e := newGateEnv(t, agui, newFakeResolver(ResolverDesktop))
	cs := e.h.connect(t, nil, &Principal{ID: "chat-island", Kind: KindUI})

	conf := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired).Confirmation
	validateConfirmationDef(t, "ConfirmationRequired", conf)
	assert.True(t, conf.RequiresNativePrompt)
	assert.NotContains(t, conf.Resolvers, ResolverAGUIInterrupt)
	p, resolve := agui.last(t)
	assert.True(t, p.NativePrompt)

	// The chat answer alone does not approve a destructive call.
	assert.ErrorIs(t, resolve(true), ErrNativePromptRequired)
	assert.Zero(t, e.d.count("pin/rm"))
	// The chat can still deny.
	require.NoError(t, resolve(false))
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmDeclined)

	// A native prompt resolver may approve a fresh confirmation.
	again := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired).Confirmation
	require.NoError(t, e.s.ResolveConfirmation(again.ConfirmationID, ResolverDesktop, true))
	require.False(t, e.rm(t, cs, nil).IsError)
	assert.Equal(t, 1, e.d.count("pin/rm"))
}

func TestUIPrincipalWriteCanBeApprovedByTheChat(t *testing.T) {
	agui := newFakeResolver(ResolverAGUIInterrupt)
	e := newGateEnv(t, agui)
	cs := e.h.connect(t, nil, &Principal{ID: "chat-island", Kind: KindUI})
	args := map[string]any{"path": "/ipfs/" + testCID(t, 50)}

	require.True(t, toolCall(t, cs, "pin_add", args).IsError)
	p, resolve := agui.last(t)
	assert.False(t, p.NativePrompt, "only destructive calls require the native prompt")
	require.NoError(t, resolve(true))
	require.False(t, toolCall(t, cs, "pin_add", args).IsError)
	assert.Equal(t, 1, e.d.count("pin/add"))
}

func TestWriteClassCatalogAndBrokerAgree(t *testing.T) {
	cat, err := contract.Load()
	require.NoError(t, err)
	tool, _ := cat.Tool("pin_add")
	assert.Equal(t, contract.ClassWrite, tool.Confirmation)
	for _, name := range []string{"pin_update", "pin_rm"} {
		tool, _ := cat.Tool(name)
		assert.Equal(t, contract.ClassDestructive, tool.Confirmation, name)
	}
}

func TestElicitingClientIsNotSentToTheBroker(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, elicitOptions(func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	}), nil)
	require.False(t, e.rm(t, cs, nil).IsError)
	e.resolver.mu.Lock()
	defer e.resolver.mu.Unlock()
	assert.Empty(t, e.resolver.requests, "elicitation comes first; no external prompt is raised")
}

// sessionIDOf returns the gate's session ID for the only live session.
func (e *gateEnv) sessionIDOf(t *testing.T) string {
	t.Helper()
	e.s.gate.mu.Lock()
	defer e.s.gate.mu.Unlock()
	require.NotEmpty(t, e.s.gate.localIDs)
	var id string
	for _, v := range e.s.gate.localIDs {
		id = v
	}
	return id
}

func mustDigest(t *testing.T, args map[string]any) string {
	t.Helper()
	d, _, _, err := argsDigest(asJSON(t, args))
	require.NoError(t, err)
	return d
}
