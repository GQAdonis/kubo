package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func capConfirmation(principalID, sid, hash, mechanism string) newConfirmation {
	return newConfirmation{
		principal: Principal{ID: principalID, Kind: KindUI}, sid: sid, tool: "pin_rm",
		argsHash: hash, mechanism: mechanism,
	}
}

func TestBrokerCapsOpenExternalConfirmationsPerPrincipalAndSession(t *testing.T) {
	b := newBroker(time.Now, time.Minute)

	// Per principal: two sessions of one principal share the cap.
	_, err := b.reserve(capConfirmation("p1", "s1", "a", MechanismExternal), maxOpenExternal)
	require.NoError(t, err)
	_, err = b.reserve(capConfirmation("p1", "s2", "b", MechanismExternal), maxOpenExternal)
	require.NoError(t, err)
	_, err = b.reserve(capConfirmation("p1", "s3", "c", MechanismExternal), maxOpenExternal)
	require.ErrorIs(t, err, ErrTooManyOpen)

	// Per session: two principals in one session share the cap.
	_, err = b.reserve(capConfirmation("q1", "shared", "d", MechanismExternal), maxOpenExternal)
	require.NoError(t, err)
	_, err = b.reserve(capConfirmation("q2", "shared", "e", MechanismExternal), maxOpenExternal)
	require.NoError(t, err)
	_, err = b.reserve(capConfirmation("q3", "shared", "f", MechanismExternal), maxOpenExternal)
	require.ErrorIs(t, err, ErrTooManyOpen)

	// Other principals are not affected.
	_, err = b.reserve(capConfirmation("other", "s9", "g", MechanismExternal), maxOpenExternal)
	require.NoError(t, err)
}

func TestBrokerCapFreesASlotWhenAConfirmationIsDecided(t *testing.T) {
	b := newBroker(time.Now, time.Minute)
	first, err := b.reserve(capConfirmation("p1", "s1", "a", MechanismExternal), maxOpenExternal)
	require.NoError(t, err)
	_, err = b.reserve(capConfirmation("p1", "s1", "b", MechanismExternal), maxOpenExternal)
	require.NoError(t, err)
	_, err = b.reserve(capConfirmation("p1", "s1", "c", MechanismExternal), maxOpenExternal)
	require.ErrorIs(t, err, ErrTooManyOpen)

	require.NoError(t, b.Resolve(first.ID, ResolverMacOSLocalAuth, false))
	_, err = b.reserve(capConfirmation("p1", "s1", "c", MechanismExternal), maxOpenExternal)
	require.NoError(t, err, "a denied confirmation no longer counts")
}

func TestBrokerCapDoesNotApplyToElicitationOrUnlimitedCreate(t *testing.T) {
	b := newBroker(time.Now, time.Minute)
	for _, h := range []string{"a", "b", "c", "d"} {
		_, err := b.reserve(capConfirmation("p1", "s1", h, MechanismElicitation), maxOpenExternal)
		require.NoError(t, err, "an elicitation is answered in the client, not by a system prompt")
	}
	for _, h := range []string{"e", "f", "g"} {
		b.create(capConfirmation("p2", "s2", h, MechanismExternal))
	}
	assert.Len(t, b.Pending(), 7)
}

// A prompt-injected loop of distinct destructive calls gets at most two system
// prompts; the rest are refused with confirmation_pending (R11-M3).
func TestGateRefusesExtraOpenConfirmations(t *testing.T) {
	native := newFakeResolver(ResolverMacOSLocalAuth)
	e := newGateEnv(t, native)
	cs := e.h.connect(t, nil, &Principal{ID: "chat", Kind: KindUI})

	call := func(n int) string {
		res := toolCall(t, cs, "pin_rm", map[string]any{"path": "/ipfs/" + testCID(t, n)})
		require.True(t, res.IsError)
		return decodeToolError(t, res).Code
	}
	assert.Equal(t, CodeConfirmRequired, call(10))
	assert.Equal(t, CodeConfirmRequired, call(11))
	for _, n := range []int{12, 13, 14} {
		assert.Equal(t, CodeConfirmPending, call(n))
	}
	native.mu.Lock()
	prompts := len(native.requests)
	native.mu.Unlock()
	assert.Equal(t, 2, prompts, "only two system prompts were raised")

	// The refused call has a reason and is retryable once a prompt is answered.
	res := toolCall(t, cs, "pin_rm", map[string]any{"path": "/ipfs/" + testCID(t, 12)})
	te := decodeToolError(t, res)
	assert.True(t, te.Retryable)
	assert.Equal(t, "too_many_open_confirmations", te.Details["reason"])
	assert.Zero(t, e.d.count("pin/rm"))
}
