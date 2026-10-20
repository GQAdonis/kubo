//go:build noagent

package server

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoAgentBuildHasNoDecisionReadRoutesOrObserverHistory(t *testing.T) {
	e := newHTTPEnv(t, nil)
	for _, path := range []string{RouteDecisions, RouteDecisions + "/events", RouteDecisions + "/cf_fixture_000000000001"} {
		r := e.do(t, rawReq{method: http.MethodGet, path: path, token: tokChat})
		assert.Equal(t, http.StatusNotFound, r.StatusCode)
	}
	assert.Nil(t, e.s.decisionRuntimeExt)
	assert.Empty(t, e.hs.serveHooks)
	assert.False(t, e.s.gate.broker.historyEnabled)
	assert.Empty(t, e.s.gate.broker.retained)
}
