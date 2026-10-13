//go:build !noagent

package server

import (
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The agent card URL comes from the Host header, so a shared cache must not
// hand one host's card to a request for another (R11-L1).
func TestAgentCardIsPrivateAndVariesOnHost(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	resp, err := http.Get(e.base + RouteAgentCard)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	cc := resp.Header.Get("Cache-Control")
	assert.True(t, strings.HasPrefix(cc, "private"), "Cache-Control: %s", cc)
	assert.NotContains(t, cc, "public")
	assert.Contains(t, resp.Header.Values("Vary"), "Host")
}

// goroutinesIn counts goroutines whose stack mentions a function.
func goroutinesIn(fn string) int {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	count := 0
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, fn) {
			count++
		}
	}
	return count
}

// A task waiting for approval holds a goroutine in Broker.Await. Stopping the
// listener cancels and joins it, so nothing outlives Serve (R11 race fix).
func TestServeReturnsOnlyAfterTheTaskGoroutinesEnd(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	task := taskOf(t, e.send(a2aTokPeer, toolPart("pin_add", map[string]any{"path": "/ipfs/" + testCID(t, 88)})))
	require.Equal(t, a2aInputRequired, stateOf(task))
	require.Positive(t, goroutinesIn("(*a2aHandler).drive"), "the task is waiting in a goroutine")

	e.stopServe()

	assert.Zero(t, goroutinesIn("(*a2aHandler).drive"), "a task goroutine outlived Serve")
	assert.Zero(t, goroutinesIn("(*Broker).Await"), "a waiter outlived Serve")
}

func TestStoppedHandlerRefusesNewWork(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	e.s.runShutdownHooks()
	_, resp := e.call(a2aTokPeer, "SendMessage", map[string]any{"message": userMessage(toolPart("pin_add", map[string]any{"path": "/ipfs/" + testCID(t, 89)}))})
	assert.NotNil(t, resp["error"], "no task starts once the endpoint is stopping")
	assert.Empty(t, e.s.PendingConfirmations())
}
