package server

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func awaitBroker(now func() time.Time) (*Broker, Pending) {
	b := newBroker(now, 2*time.Minute)
	p := b.create(newConfirmation{principal: Principal{ID: "p", Kind: KindUI}, sid: "s", tool: "t", argsHash: "h", mechanism: MechanismExternal})
	return b, p
}

func TestAwaitReturnsWhenADecisionIsRecorded(t *testing.T) {
	for _, approve := range []bool{true, false} {
		synctest.Test(t, func(t *testing.T) {
			b, p := awaitBroker(time.Now)
			done := make(chan string, 1)
			go func() {
				state, err := b.Await(t.Context(), p.ID)
				assert.NoError(t, err)
				done <- state
			}()
			synctest.Wait()
			select {
			case s := <-done:
				t.Fatalf("Await returned %q while the confirmation was pending", s)
			default:
			}
			require.NoError(t, b.Resolve(p.ID, ResolverLinuxPolkit, approve))
			synctest.Wait()
			want := StateDenied
			if approve {
				want = StateApproved
			}
			assert.Equal(t, want, <-done)
		})
	}
}

func TestAwaitReturnsAtExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b, p := awaitBroker(time.Now)
		start := time.Now()
		state, err := b.Await(t.Context(), p.ID)
		require.NoError(t, err)
		assert.Equal(t, StateExpired, state)
		assert.Equal(t, 2*time.Minute, time.Since(start), "no earlier and no later than the deadline")
	})
}

func TestAwaitReportsAnExecutedCall(t *testing.T) {
	b, p := awaitBroker(time.Now)
	require.NoError(t, b.Resolve(p.ID, ResolverLinuxPolkit, true))
	state, err := b.Await(t.Context(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, StateApproved, state, "already decided: returns at once")
	require.NoError(t, b.claim(p.ID, ""))
	state, err = b.Await(t.Context(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, StateExecuted, state)
}

func TestAwaitStopsWithItsContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b, p := awaitBroker(time.Now)
		ctx, cancel := context.WithCancel(t.Context())
		errc := make(chan error, 1)
		go func() {
			_, err := b.Await(ctx, p.ID)
			errc <- err
		}()
		synctest.Wait()
		cancel()
		synctest.Wait()
		require.ErrorIs(t, <-errc, context.Canceled)
		got, _ := b.get(p.ID)
		assert.Equal(t, StatePending, got.State, "waiting decides nothing")
	})
}

func TestAwaitUnknownConfirmation(t *testing.T) {
	b := newBroker(time.Now, time.Minute)
	_, err := b.Await(t.Context(), "cf_unknown")
	require.ErrorIs(t, err, ErrUnknownConfirmation)
}

func TestAwaitNoticesAFakeClockThatMoved(t *testing.T) {
	clock := newFakeClock()
	b, p := awaitBroker(clock.Now)
	b.recheck = 5 * time.Millisecond
	done := make(chan string, 1)
	go func() {
		state, _ := b.Await(t.Context(), p.ID)
		done <- state
	}()
	time.Sleep(20 * time.Millisecond)
	clock.Advance(3 * time.Minute)
	select {
	case s := <-done:
		assert.Equal(t, StateExpired, s)
	case <-time.After(5 * time.Second):
		t.Fatal("Await did not notice the expiry")
	}
}
