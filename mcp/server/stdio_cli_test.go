package server

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The cli resolver never approves the stdio principal: stdio cannot assert
// shell_capable: false (R12 SEC-M1).

func TestCLIResolverIsNotOfferedToTheStdioPrincipal(t *testing.T) {
	cli := newFakeResolver(ResolverCLI)
	e := newGateEnv(t, cli)

	// Non-eliciting stdio session (the default principal is LocalPrincipal):
	// with cli as the only resolver the call is refused, and no pending
	// confirmation is opened or listed.
	cs := e.h.connect(t, nil, nil)
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmUnavailable)
	assert.Empty(t, e.s.PendingConfirmations())
	assert.Zero(t, e.d.count("pin/rm"))
	assert.True(t, e.pins.has(e.target))
}

func TestCLIResolverStillServesAConfiguredHumanMCPPrincipal(t *testing.T) {
	cli := newFakeResolver(ResolverCLI)
	e := newGateEnv(t, cli)
	cs := e.h.connect(t, nil, &Principal{ID: "asserted", Kind: KindHumanMCP})

	res := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
	assert.Equal(t, []string{ResolverCLI}, res.Confirmation.Resolvers)
}

func TestCLIResolverCannotResolveAStdioConfirmationEvenIfAsked(t *testing.T) {
	// A second resolver keeps the confirmation open; the broker still refuses
	// a cli decision for the stdio principal.
	cli := newFakeResolver(ResolverCLI)
	other := newFakeResolver(ResolverLinuxPolkit)
	e := newGateEnv(t, cli, other)
	cs := e.h.connect(t, nil, nil)
	res := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
	assert.Equal(t, []string{ResolverLinuxPolkit}, res.Confirmation.Resolvers, "cli is not listed for stdio")
	assert.ErrorIs(t, e.s.gate.broker.Resolve(res.Confirmation.ConfirmationID, ResolverCLI, true), ErrCLINotForStdio)
	assert.NoError(t, e.s.gate.broker.Resolve(res.Confirmation.ConfirmationID, ResolverLinuxPolkit, false))
}

func TestElicitationStillWorksForStdioWhenCLIIsTheOnlyResolver(t *testing.T) {
	cli := newFakeResolver(ResolverCLI)
	e := newGateEnv(t, cli)
	cs := e.h.connect(t, elicitOptions(func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	}), nil)
	res := e.rm(t, cs, nil)
	require.False(t, res.IsError, "%+v", res.StructuredContent)
	assert.Equal(t, 1, e.d.count("pin/rm"))
}
