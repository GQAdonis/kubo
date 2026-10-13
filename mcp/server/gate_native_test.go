package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUIDestructiveWithOnlyCLIResolverIsRefused(t *testing.T) {
	cli := newFakeResolver(ResolverCLI)
	e := newGateEnv(t, cli)
	cs := e.h.connect(t, nil, &Principal{ID: "chat-island", Kind: KindUI})

	te := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmUnavailable)
	assert.Contains(t, te.Message, "native system prompt")
	assert.Empty(t, cli.requests, "the cli resolver is never asked")
	assert.Zero(t, e.d.count("pin/rm"))
	assert.True(t, e.pins.has(e.target))
}

func TestUIDestructiveWaitsForTheNativePromptNotTheCLI(t *testing.T) {
	cli := newFakeResolver(ResolverCLI)
	native := newFakeResolver(ResolverMacOSLocalAuth)
	e := newGateEnv(t, cli, native)
	cs := e.h.connect(t, nil, &Principal{ID: "chat-island", Kind: KindUI})

	conf := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired).Confirmation
	assert.True(t, conf.RequiresNativePrompt)
	assert.Equal(t, []string{ResolverMacOSLocalAuth}, conf.Resolvers, "cli is not offered for a native-prompt confirmation")

	_, approveCLI := cli.last(t)
	assert.ErrorIs(t, approveCLI(true), ErrNativePromptRequired)
	assert.Zero(t, e.d.count("pin/rm"))
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmPending)

	_, approveNative := native.last(t)
	require.NoError(t, approveNative(true))
	require.False(t, e.rm(t, cs, nil).IsError)
	assert.Equal(t, 1, e.d.count("pin/rm"))
}

func TestIsPresenceResolver(t *testing.T) {
	for _, n := range []string{ResolverMacOSLocalAuth, ResolverLinuxPolkit} {
		assert.True(t, IsPresenceResolver(n), n)
	}
	// Desktop is not a presence resolver until a Desktop confirmation window
	// consumes the pairing key (R11-H4).
	for _, n := range []string{ResolverDesktop, ResolverCLI, ResolverAGUIInterrupt, ResolverElicitation, ""} {
		assert.False(t, IsPresenceResolver(n), n)
	}
}
