package server

import (
	"errors"
	"slices"
	"time"
)

const (
	maxDecisionStreamsPerOwner     = 4
	maxDecisionStreamsPerPrincipal = 16
	maxDecisionStreams             = 64
	decisionChangeQueue            = 16
	maxDecisionEventBytes          = 1024
	maxDecisionLiveSources         = 64
)

var (
	errDecisionObserverDisabled  = errors.New("decision observation is disabled")
	errDecisionSubscriptionsFull = errors.New("decision subscriptions are full")
)

// decisionChanged is only an invalidation hint. Read an authorized snapshot to
// learn actual facts; no private owner, activity, operation or receipt is sent.
type decisionChanged struct {
	ID       string `json:"id"`
	State    string `json:"state"`
	Revision string `json:"revision"`
}

type decisionSnapshotRequired struct {
	Epoch    string `json:"server_epoch"`
	Revision string `json:"revision"`
	Reason   string `json:"reason"`
}

type decisionNotice struct {
	change   *decisionChanged
	snapshot *decisionSnapshotRequired
}

// decisionSubscription owns one bounded queue. Initial is separate so overflow
// cannot replace the first snapshot marker before a transport has sent it.
// broker.mu guards ownership, overflow, closing and every queue producer.
type decisionSubscription struct {
	b                *Broker
	owner, principal string
	initial          decisionSnapshotRequired
	events           chan decisionNotice
	done             chan struct{}
	overflow         bool
	closed           bool
}

func (b *Broker) subscribeDecisions(p Principal) (*decisionSubscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.historyEnabled || p.ID == "" || !slices.Contains([]string{KindUI, KindHumanMCP, KindA2A, KindUAR}, p.Kind) {
		return nil, errDecisionObserverDisabled
	}
	b.sweep()
	ownerCount, principalCount := 0, 0
	for sub := range b.decisionSubs {
		if sub.owner == p.BindingKey() {
			ownerCount++
		}
		if sub.principal == p.ID {
			principalCount++
		}
	}
	if len(b.decisionSubs) >= maxDecisionStreams || ownerCount >= maxDecisionStreamsPerOwner || principalCount >= maxDecisionStreamsPerPrincipal {
		return nil, errDecisionSubscriptionsFull
	}
	sub := &decisionSubscription{b: b, owner: p.BindingKey(), principal: p.ID, events: make(chan decisionNotice, decisionChangeQueue), done: make(chan struct{})}
	if b.decisionSubs == nil {
		b.decisionSubs = make(map[*decisionSubscription]struct{})
	}
	b.decisionSubs[sub] = struct{}{}
	// Register before reading the equality token, in the same atomic section.
	sub.initial = decisionSnapshotRequired{Epoch: b.historyEpoch, Revision: b.decisionRevisionLocked(sub.owner), Reason: "initial"}
	return sub, nil
}

func (s *decisionSubscription) close() {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	s.b.closeDecisionSubscriptionLocked(s)
}

func (b *Broker) closeDecisionSubscriptionLocked(sub *decisionSubscription) {
	if sub.closed {
		return
	}
	delete(b.decisionSubs, sub)
	sub.closed = true
	close(sub.done)
	for {
		select {
		case <-sub.events:
		default:
			goto drained
		}
	}
drained:
	close(sub.events)
}

func decisionHintState(p Pending) string {
	if p.Execution != nil && p.Execution.State != "" {
		return p.Execution.State
	}
	return p.State
}

// publishDecisionLocked does no I/O, callback or source/session locking. A
// slow reader can only replace its own queue with a reconciliation marker.
func (b *Broker) publishDecisionLocked(owner, id, state string) {
	if !b.historyEnabled {
		return
	}
	close(b.decisionWake)
	b.decisionWake = make(chan struct{})
	if owner == "" || len(id) == 0 || len(id) > 128 || !slices.Contains([]string{StatePending, StateApproved, StateDenied, StateExpired, StateExecuted, ExecutionNotStarted, ExecutionRunning, ExecutionSucceeded, ExecutionFailed, ExecutionUnknown}, state) {
		return
	}
	if len(b.decisionSubs) == 0 {
		return
	}
	revision := b.decisionRevisionLocked(owner)
	for sub := range b.decisionSubs {
		if sub.owner != owner {
			continue
		}
		if sub.overflow && len(sub.events) == 0 {
			sub.overflow = false
		}
		if !sub.overflow {
			change := &decisionChanged{ID: id, State: state, Revision: revision}
			select {
			case sub.events <- decisionNotice{change: change}:
				continue
			default:
			}
		}
		// One producer holds broker.mu. A concurrent consumer may drain too;
		// nonblocking receives avoid depending on that consumer's timing.
		for {
			select {
			case <-sub.events:
			default:
				goto drained
			}
		}
	drained:
		sub.overflow = true
		select {
		case sub.events <- decisionNotice{snapshot: &decisionSnapshotRequired{Epoch: b.historyEpoch, Revision: revision, Reason: "overflow"}}:
		default:
			b.closeDecisionSubscriptionLocked(sub) // force a fresh initial snapshot
		}
	}
}

// decisionObservationSchedule is the bounded scheduler hook for I/1.6. Read
// deadline and wake channel together, then wait outside every broker/source
// lock. A new relevant mutation replaces wake; no timer goroutine exists here.
func (b *Broker) decisionObservationSchedule() (time.Time, <-chan struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var next time.Time
	include := func(at time.Time) {
		if !at.IsZero() && (next.IsZero() || at.Before(next)) {
			next = at
		}
	}
	if b.historyEnabled {
		for _, r := range b.byID {
			if r.State == StatePending || r.State == StateApproved {
				include(r.ExpiresAt)
			}
		}
		for _, e := range b.retained {
			if !e.closedAt.IsZero() {
				include(e.closedAt.Add(decisionHistoryTTL))
			}
		}
		for _, source := range b.decisionSources {
			include(source.deadline)
		}
	}
	return next, b.decisionWake
}

func (b *Broker) sweepDecisionObservations() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.historyEnabled {
		b.sweep()
	}
}

func (b *Broker) decisionObservationEnabled() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.historyEnabled
}

// invalidateDecisionSubscriptions is the in-process known-invalidation hook.
// Existing credentials/configuration are immutable until restart; callers that
// know an owner or configured principal was invalidated can close its streams
// immediately without a credential lookup, refresh service or polling.
// Empty filters together close all subscriptions during listener shutdown.
func (b *Broker) invalidateDecisionSubscriptions(owner, principal string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for sub := range b.decisionSubs {
		if owner == "" && principal == "" || owner != "" && sub.owner == owner || principal != "" && sub.principal == principal {
			b.closeDecisionSubscriptionLocked(sub)
		}
	}
}

func (b *Broker) decisionObservationNow() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.now()
}

// liveDecisionSource is a bounded invalidation index, never another live
// observation/authority store. Only opaque IDs, private ownership, versions and
// cleanup metadata are retained; summaries/arguments/continuations stay out.
type liveDecisionSource struct {
	id, owner, thread, state string
	version                  uint64
	deadline                 time.Time
	factHash                 [32]byte
}

func (b *Broker) setDecisionSourceGeneration(generation string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if generation == b.decisionSourceGeneration {
		return
	}
	b.decisionSourceGeneration = generation
	for id := range b.decisionSources {
		b.removeDecisionSourceLocked(id)
	}
}

// replaceDecisionSources is called with the UAR source lock before broker.mu.
// An old backend's in-flight release cannot republish a replaced generation.
func (b *Broker) replaceDecisionSources(generation string, sources []liveDecisionSource) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.historyEnabled || generation != b.decisionSourceGeneration {
		return
	}
	desired := make(map[string]liveDecisionSource, min(len(sources), maxDecisionLiveSources))
	threads := make(map[string]struct{}, min(len(sources), maxDecisionLiveSources))
	for _, source := range sources {
		if len(desired) == maxDecisionLiveSources {
			break
		}
		if source.id == "" || source.owner == "" || !b.now().Before(source.deadline) {
			continue
		}
		if _, duplicate := threads[source.thread]; duplicate {
			continue
		}
		threads[source.thread] = struct{}{}
		desired[source.id] = source
	}
	var changes []liveDecisionSource
	for id, previous := range b.decisionSources {
		if _, exists := desired[id]; !exists {
			delete(b.decisionSources, id)
			changes = append(changes, previous)
		}
	}
	if b.decisionSources == nil {
		b.decisionSources = make(map[string]liveDecisionSource, len(desired))
	}
	for id, source := range desired {
		previous, exists := b.decisionSources[id]
		if exists && previous.owner == source.owner && previous.thread == source.thread && previous.state == source.state && previous.factHash == source.factHash {
			source.version = previous.version
		} else {
			source.version = b.nextVersionLocked()
			changes = append(changes, source)
		}
		b.decisionSources[id] = source
	}
	for _, change := range changes {
		b.publishDecisionLocked(change.owner, change.id, change.state)
	}
}

func (b *Broker) removeDecisionSourceLocked(id string) {
	source, exists := b.decisionSources[id]
	if !exists {
		return
	}
	delete(b.decisionSources, id)
	b.publishDecisionLocked(source.owner, id, source.state)
}

func (b *Broker) pruneDecisionSourcesLocked() {
	for id, source := range b.decisionSources {
		if !b.now().Before(source.deadline) {
			b.removeDecisionSourceLocked(id)
		}
	}
}
