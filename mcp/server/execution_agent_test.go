//go:build !noagent

package server

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionChatResumeRecordsTheActualNodeOutcome(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc-1", 5), sayStep("Pinned."))
	in := e.startWrite(aguiTokChat, "execution-chat")
	meta := in["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)
	r := e.resumeReq(aguiTokChat, "execution-chat", in["id"].(string), ResumeResolved, map[string]any{"approved": true})
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	execution := executionOf(t, e.s, meta["confirmation_id"].(string))
	assert.Equal(t, ExecutionSucceeded, execution.State)
	assert.Equal(t, "pin_add", execution.Receipt.Tool)
	assert.Equal(t, meta["effect_digest"], execution.Receipt.EffectDigest)
	assert.Equal(t, []ReceiptReference{{Kind: "cid", Value: testCID(t, 5)}}, execution.Receipt.ResultRefs)
	assert.Equal(t, 1, e.d.count("pin/add"))
}

func TestExecutionA2ADataCallUsesTheSameClaimAndReceipt(t *testing.T) {
	r := newFakeResolver(ResolverLinuxPolkit)
	s, pins := callEnv(t, r)
	s.gate.broker.setHistoryEnabled(true)
	id := peerID([]string{"pin"}, []string{"pin"}, "local")
	a := args(t, map[string]any{"path": "/ipfs/" + testCID(t, 1)})
	first, err := s.CallTool(t.Context(), id, "execution-a2a", "pin_rm", a)
	require.NoError(t, err)
	conf := confirmationOf(t, first, CodeConfirmRequired).Confirmation
	_, resolve := r.last(t)
	require.NoError(t, resolve(true))
	res, err := s.CallTool(t.Context(), id, "execution-a2a", "pin_rm", a)
	require.NoError(t, err)
	require.False(t, res.IsError)
	assert.Equal(t, conf.ConfirmationID, currentOutcomeOf(t, res).ConfirmationID)
	assert.False(t, pins.has(testCID(t, 1)))
	execution := executionOf(t, s, conf.ConfirmationID)
	assert.Equal(t, ExecutionSucceeded, execution.State)
	assert.Equal(t, "node_handler", execution.Receipt.Provenance)
	assert.Equal(t, conf.EffectDigest, execution.Receipt.EffectDigest)
}

func TestExecutionUARTextIsNotANodeReceipt(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-tool-approval-interrupt.sse", "run-resumed-executed.sse"), uarProxyOpts{})
	in := finishedInterrupt(t, e.say("execution-uar", "r1", "pin").Events)
	r := e.answer("execution-uar", "r2", in["id"].(string), true)
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	meta := in["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)
	p, _, ok := e.s.gate.broker.observed(meta["confirmation_id"].(string))
	require.True(t, ok)
	assert.Equal(t, SourceKindBackendApproval, p.SourceKind)
	assert.Equal(t, StateExecuted, p.State)
	assert.Nil(t, p.Execution, "backend text and RUN_FINISHED do not create node execution evidence")
	assert.Empty(t, e.s.PendingConfirmations())
}
