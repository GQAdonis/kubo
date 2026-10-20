//go:build !noagent

package server

import (
	"crypto/sha256"
	"slices"
	"time"
)

// decisionSnapshotFor reads authoritative idle UAR threads under their source
// lock, followed by broker.mu. No broker mutation acquires a source lock or
// calls into a session. Running threads are unavailable until they settle;
// reading their fields concurrently with the run slot would race.
func (s *Server) decisionSnapshotFor(owner string) decisionSnapshot {
	b := s.UARBackend()
	if b == nil {
		return s.gate.broker.snapshotFor(owner)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	live := b.decisionSourcesLocked(owner)
	return s.gate.broker.snapshotForSources(owner, live)
}

// decisionSourcesLocked derives at most one current projection per owned
// thread, 64 total. The broker receives only minimal version/cleanup markers;
// summary and continuation authority stay on the actual idle thread.
func (b *uarBackend) decisionSourcesLocked(owner string) []Pending {
	if b.srv == nil || b.srv.gate == nil || !b.srv.gate.broker.decisionObservationEnabled() {
		return nil
	}
	keys := make([]string, 0, len(b.threads))
	for key := range b.threads {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	live := make([]Pending, 0, min(len(keys), maxDecisionLiveSources))
	sources := make([]liveDecisionSource, 0, min(len(keys), maxDecisionLiveSources))
	for _, key := range keys {
		if len(sources) == maxDecisionLiveSources {
			break
		}
		th := b.threads[key]
		deadline := th.touched.Add(sessionIdleTTL)
		if th.running || !b.now().Before(deadline) {
			continue
		}
		p := th.shownPending()
		if p == nil || p.retained || p.decisionID == "" {
			continue
		}
		var expiry time.Time
		if t, err := time.Parse(time.RFC3339, p.ExpiresAt); err == nil {
			expiry = t.UTC()
			if expiry.Before(deadline) {
				deadline = expiry
			}
		}
		if !b.now().Before(deadline) {
			continue
		}
		// Store only a digest of visible facts in the marker index. A changed
		// advertised expiry/tool invalidates the snapshot; an extended idle
		// cleanup deadline alone does not invent an observation change.
		facts := sha256.Sum256([]byte(p.createdAt.UTC().Format(time.RFC3339Nano) + "\x00" + expiry.Format(time.RFC3339Nano) + "\x00" + p.activityID + "\x00" + p.Tool))
		sources = append(sources, liveDecisionSource{id: p.decisionID, owner: th.owner, thread: key, state: StatePending, deadline: deadline, factHash: facts})
		if th.owner != owner {
			continue
		}
		observation := Pending{
			ID: p.decisionID, Source: DecisionSourceUAR, SourceKind: SourceKindBackendApproval, State: StatePending,
			Principal: Principal{ID: th.principal, Kind: KindUI}, CreatedAt: p.createdAt,
			ActivityID: p.activityID, ExpiresAt: expiry,
			Summary: Summary{Tool: p.Tool, Class: "write", Text: "UAR backend approval", Targets: []Target{}},
		}
		live = append(live, observation)
	}
	b.srv.gate.broker.replaceDecisionSources(b.gen, sources)
	return live
}
