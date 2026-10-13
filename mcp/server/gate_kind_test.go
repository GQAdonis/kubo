package server

import (
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acceptingElicitor answers every elicitation with yes and counts them.
func acceptingElicitor(asked *atomic.Int32) *mcp.ClientOptions {
	return elicitOptions(func(*mcp.ElicitRequest) *mcp.ElicitResult {
		asked.Add(1)
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	})
}

// A ui, uar or a2a principal can declare the elicitation capability and answer
// its own prompt. Elicitation is for a human-driven MCP host only (R11-H1).
func TestElicitationIsOnlyForHumanMCPPrincipals(t *testing.T) {
	for _, kind := range []string{KindUI, KindUAR, KindA2A} {
		t.Run(kind, func(t *testing.T) {
			native := newFakeResolver(ResolverMacOSLocalAuth)
			e := newGateEnv(t, native)
			var asked atomic.Int32
			cs := e.h.connect(t, acceptingElicitor(&asked), &Principal{ID: "p-" + kind, Kind: kind})

			te := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
			assert.Zero(t, asked.Load(), "the principal's own elicitation handler is never asked")
			assert.Zero(t, e.d.count("pin/rm"), "nothing ran")
			assert.True(t, e.pins.has(e.target), "the pin is still there")
			if kind != KindUAR {
				assert.True(t, te.Confirmation.RequiresNativePrompt, "a destructive %s call needs the native prompt", kind)
			}
			assert.Equal(t, []string{ResolverMacOSLocalAuth}, te.Confirmation.Resolvers)
		})
	}
}

func TestHumanMCPPrincipalStillConfirmsThroughElicitation(t *testing.T) {
	e := newGateEnv(t, newFakeResolver(ResolverMacOSLocalAuth))
	var asked atomic.Int32
	cs := e.h.connect(t, acceptingElicitor(&asked), &Principal{ID: "host", Kind: KindHumanMCP})

	res := e.rm(t, cs, nil)
	require.False(t, res.IsError, "%+v", res.StructuredContent)
	assert.Equal(t, int32(1), asked.Load())
	assert.Equal(t, 1, e.d.count("pin/rm"))
}

func TestUIDestructiveWithoutNativePromptIsRefusedEvenWithElicitation(t *testing.T) {
	e := newGateEnv(t, newFakeResolver(ResolverCLI))
	var asked atomic.Int32
	cs := e.h.connect(t, acceptingElicitor(&asked), &Principal{ID: "chat", Kind: KindUI})

	te := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmUnavailable)
	assert.Contains(t, te.Message, "native system prompt")
	assert.Zero(t, asked.Load())
	assert.Zero(t, e.d.count("pin/rm"))
}

// A request state minted for a ui principal's elicitation confirmation does not
// let that principal approve: elicitAnswer checks the kind again.
func TestElicitAnswerRechecksThePrincipalKind(t *testing.T) {
	e := newGateEnv(t, newFakeResolver(ResolverMacOSLocalAuth))
	ct, st := mcp.NewInMemoryTransports()
	ss, err := e.s.MCP().Connect(t.Context(), st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	var asked atomic.Int32
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, acceptingElicitor(&asked)).Connect(t.Context(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	require.True(t, canElicit(ss), "the session advertised elicitation")

	rt := e.s.registry["pin_rm"]
	args := map[string]any{"path": "/ipfs/" + e.target}
	c := &call{
		ss: ss, principal: Principal{ID: "chat", Kind: KindUI}, sid: "s1",
		tool: rt.tool, argsHash: "h", args: args, sc: rt.sc,
	}
	p, err := e.s.gate.open(c, false, MechanismElicitation)
	require.NoError(t, err)
	state, err := e.s.gate.mintFor(useRequestState, c, p)
	require.NoError(t, err)

	answer := mcp.InputResponseMap{elicitKey: &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}}
	te := e.s.gate.elicitAnswer(c, state, answer)
	require.NotNil(t, te, "a ui principal cannot approve through elicitation")
	assert.Equal(t, CodeConfirmInvalid, te.Code)
	got, ok := e.s.gate.broker.get(p.ID)
	require.True(t, ok)
	assert.Equal(t, StatePending, got.State, "nothing was approved")

	// The same state for a human-mcp principal does approve, as before.
	human := *c
	human.principal = Principal{ID: "chat", Kind: KindHumanMCP}
	p2, err := e.s.gate.open(&human, false, MechanismElicitation)
	require.NoError(t, err)
	state2, err := e.s.gate.mintFor(useRequestState, &human, p2)
	require.NoError(t, err)
	assert.Nil(t, e.s.gate.elicitAnswer(&human, state2, answer))
}
