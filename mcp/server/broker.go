package server

import (
	"context"
	"errors"
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
	ErrUnknownConfirmation  = errors.New("unknown confirmation")
	ErrConfirmationResolved = errors.New("confirmation is already resolved")
	ErrConfirmationExpired  = errors.New("confirmation expired")
	ErrNativePromptRequired = errors.New("this confirmation needs a native prompt; a chat answer alone does not approve it")
	ErrWrongMechanism       = errors.New("the elicitation resolver may only answer a confirmation opened by elicitation")
	ErrCLINotForStdio       = errors.New("cli confirmation cannot approve a call on a stdio session")
	errNotApproved          = errors.New("confirmation is not approved")
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

// Pending is a snapshot of one confirmation held by the Broker
// (PendingConfirmation in the confirmation schema). Resolvers receive it.
type Pending struct {
	ID           string
	State        string
	Principal    Principal
	Summary      Summary
	Resolver     string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	ResolvedAt   time.Time
	NativePrompt bool
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
}

func newBroker(now func() time.Time, ttl time.Duration) *Broker {
	return &Broker{
		now:     now,
		ttl:     ttl,
		recheck: defaultAwaitRecheck,
		byID:    make(map[string]*record),
		active:  make(map[string]string),
		spent:   make(map[string]time.Time),

		changed: make(chan struct{}),
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
	principal Principal
	sid       string
	tool      string
	argsHash  string
	summary   Summary
	nativeReq bool
	mechanism string
}

// refresh applies expiry: a confirmation that is still pending or approved
// past its deadline is expired. Callers hold b.mu.
func (b *Broker) refresh(r *record) {
	if (r.State == StatePending || r.State == StateApproved) && !b.now().Before(r.ExpiresAt) {
		r.State = StateExpired
		r.ResolvedAt = b.now()
	}
}

// sweep drops records and nonces that can no longer matter: a grace period
// after expiry keeps tokens answering "expired" instead of "unknown".
func (b *Broker) sweep() {
	// Keep an expired record for at least DefaultConfirmTTL so that a retry,
	// or a token, still sees "expired" rather than "unknown" with a short ttl.
	cutoff := b.now().Add(-max(b.ttl, DefaultConfirmTTL))
	for id, r := range b.byID {
		if r.ExpiresAt.Before(cutoff) {
			delete(b.byID, id)
			if b.active[r.dedupe] == id {
				delete(b.active, r.dedupe)
			}
		}
	}
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
			ID:           "cf_" + randomID(16),
			State:        StatePending,
			Principal:    n.principal,
			Summary:      n.summary,
			CreatedAt:    now,
			ExpiresAt:    now.Add(b.ttl),
			NativePrompt: n.nativeReq,
			Mechanism:    n.mechanism,
		},
		sid:      n.sid,
		tool:     n.tool,
		argsHash: n.argsHash,
		dedupe:   dedupeKey(n.sid, n.principal.ID, n.tool, n.argsHash),
	}
	b.byID[r.ID] = r
	b.active[r.dedupe] = r.ID
	return r.Pending, nil
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
	return r.Pending, r.token, true
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
	return r.Pending, true
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
	r.Resolver = resolver
	r.ResolvedAt = b.now()
	if approve {
		r.State = StateApproved
	} else {
		r.State = StateDenied
	}
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
	if b.active[r.dedupe] == id {
		delete(b.active, r.dedupe)
	}
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
			out = append(out, r.Pending)
		}
	}
	return out
}

// claim moves an approved confirmation to executed. It succeeds once: the
// caller that gets nil runs the call, every other caller is refused. When
// nonce is not empty it is recorded as spent in the same step.
func (b *Broker) claim(id, nonce string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, ok := b.byID[id]
	if !ok {
		return ErrUnknownConfirmation
	}
	b.refresh(r)
	if r.State != StateApproved {
		return errNotApproved
	}
	if nonce != "" {
		if _, used := b.spent[nonce]; used {
			return errNotApproved
		}
		b.spent[nonce] = r.ExpiresAt
	}
	r.State = StateExecuted
	r.ResolvedAt = b.now()
	if b.active[r.dedupe] == id {
		delete(b.active, r.dedupe)
	}
	b.signal()
	return nil
}

// nonceSpent reports whether a token nonce was already used.
func (b *Broker) nonceSpent(nonce string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, used := b.spent[nonce]
	return used
}
