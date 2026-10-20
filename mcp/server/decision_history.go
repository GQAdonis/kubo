package server

import (
	"cmp"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"time"
)

// decisionSnapshot is an internal atomic source snapshot. Public adapters must
// project its already owner-filtered items through DecisionObservation bounds.
// Revision is an opaque equality token; it has no ordering or numeric meaning.
type decisionSnapshot struct {
	Epoch            string
	Revision         string
	HistoryStartedAt time.Time
	Items            []Pending
}

func (b *Broker) nextVersionLocked() uint64 {
	b.historyVersion++
	return b.historyVersion
}

func clonePending(p Pending) Pending {
	p.Principal.sub = ""
	p.Summary.Targets = slices.Clone(p.Summary.Targets)
	p.Destination = cloneDestination(p.Destination)
	p.ResolvedSurface = cloneSurface(p.ResolvedSurface)
	p.Execution = cloneExecution(p.Execution)
	return p
}

func (e *retainedDecision) snapshot() Pending {
	p := clonePending(e.pending)
	if e.execution.State != "" {
		p.Execution = cloneExecution(&e.execution)
	}
	return p
}

func (b *Broker) closeRetainedLocked(e *retainedDecision, at time.Time) {
	if !e.closedAt.IsZero() {
		return
	}
	e.closedAt = at.UTC()
	e.version = b.nextVersionLocked()
	e.closedOrder = e.version
}

// retainNodeLocked runs only after an authoritative denial/withdrawal/expiry.
// The private owner is captured before scrubbing; the safe principal must never
// be used to reconstruct ownership for a subject-bound identity.
func (b *Broker) retainNodeLocked(r *record) {
	if r.observedOnce {
		return
	}
	r.observedOnce = true
	if !b.historyEnabled {
		return
	}
	e := &retainedDecision{pending: clonePending(r.snapshot()), owner: r.Principal.BindingKey(), tool: r.tool}
	b.closeRetainedLocked(e, r.ResolvedAt)
	r.observation = e
	r.version = e.version
	b.retained[r.ID] = e
	b.pruneRetainedLocked()
}

// retainBackendDecision copies only an already settled observation, not a UAR
// continuation or authority. Its caller supplies the authenticated thread's
// private owner separately from the scrubbed public principal. First ingestion
// wins, so repeated settlement cannot extend retention or change provenance.
func (b *Broker) retainBackendDecision(p Pending, owner string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.historyEnabled || owner == "" || p.Source != DecisionSourceUAR || p.SourceKind != SourceKindBackendApproval || p.Execution != nil {
		return
	}
	if p.CreatedAt.IsZero() || p.ResolvedAt.IsZero() || !slices.Contains([]string{StateDenied, StateExpired, StateExecuted}, p.State) {
		return
	}
	if _, exists := b.retained[p.ID]; exists {
		return
	}
	e := &retainedDecision{pending: clonePending(p), owner: owner}
	b.closeRetainedLocked(e, p.ResolvedAt)
	b.retained[p.ID] = e
	b.pruneRetainedLocked()
	b.publishDecisionLocked(e.owner, p.ID, p.State)
}

// pruneRetainedLocked is the single server-wide resolved ring. Active claims
// have no closedAt and are never removed to make room for completed history.
// Equal timestamps use private insertion order, not random IDs, for eviction.
func (b *Broker) pruneRetainedLocked() {
	cutoff := b.now().Add(-decisionHistoryTTL)
	resolved := make([]string, 0, len(b.retained))
	for id, e := range b.retained {
		if e.closedAt.IsZero() {
			continue
		}
		if !e.closedAt.After(cutoff) {
			b.removeRetainedLocked(id)
			continue
		}
		resolved = append(resolved, id)
	}
	slices.SortFunc(resolved, func(a, z string) int {
		if d := b.retained[a].closedAt.Compare(b.retained[z].closedAt); d != 0 {
			return d
		}
		return cmp.Compare(b.retained[a].closedOrder, b.retained[z].closedOrder)
	})
	for _, id := range resolved[:max(0, len(resolved)-decisionHistoryLimit)] {
		b.removeRetainedLocked(id)
	}
}

func (b *Broker) removeRetainedLocked(id string) {
	e, exists := b.retained[id]
	if !exists {
		return
	}
	if r := b.byID[id]; r != nil {
		// Keep only a tombstone in the authorization grace record. Holding its
		// old snapshot here would secretly defeat the global history bound.
		r.observation = nil
	}
	delete(b.retained, id)
	state := decisionHintState(Pending{State: e.pending.State, Execution: &e.execution})
	b.publishDecisionLocked(e.owner, id, state)
}

// observedLocked prefers retained evidence, including after token-grace sweep.
// An evicted resolved observation must not reappear from an authorization map.
func (b *Broker) observedLocked(id string) (Pending, string, uint64, bool) {
	if e, ok := b.retained[id]; ok {
		return e.snapshot(), e.owner, e.version, true
	}
	if r, ok := b.byID[id]; ok && !r.observedOnce {
		return clonePending(r.snapshot()), r.Principal.BindingKey(), r.version, true
	}
	return Pending{}, "", 0, false
}

// observed is an internal lookup used by trusted adapters and fixtures. A
// public read must use snapshotFor/observedFor with actual authenticated owner.
func (b *Broker) observed(id string) (Pending, string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sweep()
	p, owner, _, ok := b.observedLocked(id)
	return p, owner, ok
}

func (b *Broker) observedFor(owner, id string) (Pending, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sweep()
	p, actual, _, ok := b.observedLocked(id)
	if !ok || owner == "" || actual != owner {
		return Pending{}, false
	}
	return p, true
}

func (b *Broker) snapshotFor(owner string) decisionSnapshot {
	return b.snapshotForSources(owner, nil)
}

// snapshotForSources takes already owner-filtered live source copies. Source
// locks are acquired before broker.mu, never from a mutation callback.
func (b *Broker) snapshotForSources(owner string, live []Pending) decisionSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sweep()
	out := decisionSnapshot{Epoch: b.historyEpoch, HistoryStartedAt: b.historyStartedAt, Items: []Pending{}}
	ids := make([]string, 0, len(b.byID)+len(b.retained))
	for id, r := range b.byID {
		if !r.observedOnce && r.Principal.BindingKey() == owner && owner != "" {
			ids = append(ids, id)
		}
	}
	for id, e := range b.retained {
		if e.owner == owner && owner != "" {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		p, _, _, _ := b.observedLocked(id)
		out.Items = append(out.Items, p)
	}
	for _, p := range live {
		source, exists := b.decisionSources[p.ID]
		if !exists || source.owner != owner {
			continue
		}
		if _, _, _, exists := b.observedLocked(p.ID); exists {
			continue
		}
		out.Items = append(out.Items, clonePending(p))
	}
	out.Revision = b.decisionRevisionLocked(owner)
	slices.SortFunc(out.Items, func(a, z Pending) int {
		if d := z.CreatedAt.Compare(a.CreatedAt); d != 0 {
			return d
		}
		return cmp.Compare(z.ID, a.ID)
	})
	return out
}

// decisionRevisionLocked is shared by snapshots and publication. Filtering
// precedes hashing; no source callback, global count or owner registry is used.
func (b *Broker) decisionRevisionLocked(owner string) string {
	versions := make(map[string]uint64)
	for id, r := range b.byID {
		if !r.observedOnce && owner != "" && r.Principal.BindingKey() == owner {
			versions[id] = r.version
		}
	}
	for id, e := range b.retained {
		if owner != "" && e.owner == owner {
			versions[id] = e.version
		}
	}
	for id, source := range b.decisionSources {
		if _, exists := versions[id]; !exists && owner != "" && source.owner == owner {
			versions[id] = source.version
		}
	}
	ids := make([]string, 0, len(versions))
	for id := range versions {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	mac := hmac.New(sha256.New, b.historySecret)
	fmt.Fprintf(mac, "%s\x00%s\x00", b.historyEpoch, owner)
	for _, id := range ids {
		fmt.Fprintf(mac, "%s\x00%d\x00", id, versions[id])
	}
	return "rev_" + hex.EncodeToString(mac.Sum(nil))
}

// setHistoryEnabled releases closed observer history without changing a gate,
// pending request or in-flight claim. The winning claim retains current-call
// evidence only until result delivery, including while history is disabled.
func (b *Broker) setHistoryEnabled(enabled bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.historyEnabled == enabled {
		return
	}
	b.historyEnabled = enabled
	b.historyEpoch, b.historySecret = "epoch_"+randomID(16), []byte(randomID(32))
	b.historyStartedAt = b.now().UTC()
	close(b.decisionWake)
	b.decisionWake = make(chan struct{})
	if !enabled {
		for sub := range b.decisionSubs {
			b.closeDecisionSubscriptionLocked(sub)
		}
		clear(b.decisionSources)
		for id, e := range b.retained {
			if !e.closedAt.IsZero() {
				b.removeRetainedLocked(id)
			}
		}
	}
}
