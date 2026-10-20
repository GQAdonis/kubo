//go:build !noagent

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decisionObservationRequest(t *testing.T, handler http.Handler, token, id string) (decisionObservation, *httptest.ResponseRecorder) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+RouteDecisions+"/"+id, nil).WithContext(t.Context())
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	var observation decisionObservation
	if w.Code == http.StatusOK {
		validateDecisionRead(t, "ItemResponse", w.Body.Bytes())
		var response decisionItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.NoError(t, json.Unmarshal(response.Item, &observation))
	}
	return observation, w
}

// A retained owned authority fixture permits precise private-state negative
// controls. The real HTTP/model/handler lifecycle is covered separately below.
func continuationAuthority(t *testing.T) (*decisionReadEnv, Identity, Pending, *agentSession) {
	t.Helper()
	id := NewIdentity(Principal{ID: "chat", Kind: KindUI}, []string{AllGroups}, []string{"pin"}, nil)
	ro := NewIdentity(id.Principal, []string{AllGroups}, nil, nil)
	peer := NewIdentity(Principal{ID: "chat", Kind: KindA2A}, []string{AllGroups}, nil, nil)
	uar := NewIdentity(Principal{ID: "chat", Kind: KindUAR}, []string{AllGroups}, nil, nil)
	v := decisionIdentityVerifier{AuthVerifier: testVerifier(t), identities: map[string]Identity{tokChat: id, tokReader: ro, tokPeer: peer, tokOther: uar}}
	e := newDecisionReadEnv(t, DecisionsConfig{}, v, "node", "pin")
	require.NoError(t, e.s.EnableAGUI(AGUISettings{}))
	require.NoError(t, mountAGUI(e.s, e.h))
	args := `{"path":"/ipfs/` + testCID(t, 1) + `"}`
	hash, _, decoded, err := argsDigest(json.RawMessage(args))
	require.NoError(t, err)
	digest, err := effectDigest("pin_add", decoded)
	require.NoError(t, err)
	p := e.s.gate.broker.create(newConfirmation{principal: id.Principal, sid: agentSessionPrefix + "owned-thread", tool: "pin_add", argsHash: hash,
		mechanism: MechanismExternal, effectDigest: digest, destination: nodeDestination("pin_add"), summary: Summary{Tool: "pin_add", Class: "write", Text: "Pin content", Targets: []Target{}}})
	session := &agentSession{id: "owned-thread", principal: id.ID, owner: id.BindingKey(), touched: e.clock.Now(),
		snap: &agentSnapshot{backendHost: "localhost"}, interrupt: &Interrupt{ID: "int-" + p.ID, ConfirmationID: p.ID, ToolCallID: "call-one", Tool: "pin_add", Class: "write",
			EffectDigest: p.EffectDigest, Destination: cloneDestination(p.Destination), ExpiresAt: p.ExpiresAt, Resolvers: []string{ResolverAGUIInterrupt}},
		queue: []queuedCall{{call: ChatToolCall{ID: "call-one", Function: ChatFunctionCall{Name: "pin_add", Arguments: args}}}}}
	a := &Agent{srv: e.s, now: e.clock.Now, sessions: map[string]*agentSession{session.id: session}}
	e.s.agentMu.Lock()
	e.s.agentExt = a
	e.s.agentMu.Unlock()
	return e, id, p, session
}

func TestDecisionContinuationUsesOwnedAuthorityAndActualGrants(t *testing.T) {
	e, _, p, _ := continuationAuthority(t)
	observation, w := decisionObservationRequest(t, e.h.Handler(), tokChat, p.ID)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.True(t, observation.Actions.Approve)
	assert.True(t, observation.Actions.Deny)
	assert.Equal(t, "owned_chat", observation.Actions.Surface)
	require.NotNil(t, observation.Actions.Continuation)
	assert.Equal(t, decisionContinuation{Method: "POST", Route: "/agui", ThreadID: "owned-thread", InterruptID: "int-" + p.ID}, *observation.Actions.Continuation)
	ro, _ := decisionObservationRequest(t, e.h.Handler(), tokReader, p.ID)
	assert.False(t, ro.Actions.Approve, "a narrower credential does not borrow another credential's write grant")
	assert.True(t, ro.Actions.Deny, "owned UI denial is supported by the existing resolver")
	peer, _ := decisionObservationRequest(t, e.h.Handler(), tokPeer, p.ID)
	assert.False(t, peer.Actions.Approve)
	assert.False(t, peer.Actions.Deny)
	assert.Nil(t, peer.Actions.Continuation, "read-only A2A observation never acquires the chat resolver")
	uar, _ := decisionObservationRequest(t, e.h.Handler(), tokOther, p.ID)
	assert.False(t, uar.Actions.Approve)
	assert.False(t, uar.Actions.Deny)
	assert.Nil(t, uar.Actions.Continuation)
	assert.Empty(t, e.s.gate.broker.retained, "read projection cannot resolve or claim")
}

func TestDecisionContinuationRefusesStalePrivateBindings(t *testing.T) {
	for _, name := range []string{"busy", "expired-session", "revoked-backend", "missing-snapshot", "interrupt", "tool", "args", "digest", "destination", "broker-owner", "broker-session", "broker-digest", "resolver", "expiry"} {
		t.Run(name, func(t *testing.T) {
			e, id, p, session := continuationAuthority(t)
			a := e.s.Agent()
			a.mu.Lock()
			switch name {
			case "busy":
				session.busy = true
			case "expired-session":
				session.touched = e.clock.Now().Add(-sessionIdleTTL - time.Second)
			case "revoked-backend":
				session.backendChanged = true
			case "missing-snapshot":
				session.snap = nil
			case "interrupt":
				session.interrupt.ConfirmationID = "different"
			case "tool":
				session.queue[0].call.Function.Name = "pin_rm"
			case "args":
				session.queue[0].call.Function.Arguments = `{"cid":"different"}`
			case "digest":
				session.interrupt.EffectDigest = "sha256:" + strings.Repeat("0", 64)
			case "destination":
				session.interrupt.Destination = &Destination{Kind: "node", Name: "different"}
			case "resolver":
				session.interrupt.Resolvers = nil
			}
			a.mu.Unlock()
			b := e.s.gate.broker
			b.mu.Lock()
			switch name {
			case "broker-owner":
				b.byID[p.ID].Principal.sub = "another-subject"
			case "broker-session":
				b.byID[p.ID].sid = agentSessionPrefix + "another-thread"
			case "broker-digest":
				b.byID[p.ID].EffectDigest = "sha256:" + strings.Repeat("0", 64)
			}
			b.mu.Unlock()
			if name == "expiry" {
				e.clock.Advance(confirmTTL)
			}
			actions := e.s.decisionActions(p, id, true)
			assert.False(t, actions.Approve)
			assert.False(t, actions.Deny)
			assert.Nil(t, actions.Continuation)
		})
	}
}

func TestDecisionNativeContinuationCannotApproveOrDenyPriorApproval(t *testing.T) {
	e, id, p, session := continuationAuthority(t)
	e.s.gate.addResolver(newFakeResolver(ResolverLinuxPolkit))
	a := e.s.Agent()
	a.mu.Lock()
	session.interrupt.RequiresNativePrompt = true
	session.interrupt.Resolvers = []string{ResolverLinuxPolkit}
	a.mu.Unlock()
	b := e.s.gate.broker
	b.mu.Lock()
	b.byID[p.ID].NativePrompt = true
	b.mu.Unlock()
	p.NativePrompt = true
	actions := e.s.decisionActions(p, id, true)
	assert.False(t, actions.Approve)
	assert.True(t, actions.Deny)
	assert.Equal(t, "native_system_prompt", actions.Surface)
	require.NotNil(t, actions.Continuation, "the existing native wait/deny path is available")
	require.NoError(t, b.Resolve(p.ID, ResolverLinuxPolkit, true))
	p.State = StateApproved
	actions = e.s.decisionActions(p, id, true)
	assert.False(t, actions.Approve)
	assert.False(t, actions.Deny, "a prior real approval cannot be represented as still deniable")
	require.NotNil(t, actions.Continuation, "native wait may observe and continue an already approved interruption")
	claim, err := b.claimEffect(p.ID, "", nil)
	require.NoError(t, err)
	require.True(t, claim.start())
	assert.Nil(t, e.s.decisionActions(p, id, true).Continuation, "a stale approved snapshot cannot expose a consumed claim")
}

func TestDecisionContinuationReadRaceNeverBecomesApproval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, id, p, session := continuationAuthority(t)
		var group sync.WaitGroup
		group.Go(func() {
			for range 100 {
				e.s.decisionActions(p, id, true)
			}
		})
		group.Go(func() {
			for range 100 {
				a := e.s.Agent()
				a.mu.Lock()
				session.busy = !session.busy
				a.mu.Unlock()
			}
		})
		group.Go(func() { _ = e.s.gate.broker.Resolve(p.ID, ResolverAGUIInterrupt, false) })
		group.Wait()
		actions := e.s.decisionActions(p, id, true)
		assert.False(t, actions.Approve)
		assert.False(t, actions.Deny)
		assert.Nil(t, actions.Continuation)
		assert.Equal(t, StateDenied, e.s.gate.broker.byID[p.ID].State)
	})
}

func TestDecisionContinuationRealResumeAndBoundOutcomeLookup(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "call-one", 7), sayStep("Pinned."))
	in := e.startWrite(aguiTokChat, "inbox-thread")
	meta := in["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)
	id := meta["confirmation_id"].(string)
	observation, w := decisionObservationRequest(t, e.hs.Handler(), aguiTokChat, id)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.True(t, observation.Actions.Approve)
	locator := observation.Actions.Continuation
	require.NotNil(t, locator)
	entry := resumeEntry(locator.InterruptID, ResumeResolved, map[string]any{"approved": true})
	entry["reviewed_effect"] = map[string]any{"effect_digest": "sha256:" + strings.Repeat("0", 64)}
	body := aguiBody(locator.ThreadID, "inbox-resume", nil, entry)
	assert.Equal(t, 409, e.post(aguiTokChat, body, nil).Status)
	assert.Zero(t, e.d.count("pin/add"))
	entry["reviewed_effect"] = map[string]any{"effect_digest": observation.EffectDigest}
	require.Equal(t, 200, e.post(aguiTokChat, body, nil).Status)
	assert.Equal(t, 1, e.d.count("pin/add"))
	settled, response := decisionObservationRequest(t, e.hs.Handler(), aguiTokChat, id)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.NotNil(t, settled.Execution)
	require.NotNil(t, settled.Execution.Receipt)
	assert.Equal(t, id, settled.Execution.Receipt.DecisionID)
	assert.Equal(t, observation.EffectDigest, settled.Execution.Receipt.EffectDigest)
	assert.False(t, settled.Actions.Approve)
	assert.Nil(t, settled.Actions.Continuation)
	assert.Equal(t, 422, e.post(aguiTokChat, body, nil).Status)
	assert.Equal(t, 1, e.d.count("pin/add"))
}

// A changed route mapping must not be trusted just because /decisions
// authenticated successfully. This verifier changes only the answering route.
type decisionRoutedVerifier struct{ AuthVerifier }

func (v decisionRoutedVerifier) Verify(ctx context.Context, token string, r *http.Request) (Identity, time.Time, error) {
	id, end, err := v.AuthVerifier.Verify(ctx, token, r)
	if r.URL.Path == RouteAGUI {
		id.Principal.sub = "different-routed-subject"
	}
	return id, end, err
}

func TestDecisionContinuationProbeRequiresMountedRouteAndSameRoutedOwner(t *testing.T) {
	e, owner, _, _ := continuationAuthority(t)
	h := &decisionReadsHandler{srv: e.s, listener: e.h}
	r := httptest.NewRequest("GET", "http://127.0.0.1"+RouteDecisions, nil).WithContext(t.Context())
	r.Header.Set("Authorization", "Bearer "+tokChat)
	_, allowed := h.resumeIdentity(r, owner)
	require.True(t, allowed)
	originalMethod, originalPath, originalBody := r.Method, r.URL.Path, r.Body
	routed := newDecisionReadEnv(t, DecisionsConfig{}, decisionRoutedVerifier{AuthVerifier: testVerifier(t)})
	require.NoError(t, routed.s.EnableAGUI(AGUISettings{}))
	require.NoError(t, mountAGUI(routed.s, routed.h))
	_, allowed = (&decisionReadsHandler{srv: routed.s, listener: routed.h}).resumeIdentity(r, owner)
	assert.False(t, allowed)
	assert.Equal(t, originalMethod, r.Method)
	assert.Equal(t, originalPath, r.URL.Path)
	assert.Equal(t, originalBody, r.Body)
	e.h.mu.Lock()
	delete(e.h.routes, RouteAGUI)
	e.h.mu.Unlock()
	_, allowed = h.resumeIdentity(r, owner)
	assert.False(t, allowed, "unmounted routes cannot issue locators")
}

func TestDecisionContinuationOIDCRequiresAnsweringRouteAndPOSTOrigin(t *testing.T) {
	iss := newTestIssuer(t)
	owner := Principal{ID: "shared-ui", Kind: KindUI, sub: "alice"}
	for _, name := range []string{"decisions-only", "missing-origin", "allowed", "expired-skew", "private-route-origin", "unmounted"} {
		t.Run(name, func(t *testing.T) {
			settings := iss.settings()
			settings.Routes = []string{OIDCRouteDecisions}
			if name != "decisions-only" {
				settings.Routes = append(settings.Routes, OIDCRouteAGUI)
			}
			v, err := NewJWTVerifier(JWTOptions{OIDC: settings, Principals: []SessionPrincipal{{
				Identity: NewIdentity(Principal{ID: owner.ID, Kind: KindUI}, []string{AllGroups}, []string{"pin"}, nil), Subjects: []string{"alice"},
			}}, OriginAllowlist: []string{sessionOrigin}})
			require.NoError(t, err)
			e := newDecisionReadEnv(t, DecisionsConfig{}, v)
			if name != "unmounted" {
				origins := []string{sessionOrigin}
				if name == "private-route-origin" {
					origins = []string{"https://private.example"}
				}
				require.NoError(t, e.s.EnableAGUI(AGUISettings{OriginAllowlist: origins}))
				require.NoError(t, mountAGUI(e.s, e.h))
			}
			claims := claimsAt(time.Now(), "alice")
			if name == "expired-skew" {
				claims["exp"] = time.Now().Add(-time.Second).Unix()
			}
			r := httptest.NewRequest("GET", "http://127.0.0.1"+RouteDecisions, nil).WithContext(t.Context())
			r.Header.Set("Authorization", "Bearer "+iss.sign(testKidRSA, claims))
			if name != "missing-origin" {
				r.Header.Set("Origin", sessionOrigin)
			}
			_, allowed := (&decisionReadsHandler{srv: e.s, listener: e.h}).resumeIdentity(r, Identity{Principal: owner})
			assert.Equal(t, name == "allowed", allowed)
			assert.Empty(t, e.s.gate.broker.byID, "probing cannot open a confirmation")
		})
	}
}

func TestDecisionUARContinuationKeepsOriginalInterruptionAndNoNodeReceipt(t *testing.T) {
	e, owner, _, _ := continuationAuthority(t)
	b := uarHistoryBackend(t, e.clock)
	b.srv = e.s
	th := &uarThread{principal: owner.ID, owner: owner.BindingKey(), client: "backend-thread", uarID: "backend-internal-thread", lastRun: "backend-segment", touched: e.clock.Now(), tools: map[string]string{"tc": "pin_add"}}
	pending, err := b.parseInterrupts(th, []any{map[string]any{"id": "original-backend-interrupt", "reason": uarReasonToolApproval, "toolCallId": "tc"}})
	require.NoError(t, err)
	th.pending, th.shown = pending, pending[0].ShownID
	b.threads = map[string]*uarThread{uarThreadKey(th.owner, th.client): th}
	e.s.agentMu.Lock()
	e.s.uarExt = b
	e.s.agentMu.Unlock()
	decisionID := pending[0].decisionID
	observation, w := decisionObservationRequest(t, e.h.Handler(), tokChat, decisionID)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Equal(t, SourceKindBackendApproval, observation.SourceKind)
	require.NotNil(t, observation.Actions.Continuation)
	assert.Equal(t, th.client, observation.Actions.Continuation.ThreadID)
	assert.Equal(t, th.shown, observation.Actions.Continuation.InterruptID)
	assert.True(t, observation.Actions.Approve)
	assert.True(t, observation.Actions.Deny)
	assert.Empty(t, observation.EffectDigest)
	assert.Nil(t, observation.Execution)
	assert.NotEqual(t, decisionID, observation.Actions.Continuation.InterruptID)
	b.mu.Lock()
	pending[0].Answer = pending[0].answerFor(true, false)
	pending[0].Answer.ResolvedBy = ResolverAGUIInterrupt
	body := b.resumeBody(th, "resume-run")
	b.settle(th, relayResult{responded: true})
	b.mu.Unlock()
	entry := body["resume"].([]any)[0].(map[string]any)
	assert.Equal(t, "original-backend-interrupt", entry["interruptId"])
	assert.NotContains(t, entry, "reviewed_effect")
	settled, response := decisionObservationRequest(t, e.h.Handler(), tokChat, decisionID)
	require.Equal(t, 200, response.Code, response.Body.String())
	assert.Equal(t, StateExecuted, settled.State, "only backend approval consumption was acknowledged")
	assert.Nil(t, settled.Execution, "no execution outcome or success receipt can be inferred")
	assert.False(t, settled.Actions.Approve)
	assert.Nil(t, settled.Actions.Continuation)
}

func TestDecisionOutcomeLookupRejectsCrossEffectReceipts(t *testing.T) {
	e, _, p, _ := continuationAuthority(t)
	b := e.s.gate.broker
	require.NoError(t, b.Resolve(p.ID, ResolverAGUIInterrupt, true))
	claim, err := b.claimEffect(p.ID, "", nil)
	require.NoError(t, err)
	require.True(t, claim.start())
	claim.finish(ExecutionSucceeded, nil, "operation_completed", nil)
	observation, w := decisionObservationRequest(t, e.h.Handler(), tokChat, p.ID)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NotNil(t, observation.Execution.Receipt)
	b.mu.Lock()
	b.retained[p.ID].execution.Receipt.DecisionID = "cf_another_effect_00000001"
	b.mu.Unlock()
	_, invalid := decisionObservationRequest(t, e.h.Handler(), tokChat, p.ID)
	assert.Equal(t, 500, invalid.Code)
	assert.NotContains(t, invalid.Body.String(), "cf_another_effect_00000001")
	assert.NotContains(t, invalid.Body.String(), "receipt")
}

func TestDecisionOutcomeLookupKeepsFailureUnknownAndUnreportedDistinct(t *testing.T) {
	for _, state := range []string{ExecutionFailed, ExecutionUnknown, "unreported"} {
		t.Run(state, func(t *testing.T) {
			e, _, p, _ := continuationAuthority(t)
			b := e.s.gate.broker
			require.NoError(t, b.Resolve(p.ID, ResolverAGUIInterrupt, true))
			claim, err := b.claimEffect(p.ID, "", nil)
			require.NoError(t, err)
			if state != "unreported" {
				require.True(t, claim.start())
				claim.finish(state, &SafeExecutionError{Code: CodeRPCError, Message: "Outcome could not be completed."}, "", nil)
			}
			observation, w := decisionObservationRequest(t, e.h.Handler(), tokChat, p.ID)
			require.Equal(t, 200, w.Code, w.Body.String())
			require.NotNil(t, observation.Execution)
			if state == "unreported" {
				assert.Equal(t, ExecutionNotStarted, observation.Execution.State)
			} else {
				assert.Equal(t, state, observation.Execution.State)
			}
			assert.Nil(t, observation.Execution.Receipt)
			assert.False(t, observation.Actions.Approve)
			assert.False(t, observation.Actions.Deny)
			assert.Nil(t, observation.Actions.Continuation)
		})
	}
}

func TestDecisionContinuationStaleLocatorCannotOverrideOtherDenial(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "call-one", 9), sayStep("Denied."))
	in := e.startWrite(aguiTokChat, "stale-inbox-thread")
	id := in["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)["confirmation_id"].(string)
	observation, w := decisionObservationRequest(t, e.hs.Handler(), aguiTokChat, id)
	require.Equal(t, 200, w.Code)
	locator := observation.Actions.Continuation
	require.NotNil(t, locator)
	require.NoError(t, e.s.ResolveConfirmation(id, ResolverLinuxPolkit, false))
	entry := resumeEntry(locator.InterruptID, ResumeResolved, map[string]any{"approved": true})
	entry["reviewed_effect"] = map[string]any{"effect_digest": observation.EffectDigest}
	response := e.post(aguiTokChat, aguiBody(locator.ThreadID, "late-answer", nil, entry), nil)
	require.Equal(t, 200, response.Status, response.Body)
	assert.Zero(t, e.d.count("pin/add"))
	settled, _ := decisionObservationRequest(t, e.hs.Handler(), aguiTokChat, id)
	assert.Equal(t, StateDenied, settled.State)
	assert.Nil(t, settled.Execution)
	assert.Nil(t, settled.Actions.Continuation)
}

func TestDecisionHostSurfacesDoNotInventChatCapability(t *testing.T) {
	for _, fixture := range []struct {
		name, mechanism, resolver, surface string
		native                             bool
	}{
		{"mcp", MechanismElicitation, "", "originating_mcp_host", false},
		{"native", MechanismExternal, ResolverLinuxPolkit, "native_system_prompt", true},
		{"desktop", MechanismExternal, ResolverDesktop, "paired_desktop", false},
		{"cli", MechanismExternal, ResolverCLI, "headless_cli", false},
		{"unknown", MechanismExternal, "", "", false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
			if fixture.resolver != "" {
				e.s.gate.addResolver(newFakeResolver(fixture.resolver))
			}
			p := historyPending(e.s.gate.broker, Principal{ID: "chat", Kind: KindUI})
			p.Mechanism, p.NativePrompt = fixture.mechanism, fixture.native
			actions := e.s.decisionActions(p, Identity{}, false)
			assert.Equal(t, fixture.surface, actions.Surface)
			assert.False(t, actions.Approve)
			assert.False(t, actions.Deny)
			assert.Nil(t, actions.Continuation)
		})
	}
}

func TestDecisionContinuationLegacyOptionalMetadataStillUsesImmutableArguments(t *testing.T) {
	e, id, p, session := continuationAuthority(t)
	a := e.s.Agent()
	a.mu.Lock()
	session.interrupt.EffectDigest = ""
	session.interrupt.Destination = nil
	a.mu.Unlock()
	b := e.s.gate.broker
	b.mu.Lock()
	b.byID[p.ID].EffectDigest = ""
	b.byID[p.ID].Destination = nil
	b.mu.Unlock()
	p.EffectDigest, p.Destination = "", nil
	actions := e.s.decisionActions(p, id, true)
	assert.True(t, actions.Approve)
	require.NotNil(t, actions.Continuation)
	a.mu.Lock()
	session.queue[0].call.Function.Arguments = `{"path":"/ipfs/different"}`
	a.mu.Unlock()
	actions = e.s.decisionActions(p, id, true)
	assert.False(t, actions.Approve)
	assert.Nil(t, actions.Continuation, "omitting additive metadata cannot bypass immutable arguments")
}

func TestDecisionOIDCReadGrantDoesNotAcquireOwnedChatContinuation(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		name := "decisions-only"
		if allowed {
			name = "decisions-and-agui"
		}
		t.Run(name, func(t *testing.T) {
			iss := newTestIssuer(t)
			identity := aguiIdentity("shared-ui", KindUI, []string{"pin"}, nil, []string{"local"})
			e := newAGUIEnv(t, aguiEnvOpts{verifier: func(t *testing.T) AuthVerifier {
				settings := iss.settings()
				settings.RequireOrigin = false
				settings.Routes = []string{OIDCRouteDecisions}
				if allowed {
					settings.Routes = append(settings.Routes, OIDCRouteAGUI)
				}
				v, err := NewJWTVerifier(JWTOptions{OIDC: settings, Principals: []SessionPrincipal{{Identity: identity, Subjects: []string{"alice"}}}})
				require.NoError(t, err)
				return v
			}}, pinAddScript(t, "call-one", 11))
			owned := identity
			owned.Principal.sub = "alice"
			run, err := e.s.Agent().Run(t.Context(), owned, RunInput{SessionID: "jwt-owned-thread", Message: "pin it"}, nil)
			require.NoError(t, err)
			require.Len(t, run.Interrupts, 1)
			token := iss.sign(testKidRSA, claimsAt(time.Now(), "alice"))
			observation, w := decisionObservationRequest(t, e.hs.Handler(), token, run.Interrupts[0].ConfirmationID)
			require.Equal(t, 200, w.Code, w.Body.String())
			assert.Equal(t, allowed, observation.Actions.Approve)
			assert.Equal(t, allowed, observation.Actions.Deny)
			assert.Equal(t, allowed, observation.Actions.Continuation != nil)
			assert.Zero(t, e.d.count("pin/add"))
		})
	}
}
