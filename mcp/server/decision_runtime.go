//go:build !noagent

package server

import (
	"context"
	"sync"
	"time"
)

// decisionStreamRuntime has one scheduler per Server, regardless of stream
// count. Construction starts no goroutine. Serve starts it and cancellation
// closes streams and joins it; disabled observation sleeps without a timer.
type decisionStreamRuntime struct {
	srv     *Server
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	started bool
	done    chan struct{}
}

func (s *Server) decisionRuntime() *decisionStreamRuntime {
	s.agentMu.Lock()
	defer s.agentMu.Unlock()
	if runtime, _ := s.decisionRuntimeExt.(*decisionStreamRuntime); runtime != nil {
		return runtime
	}
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &decisionStreamRuntime{srv: s, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	s.decisionRuntimeExt = runtime
	s.onShutdown(runtime.shutdown)
	return runtime
}

func (r *decisionStreamRuntime) start(parent context.Context) func() {
	r.mu.Lock()
	if r.started || r.ctx.Err() != nil {
		r.mu.Unlock()
		return func() {}
	}
	r.started = true
	stopParent := context.AfterFunc(parent, r.cancel)
	go func() {
		defer close(r.done)
		defer r.srv.gate.broker.invalidateDecisionSubscriptions("", "")
		r.schedule()
	}()
	r.mu.Unlock()
	return func() { stopParent(); r.shutdown() }
}

func (r *decisionStreamRuntime) shutdown() {
	r.cancel()
	r.srv.gate.broker.invalidateDecisionSubscriptions("", "")
	r.mu.Lock()
	started := r.started
	r.mu.Unlock()
	if started {
		<-r.done
	}
}

func (r *decisionStreamRuntime) schedule() {
	b := r.srv.gate.broker
	for r.ctx.Err() == nil {
		b.sweepDecisionObservations()
		deadline, wake := b.decisionObservationSchedule()
		var timer *time.Timer
		var tick <-chan time.Time
		if !deadline.IsZero() {
			// The one-second ceiling also observes injected/wall-clock jumps.
			delay := max(time.Millisecond, min(time.Second, deadline.Sub(b.decisionObservationNow())))
			timer = time.NewTimer(delay)
			tick = timer.C
		}
		select {
		case <-r.ctx.Done():
		case <-wake:
		case <-tick:
		}
		if timer != nil {
			timer.Stop()
		}
	}
}
