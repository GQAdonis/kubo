//go:build !noagent

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// L5 over AG-UI: a settings write that moves the backend while a chat waits on
// a confirmation makes the resume a 409 failed_precondition with the stable
// reason backend_changed, and the confirmed write does not run.
func TestAGUIResumeAfterBackendMoveIsBackendChanged(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc-1", 5), sayStep("must not run"))
	in := e.startWrite(aguiTokChat, "thread-moved")

	other := newScriptedModel(t, sayStep("the new backend"))
	snap, err := newSnapshot(AgentSettings{BaseURL: other.baseURL(), Model: "m", Timeout: 5 * time.Second})
	require.NoError(t, err)
	e.s.Agent().swapSnapshot(snap)

	r := e.resumeReq(aguiTokChat, "thread-moved", in["id"].(string), "resolved", map[string]any{"approved": true})
	require.Equal(t, http.StatusConflict, r.Status, r.Body)
	var doc struct {
		Error struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Body), &doc), r.Body)
	assert.Equal(t, CodeFailedPrecondition, doc.Error.Code)
	assert.Equal(t, ReasonBackendChanged, doc.Error.Details["reason"])
	assert.Zero(t, e.d.count("pin/add"))
	assert.Zero(t, other.hits())
}

// The A2A mapping uses the same shared code with its own reason.
func TestA2AMapsBackendChanged(t *testing.T) {
	re := rpcErrorOf(fmt.Errorf("resume: %w", ErrBackendChanged))
	require.NotNil(t, re)
	raw, err := json.Marshal(re)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "BACKEND_CHANGED")
	assert.Contains(t, string(raw), CodeFailedPrecondition)
}
