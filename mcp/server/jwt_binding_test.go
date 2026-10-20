package server

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	bindSubA = "subject-a-0000-4000-8000-0000000000a1"
	bindSubB = "subject-b-0000-4000-8000-0000000000b2"
)

const pingBody = `{"jsonrpc":"2.0","id":2,"method":"ping"}`

// Two people mapped to one principal cannot continue each other's MCP session.
func TestSessionIdentitiesBindPerSubjectOnMCPSession(t *testing.T) {
	e := newSessionEnv(t, sessionEnvOptions{
		oidc: func(s *OIDCSettings) {
			s.Routes = append(s.Routes, OIDCRouteMCP)
			s.RequireOrigin = false // the MCP client sends no Origin
		},
		principals: []SessionPrincipal{{
			Identity: NewIdentity(Principal{ID: "shared-ui", Kind: KindUI}, []string{AllGroups}, nil, nil),
			Subjects: []string{bindSubA, bindSubB},
		}},
	})
	tokA, tokB := e.token(bindSubA, nil), e.token(bindSubB, nil)

	open := e.do(t, rawReq{path: RouteMCP, token: tokA, body: initializeBody})
	require.Equal(t, http.StatusOK, open.StatusCode)
	sid := open.Header.Get("Mcp-Session-Id")
	require.NotEmpty(t, sid)

	cont := func(token string) (int, string, http.Header) {
		resp := e.do(t, rawReq{path: RouteMCP, token: token, body: pingBody, header: map[string]string{"Mcp-Session-Id": sid}})
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, string(body), resp.Header
	}

	code, _, _ := cont(tokA)
	assert.Equal(t, http.StatusOK, code, "A continues A's session")

	code, body, hdr := cont(tokB)
	assert.Equal(t, http.StatusForbidden, code, "B presenting A's session is refused as a session mismatch")
	for _, sub := range []string{bindSubA, bindSubB} {
		assert.NotContains(t, body, sub)
		for k, vs := range hdr {
			assert.NotContains(t, strings.Join(vs, ","), sub, k)
		}
	}
	assert.NotContains(t, body, "#", "the binding key stays internal")

	// A's session is untouched by B's attempt.
	code, _, _ = cont(tokA)
	assert.Equal(t, http.StatusOK, code)
}

// A static bearer keeps the principal ID as its binding.
func TestStaticIdentityBindingIsThePrincipalID(t *testing.T) {
	assert.Equal(t, "writer", Principal{ID: "writer", Kind: KindUI}.BindingKey())
	a := Principal{ID: "shared-ui", Kind: KindUI, sub: bindSubA}
	b := Principal{ID: "shared-ui", Kind: KindUI, sub: bindSubB}
	assert.NotEqual(t, a.BindingKey(), b.BindingKey())
	assert.Equal(t, a.BindingKey(), Principal{ID: "shared-ui", Kind: KindUI, sub: bindSubA}.BindingKey())
	assert.True(t, strings.HasPrefix(a.BindingKey(), "shared-ui#"))
	assert.NotContains(t, a.BindingKey(), bindSubA, "only a digest of the subject is in the key")
}

// The verifier stamps the subject on the principal of a session identity only.
func TestVerifierStampsTheSubjectOnSessionIdentities(t *testing.T) {
	iss := newTestIssuer(t)
	v := newTestJWTVerifier(t, iss, nil)
	id, _, err := v.Verify(t.Context(), iss.sign(testKidRSA, claimsAt(jwtTestNow, jwtTestSub)), reqTo(RouteAGUI))
	require.NoError(t, err)
	assert.NotEqual(t, id.ID, id.BindingKey())
	assert.Equal(t, id.ID, id.Principal.ID, "the display ID is unchanged")

	static, _, err := testVerifier(t).Verify(t.Context(), tokChat, reqTo(RouteAGUI))
	require.NoError(t, err)
	assert.Equal(t, static.ID, static.BindingKey())
}

// Two subjects on one principal and one session ID cannot answer each other's
// pending confirmations, nor see each other's open call.
func TestConfirmationsBindPerSubject(t *testing.T) {
	e := newGateEnv(t, newFakeResolver(ResolverMacOSLocalAuth))
	ct, st := mcp.NewInMemoryTransports()
	ss, err := e.s.MCP().Connect(t.Context(), st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, elicitOptions(func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	})).Connect(t.Context(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	rt := e.s.registry["pin_rm"]
	args := map[string]any{"path": "/ipfs/" + e.target}
	mk := func(sub string) *call {
		return &call{
			ss: ss, principal: Principal{ID: "shared", Kind: KindHumanMCP, sub: sub}, sid: "shared-session",
			tool: rt.tool, argsHash: "h", args: args, sc: rt.sc,
		}
	}
	a, b := mk(bindSubA), mk(bindSubB)
	g := e.s.gate

	// Elicitation: A opens, B cannot answer with A's request state.
	p, err := g.open(a, false, MechanismElicitation)
	require.NoError(t, err)
	state, err := g.mintFor(useRequestState, a, p)
	require.NoError(t, err)
	answer := mcp.InputResponseMap{elicitKey: &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}}
	te := g.elicitAnswer(b, state, answer)
	require.NotNil(t, te)
	assert.Equal(t, CodeConfirmInvalid, te.Code)
	got, ok := g.broker.get(p.ID)
	require.True(t, ok)
	assert.Equal(t, StatePending, got.State, "B's answer left the call pending")

	// B does not see A's open call as its own.
	_, _, seen := g.broker.activeFor(dedupeKey(a.sid, a.principal.BindingKey(), rt.tool.Name, "h"))
	assert.True(t, seen)
	_, _, seen = g.broker.activeFor(dedupeKey(b.sid, b.principal.BindingKey(), rt.tool.Name, "h"))
	assert.False(t, seen)

	// The confirm token path: A's token is not B's.
	px, err := g.open(a, false, MechanismExternal)
	require.NoError(t, err)
	tok, err := g.mintFor(useConfirmToken, a, px)
	require.NoError(t, err)
	te = g.acceptToken(b, tok)
	require.NotNil(t, te)
	assert.Equal(t, CodeConfirmInvalid, te.Code)
	assert.Contains(t, te.Message, "another principal")
	for _, s := range []string{bindSubA, bindSubB} {
		assert.NotContains(t, tok, s)
	}
	te = g.acceptToken(a, tok)
	require.NotNil(t, te)
	assert.Equal(t, CodeConfirmPending, te.Code, "A's own token is still honoured")

	// A answers its own request state, as before.
	assert.Nil(t, g.elicitAnswer(a, state, answer))
}
