//go:build !noagent

package server

import (
	"encoding/json"
	"net/http"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// decisionAuthProbe discards the existing protection chain's response. It
// never invokes a run handler, decodes a body or exposes probe failures.
type decisionAuthProbe struct{ header http.Header }

func (p *decisionAuthProbe) Header() http.Header          { return p.header }
func (*decisionAuthProbe) WriteHeader(int)                {}
func (*decisionAuthProbe) Write(data []byte) (int, error) { return len(data), nil }

// resumeIdentity checks the very same credential on the actual answering
// route. A decisions-only JWT, a different routed principal or a GET accepted
// without Origin cannot thereby acquire POST /agui authority.
func (h *decisionReadsHandler) resumeIdentity(r *http.Request, owner Identity) (Identity, bool) {
	if h.listener == nil || owner.Kind != KindUI || r.Context().Err() != nil {
		return Identity{}, false
	}
	h.listener.mu.Lock()
	_, mounted := h.listener.routes[RouteAGUI]
	h.listener.mu.Unlock()
	cfg, _ := h.srv.aguiExt.(*aguiConfig) // immutable before listener construction
	if !mounted || cfg == nil {
		return Identity{}, false
	}
	var result Identity
	allowed := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		info := auth.TokenInfoFromContext(req.Context())
		if info == nil || !time.Now().Before(info.Expiration) {
			return
		}
		id, ok := info.Extra[identityExtraKey].(Identity)
		authenticated, _ := info.Extra[authenticatedExtraKey].(bool)
		if !ok || !authenticated || id.Kind != KindUI || id.BindingKey() != owner.BindingKey() ||
			id.Session != nil && !time.Now().Before(id.Session.Expiry) {
			return
		}
		result, allowed = id, true
	})
	probe := r.Clone(r.Context())
	probe.Method = http.MethodPost
	probe.URL.Path, probe.URL.RawPath, probe.URL.RawQuery = RouteAGUI, "", ""
	probe.RequestURI = RouteAGUI
	probe.Body, probe.GetBody, probe.ContentLength = http.NoBody, nil, 0
	h.listener.protect(next, false, defaultKinds(RouteAGUI), cfg.origins, &aguiCORS).
		ServeHTTP(&decisionAuthProbe{header: make(http.Header)}, probe)
	return result, allowed
}

func continuation(thread, interrupt string) *decisionContinuation {
	if utf8.RuneCountInString(thread) < 1 || utf8.RuneCountInString(thread) > 256 ||
		utf8.RuneCountInString(interrupt) < 1 || utf8.RuneCountInString(interrupt) > 256 {
		return nil
	}
	return &decisionContinuation{Method: http.MethodPost, Route: RouteAGUI, ThreadID: thread, InterruptID: interrupt}
}

// decisionActions returns advisory facts from the current resolver authority;
// the existing resume endpoint still rechecks every owner/interrupt/claim.
// All source locks precede broker.mu, and no authentication or callback runs
// while either is held.
func (s *Server) decisionActions(p Pending, id Identity, canResume bool) decisionActions {
	actions := defaultDecisionActions(p)
	if p.State != StatePending && p.State != StateApproved || p.Execution != nil {
		return decisionActions{}
	}
	resolvers := s.gate.list()
	if actions.Surface == "" && p.Mechanism == MechanismExternal {
		for _, r := range resolvers {
			if !resolverFits(r, p.Principal, false) {
				continue
			}
			switch r.Name() {
			case ResolverMacOSLocalAuth, ResolverLinuxPolkit:
				actions.Surface = "native_system_prompt"
			case ResolverDesktop:
				actions.Surface = "paired_desktop"
			case ResolverCLI:
				actions.Surface = "headless_cli"
			}
			if actions.Surface != "" {
				break
			}
		}
	}
	if !canResume || id.Kind != KindUI {
		return actions
	}
	endpoint := ""
	if s.gate.endpoint != nil {
		endpoint = s.gate.endpoint()
	}
	shellRestricted := s.shellRestriction(id) != nil
	s.agentMu.RLock()
	defer s.agentMu.RUnlock()
	if b, _ := s.uarExt.(*uarBackend); b != nil {
		if p.Source == DecisionSourceUAR {
			return b.continuationActions(p, id, actions)
		}
		return actions
	}
	a, _ := s.agentExt.(*Agent)
	if a == nil || p.Source != DecisionSourceKubo || p.Mechanism != MechanismExternal ||
		!slices.ContainsFunc(resolvers, func(r Resolver) bool { return r.Name() == ResolverAGUIInterrupt && resolverFits(r, id.Principal, true) }) {
		return actions
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, session := range a.sessions {
		open := session.interrupt
		if session.owner != id.BindingKey() || session.busy || a.now().Sub(session.touched) > sessionIdleTTL ||
			session.snap == nil || session.backendChanged || open == nil || open.ConfirmationID != p.ID || len(session.queue) == 0 {
			continue
		}
		q := session.queue[0]
		if q.skip != "" || q.call.ID != open.ToolCallID || q.call.Function.Name != open.Tool || open.Tool != p.Summary.Tool {
			return actions
		}
		if !p.NativePrompt && !slices.Contains(open.Resolvers, ResolverAGUIInterrupt) {
			return actions
		}
		// Match the actual agent cleaning path before comparing legacy args and
		// the additive reviewed-effect binding. Raw arguments stay private.
		var args map[string]any
		if json.Unmarshal([]byte(q.call.Function.Arguments), &args) != nil || args == nil {
			return actions
		}
		delete(args, "confirm_token")
		cleaned, err := json.Marshal(args)
		if err != nil {
			return actions
		}
		hash, _, canonicalArgs, err := argsDigest(cleaned)
		if err != nil {
			return actions
		}
		digest, err := effectDigest(open.Tool, canonicalArgs)
		if err != nil {
			return actions
		}
		b := s.gate.broker
		b.mu.Lock()
		r := b.byID[p.ID]
		valid := r != nil && r.Principal.BindingKey() == id.BindingKey() && r.sid == agentSessionPrefix+session.id &&
			(r.State == StatePending || r.State == StateApproved) && b.now().Before(r.ExpiresAt) &&
			r.tool == open.Tool && r.argsHash == hash && r.Mechanism == MechanismExternal &&
			r.State == p.State && r.EffectDigest == p.EffectDigest && r.EffectDigest == open.EffectDigest &&
			(r.EffectDigest == "" || r.EffectDigest == digest) && (r.endpoint == "" || r.endpoint == endpoint) &&
			r.ExpiresAt.Equal(open.ExpiresAt) && r.ExpiresAt.Equal(p.ExpiresAt) &&
			r.NativePrompt == open.RequiresNativePrompt && r.NativePrompt == p.NativePrompt &&
			sameDecisionDestination(r.Destination, open.Destination) && sameDecisionDestination(r.Destination, p.Destination)
		b.mu.Unlock()
		if !valid {
			return actions
		}
		locator := continuation(session.id, open.ID)
		if locator == nil {
			return actions
		}
		actions.Continuation = locator
		// Native wait is represented by the locator and native prompt fact;
		// it never becomes an approval gesture or an invented action key.
		actions.Deny = p.State == StatePending
		if !p.NativePrompt {
			actions.Surface = "owned_chat"
			rt, ok := s.registry[open.Tool]
			actions.Approve = p.State == StatePending && ok && !shellRestricted && id.Allows(rt.tool) &&
				a.egressRefusalFor(&runCtx{snap: session.snap}, id, rt.tool) == ""
		}
		return actions
	}
	return actions
}

func sameDecisionDestination(a, b *Destination) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (b *uarBackend) continuationActions(p Pending, id Identity, actions decisionActions) decisionActions {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, th := range b.threads {
		if th.owner != id.BindingKey() || th.running || th.uarID == "" || th.lastRun == "" || !b.now().Before(th.touched.Add(sessionIdleTTL)) {
			continue
		}
		cur := th.shownPending()
		if cur == nil || cur.ID == "" || !cur.NeedsUser || cur.retained || cur.decisionID != p.ID || cur.Tool != p.Summary.Tool ||
			p.State != StatePending || p.EffectDigest != "" || p.Execution != nil || !cur.createdAt.Equal(p.CreatedAt) || cur.activityID != p.ActivityID {
			continue
		}
		var currentExpiry time.Time
		if expiry, err := time.Parse(time.RFC3339, cur.ExpiresAt); err == nil {
			currentExpiry = expiry
		}
		if !currentExpiry.Equal(p.ExpiresAt) {
			return actions
		}
		if !p.ExpiresAt.IsZero() && !b.now().Before(p.ExpiresAt) {
			return actions
		}
		if locator := continuation(th.client, cur.ShownID); locator != nil {
			actions.Surface, actions.Approve, actions.Deny, actions.Continuation = "owned_chat", true, true, locator
		}
		return actions
	}
	return actions
}

func validDecisionOutcome(p Pending) bool {
	e := p.Execution
	if e == nil {
		return true
	}
	if p.Source != DecisionSourceKubo {
		return false
	}
	if e.Receipt == nil {
		return e.State != ExecutionSucceeded
	}
	r := e.Receipt
	return p.State == StateExecuted && e.State == ExecutionSucceeded && validEffectDigest(p.EffectDigest) &&
		r.DecisionID == p.ID && r.Tool == p.Summary.Tool && r.EffectDigest == p.EffectDigest &&
		r.Outcome == ExecutionSucceeded && r.Provenance == "node_handler" && e.CompletedAt != nil && r.CompletedAt.Equal(*e.CompletedAt)
}
