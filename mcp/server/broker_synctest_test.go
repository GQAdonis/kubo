package server

import (
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Task 1.2: the broker under virtual time. synctest makes expiry and
// concurrent resolvers deterministic.

func newSynctestBroker() *Broker { return newBroker(time.Now, confirmTTL) }

func openPending(b *Broker, tool string) Pending {
	return b.create(newConfirmation{
		principal: Principal{ID: "p", Kind: KindUAR},
		sid:       "s",
		tool:      tool,
		argsHash:  "h",
		summary:   Summary{Tool: tool, Class: "destructive", Text: "Remove a pin"},
		mechanism: MechanismExternal,
	})
}

func TestBrokerExpiryIsDenial(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newSynctestBroker()
		p := openPending(b, "pin_rm")
		require.Equal(t, StatePending, p.State)

		time.Sleep(confirmTTL - time.Second)
		got, _ := b.get(p.ID)
		assert.Equal(t, StatePending, got.State, "still pending just before the deadline")

		time.Sleep(time.Second)
		got, _ = b.get(p.ID)
		assert.Equal(t, StateExpired, got.State)
		assert.Empty(t, b.Pending())
		assert.ErrorIs(t, b.Resolve(p.ID, ResolverLinuxPolkit, true), ErrConfirmationExpired, "a late approval is rejected")
		assert.Error(t, b.claim(p.ID, ""), "an expired confirmation never executes")
	})
}

func TestBrokerApprovedButUnclaimedExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newSynctestBroker()
		p := openPending(b, "pin_rm")
		require.NoError(t, b.Resolve(p.ID, ResolverLinuxPolkit, true))
		time.Sleep(confirmTTL)
		assert.Error(t, b.claim(p.ID, ""), "approval does not outlive the deadline")
	})
}

func TestBrokerDoubleApprovalRunsOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newSynctestBroker()
		p := openPending(b, "pin_rm")
		require.NoError(t, b.Resolve(p.ID, ResolverLinuxPolkit, true))
		assert.ErrorIs(t, b.Resolve(p.ID, ResolverMacOSLocalAuth, true), ErrConfirmationResolved)
		assert.ErrorIs(t, b.Resolve(p.ID, ResolverCLI, false), ErrConfirmationResolved, "a second resolver cannot flip the decision")

		require.NoError(t, b.claim(p.ID, ""))
		assert.Error(t, b.claim(p.ID, ""), "the approved call executes once")
		assert.ErrorIs(t, b.Resolve(p.ID, ResolverLinuxPolkit, true), ErrConfirmationResolved)
	})
}

func TestBrokerConcurrentResolversOneWins(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newSynctestBroker()
		p := openPending(b, "pin_rm")
		names := []string{ResolverLinuxPolkit, ResolverMacOSLocalAuth, ResolverLinuxPolkit, ResolverCLI}
		var wins atomic.Int32
		var wg sync.WaitGroup
		for i := range 16 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if b.Resolve(p.ID, names[i%len(names)], i%2 == 0) == nil {
					wins.Add(1)
				}
			}()
		}
		wg.Wait()
		assert.EqualValues(t, 1, wins.Load(), "exactly one resolver decides")

		got, _ := b.get(p.ID)
		if got.State != StateApproved {
			return
		}
		var claims atomic.Int32
		for range 16 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if b.claim(p.ID, "") == nil {
					claims.Add(1)
				}
			}()
		}
		wg.Wait()
		assert.EqualValues(t, 1, claims.Load(), "concurrent claims execute once")
	})
}

func TestBrokerNativePromptRefusesChatApproval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newSynctestBroker()
		p := b.create(newConfirmation{principal: Principal{ID: "ui", Kind: KindUI}, sid: "s", tool: "pin_rm", argsHash: "h", nativeReq: true, mechanism: MechanismExternal})
		assert.ErrorIs(t, b.Resolve(p.ID, ResolverAGUIInterrupt, true), ErrNativePromptRequired)
		require.NoError(t, b.Resolve(p.ID, ResolverMacOSLocalAuth, true))
	})
}
