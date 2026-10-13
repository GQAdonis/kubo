package server

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenRoundTripAndContractShape(t *testing.T) {
	signer, err := newTokenSigner()
	require.NoError(t, err)
	claims := tokenClaims{
		CID: "cf_3pQm8vK2xR7tLw9a", SID: "local-abc", PID: "local-stdio", Tool: "pin_rm",
		ArgsSHA256: "9f2b5c1e0d4a7b3c6e8f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d",
		Exp:        1791547320, Nonce: randomID(16),
	}
	tok, err := signer.mint(useConfirmToken, claims)
	require.NoError(t, err)

	assert.Regexp(t, regexp.MustCompile(`^[A-Za-z0-9_-]{32,1000}$`), tok, "inside the contract's ConfirmToken pattern")
	validateAgainstCommon(t, "ConfirmToken", tok)

	got, err := signer.verify(useConfirmToken, tok)
	require.NoError(t, err)
	claims.V = tokenVersion
	claims.Use = useConfirmToken
	assert.Equal(t, claims, got)
	validateConfirmationDef(t, "ConfirmTokenClaims", got)
}

func TestTokenIsBoundToTheProcessKey(t *testing.T) {
	a, err := newTokenSigner()
	require.NoError(t, err)
	b, err := newTokenSigner()
	require.NoError(t, err)
	tok, err := a.mint(useConfirmToken, tokenClaims{CID: "cf_aaaaaaaaaaaaaaaa", Tool: "pin_rm", Nonce: randomID(16)})
	require.NoError(t, err)
	_, err = b.verify(useConfirmToken, tok)
	assert.ErrorIs(t, err, errBadToken, "a restart (new key) invalidates outstanding tokens")
}

func TestTokenRejectsShortAndMalformed(t *testing.T) {
	signer, err := newTokenSigner()
	require.NoError(t, err)
	for _, tok := range []string{"", "abc", "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"} {
		_, err := signer.verify(useConfirmToken, tok)
		assert.Error(t, err, tok)
	}
}

func TestCanonicalArgsIgnoreKeyOrderSpacingAndNumberSpelling(t *testing.T) {
	digest := func(s string) string {
		d, _, _, err := argsDigest(json.RawMessage(s))
		require.NoError(t, err)
		return d
	}
	base := digest(`{"path":"/ipfs/x","recursive":true,"limit":100}`)
	assert.Equal(t, base, digest(`{ "limit": 100, "recursive": true, "path": "/ipfs/x" }`), "key order and whitespace")
	assert.Equal(t, base, digest(`{"path":"/ipfs/x","recursive":true,"limit":1e2}`), "1e2 is 100")
	assert.Equal(t, base, digest(`{"path":"/ipfs/x","recursive":true,"limit":100.0}`), "100.0 is 100")
	assert.Equal(t, base, digest(`{"path":"/ipfs/x","recursive":true,"limit":100,"confirm_token":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`), "the token is not part of what is bound")

	assert.NotEqual(t, base, digest(`{"path":"/ipfs/y","recursive":true,"limit":100}`), "different value")
	assert.NotEqual(t, base, digest(`{"path":"/ipfs/x","recursive":false,"limit":100}`), "different boolean")
	assert.NotEqual(t, base, digest(`{"path":"/ipfs/x","recursive":true,"limit":101}`), "different number")
	assert.NotEqual(t, base, digest(`{"path":"/ipfs/x","recursive":true}`), "missing key")
	assert.NotEqual(t,
		digest(`{"n":9007199254740993}`), digest(`{"n":9007199254740992}`),
		"large integers beyond float64 precision stay distinct")
	assert.NotEqual(t, digest(`{"a":"1"}`), digest(`{"a":1}`), "string versus number")
	assert.NotEqual(t, digest(`{"a":[1,2]}`), digest(`{"a":[2,1]}`), "array order matters")
}

func TestCanonicalJSONForm(t *testing.T) {
	cases := map[string]string{
		`{"b":1,"a":[true,null,"x"]}`: `{"a":[true,null,"x"],"b":1}`,
		`{"s":"<&>"}`:                 `{"s":"<&>"}`,
		`{"n":-0}`:                    `{"n":0}`,
		`{"n":1.5}`:                   `{"n":1.5}`,
		`{"n":1e21}`:                  `{"n":1e+21}`,
		`{"n":0.0000001}`:             `{"n":1e-7}`,
	}
	for in, want := range cases {
		_, _, args, err := argsDigest(json.RawMessage(in))
		require.NoError(t, err, in)
		got, err := canonicalJSON(args)
		require.NoError(t, err)
		assert.Equal(t, want, string(got), in)
	}
}

func TestBrokerStateMachine(t *testing.T) {
	clock := newFakeClock()
	b := newBroker(clock.Now, confirmTTL)
	mk := func() Pending {
		return b.create(newConfirmation{
			principal: LocalPrincipal, sid: "s", tool: "pin_rm", argsHash: "h",
			summary: Summary{Tool: "pin_rm", Class: "destructive", Text: "Remove it", Targets: []Target{}},
		})
	}

	// pending -> approved -> executed, once.
	p := mk()
	assert.Equal(t, StatePending, p.State)
	assert.ErrorIs(t, b.claim(p.ID, ""), errNotApproved, "a pending confirmation cannot be claimed")
	require.NoError(t, b.Resolve(p.ID, ResolverDesktop, true))
	require.NoError(t, b.claim(p.ID, "n1"))
	assert.ErrorIs(t, b.claim(p.ID, "n2"), errNotApproved, "executes at most once")
	got, _ := b.get(p.ID)
	assert.Equal(t, StateExecuted, got.State)
	assert.True(t, b.nonceSpent("n1"))

	// pending -> denied is final.
	p = mk()
	require.NoError(t, b.Resolve(p.ID, ResolverDesktop, false))
	assert.ErrorIs(t, b.Resolve(p.ID, ResolverDesktop, true), ErrConfirmationResolved)
	assert.ErrorIs(t, b.claim(p.ID, ""), errNotApproved)

	// pending -> expired; approved -> expired.
	p = mk()
	clock.Advance(confirmTTL)
	assert.ErrorIs(t, b.Resolve(p.ID, ResolverDesktop, true), ErrConfirmationExpired)
	p = mk()
	require.NoError(t, b.Resolve(p.ID, ResolverDesktop, true))
	clock.Advance(confirmTTL)
	assert.ErrorIs(t, b.claim(p.ID, ""), errNotApproved)
	got, _ = b.get(p.ID)
	assert.Equal(t, StateExpired, got.State)

	assert.ErrorIs(t, b.Resolve("cf_nope", ResolverDesktop, true), ErrUnknownConfirmation)
}

func TestBrokerSweepForgetsOldRecords(t *testing.T) {
	clock := newFakeClock()
	b := newBroker(clock.Now, confirmTTL)
	old := b.create(newConfirmation{principal: LocalPrincipal, sid: "s", tool: "t", argsHash: "h"})
	clock.Advance(3 * confirmTTL)
	b.create(newConfirmation{principal: LocalPrincipal, sid: "s", tool: "t", argsHash: "h2"})
	_, ok := b.get(old.ID)
	assert.False(t, ok)
}

func TestHandlesAreSeparatedByUse(t *testing.T) {
	signer, err := newTokenSigner()
	require.NoError(t, err)
	claims := tokenClaims{
		CID: "cf_aaaaaaaaaaaaaaaa", SID: "s", PID: "p", Tool: "pin_rm", Nonce: randomID(16),
		ArgsSHA256: strings.Repeat("a", 64), Exp: 1791547320,
	}
	confirm, err := signer.mint(useConfirmToken, claims)
	require.NoError(t, err)
	state, err := signer.mint(useRequestState, claims)
	require.NoError(t, err)

	_, err = signer.verify(useRequestState, confirm)
	assert.ErrorIs(t, err, errBadToken, "a confirm token is not a request state")
	_, err = signer.verify(useConfirmToken, state)
	assert.ErrorIs(t, err, errBadToken, "a request state is not a confirm token")

	got, err := signer.verify(useRequestState, state)
	require.NoError(t, err)
	assert.Equal(t, useRequestState, got.Use)
	validateConfirmationDef(t, "RequestStateClaims", got)

	_, err = signer.mint("other", claims)
	assert.Error(t, err)

	// Even signed with the other key, a claims body naming the wrong use fails.
	body, err := json.Marshal(tokenClaims{V: tokenVersion, Use: useConfirmToken})
	require.NoError(t, err)
	payload := base64.RawURLEncoding.EncodeToString(body)
	_, err = signer.verify(useRequestState, payload+signer.mac(useRequestState, payload))
	assert.ErrorIs(t, err, errBadToken, "the use claim must match")
}
