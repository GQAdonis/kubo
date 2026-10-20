//go:build !noagent

package server

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"
)

// Bounds of the in-memory A2A state. Nothing is written to disk.
const (
	// maxA2AContexts matches the agent loop's session cap: a context is one
	// conversation.
	maxA2AContexts = maxSessions
	// maxA2AContextsPerOwner is the share of the table one A2A principal may
	// hold, the same quota as agent sessions.
	maxA2AContextsPerOwner = maxSessionsPerPrincipal
	// maxA2ATasksPerContext bounds the finished tasks a context keeps for
	// GetTask; the oldest are dropped first.
	maxA2ATasksPerContext = 32
	// a2aContextIdleTTL ends a context with no open task that nobody touched.
	a2aContextIdleTTL = sessionIdleTTL
)

var errTooManyTasks = errors.New("too many open a2a contexts")

// a2aEvent is one thing that happened to a task, in order. Streams replay them.
type a2aEvent struct {
	status   *a2aStatusUpdate
	artifact *a2aArtifactUpdate
}

// a2aTask is one task: the unit of work for one message. All of its state is
// behind mu; the driver goroutine writes it and handlers read it.
type a2aTask struct {
	id, contextID, owner string
	created              time.Time
	// ctx ends when the task is cancelled; the work and the wait for approval
	// run under it.
	ctx    context.Context
	cancel context.CancelFunc
	now    func() time.Time

	// started closes once the work began, or failed before it did (preErr).
	started   chan struct{}
	startOnce sync.Once
	preErr    error

	mu        sync.Mutex
	state     string
	msg       *a2aMessage
	updated   time.Time
	artifacts []a2aArtifact
	meta      map[string]any
	events    []a2aEvent
	wake      chan struct{}

	// pending is set while the task is input-required: what waits for a person.
	pending *a2aPending
}

// a2aPending is the confirmation an input-required task waits on. The task
// never decides it: the peer's messages do not reach it.
type a2aPending struct {
	confirmationID string
	tool           string
	effectDigest   string
	destination    *Destination
	// text is true when the call came from the agent loop; otherwise it is a
	// data-part call, repeated with args after approval.
	text bool
	args json.RawMessage
	// interruptID is the loop's interrupt for a text task.
	interruptID string
}

func newA2ATask(owner, contextID string, now func() time.Time) *a2aTask {
	return newA2ATaskIn(context.Background(), owner, contextID, now)
}

// newA2ATaskIn makes a task whose context ends with parent, so that stopping the
// endpoint cancels every task it started.
func newA2ATaskIn(parent context.Context, owner, contextID string, now func() time.Time) *a2aTask {
	ctx, cancel := context.WithCancel(parent)
	t := &a2aTask{
		id:        "task-" + randomID(12),
		contextID: contextID,
		owner:     owner,
		ctx:       ctx,
		cancel:    cancel,
		now:       now,
		started:   make(chan struct{}),
		state:     a2aSubmitted,
		wake:      make(chan struct{}),
	}
	t.created = now()
	t.updated = t.created
	return t
}

// markStarted records that the work began (err nil) or failed before it did.
func (t *a2aTask) markStarted(err error) bool {
	first := false
	t.startOnce.Do(func() {
		first = true
		t.preErr = err
		close(t.started)
	})
	return first
}

// open reports whether the task still runs or waits for a person.
func (t *a2aTask) open() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.openLocked()
}

func (t *a2aTask) openLocked() bool {
	switch t.state {
	case a2aSubmitted, a2aWorking, a2aInputRequired:
		return true
	}
	return false
}

// signal wakes the readers of the event log. Callers hold t.mu.
func (t *a2aTask) signal() {
	close(t.wake)
	t.wake = make(chan struct{})
}

// setStatus moves the task to a state. A task that ended stays as it is. It
// reports whether the transition happened. pending, when not nil, is the
// confirmation of an input-required task; metadata is the task metadata for the
// new state and replaces the old.
func (t *a2aTask) setStatus(state, text string, pending *a2aPending, meta map[string]any) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if a2aTerminal(t.state) {
		return false
	}
	t.state = state
	t.msg = nil
	if text != "" {
		t.msg = agentMessage(t.contextID, t.id, text)
	}
	t.pending = pending
	t.meta = meta
	t.updated = t.now()
	t.events = append(t.events, a2aEvent{status: &a2aStatusUpdate{
		TaskID: t.id, ContextID: t.contextID, Status: t.statusLocked(), Metadata: meta,
	}})
	t.signal()
	return true
}

// addArtifact attaches a finished artifact. A task that ended gets none.
func (t *a2aTask) addArtifact(a a2aArtifact) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if a2aTerminal(t.state) {
		return
	}
	t.artifacts = append(t.artifacts, a)
	t.events = append(t.events, a2aEvent{artifact: &a2aArtifactUpdate{
		TaskID: t.id, ContextID: t.contextID, Artifact: a, LastChunk: true,
	}})
	t.signal()
}

func (t *a2aTask) statusLocked() a2aStatus {
	return a2aStatus{State: t.state, Message: t.msg, Timestamp: a2aTimestamp(t.updated)}
}

// view returns the task as it is now. message, when not empty, replaces the
// status message in the copy only: the answer to a follow-up message must not
// rewrite what the task says.
func (t *a2aTask) view(message string) *a2aTaskView {
	t.mu.Lock()
	defer t.mu.Unlock()
	v := &a2aTaskView{
		ID:        t.id,
		ContextID: t.contextID,
		Status:    t.statusLocked(),
		Artifacts: slices.Clone(t.artifacts),
		Metadata:  t.meta,
	}
	if message != "" {
		v.Status.Message = agentMessage(t.contextID, t.id, message)
	}
	return v
}

// snapshot is view plus the index of the first event after it, taken together
// so a stream neither repeats nor misses an update.
func (t *a2aTask) snapshot(message string) (*a2aTaskView, int) {
	t.mu.Lock()
	n := len(t.events)
	t.mu.Unlock()
	// Events only grow, and the view is at least as new as n; a repeated update
	// is harmless, a missed one is not.
	return t.view(message), n
}

// initialView is the task as it was created, before any event. A stream starts
// from it and replays every event, so the peer sees each step.
func (t *a2aTask) initialView() *a2aTaskView {
	return &a2aTaskView{ID: t.id, ContextID: t.contextID, Status: a2aStatus{State: a2aSubmitted, Timestamp: a2aTimestamp(t.created)}}
}

// waitSettled blocks until the task ended or waits for something the caller
// cannot give it, or ctx ends.
func (t *a2aTask) waitSettled(ctx context.Context) error {
	for {
		t.mu.Lock()
		done := t.state != a2aSubmitted && t.state != a2aWorking
		wake := t.wake
		t.mu.Unlock()
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wake:
		}
	}
}

// pendingCopy returns the confirmation the task waits on, or nil.
func (t *a2aTask) pendingCopy() *a2aPending {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pending == nil {
		return nil
	}
	cp := *t.pending
	return &cp
}

// eventsFrom returns the events at index i and after, the next index, the
// channel that closes when more arrive, and whether the task has ended (so no
// more will).
func (t *a2aTask) eventsFrom(i int) (evs []a2aEvent, next int, wake <-chan struct{}, ended bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.events[i:]), len(t.events), t.wake, a2aTerminal(t.state)
}

// a2aContext is one conversation and the tasks sent in it.
type a2aContext struct {
	id, owner string
	touched   time.Time
	taskIDs   []string
	// ranText is true once a text task of the context has started, which is
	// when the context's loop session exists.
	ranText bool
}

// a2aStore holds the contexts and tasks, and enforces who owns them. Every
// lookup takes the caller's principal ID: a task or a context of someone else
// is indistinguishable from one that does not exist.
type a2aStore struct {
	// parent is the context every task derives from; nil means Background.
	parent   context.Context
	mu       sync.Mutex
	now      func() time.Time
	contexts map[string]*a2aContext
	tasks    map[string]*a2aTask
}

func newA2AStore(now func() time.Time) *a2aStore {
	return &a2aStore{now: now, contexts: map[string]*a2aContext{}, tasks: map[string]*a2aTask{}}
}

// newTask creates a task for the owner. With contextID empty it also creates
// the context; otherwise the context must be the owner's, and must have no open
// task (a context has at most one, so at most one confirmation is open per
// session). fresh reports whether the context is new.
func (s *a2aStore) newTask(owner, contextID string) (t *a2aTask, fresh bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	var c *a2aContext
	if contextID == "" {
		if err := s.makeRoomLocked(owner); err != nil {
			return nil, false, err
		}
		c = &a2aContext{id: "ctx-" + randomID(12), owner: owner}
		s.contexts[c.id] = c
		fresh = true
	} else {
		c = s.contexts[contextID]
		if c == nil || c.owner != owner {
			return nil, false, errContextUnknown()
		}
		for _, id := range c.taskIDs {
			if s.tasks[id].open() {
				return nil, false, errContextBusy()
			}
		}
	}
	parent := s.parent
	if parent == nil {
		parent = context.Background()
	}
	t = newA2ATaskIn(parent, owner, c.id, s.now)
	s.tasks[t.id] = t
	c.taskIDs = append(c.taskIDs, t.id)
	c.touched = s.now()
	for len(c.taskIDs) > maxA2ATasksPerContext {
		delete(s.tasks, c.taskIDs[0])
		c.taskIDs = c.taskIDs[1:]
	}
	return t, fresh, nil
}

// task returns the owner's task, or nil.
func (s *a2aStore) task(owner, id string) *a2aTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tasks[id]
	if t == nil || t.owner != owner {
		return nil
	}
	s.contexts[t.contextID].touched = s.now()
	return t
}

// drop removes a task that failed before it began, and its context when the
// task created it and nothing else lives there.
func (s *a2aStore) drop(t *a2aTask, freshContext bool) {
	t.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tasks, t.id)
	c := s.contexts[t.contextID]
	if c == nil {
		return
	}
	c.taskIDs = slices.DeleteFunc(c.taskIDs, func(id string) bool { return id == t.id })
	if freshContext && len(c.taskIDs) == 0 {
		delete(s.contexts, c.id)
	}
}

func (s *a2aStore) idleLocked(c *a2aContext) bool {
	for _, id := range c.taskIDs {
		if s.tasks[id].open() {
			return false
		}
	}
	return true
}

// sweepLocked drops contexts nobody has touched for the idle TTL.
func (s *a2aStore) sweepLocked() {
	now := s.now()
	for id, c := range s.contexts {
		if s.idleLocked(c) && now.Sub(c.touched) > a2aContextIdleTTL {
			s.removeLocked(id)
		}
	}
}

// makeRoomLocked frees a slot for a new context of owner. At its quota, or
// when the table is full, it evicts only the owner's own oldest idle context;
// a context with an open task (working, or waiting on an approval) is never
// evicted, and another owner's context never is. Without a candidate the
// caller gets errTooManyTasks.
func (s *a2aStore) makeRoomLocked(owner string) error {
	own := 0
	for _, c := range s.contexts {
		if c.owner == owner {
			own++
		}
	}
	if own < maxA2AContextsPerOwner && len(s.contexts) < maxA2AContexts {
		return nil
	}
	if !s.evictOldestIdleLocked(owner) {
		return errTooManyTasks
	}
	return nil
}

// evictOldestIdleLocked drops the owner's least recently touched idle context.
func (s *a2aStore) evictOldestIdleLocked(owner string) bool {
	oldest := ""
	for id, c := range s.contexts {
		if c.owner != owner || !s.idleLocked(c) {
			continue
		}
		if oldest == "" || c.touched.Before(s.contexts[oldest].touched) {
			oldest = id
		}
	}
	if oldest == "" {
		return false
	}
	s.removeLocked(oldest)
	return true
}

func (s *a2aStore) removeLocked(contextID string) {
	for _, id := range s.contexts[contextID].taskIDs {
		delete(s.tasks, id)
	}
	delete(s.contexts, contextID)
}

// ranText reports whether the owner's context already ran a text task.
func (s *a2aStore) ranText(owner, contextID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.contexts[contextID]
	return c != nil && c.owner == owner && c.ranText
}

// markRanText records that a text task of the owner's context started.
func (s *a2aStore) markRanText(owner, contextID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.contexts[contextID]; c != nil && c.owner == owner {
		c.ranText = true
	}
}

// dropContext removes the owner's context and its tasks.
func (s *a2aStore) dropContext(owner, contextID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.contexts[contextID]; c != nil && c.owner == owner {
		s.removeLocked(contextID)
	}
}
