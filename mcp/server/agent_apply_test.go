//go:build !noagent

package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ApplyAgentSettings on a server that started with no backend installs the
// loop: a mounted /agui stops answering 503 backend_not_configured without a
// restart.
func TestApplyAgentSettingsInstallsBackendWhenAbsent(t *testing.T) {
	model := newScriptedModel(t, sayStep("applied"))
	d := catFake(t, nil)
	s := mustNew(t, d, Options{Tools: []string{"node", "content", "pin", "destructive"}})
	require.Nil(t, s.Agent(), "no backend at startup: no agent loop")

	require.NoError(t, s.ApplyAgentSettings(&AgentSettings{BaseURL: model.baseURL(), Model: "m", Timeout: 5 * time.Second}))
	a := s.Agent()
	require.NotNil(t, a, "the apply installed the agent loop")
	assert.Equal(t, model.host(), a.BackendHost())

	res, err := a.Run(t.Context(), uiID(nil), RunInput{Message: "hi"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "applied", res.Final)
	assert.Equal(t, 1, model.hits())
}

// ApplyAgentSettings on a running agent swaps the backend in place: the Agent
// (and with it sessions and rate buckets) survives, and the next run talks to
// the new backend.
func TestApplyAgentSettingsSwapsRunningBackend(t *testing.T) {
	e := newAgentEnv(t, agentEnvOpts{}, sayStep("old"))
	res, _ := e.run(uiID(nil), "hi")
	require.Equal(t, "old", res.Final)

	newModel := newScriptedModel(t, sayStep("new"))
	before := e.a
	require.NoError(t, e.s.ApplyAgentSettings(&AgentSettings{BaseURL: newModel.baseURL(), Model: "m2", Timeout: 5 * time.Second}))
	assert.Same(t, before, e.s.Agent(), "the loop is swapped in place, not rebuilt")

	res2, _ := e.run(uiID(nil), "again")
	assert.Equal(t, "new", res2.Final)
	assert.Equal(t, 1, newModel.hits())
	assert.Equal(t, 1, e.model.hits(), "the old backend got no further calls")
}

// A refused apply changes nothing: the running backend and the hook stay
// untouched.
func TestApplyAgentSettingsValidationFailureKeepsBackend(t *testing.T) {
	e := newAgentEnv(t, agentEnvOpts{}, sayStep("old"))
	var calls int
	e.s.OnAgentChange(func() { calls++ })

	err := e.s.ApplyAgentSettings(&AgentSettings{BaseURL: "://not a url", Model: "m"})
	require.Error(t, err)
	err = e.s.ApplyAgentSettings(&AgentSettings{Model: "m"})
	require.Error(t, err, "a backend without a base URL is refused")

	res, _ := e.run(uiID(nil), "hi")
	assert.Equal(t, "old", res.Final, "a refused apply leaves the running backend unchanged")
	assert.Zero(t, calls, "a refused apply does not notify")
}

func TestApplyAgentSettingsNilRefused(t *testing.T) {
	e := newAgentEnv(t, agentEnvOpts{})
	require.Error(t, e.s.ApplyAgentSettings(nil))
	require.NotNil(t, e.s.Agent(), "the running backend is unchanged")
}

func TestApplyAgentSettingsNotifiesHook(t *testing.T) {
	var calls int
	s := mustNew(t, newFakeDaemon(t), Options{})
	s.OnAgentChange(func() { calls++ })
	m1 := newScriptedModel(t)
	m2 := newScriptedModel(t)

	require.NoError(t, s.ApplyAgentSettings(&AgentSettings{BaseURL: m1.baseURL(), Model: "m"}))
	require.NoError(t, s.ApplyAgentSettings(&AgentSettings{BaseURL: m2.baseURL(), Model: "m"}))
	assert.Equal(t, 2, calls, "one notification per successful apply")

	s.OnAgentChange(nil)
	require.NoError(t, s.ApplyAgentSettings(&AgentSettings{BaseURL: m1.baseURL(), Model: "m"}))
	assert.Equal(t, 2, calls, "a cleared hook is not called")
}
