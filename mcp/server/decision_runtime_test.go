//go:build !noagent

package server

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecisionSchedulerExpiresAndRemovesWithoutToolOrReadRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
		runtime := e.s.decisionRuntime()
		assert.False(t, runtime.started, "construction allocates no worker")
		assert.Len(t, e.h.serveHooks, 1)
		stop := runtime.start(t.Context())
		defer stop()
		again := runtime.start(t.Context())
		defer again()
		b := e.s.gate.broker
		b.mu.Lock()
		b.ttl = time.Second
		b.mu.Unlock()
		owner := Principal{ID: "chat", Kind: KindUI}
		sub, err := b.subscribeDecisions(owner)
		require.NoError(t, err)
		defer sub.close()
		p := historyPending(b, owner)
		assert.Equal(t, StatePending, nextDecisionChange(t, sub).State)
		synctest.Wait()
		e.clock.Advance(time.Second)
		time.Sleep(time.Second)
		synctest.Wait()
		expired := nextDecisionChange(t, sub)
		assert.Equal(t, p.ID, expired.ID)
		assert.Equal(t, StateExpired, expired.State)
		e.clock.Advance(decisionHistoryTTL + time.Second)
		time.Sleep(time.Second)
		synctest.Wait()
		removed := nextDecisionChange(t, sub)
		assert.Equal(t, p.ID, removed.ID)
		assert.Equal(t, StateExpired, removed.State, "removal invalidates the last known state")
		b.mu.Lock()
		assert.Empty(t, b.retained)
		b.mu.Unlock()
		stop()
		select {
		case <-runtime.done:
		default:
			t.Fatal("scheduler still running")
		}
	})
}

func TestDecisionRuntimeAbsentWithoutHTTPAndDisabledHTTP(t *testing.T) {
	s := mustNew(t, catFake(t, nil), Options{Tools: []string{"node"}})
	assert.Nil(t, s.decisionRuntimeExt, "stdio does not construct the observer runtime")
	assert.False(t, s.gate.broker.historyEnabled)
	disabled := false
	e := newDecisionReadEnv(t, DecisionsConfig{Enabled: &disabled}, testVerifier(t))
	assert.Nil(t, e.s.decisionRuntimeExt)
	assert.Empty(t, e.h.serveHooks)
	assert.Empty(t, e.s.gate.broker.decisionSubs)
	assert.Empty(t, e.s.gate.broker.decisionSources)
}

func TestDecisionSchedulerExpiresUARVisibilityWithoutBackendResume(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
		runtime := e.s.decisionRuntime()
		stop := runtime.start(t.Context())
		defer stop()
		b := e.s.gate.broker
		owner := Principal{ID: "chat", Kind: KindUI}
		sub, err := b.subscribeDecisions(owner)
		require.NoError(t, err)
		defer sub.close()
		b.setDecisionSourceGeneration("fixture-generation")
		b.replaceDecisionSources("fixture-generation", []liveDecisionSource{{
			id: "cf_uar_visibility_00000001", owner: owner.BindingKey(), thread: "owned-thread", state: StatePending, deadline: e.clock.Now().Add(time.Second),
		}})
		created := nextDecisionChange(t, sub)
		assert.Equal(t, StatePending, created.State)
		synctest.Wait()
		e.clock.Advance(time.Second)
		time.Sleep(time.Second)
		synctest.Wait()
		removed := nextDecisionChange(t, sub)
		assert.Equal(t, created.ID, removed.ID)
		assert.Equal(t, StatePending, removed.State, "projection removal does not deny or consume backend approval")
		b.mu.Lock()
		assert.Empty(t, b.decisionSources)
		assert.Empty(t, b.retained)
		b.mu.Unlock()
	})
}
