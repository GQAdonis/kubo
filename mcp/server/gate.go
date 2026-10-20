package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ConfirmationRequired is the confirmation object of a confirmation_required
// or confirmation_pending result (ConfirmationRequired in the confirmation
// schema). It never contains consent; it says what is waiting and who may
// grant it.
type ConfirmationRequired struct {
	ConfirmationID string    `json:"confirmation_id"`
	State          string    `json:"state"`
	Summary        Summary   `json:"summary"`
	ExpiresAt      time.Time `json:"expires_at"`
	Resolvers      []string  `json:"resolvers"`
	// ConfirmToken is a binding handle for the repeat call after a resolver
	// approved. It is never present for principals of kind a2a or uar.
	ConfirmToken string `json:"confirm_token,omitempty"`
	// RequiresNativePrompt is true for destructive calls from the ui and a2a principals:
	// only a system prompt approves, so agui-interrupt is not in Resolvers.
	RequiresNativePrompt bool              `json:"requires_native_prompt,omitempty"`
	PrincipalKind        string            `json:"principal_kind,omitempty"`
	EffectDigest         string            `json:"effect_digest,omitempty"`
	Destination          *Destination      `json:"destination,omitempty"`
	ResolvedBy           string            `json:"resolved_by,omitempty"`
	ResolvedSurface      *ResolvingSurface `json:"resolved_surface,omitempty"`
}

// ResolveFunc records the human's decision for one pending confirmation.
type ResolveFunc func(approve bool) error

// ReviewedResolveFunc is the authorized resolver's callback with an optional
// echo of the operation shown to the person. The callback supplies no new rights.
type ReviewedResolveFunc func(approve bool, reviewed *ReviewedEffect) error

// ReviewedResolver accepts the contract's optional reviewed-effect echo while
// preserving the legacy Resolver interface for existing prompts.
type ReviewedResolver interface {
	Resolver
	RequestReviewed(ctx context.Context, p Pending, resolve ReviewedResolveFunc) error
}

// Resolver is a human-presence mechanism that can approve a pending
// confirmation out of band: the paired IPFS Desktop window, a macOS
// LocalAuthentication prompt, a Linux polkit prompt, an AG-UI interrupt
// answered by a user gesture, or the headless CLI opt-in. Implementations
// belong to later changes; this package defines the contract and the gate.
type Resolver interface {
	// Name is one of the Resolver* constants.
	Name() string
	// Request asks the human to decide about p. It must not block: start the
	// prompt and return. Later, call resolve exactly once with the decision.
	// ctx outlives the tool call and ends when p expires.
	Request(ctx context.Context, p Pending, resolve ResolveFunc) error
}

// gate decides whether a write or destructive call may run. The first
// mechanism that applies wins:
//
//  1. MCP elicitation, when the client advertised it.
//  2. A pending confirmation that an out-of-band Resolver approves.
//  3. Refusal with confirmation_unavailable.
//
// Nothing ever executes silently.
type gate struct {
	log       *slog.Logger
	broker    *Broker
	signer    *tokenSigner
	now       func() time.Time
	resolvers []Resolver
	// summaryArgs, when set, gives a confirmation text the values a call will
	// use and an extra sentence (index.go). The call's argument hash is not
	// affected.
	summaryArgs func(tool string, args map[string]any) (map[string]any, string)
	// endpoint is read at preparation and claim so discovery cannot silently
	// substitute a different daemon after human review.
	endpoint func() string

	mu       sync.Mutex
	localIDs map[*mcp.ServerSession]string
}

// list returns the registered resolvers.
func (g *gate) list() []Resolver {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.resolvers)
}

// addResolver registers a resolver once per name. The AG-UI endpoint adds its
// chat resolver when it is mounted, before the listener serves.
func (g *gate) addResolver(r Resolver) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if slices.ContainsFunc(g.resolvers, func(have Resolver) bool { return have.Name() == r.Name() }) {
		return
	}
	g.resolvers = append(g.resolvers, r)
}

// agentOnlyResolver marks a resolver that answers only for calls the agent loop
// makes, because the person it asks is in a chat that the loop serves.
type agentOnlyResolver interface{ agentOnly() }

// resolverFits reports whether a resolver may be asked about, or listed for, a
// confirmation of a principal of this kind. The chat resolver is for the chat
// principal only: a remote agent or an MCP host is never answered by a chat
// gesture. The real chat resolver is also for the agent loop's own calls only;
// a call on an MCP session has no chat to answer it. (For a call that needs the
// native prompt the chat may still deny; the broker refuses its approval.)
//
// The cli resolver never fits the stdio principal (LocalPrincipal): the CLI
// prompt has no presence factor, and stdio cannot carry the shell_capable: false
// assertion that makes it acceptable, so a shell-capable host could approve its
// own calls. Such a call is confirmation_unavailable unless the client elicits.
func resolverFits(r Resolver, p Principal, agent bool) bool {
	switch r.Name() {
	case ResolverCLI:
		return p != LocalPrincipal
	case ResolverAGUIInterrupt:
		if _, only := r.(agentOnlyResolver); only && !agent {
			return false
		}
		return p.Kind == KindUI
	}
	return true
}

// fromAgent reports whether the call comes from the agent loop, which has no
// MCP session and names its own gate session.
func (c *call) fromAgent() bool {
	return c.ss == nil && strings.HasPrefix(c.sid, agentSessionPrefix)
}

func newGate(log *slog.Logger, resolvers []Resolver, now func() time.Time) (*gate, error) {
	signer, err := newTokenSigner()
	if err != nil {
		return nil, err
	}
	valid := []string{ResolverAGUIInterrupt, ResolverDesktop, ResolverMacOSLocalAuth, ResolverLinuxPolkit, ResolverCLI}
	for _, r := range resolvers {
		if !slices.Contains(valid, r.Name()) {
			return nil, fmt.Errorf("confirmation resolver %q is not one of %v", r.Name(), valid)
		}
	}
	return &gate{
		log:       log,
		broker:    newBroker(now, confirmTTL),
		signer:    signer,
		now:       now,
		resolvers: resolvers,
		localIDs:  make(map[*mcp.ServerSession]string),
	}, nil
}

// sessionID is the MCP session ID, or a per-process random ID for transports
// that have none (stdio, in-memory).
func (g *gate) sessionID(ss *mcp.ServerSession) string {
	if ss == nil {
		return ""
	}
	if id := ss.ID(); id != "" {
		return id
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	id, ok := g.localIDs[ss]
	if !ok {
		id = "local-" + randomID(12)
		g.localIDs[ss] = id
	}
	return id
}

// canElicit reports whether the client advertised form elicitation.
func canElicit(ss *mcp.ServerSession) bool {
	if ss == nil {
		return false
	}
	p := ss.InitializeParams()
	if p == nil || p.Capabilities == nil || p.Capabilities.Elicitation == nil {
		return false
	}
	c := p.Capabilities.Elicitation
	return c.Form != nil || c.URL == nil
}

// mayElicit reports whether a confirmation for this principal may be asked
// through MCP elicitation: only a human-mcp principal (an MCP host a person
// drives, and stdio) has a person on the other end of the elicitation. A ui,
// a2a or uar principal can declare elicitation and answer its own prompt, so
// its calls go to an out-of-band resolver, and a destructive ui call to the
// native prompt.
func mayElicit(p Principal, ss *mcp.ServerSession) bool {
	return p.Kind == KindHumanMCP && canElicit(ss)
}

// agentSessionPrefix marks the gate session IDs of the agent loop.
const agentSessionPrefix = "agent-"

// sessionIDKey is the context key of the session ID an in-process caller names
// for the confirmation gate.
type sessionIDKey struct{}

// call identifies one tool call for the gate.
type call struct {
	ss           *mcp.ServerSession
	principal    Principal
	sid          string
	tool         contract.Tool
	argsHash     string
	args         map[string]any
	sc           *schemas
	effectDigest string
	endpoint     string
	reviewed     *ReviewedEffect
	reviewError  *ToolError
	claim        *executionClaim
	activityID   string
}

// authorize returns a winning execution claim only when this call was
// confirmed and consumed. A non-nil result asks the user through elicitation;
// any ToolError means this attempt has no side effect. Only the claim owner
// reaches the implementation in the shared handler.
func (g *gate) authorize(ctx context.Context, req *mcp.CallToolRequest, t contract.Tool, sc *schemas, raw json.RawMessage) (*mcp.CallToolResult, *executionClaim, *ToolError) {
	hash, token, args, err := argsDigest(raw)
	if err != nil {
		return nil, nil, invalidArgument(err.Error())
	}
	effect, err := effectDigest(t.Name, args)
	if err != nil {
		return nil, nil, invalidArgument(err.Error())
	}
	var ss *mcp.ServerSession
	if req != nil {
		ss = req.Session
	}
	sid := g.sessionID(ss)
	if sid == "" {
		// An in-process caller (the agent loop) has no MCP session; it names
		// its own, so confirmations bind to that conversation.
		sid, _ = ctx.Value(sessionIDKey{}).(string)
	}
	c := &call{
		ss:           ss,
		principal:    PrincipalFromContext(ctx),
		sid:          sid,
		tool:         t,
		argsHash:     hash,
		args:         args,
		sc:           sc,
		effectDigest: effect,
	}
	c.activityID, _ = ctx.Value(activityIDKey{}).(string)
	if g.endpoint != nil {
		c.endpoint = g.endpoint()
	}
	if req != nil && req.Params != nil {
		c.reviewed, c.reviewError = reviewedFromMeta(req.Params.Meta)
	}
	var ask *mcp.CallToolResult
	var gerr *ToolError
	switch {
	case token != "":
		gerr = g.acceptToken(c, token)
	case req != nil && req.Params != nil && req.Params.RequestState != "":
		gerr = g.elicitAnswer(c, req.Params.RequestState, req.Params.InputResponses)
	default:
		if p, tok, ok := g.broker.activeFor(dedupeKey(c.sid, c.principal.BindingKey(), t.Name, hash)); ok {
			gerr = g.repeat(c, p, tok)
		} else if c.reviewError != nil {
			gerr = c.reviewError
		} else if c.reviewed != nil {
			gerr = &ToolError{Code: CodeConfirmInvalid, Message: "No waiting confirmation matches this reviewed operation. Request a fresh review."}
		} else if mayElicit(c.principal, ss) {
			ask, gerr = g.elicitRequest(c)
		} else {
			gerr = g.openExternal(ctx, c)
		}
	}
	return ask, c.claim, gerr
}

// repeat handles a token-less repeat of a call that has an open confirmation.
// Approved runs exactly once; pending, denied and expired are reported. A
// repeat never approves anything.
func (g *gate) repeat(c *call, p Pending, tok string) *ToolError {
	if err := g.checkReview(c, p, nil); err != nil {
		return err
	}
	if errors.Is(g.broker.checkEffect(p.ID, c), ErrConfirmationDigestMismatch) {
		return digestMismatch(c.tool.Name, p.ID)
	}
	switch p.State {
	case StateApproved:
		claim, err := g.broker.claimEffect(p.ID, "", c)
		if err != nil {
			if errors.Is(err, ErrConfirmationDigestMismatch) {
				return digestMismatch(c.tool.Name, p.ID)
			}
			return g.outcomeError(c.tool.Name, p, StateExecuted)
		}
		c.claim = claim
		return nil
	case StatePending:
		return g.requiredError(c, p, tok, CodeConfirmPending)
	default:
		g.broker.forget(p.ID)
		return g.outcomeError(c.tool.Name, p, p.State)
	}
}

// openExternal opens a pending confirmation for an out-of-band resolver. With
// no resolver the call is refused.
func (g *gate) openExternal(ctx context.Context, c *call) *ToolError {
	resolvers := g.list()
	if !slices.ContainsFunc(resolvers, func(r Resolver) bool { return resolverFits(r, c.principal, c.fromAgent()) }) {
		return &ToolError{
			Code: CodeConfirmUnavailable,
			Message: c.tool.Name + " needs confirmation, but this client cannot ask the user (no elicitation) and no " +
				"out-of-band confirmation prompt is available, so it did not run",
		}
	}
	// The chat (ui) and a remote agent (a2a) cannot approve a destructive call
	// themselves: only a presence resolver (the system prompt) can.
	native := (c.principal.Kind == KindUI || c.principal.Kind == KindA2A) && c.tool.Confirmation == contract.ClassDestructive
	if native && !slices.ContainsFunc(resolvers, func(r Resolver) bool { return IsPresenceResolver(r.Name()) }) {
		return &ToolError{
			Code: CodeConfirmUnavailable,
			Message: c.tool.Name + " is destructive and needs a native system prompt, but none is available (a chat answer " +
				"alone cannot approve it), so it did not run",
		}
	}
	p, err := g.open(c, native, MechanismExternal)
	if errors.Is(err, ErrTooManyOpen) {
		return &ToolError{
			Code:      CodeConfirmPending,
			Message:   "Other confirmations are still waiting for a person (" + err.Error() + "). Nothing has changed; answer or let them expire, then ask again.",
			Retryable: true,
			Details:   map[string]any{"reason": "too_many_open_confirmations"},
		}
	}
	if err != nil {
		return &ToolError{Code: CodeInternal, Message: err.Error()}
	}
	tok := ""
	if c.principal.AcceptsToken() {
		var err error
		if tok, err = g.mintFor(useConfirmToken, c, p); err != nil {
			_ = g.broker.Resolve(p.ID, "", false)
			g.broker.forget(p.ID)
			return &ToolError{Code: CodeInternal, Message: err.Error()}
		}
		g.broker.setToken(p.ID, tok)
	}
	if !g.notify(ctx, p, c.fromAgent()) {
		_ = g.broker.Resolve(p.ID, "", false)
		g.broker.forget(p.ID)
		return &ToolError{
			Code:    CodeConfirmUnavailable,
			Message: "no confirmation prompt could be shown, so " + c.tool.Name + " did not run",
		}
	}
	return g.requiredError(c, p, tok, CodeConfirmRequired)
}

// open creates the pending confirmation for a call.
func (g *gate) open(c *call, nativePrompt bool, mechanism string) (Pending, error) {
	return g.broker.reserve(newConfirmation{
		principal:    c.principal,
		sid:          c.sid,
		tool:         c.tool.Name,
		argsHash:     c.argsHash,
		summary:      g.summaryOf(c),
		nativeReq:    nativePrompt,
		mechanism:    mechanism,
		effectDigest: c.effectDigest,
		destination:  nodeDestination(c.tool.Name),
		endpoint:     c.endpoint,
		activityID:   c.activityID,
	}, maxOpenExternal)
}

// mintFor signs the handle of the given use for a call and its confirmation.
func (g *gate) mintFor(use string, c *call, p Pending) (string, error) {
	return g.signer.mint(use, tokenClaims{
		CID:        p.ID,
		SID:        c.sid,
		PID:        c.principal.BindingKey(),
		Tool:       c.tool.Name,
		ArgsSHA256: c.argsHash,
		Exp:        p.ExpiresAt.Unix(),
		Nonce:      randomID(16),
	})
}

// notify asks every resolver to prompt the human. It reports whether at least
// one accepted the request.
func (g *gate) notify(ctx context.Context, p Pending, agent bool) bool {
	rctx, cancel := context.WithDeadline(context.WithoutCancel(ctx), p.ExpiresAt)
	_ = cancel // released when the deadline passes
	asked := false
	for _, r := range g.list() {
		if !resolverFits(r, p.Principal, agent) {
			continue
		}
		resolve := func(approve bool, reviewed *ReviewedEffect) error {
			return g.broker.ResolveReviewed(p.ID, r.Name(), approve, reviewed)
		}
		var err error
		if rr, ok := r.(ReviewedResolver); ok {
			err = rr.RequestReviewed(rctx, p, resolve)
		} else {
			err = r.Request(rctx, p, func(approve bool) error { return resolve(approve, nil) })
		}
		if err != nil {
			g.log.Warn("confirmation resolver failed", "resolver", r.Name(), "error", err)
			continue
		}
		asked = true
	}
	return asked
}

// requiredError builds confirmation_required or confirmation_pending.
func (g *gate) requiredError(c *call, p Pending, tok, code string) *ToolError {
	all := g.list()
	names := make([]string, 0, len(all))
	for _, r := range all {
		if p.NativePrompt && !IsPresenceResolver(r.Name()) {
			continue
		}
		if !resolverFits(r, c.principal, c.fromAgent()) {
			continue
		}
		names = append(names, r.Name())
	}
	msg := "Confirmation required: " + p.Summary.Text + ". Nothing has changed."
	if code == CodeConfirmPending {
		msg = "Still waiting for a person to confirm: " + p.Summary.Text + ". Repeating the call does not approve it."
	}
	return &ToolError{
		Code:      code,
		Message:   msg,
		Retryable: code == CodeConfirmPending,
		Confirmation: &ConfirmationRequired{
			ConfirmationID:       p.ID,
			State:                StatePending,
			Summary:              p.Summary,
			ExpiresAt:            p.ExpiresAt,
			Resolvers:            names,
			ConfirmToken:         tok,
			RequiresNativePrompt: p.NativePrompt,
			PrincipalKind:        c.principal.Kind,
			EffectDigest:         p.EffectDigest,
			Destination:          cloneDestination(p.Destination),
			ResolvedBy:           p.ResolvedBy,
			ResolvedSurface:      cloneSurface(p.ResolvedSurface),
		},
	}
}

// outcomeError reports a confirmation that ended without running the call.
// status is an Outcome status: declined, denied, expired or invalid; the
// recorded state is mapped when it is one of the broker states.
func (g *gate) outcomeError(tool string, p Pending, status string) *ToolError {
	te := &ToolError{Tool: tool}
	switch status {
	case "declined", StateDenied:
		te.Code = CodeConfirmDeclined
		te.Message = "Denied. " + p.Summary.Text + " requires confirmation and was not confirmed. Nothing changed."
	case StateExpired:
		te.Code = CodeConfirmExpired
		te.Message = "The confirmation expired before this call could run. Nothing changed. Start a new call to ask again."
	default:
		te.Code = CodeConfirmInvalid
		te.Message = "This confirmation was already used or is not valid. Nothing changed."
		status = "invalid"
	}
	te.Details = map[string]any{"confirmation_id": p.ID, "status": status}
	if p.ResolvedBy != "" {
		te.Details["resolved_by"] = p.ResolvedBy
	}
	if p.ResolvedSurface != nil {
		te.Details["resolved_surface"] = cloneSurface(p.ResolvedSurface)
	}
	if p.EffectDigest != "" {
		te.Details["effect_digest"] = p.EffectDigest
	}
	return te
}

// acceptToken checks a presented confirm token. It is a binding handle, not
// consent: it is accepted only for a confirmation a person already approved,
// and only for exactly the call it was issued for.
func (g *gate) acceptToken(c *call, tok string) *ToolError {
	invalid := func(msg string) *ToolError {
		return &ToolError{Code: CodeConfirmInvalid, Message: msg + " Nothing changed.", Details: map[string]any{"status": "invalid"}}
	}
	if !c.principal.AcceptsToken() {
		return invalid("Confirm tokens are not accepted from this kind of caller; wait for the out-of-band approval and repeat the call without a token.")
	}
	claims, err := g.signer.verify(useConfirmToken, tok)
	if err != nil {
		return invalid(err.Error() + ".")
	}
	switch {
	case claims.SID != c.sid:
		return invalid("The confirm token belongs to another session.")
	case claims.PID != c.principal.BindingKey():
		return invalid("The confirm token belongs to another principal.")
	case claims.Tool != c.tool.Name:
		return invalid("The confirm token was issued for another tool.")
	case claims.ArgsSHA256 != c.argsHash:
		return invalid("The confirm token was issued for different arguments.")
	}
	if g.now().Unix() >= claims.Exp {
		return &ToolError{Code: CodeConfirmExpired, Message: "The confirm token expired. Nothing changed. Start a new call to ask again.", Details: map[string]any{"confirmation_id": claims.CID, "status": StateExpired}}
	}
	if g.broker.nonceSpent(claims.Nonce) {
		return invalid("The confirm token was already used.")
	}
	p, ok := g.broker.get(claims.CID)
	if !ok {
		return invalid("The confirmation behind this token is unknown.")
	}
	if p.Mechanism != MechanismExternal {
		return invalid("The confirmation behind this token was opened for the user's own prompt, not for out-of-band approval.")
	}
	if err := g.checkReview(c, p, nil); err != nil {
		return err
	}
	if errors.Is(g.broker.checkEffect(p.ID, c), ErrConfirmationDigestMismatch) {
		return digestMismatch(c.tool.Name, p.ID)
	}
	switch p.State {
	case StateApproved:
		claim, err := g.broker.claimEffect(p.ID, claims.Nonce, c)
		if err != nil {
			if errors.Is(err, ErrConfirmationDigestMismatch) {
				return digestMismatch(c.tool.Name, p.ID)
			}
			return invalid("The confirm token was already used.")
		}
		c.claim = claim
		return nil
	case StatePending:
		return g.requiredError(c, p, tok, CodeConfirmPending)
	case StateDenied:
		return g.outcomeError(c.tool.Name, p, StateDenied)
	case StateExpired:
		return g.outcomeError(c.tool.Name, p, StateExpired)
	default:
		return g.outcomeError(c.tool.Name, p, "invalid")
	}
}

// digestMismatch discloses no expected digest or arguments. A fresh proposal
// requires fresh review; retrying this confirmation cannot repair it.
func digestMismatch(tool, id string) *ToolError {
	return &ToolError{
		Tool: tool, Code: CodeConfirmDigestMismatch,
		Message: "The operation no longer matches the reviewed confirmation. Review a fresh request before approving.",
		Details: map[string]any{"confirmation_id": id},
	}
}

func nodeDestination(tool string) *Destination {
	// Index writes affect the configured embedding store; its destination is
	// not a Kubo node. Leave that unknown rather than mislabeling it. Node RPC
	// writes share a safe configured label, never a credentialed API address.
	if strings.HasPrefix(tool, "ipfs_index_") {
		return nil
	}
	return &Destination{Kind: "node", Name: "Configured Kubo node"}
}

// summaryOf builds the confirmation text of a call.
func (g *gate) summaryOf(c *call) Summary {
	args, extra := c.args, ""
	if g.summaryArgs != nil {
		args, extra = g.summaryArgs(c.tool.Name, c.args)
	}
	sum := buildSummary(c.tool, c.sc.rawIn, args)
	if extra != "" {
		sum.Text = clipRunes(sum.Text, maxSummaryRunes-utf8.RuneCountInString(extra)) + extra
	}
	return sum
}
