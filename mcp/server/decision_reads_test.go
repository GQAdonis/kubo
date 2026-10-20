//go:build !noagent

package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type decisionReadEnv struct {
	s     *Server
	h     *HTTPServer
	clock *fakeClock
}

func newDecisionReadEnv(t *testing.T, cfg DecisionsConfig, verifier AuthVerifier, tools ...string) *decisionReadEnv {
	t.Helper()
	clock := newFakeClock()
	if len(tools) == 0 {
		tools = []string{"node"}
	}
	s := mustNew(t, catFake(t, nil), Options{Tools: tools, now: clock.Now})
	require.NoError(t, s.ConfigureDecisionReads(cfg, []string{sessionOrigin}))
	h, err := NewHTTPServer(s, HTTPOptions{Listen: "127.0.0.1:0", Verifier: verifier})
	require.NoError(t, err)
	return &decisionReadEnv{s: s, h: h, clock: clock}
}

func (e *decisionReadEnv) request(t *testing.T, method, path, token, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body)).WithContext(t.Context())
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range headers {
		r.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	e.h.Handler().ServeHTTP(w, r)
	return w
}

func validateDecisionRead(t *testing.T, def string, data []byte) {
	t.Helper()
	defs := readSchemaDefs(t, "decisions/decision-read.schema.json")
	confirmation := readSchemaDefs(t, "confirmation/confirmation.schema.json")
	for key, value := range defs {
		defs[key] = rewriteRefs(value, func(ref string) string {
			if rest, ok := strings.CutPrefix(ref, "../confirmation/confirmation.schema.json#/$defs/"); ok {
				return "#/$defs/confirmation." + rest
			}
			return ref
		})
	}
	for key, value := range confirmation {
		defs["confirmation."+key] = rewriteRefs(value, func(ref string) string {
			if rest, ok := strings.CutPrefix(ref, "#/$defs/"); ok {
				return "#/$defs/confirmation." + rest
			}
			return ref
		})
	}
	var doc any
	require.NoError(t, json.Unmarshal(data, &doc))
	validateJSON(t, asJSON(t, map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$defs": defs, "$ref": "#/$defs/" + def}), doc)
}

func readDecisionList(t *testing.T, w *httptest.ResponseRecorder) decisionListResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	validateDecisionRead(t, "ListResponse", w.Body.Bytes())
	var result decisionListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	return result
}

func TestDecisionReadsNoLLMDefaultConfigAndOffSwitch(t *testing.T) {
	for _, fixture := range []struct {
		input   string
		enabled bool
	}{
		{`{}`, true}, {`{"agent":{"decisions":{"enabled":true}}}`, true}, {`{"agent":{"decisions":{"enabled":false}}}`, false},
	} {
		cfg, err := ParseConfig([]byte(fixture.input))
		require.NoError(t, err)
		assert.Equal(t, fixture.enabled, cfg.Agent.DecisionsEnabled())
		e := newDecisionReadEnv(t, cfg.Agent.Decisions, testVerifier(t))
		assert.Nil(t, e.s.Agent(), "no model backend required")
		w := e.request(t, "GET", RouteDecisions, tokChat, "", nil)
		if fixture.enabled {
			snapshot := readDecisionList(t, w)
			assert.Empty(t, snapshot.Items)
			assert.Equal(t, decisionHistoryLimit, snapshot.Retention.MaxResolved)
		} else {
			assert.Equal(t, http.StatusNotFound, w.Code)
			assert.False(t, e.s.gate.broker.historyEnabled)
			assert.Empty(t, e.s.gate.broker.retained)
		}
	}
}

func TestDecisionReadsShareAuthenticationAndStrictOriginProtection(t *testing.T) {
	e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
	for _, token := range []string{"", "invalid-bearer"} {
		w := e.request(t, "GET", RouteDecisions, token, "", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
	for _, fixture := range []struct{ header, value string }{{"Origin", "https://evil.example"}, {"Origin", "http://127.0.0.1"}, {"Sec-Fetch-Site", "cross-site"}} {
		w := e.request(t, "GET", RouteDecisions, tokChat, "", map[string]string{fixture.header: fixture.value})
		assert.Equal(t, http.StatusForbidden, w.Code)
	}
	preflight := e.request(t, "OPTIONS", RouteDecisions, "", "", map[string]string{"Origin": sessionOrigin, "Access-Control-Request-Method": "GET"})
	assert.Equal(t, http.StatusNoContent, preflight.Code)
	assert.Equal(t, sessionOrigin, preflight.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, OPTIONS", preflight.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "authorization, content-type", preflight.Header().Get("Access-Control-Allow-Headers"))
	assert.Empty(t, preflight.Header().Get("Access-Control-Allow-Credentials"))
	assert.Empty(t, preflight.Body.String())
	anonymous := newDecisionReadEnv(t, DecisionsConfig{}, nil)
	assert.Equal(t, http.StatusForbidden, anonymous.request(t, "GET", RouteDecisions, "", "", nil).Code, "no anonymous-loopback or LocalPrincipal ownership fallback")
}

func TestDecisionReadsAdmitToolPrincipalsWithoutWriteOrEgressAndRefuseAdmin(t *testing.T) {
	for _, kind := range []string{KindUI, KindHumanMCP, KindA2A, KindUAR, KindAdmin} {
		id := NewIdentity(Principal{ID: "fixture", Kind: kind}, nil, nil, nil)
		v, err := NewStaticVerifier([]StaticCredential{{Identity: id, Token: tokChat}})
		require.NoError(t, err)
		e := newDecisionReadEnv(t, DecisionsConfig{}, v)
		w := e.request(t, "GET", RouteDecisions, tokChat, "", nil)
		if kind == KindAdmin {
			assert.Equal(t, http.StatusForbidden, w.Code)
			continue
		}
		readDecisionList(t, w)
	}
}

func TestDecisionReadUnknownForeignAndEvictedIDsAreIndistinguishable(t *testing.T) {
	e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
	p := historyPending(e.s.gate.broker, Principal{ID: "other", Kind: KindHumanMCP})
	unknown := e.request(t, "GET", RouteDecisions+"/cf_missing_00000001", tokChat, "", nil)
	foreign := e.request(t, "GET", RouteDecisions+"/"+p.ID, tokChat, "", nil)
	assert.Equal(t, http.StatusNotFound, unknown.Code)
	assert.Equal(t, unknown.Code, foreign.Code)
	assert.Equal(t, unknown.Body.String(), foreign.Body.String())
	validateDecisionRead(t, "ReadError", unknown.Body.Bytes())
	var evicted string
	for i := range decisionHistoryLimit + 1 {
		d := historyPending(e.s.gate.broker, Principal{ID: "chat", Kind: KindUI})
		if i == 0 {
			evicted = d.ID
		}
		require.NoError(t, e.s.gate.broker.Resolve(d.ID, ResolverAGUIInterrupt, false))
	}
	removed := e.request(t, "GET", RouteDecisions+"/"+evicted, tokChat, "", nil)
	assert.Equal(t, unknown.Code, removed.Code)
	assert.Equal(t, unknown.Body.String(), removed.Body.String())
	assert.Empty(t, readDecisionList(t, e.request(t, "GET", RouteDecisions, tokReader, "", nil)).Items)
}

func TestDecisionReadsDisabledLeavesGateAndCurrentOutcomeEffective(t *testing.T) {
	e := newGateEnv(t)
	off := false
	require.NoError(t, e.s.ConfigureDecisionReads(DecisionsConfig{Enabled: &off}, nil))
	cs := e.h.connect(t, nil, nil)
	conf := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired).Confirmation
	e.assertNothingRan(t)
	_, resolve := e.resolver.last(t)
	require.NoError(t, resolve(true))
	res := e.rm(t, cs, map[string]any{"confirm_token": conf.ConfirmToken})
	require.False(t, res.IsError)
	assert.Equal(t, ExecutionSucceeded, currentOutcomeOf(t, res).Execution.State)
	assert.Empty(t, e.s.gate.broker.retained, "closed observer history is released while current result remains truthful")
	confirmationOf(t, e.rm(t, cs, map[string]any{"confirm_token": conf.ConfirmToken}), CodeConfirmInvalid)
	assert.Equal(t, 1, e.d.count("pin/rm"), "disabling reads never enables a write replay")
}

func TestDecisionReadProjectionOmitsAuthorityHandlesAndRawArguments(t *testing.T) {
	e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
	broker := e.s.gate.broker
	p := historyPending(broker, Principal{ID: "chat", Kind: KindUI})
	broker.mu.Lock()
	broker.byID[p.ID].sid = "private-session-handle"
	broker.byID[p.ID].token = "private-confirm-token"
	broker.byID[p.ID].argsHash = "private-argument-binding"
	broker.byID[p.ID].endpoint = "https://credential:secret@private.example"
	broker.mu.Unlock()
	w := e.request(t, "GET", RouteDecisions+"/"+p.ID, tokChat, "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	validateDecisionRead(t, "ItemResponse", w.Body.Bytes())
	for _, forbidden := range []string{"private-session-handle", "private-confirm-token", "private-argument-binding", "credential", "requestState", "confirm_token", "argsHash", "BindingKey"} {
		assert.NotContains(t, w.Body.String(), forbidden)
	}
	var response struct {
		Item decisionObservation `json:"item"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.False(t, response.Item.Actions.Approve)
	assert.False(t, response.Item.Actions.Deny)
	assert.Nil(t, response.Item.Execution)
	assert.Equal(t, p.Destination, response.Item.Destination)
	assert.Equal(t, p.EffectDigest, response.Item.EffectDigest)
}

func TestDecisionReadsRejectBodiesUnknownQueriesAndMutationMethods(t *testing.T) {
	e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
	for _, suffix := range []string{"?limit=0", "?limit=101", "?limit=1&limit=2", "?limit=1.0", "?limit=", "?cursor=", "?token=private", "?identity=chat", "?foo=x", "/bad.id", "/events?access_token=private"} {
		w := e.request(t, "GET", RouteDecisions+suffix, tokChat, "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, suffix)
		assert.NotContains(t, w.Body.String(), "private")
	}
	assert.Equal(t, http.StatusNotFound, e.request(t, "GET", RouteDecisions+"/safe_but_unknown", tokChat, "", nil).Code)
	w := e.request(t, "GET", RouteDecisions, tokChat, `{}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	for _, method := range []string{"POST", "PATCH", "DELETE", "HEAD"} {
		w := e.request(t, method, RouteDecisions+"/cf_example_00001/resolve", tokChat, "", nil)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Equal(t, "GET, OPTIONS", w.Header().Get("Allow"))
	}
}

func TestDecisionReadCursorBindsOwnerSnapshotLimitAndExpiry(t *testing.T) {
	e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
	for range 3 {
		historyPending(e.s.gate.broker, Principal{ID: "chat", Kind: KindUI})
	}
	page := readDecisionList(t, e.request(t, "GET", RouteDecisions+"?limit=1", tokChat, "", nil))
	require.True(t, page.Page.HasMore)
	raw, err := base64.RawURLEncoding.DecodeString(page.Page.Cursor)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"chat"`, "cursor contains only process-keyed opaque owner tag")
	next := readDecisionList(t, e.request(t, "GET", RouteDecisions+"?limit=1&cursor="+page.Page.Cursor, tokChat, "", nil))
	assert.Equal(t, page.Revision, next.Revision)
	assert.NotEqual(t, string(page.Items[0]), string(next.Items[0]))
	for _, fixture := range []struct{ token, cursor, limit string }{
		{tokOther, page.Page.Cursor, "1"}, {tokChat, page.Page.Cursor + "x", "1"}, {tokChat, page.Page.Cursor, "2"},
	} {
		w := e.request(t, "GET", RouteDecisions+"?limit="+fixture.limit+"&cursor="+fixture.cursor, fixture.token, "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		validateDecisionRead(t, "ReadError", w.Body.Bytes())
	}
	// Other-owner creation does not invalidate this owner's snapshot.
	historyPending(e.s.gate.broker, Principal{ID: "other", Kind: KindHumanMCP})
	readDecisionList(t, e.request(t, "GET", RouteDecisions+"?limit=1&cursor="+page.Page.Cursor, tokChat, "", nil))
	historyPending(e.s.gate.broker, Principal{ID: "chat", Kind: KindUI})
	stale := e.request(t, "GET", RouteDecisions+"?limit=1&cursor="+page.Page.Cursor, tokChat, "", nil)
	assert.Equal(t, http.StatusConflict, stale.Code)
	validateDecisionRead(t, "ReadError", stale.Body.Bytes())
	fresh := readDecisionList(t, e.request(t, "GET", RouteDecisions+"?limit=1", tokChat, "", nil))
	e.clock.Advance(time.Minute)
	assert.Equal(t, http.StatusConflict, e.request(t, "GET", RouteDecisions+"?limit=1&cursor="+fresh.Page.Cursor, tokChat, "", nil).Code)
	restarted := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
	assert.Equal(t, http.StatusBadRequest, restarted.request(t, "GET", RouteDecisions+"?limit=1&cursor="+page.Page.Cursor, tokChat, "", nil).Code, "lost process key cannot authenticate an old cursor as owned/stale")
}

func TestDecisionReadsReturnActualOutcomeWithoutPresentationOrContinuation(t *testing.T) {
	e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
	p := historyPending(e.s.gate.broker, Principal{ID: "chat", Kind: KindUI})
	require.NoError(t, e.s.gate.broker.Resolve(p.ID, ResolverAGUIInterrupt, true))
	claim, err := e.s.gate.broker.claimEffect(p.ID, "", nil)
	require.NoError(t, err)
	require.True(t, claim.start())
	claim.finish(ExecutionSucceeded, nil, "operation_completed", nil)
	w := e.request(t, "GET", RouteDecisions+"/"+p.ID, tokChat, "", nil)
	require.Equal(t, 200, w.Code)
	validateDecisionRead(t, "ItemResponse", w.Body.Bytes())
	assert.Contains(t, w.Body.String(), `"provenance":"node_handler"`)
	assert.Contains(t, w.Body.String(), `"approval_state":"executed"`)
	assert.NotContains(t, w.Body.String(), "presentation")
	assert.NotContains(t, w.Body.String(), "continuation")
	assert.NotContains(t, w.Body.String(), `"approve":true`)
}

func TestDecisionReadPageByteBudgetAndUnrepresentableItemFailSafely(t *testing.T) {
	e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
	for range 20 {
		p := historyPending(e.s.gate.broker, Principal{ID: "chat", Kind: KindUI})
		e.s.gate.broker.mu.Lock()
		e.s.gate.broker.byID[p.ID].Summary.Targets = slices.Repeat([]Target{{Kind: "content_path", Value: strings.Repeat("x", 4096)}}, 16)
		e.s.gate.broker.mu.Unlock()
	}
	w := e.request(t, "GET", RouteDecisions+"?limit=100", tokChat, "", nil)
	page := readDecisionList(t, w)
	assert.LessOrEqual(t, w.Body.Len(), decisionPageBytes)
	assert.True(t, page.Page.HasMore)
	assert.Less(t, len(page.Items), 20)
	p := historyPending(e.s.gate.broker, Principal{ID: "chat", Kind: KindUI})
	e.s.gate.broker.mu.Lock()
	e.s.gate.broker.byID[p.ID].Summary.Targets[0].Value = strings.Repeat("private", 10000)
	e.s.gate.broker.mu.Unlock()
	w = e.request(t, "GET", RouteDecisions+"/"+p.ID, tokChat, "", nil)
	assert.Equal(t, 500, w.Code)
	validateDecisionRead(t, "ReadError", w.Body.Bytes())
	assert.NotContains(t, w.Body.String(), "private")
}
