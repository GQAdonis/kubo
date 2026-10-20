//go:build !noagent

package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecisionOIDCExplicitRouteOnlyAndSamePrincipalSubjectsStayPrivate(t *testing.T) {
	iss := newTestIssuer(t)
	settings := iss.settings()
	settings.Routes = DefaultOIDCRoutes()
	principals := []SessionPrincipal{
		{Identity: NewIdentity(Principal{ID: "shared-ui", Kind: KindUI}, nil, nil, nil), Subjects: []string{"alice", "bob"}},
		{Identity: NewIdentity(Principal{ID: "admin-only", Kind: KindAdmin}, nil, nil, nil), Subjects: []string{"admin"}},
	}
	makeEnv := func() *decisionReadEnv {
		v, err := NewJWTVerifier(JWTOptions{OIDC: settings, Principals: principals, OriginAllowlist: []string{sessionOrigin}})
		require.NoError(t, err)
		return newDecisionReadEnv(t, DecisionsConfig{}, NewChainVerifier(testVerifier(t), v))
	}
	token := func(sub string) string { return iss.sign(testKidRSA, claimsAt(time.Now(), sub)) }
	header := map[string]string{"Origin": sessionOrigin}
	closed := makeEnv()
	assert.Equal(t, http.StatusUnauthorized, closed.request(t, "GET", RouteDecisions, token("alice"), "", header).Code)
	assert.Equal(t, []string{OIDCRouteSession}, DefaultOIDCRoutes())
	settings.Routes = append(settings.Routes, OIDCRouteDecisions)
	e := makeEnv()
	a := Principal{ID: "shared-ui", Kind: KindUI, sub: "alice"}
	p := historyPending(e.s.gate.broker, a)
	second := historyPending(e.s.gate.broker, a)
	e.s.gate.broker.mu.Lock()
	e.s.gate.broker.byID[second.ID].sid = "different-owned-session"
	e.s.gate.broker.mu.Unlock()
	historyPending(e.s.gate.broker, Principal{ID: "shared-ui", Kind: KindUI, sub: "bob"})
	alice := readDecisionList(t, e.request(t, "GET", RouteDecisions, token("alice"), "", header))
	bob := readDecisionList(t, e.request(t, "GET", RouteDecisions, token("bob"), "", header))
	require.Len(t, alice.Items, 2, "reads include the same full identity across sessions")
	require.Len(t, bob.Items, 1)
	assert.NotEqual(t, string(alice.Items[0]), string(bob.Items[0]))
	assert.NotEqual(t, alice.Revision, bob.Revision)
	assert.NotContains(t, string(alice.Items[0]), "alice")
	foreign := e.request(t, "GET", RouteDecisions+"/"+p.ID, token("bob"), "", header)
	assert.Equal(t, http.StatusNotFound, foreign.Code)
	assert.Equal(t, http.StatusForbidden, e.request(t, "GET", RouteDecisions, token("admin"), "", header).Code)
	assert.Equal(t, http.StatusForbidden, e.request(t, "GET", RouteDecisions, token("unmapped"), "", header).Code)
	readDecisionList(t, e.request(t, "GET", RouteDecisions, token("alice"), "", nil)) // existing read-only GET semantics permit nonbrowser clients without Origin
	expired := claimsAt(time.Now(), "alice")
	expired["exp"] = time.Now().Add(-3 * time.Minute).Unix()
	assert.Equal(t, http.StatusUnauthorized, e.request(t, "GET", RouteDecisions, iss.sign(testKidRSA, expired), "", header).Code)
	// The decisions route opt-in cannot grant any existing resolve transport.
	assert.Equal(t, http.StatusUnauthorized, e.request(t, "POST", RouteMCP, token("alice"), initializeBody, header).Code)
}

func TestDecisionOIDCSelectsToolIdentityBeforeAdminCapability(t *testing.T) {
	iss := newTestIssuer(t)
	settings := iss.settings()
	settings.Routes = []string{OIDCRouteDecisions}
	v, err := NewJWTVerifier(JWTOptions{OIDC: settings, Principals: defaultSessionPrincipals(), OriginAllowlist: []string{sessionOrigin}})
	require.NoError(t, err)
	e := newDecisionReadEnv(t, DecisionsConfig{}, NewChainVerifier(testVerifier(t), v))
	p := historyPending(e.s.gate.broker, Principal{ID: "console-ui", Kind: KindUI, sub: sessionSub})
	w := e.request(t, "GET", RouteDecisions+"/"+p.ID, iss.sign(testKidRSA, claimsAt(time.Now(), sessionSub)), "", map[string]string{"Origin": sessionOrigin})
	require.Equal(t, http.StatusOK, w.Code)
	validateDecisionRead(t, "ItemResponse", w.Body.Bytes())
	assert.NotContains(t, w.Body.String(), "console-admin")
}

func TestDecisionEventsRejectActualJWTExpiryWithoutClockSkewGrace(t *testing.T) {
	iss := newTestIssuer(t)
	settings := iss.settings()
	settings.Routes = []string{OIDCRouteDecisions}
	v, err := NewJWTVerifier(JWTOptions{OIDC: settings, Principals: []SessionPrincipal{
		{Identity: NewIdentity(Principal{ID: "shared-ui", Kind: KindUI}, nil, nil, nil), Subjects: []string{"alice"}},
	}, OriginAllowlist: []string{sessionOrigin}})
	require.NoError(t, err)
	e := newDecisionReadEnv(t, DecisionsConfig{}, NewChainVerifier(testVerifier(t), v))
	claims := claimsAt(time.Now(), "alice")
	claims["exp"] = time.Now().Add(-time.Second).Unix()
	token := iss.sign(testKidRSA, claims)
	// Established JWT skew may admit a read, but never extends an idle feed.
	readDecisionList(t, e.request(t, "GET", RouteDecisions, token, "", nil))
	response := e.request(t, "GET", RouteDecisions+"/events", token, "", nil)
	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.Zero(t, decisionFixtureStreams(e.s.gate.broker))
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	settings.Routes = DefaultOIDCRoutes()
	closed, err := NewJWTVerifier(JWTOptions{OIDC: settings, Principals: []SessionPrincipal{
		{Identity: NewIdentity(Principal{ID: "shared-ui", Kind: KindUI}, nil, nil, nil), Subjects: []string{"alice"}},
	}, OriginAllowlist: []string{sessionOrigin}})
	require.NoError(t, err)
	absent := newDecisionReadEnv(t, DecisionsConfig{}, closed)
	fresh := iss.sign(testKidRSA, claimsAt(time.Now(), "alice"))
	assert.Equal(t, http.StatusUnauthorized, absent.request(t, "GET", RouteDecisions+"/events", fresh, "", nil).Code)
}
