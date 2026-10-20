package server

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"
)

// Confirmation states (PendingConfirmation in the confirmation schema).
const (
	StatePending  = "pending"
	StateApproved = "approved"
	StateDenied   = "denied"
	StateExpired  = "expired"
	StateExecuted = "executed"
)

// Decision sources name distinct authority domains. A backend approval is never
// a broker confirmation, even when it names this node's tool or target.
const (
	DecisionSourceKubo         = "kubo"
	DecisionSourceUAR          = "uar"
	SourceKindNodeConfirmation = "node_confirmation"
	SourceKindBackendApproval  = "backend_approval"
)

// Resolver names (Resolver in the confirmation schema): the human-presence
// mechanisms that may approve a pending confirmation. The confirm token is
// deliberately not one of them.
const (
	ResolverElicitation    = "elicitation"
	ResolverAGUIInterrupt  = "agui-interrupt"
	ResolverDesktop        = "desktop"
	ResolverMacOSLocalAuth = "macos-localauth"
	ResolverLinuxPolkit    = "linux-polkit"
	ResolverCLI            = "cli"
)

// Mechanisms (PendingConfirmation.mechanism): how a confirmation was opened,
// which fixes who may answer it. A request state is accepted only for an
// elicitation confirmation; a confirm token only for an external one.
const (
	MechanismElicitation = "elicitation"
	MechanismExternal    = "external"
)

// IsPresenceResolver reports whether a resolver carries a presence factor: the
// native system prompt. Only these may approve a confirmation that requires a
// native prompt. The paired Desktop channel never counts: a click in a window
// is not a presence factor (HG-1 evidence of 2026-10-12), and the server cannot
// verify the Touch ID check Desktop makes, so desktop resolves write-class
// confirmations only. The cli resolver has no secret (a same-user pty can drive
// it) and agui-interrupt is a chat answer, so neither counts.
func IsPresenceResolver(name string) bool {
	switch name {
	case ResolverMacOSLocalAuth, ResolverLinuxPolkit:
		return true
	}
	return false
}

// confirmTTL is how long a confirmation, and a token bound to it, lives when
// Options.ConfirmTTL (confirmation.ttl) is not set.
const confirmTTL = DefaultConfirmTTL

// Errors returned by Broker.Resolve.
var (
	ErrUnknownConfirmation        = errors.New("unknown confirmation")
	ErrConfirmationResolved       = errors.New("confirmation is already resolved")
	ErrConfirmationExpired        = errors.New("confirmation expired")
	ErrNativePromptRequired       = errors.New("this confirmation needs a native prompt; a chat answer alone does not approve it")
	ErrWrongMechanism             = errors.New("the elicitation resolver may only answer a confirmation opened by elicitation")
	ErrCLINotForStdio             = errors.New("cli confirmation cannot approve a call on a stdio session")
	ErrConfirmationDigestMismatch = errors.New("confirmation effect no longer matches the reviewed operation")
	errNotApproved                = errors.New("confirmation is not approved")
)

// Target is something a confirmation affects.
type Target struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// Summary is what the user is asked to approve.
type Summary struct {
	Tool    string   `json:"tool"`
	Class   string   `json:"class"`
	Text    string   `json:"text"`
	Targets []Target `json:"targets"`
}

// Destination identifies an affected configured destination for display only.
// It never selects an endpoint or grants permission.
type Destination struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// ReviewedEffect is an optional resolver-transport echo of the digest shown to
// the person. It is neither a tool argument nor consent.
type ReviewedEffect struct {
	EffectDigest string `json:"effect_digest"`
}

// Pending is a snapshot of one confirmation held by the Broker
// (PendingConfirmation in the confirmation schema). Resolvers receive it.
type Pending struct {
	ID              string
	Source          string
	SourceKind      string
	State           string
	Principal       Principal
	Summary         Summary
	Resolver        string
	ResolvedBy      string
	ResolvedSurface *ResolvingSurface
	CreatedAt       time.Time
	ExpiresAt       time.Time
	ResolvedAt      time.Time
	NativePrompt    bool
	EffectDigest    string
	Destination     *Destination
	Execution       *ExecutionRecord
	ActivityID      string
	// Mechanism is MechanismElicitation or MechanismExternal.
	Mechanism string
}

// record is a Pending plus what binds it to one exact call.
type record struct {
	Pending
	sid      string
	tool     string
	argsHash string
	dedupe   string
	token    string
	// endpoint is an internal execution binding; never expose it in a view.
	endpoint     string
	observation  *retainedDecision
	observedOnce bool
	version      uint64
}

// Broker is the confirmation state machine. Every confirmation moves
//
//	pending -> approved | denied | expired
//	approved -> executed (exactly once) | expired
//
// Only Resolve, called by a human-presence resolver or by the elicitation
// path after the user said yes, moves a confirmation to approved. Nothing
// else does: a repeated call, a token or an agent's own message never does.
// An in-memory broker forgets everything on restart, which counts as denial.
type Broker struct {
	mu  sync.Mutex
	now func() time.Time
	ttl time.Duration
	// recheck is the longest Await sleeps between looks at the clock.
	recheck time.Duration
	byID    map[string]*record
	active  map[string]string
	spent   map[string]time.Time
	// changed is closed, and replaced, whenever a confirmation leaves pending
	// by a decision or is claimed; Await sleeps on it.
	changed chan struct{}
	// retained is the single resolved-decision history, separate from the
	// authorization/token grace maps. Its private owner never reaches a view.
	retained                 map[string]*retainedDecision
	historyEpoch             string
	historyStartedAt         time.Time
	historySecret            []byte
	historyVersion           uint64
	historyEnabled           bool
	decisionSubs             map[*decisionSubscription]struct{}
	decisionWake             chan struct{}
	decisionSources          map[string]liveDecisionSource
	decisionSourceGeneration string
}

func newBroker(now func() time.Time, ttl time.Duration) *Broker {
	return &Broker{
		now:     now,
		ttl:     ttl,
		recheck: defaultAwaitRecheck,
		byID:    make(map[string]*record),
		active:  make(map[string]string),
		spent:   make(map[string]time.Time),

		changed:          make(chan struct{}),
		retained:         make(map[string]*retainedDecision),
		historyEpoch:     "epoch_" + randomID(16),
		historyStartedAt: now().UTC(),
		historySecret:    []byte(randomID(32)),
		historyEnabled:   true,
		decisionWake:     make(chan struct{}),
	}
}

// signal wakes every Await. Callers hold b.mu.
func (b *Broker) signal() {
	close(b.changed)
	b.changed = make(chan struct{})
}

// defaultAwaitRecheck bounds how long Await sleeps between looks at the clock,
// so an expiry that nobody triggers is still noticed and a clock that jumped
// does not strand a waiter. A Broker carries its own value (recheck), set
// before it is shared.
const defaultAwaitRecheck = time.Second

// Await returns the state of a confirmation once it is no longer pending
// (approved, denied, expired or executed), or ctx's error if ctx ends first.
// It decides nothing. An unknown ID is ErrUnknownConfirmation.
func (b *Broker) Await(ctx context.Context, id string) (string, error) {
	for {
		b.mu.Lock()
		r, ok := b.byID[id]
		if !ok {
			b.mu.Unlock()
			return "", ErrUnknownConfirmation
		}
		b.refresh(r)
		if r.State != StatePending {
			state := r.State
			b.mu.Unlock()
			return state, nil
		}
		wake := b.changed
		wait := min(r.ExpiresAt.Sub(b.now()), b.recheck)
		b.mu.Unlock()

		timer := time.NewTimer(max(wait, time.Millisecond))
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-wake:
		case <-timer.C:
		}
		timer.Stop()
	}
}

type newConfirmation struct {
	principal    Principal
	sid          string
	tool         string
	argsHash     string
	summary      Summary
	nativeReq    bool
	mechanism    string
	effectDigest string
	destination  *Destination
	endpoint     string
	activityID   string
}

// refresh applies expiry: a confirmation that is still pending or approved
// past its deadline is expired. Callers hold b.mu.
func (b *Broker) refresh(r *record) {
	if (r.State == StatePending || r.State == StateApproved) && !b.now().Before(r.ExpiresAt) {
		r.State = StateExpired
		r.ResolvedAt = r.ExpiresAt
		b.retainNodeLocked(r)
		b.publishDecisionLocked(r.Principal.BindingKey(), r.ID, StateExpired)
		b.signal()
	}
}

// sweep drops records and nonces that can no longer matter: a grace period
// after expiry keeps tokens answering "expired" instead of "unknown".
func (b *Broker) sweep() {
	// Keep an expired record for at least DefaultConfirmTTL so that a retry,
	// or a token, still sees "expired" rather than "unknown" with a short ttl.
	cutoff := b.now().Add(-max(b.ttl, DefaultConfirmTTL))
	for id, r := range b.byID {
		b.refresh(r)
		if r.ExpiresAt.Before(cutoff) {
			delete(b.byID, id)
			if b.active[r.dedupe] == id {
				delete(b.active, r.dedupe)
			}
		}
	}
	b.pruneRetainedLocked()
	b.pruneDecisionSourcesLocked()
	for n, exp := range b.spent {
		if exp.Before(cutoff) {
			delete(b.spent, n)
		}
	}
}

func dedupeKey(sid, principalID, tool, argsHash string) string {
	return sid + "\x00" + principalID + "\x00" + tool + "\x00" + argsHash
}

// maxOpenExternal caps the out-of-band confirmations that may wait at once for
// one principal, and for one session. Each one can raise a system prompt, so an
// unbounded loop of calls would flood the person with prompts.
const maxOpenExternal = 2

// ErrTooManyOpen is returned by create when a principal or a session already has
// the most out-of-band confirmations waiting.
var ErrTooManyOpen = errors.New("too many confirmations are already waiting for this principal or session")

// The cap counts by principal ID on purpose: two subjects mapped to one
// principal share it. That is a shared, fail-closed limit on system prompts, not
// a binding leak; ownership of a confirmation is keyed on Principal.BindingKey.
// openExternalLocked counts the pending out-of-band confirmations of a
// principal and of a session. Callers hold b.mu.
func (b *Broker) openExternalLocked(principalID, sid string) (byPrincipal, bySession int) {
	for _, r := range b.byID {
		b.refresh(r)
		if r.State != StatePending || r.Mechanism != MechanismExternal {
			continue
		}
		if r.Principal.ID == principalID {
			byPrincipal++
		}
		if r.sid == sid {
			bySession++
		}
	}
	return byPrincipal, bySession
}

// create opens a pending confirmation for exactly one call, with no cap on how
// many are open.
func (b *Broker) create(n newConfirmation) Pending {
	p, _ := b.reserve(n, 0)
	return p
}

// reserve is create with a cap: with limit above zero, an out-of-band
// confirmation is refused with ErrTooManyOpen when its principal or its session
// already has limit of them waiting. The count and the insert are one step.
func (b *Broker) reserve(n newConfirmation, limit int) (Pending, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sweep()
	if limit > 0 && n.mechanism == MechanismExternal {
		bp, bs := b.openExternalLocked(n.principal.ID, n.sid)
		if bp >= limit || bs >= limit {
			return Pending{}, ErrTooManyOpen
		}
	}
	now := b.now()
	r := &record{
		Pending: Pending{
			ID:           "cf_n_" + randomID(16),
			Source:       DecisionSourceKubo,
			SourceKind:   SourceKindNodeConfirmation,
			State:        StatePending,
			Principal:    n.principal,
			Summary:      n.summary,
			CreatedAt:    now,
			ExpiresAt:    now.Add(b.ttl),
			NativePrompt: n.nativeReq,
			Mechanism:    n.mechanism,
			EffectDigest: n.effectDigest,
			Destination:  cloneDestination(n.destination),
			ActivityID:   n.activityID,
		},
		sid:      n.sid,
		tool:     n.tool,
		argsHash: n.argsHash,
		endpoint: n.endpoint,
		dedupe:   dedupeKey(n.sid, n.principal.BindingKey(), n.tool, n.argsHash),
	}
	r.Summary.Targets = slices.Clone(n.summary.Targets)
	r.version = b.nextVersionLocked()
	b.byID[r.ID] = r
	b.active[r.dedupe] = r.ID
	b.publishDecisionLocked(r.Principal.BindingKey(), r.ID, StatePending)
	return r.snapshot(), nil
}

// setToken records the token handed out for a confirmation so that a repeated
// call gets the same one back.
func (b *Broker) setToken(id, tok string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r, ok := b.byID[id]; ok {
		r.token = tok
	}
}

// activeFor returns the open confirmation for the same session, principal,
// tool and canonical arguments, and its token.
func (b *Broker) activeFor(dedupe string) (Pending, string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id, ok := b.active[dedupe]
	if !ok {
		return Pending{}, "", false
	}
	r := b.byID[id]
	b.refresh(r)
	return r.snapshot(), r.token, true
}

// get returns a snapshot of one confirmation.
func (b *Broker) get(id string) (Pending, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, ok := b.byID[id]
	if !ok {
		return Pending{}, false
	}
	b.refresh(r)
	return r.snapshot(), true
}

func cloneDestination(d *Destination) *Destination {
	if d == nil {
		return nil
	}
	out := *d
	return &out
}

func (r *record) snapshot() Pending {
	p := r.Pending
	p.Summary.Targets = slices.Clone(r.Summary.Targets)
	p.Destination = cloneDestination(r.Destination)
	p.ResolvedSurface = cloneSurface(r.ResolvedSurface)
	if r.observation != nil && r.observation.execution.State != "" {
		p.Execution = cloneExecution(&r.observation.execution)
	}
	return p
}

// forget closes a confirmation to further repeated calls once its outcome was
// reported. Deny is final: the next identical call opens a new confirmation.
// Tokens still see the recorded state until the sweep.
func (b *Broker) forget(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, ok := b.byID[id]
	if !ok {
		return
	}
	if b.active[r.dedupe] == id {
		delete(b.active, r.dedupe)
	}
}

// Resolve records a human decision from the named resolver. It is the only
// way to approve. Resolving a confirmation that is not pending, or that has
// expired, fails.
func (b *Broker) Resolve(id, resolver string, approve bool) error {
	return b.ResolveReviewed(id, resolver, approve, nil)
}

// ResolveReviewed retains Resolve's authority checks and additionally rejects a
// stale supplied review. Adapters must authenticate and authorize their caller
// before invoking either method. Omission preserves the legacy gate.
func (b *Broker) ResolveReviewed(id, resolver string, approve bool, reviewed *ReviewedEffect) error {
	return b.ResolveAtSurface(id, resolver, approve, reviewed, ResolverSurface(resolver))
}

// ResolveAtSurface records attribution supplied by an already authorized
// adapter. It retains all legacy mechanism/native/expiry checks and never uses
// the surface to choose a resolver, principal or operation.
func (b *Broker) ResolveAtSurface(id, resolver string, approve bool, reviewed *ReviewedEffect, surface *ResolvingSurface) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, ok := b.byID[id]
	if !ok {
		return ErrUnknownConfirmation
	}
	b.refresh(r)
	switch r.State {
	case StateExpired:
		return ErrConfirmationExpired
	case StatePending:
	default:
		return ErrConfirmationResolved
	}
	if resolver == ResolverElicitation && r.Mechanism != MechanismElicitation {
		return ErrWrongMechanism
	}
	if resolver == ResolverCLI && r.Principal == LocalPrincipal {
		return ErrCLINotForStdio
	}
	if approve && r.NativePrompt && !IsPresenceResolver(resolver) {
		return ErrNativePromptRequired
	}
	if reviewed != nil && !validEffectDigest(reviewed.EffectDigest) {
		return ErrMalformedReviewedEffect
	}
	if reviewed != nil && (r.EffectDigest == "" || reviewed.EffectDigest != r.EffectDigest) {
		return ErrConfirmationDigestMismatch
	}
	if err := validateResolverSurface(resolver, surface); err != nil {
		return err
	}
	r.Resolver = resolver
	// Empty resolver means a system withdrawal, never a human mechanism.
	if slices.Contains([]string{ResolverElicitation, ResolverAGUIInterrupt, ResolverDesktop, ResolverMacOSLocalAuth, ResolverLinuxPolkit, ResolverCLI}, resolver) {
		r.ResolvedBy = resolver
		r.ResolvedSurface = cloneSurface(surface)
	}
	r.ResolvedAt = b.now()
	if approve {
		r.State = StateApproved
	} else {
		r.State = StateDenied
		b.retainNodeLocked(r)
	}
	r.version = b.nextVersionLocked()
	b.publishDecisionLocked(r.Principal.BindingKey(), r.ID, r.State)
	b.signal()
	return nil
}

// Cancel ends a confirmation that has not run: a pending or approved one
// becomes denied and no longer matches a repeated call, so an approval that
// can no longer be used is never claimed later. A confirmation that already
// ended (denied, expired, executed) is left as it is.
func (b *Broker) Cancel(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, ok := b.byID[id]
	if !ok {
		return
	}
	b.refresh(r)
	if r.State != StatePending && r.State != StateApproved {
		return
	}
	r.State = StateDenied
	r.ResolvedAt = b.now()
	b.retainNodeLocked(r)
	if b.active[r.dedupe] == id {
		delete(b.active, r.dedupe)
	}
	b.publishDecisionLocked(r.Principal.BindingKey(), r.ID, StateDenied)
	b.signal()
}

// Pending lists the confirmations that still wait for a decision.
func (b *Broker) Pending() []Pending {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []Pending
	for _, r := range b.byID {
		b.refresh(r)
		if r.State == StatePending {
			out = append(out, r.snapshot())
		}
	}
	return out
}

// claim moves an approved confirmation to executed. It succeeds once: the
// caller that gets nil runs the call, every other caller is refused. When
// nonce is not empty it is recorded as spent in the same step.
func (b *Broker) claim(id, nonce string) error {
	_, err := b.claimEffect(id, nonce, nil)
	return err
}

// claimEffect atomically checks the retained operation before consuming its
// approval. The legacy token/session/identity bindings are checked by the gate.
func (b *Broker) claimEffect(id, nonce string, c *call) (*executionClaim, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, ok := b.byID[id]
	if !ok {
		return nil, ErrUnknownConfirmation
	}
	b.refresh(r)
	if r.State != StateApproved {
		return nil, errNotApproved
	}
	if c != nil && !r.matchesEffect(c) {
		return nil, ErrConfirmationDigestMismatch
	}
	if nonce != "" {
		if _, used := b.spent[nonce]; used {
			return nil, errNotApproved
		}
		b.spent[nonce] = r.ExpiresAt
	}
	r.State = StateExecuted
	r.ResolvedAt = b.now()
	claim := &executionClaim{b: b, id: r.ID}
	p := r.snapshot()
	p.Principal.sub = "" // the private owner binding, not a subject, is retained
	if p.EffectDigest == "" && c != nil {
		// A legacy record may lack display metadata. Enrich only from this
		// same v1-bound canonical call, never from a displayed summary.
		p.EffectDigest = c.effectDigest
		r.EffectDigest = c.effectDigest
	}
	e := &retainedDecision{pending: p, owner: r.Principal.BindingKey(), tool: r.tool, execution: ExecutionRecord{State: ExecutionNotStarted}, claim: claim}
	claim.entry = e
	r.observation = e
	r.observedOnce = true
	e.version = b.nextVersionLocked()
	r.version = e.version
	b.retained[r.ID] = e
	if b.active[r.dedupe] == id {
		delete(b.active, r.dedupe)
	}
	b.publishDecisionLocked(e.owner, r.ID, ExecutionNotStarted)
	b.signal()
	return claim, nil
}

// checkEffect also applies to pending calls, before any elicitation answer is
// resolved. Callers already checked the legacy owner and operation bindings.
func (b *Broker) checkEffect(id string, c *call) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, ok := b.byID[id]
	if !ok {
		return ErrUnknownConfirmation
	}
	if !r.matchesEffect(c) {
		return ErrConfirmationDigestMismatch
	}
	return nil
}

func (r *record) matchesEffect(c *call) bool {
	// Records prepared by older in-process callers may lack optional metadata;
	// their authoritative tool/argument bindings remain required.
	return r.sid == c.sid && r.Principal.BindingKey() == c.principal.BindingKey() &&
		r.tool == c.tool.Name && r.argsHash == c.argsHash &&
		(r.EffectDigest == "" || r.EffectDigest == c.effectDigest) &&
		(r.endpoint == "" || r.endpoint == c.endpoint) &&
		(c.reviewed == nil || r.EffectDigest != "" && c.reviewed.EffectDigest == r.EffectDigest)
}

func (b *Broker) checkReviewed(id string, reviewed *ReviewedEffect) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, ok := b.byID[id]
	if !ok {
		return ErrUnknownConfirmation
	}
	if reviewed == nil {
		return nil
	}
	if !validEffectDigest(reviewed.EffectDigest) {
		return ErrMalformedReviewedEffect
	}
	if r.EffectDigest == "" || reviewed.EffectDigest != r.EffectDigest {
		return ErrConfirmationDigestMismatch
	}
	return nil
}

// nonceSpent reports whether a token nonce was already used.
func (b *Broker) nonceSpent(nonce string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, used := b.spent[nonce]
	return used
}
