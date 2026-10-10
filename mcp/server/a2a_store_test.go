//go:build !noagent

package server

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestA2AStoreOwnership(t *testing.T) {
	clock := newFakeClock()
	s := newA2AStore(clock.Now)
	a, fresh, err := s.newTask("alice", "")
	require.NoError(t, err)
	require.True(t, fresh)
	assert.Same(t, a, s.task("alice", a.id))
	assert.Nil(t, s.task("bob", a.id), "another owner's task does not exist")
	assert.Nil(t, s.task("alice", "task-none"))

	a.setStatus(a2aCompleted, "", nil, nil)
	// Bob cannot use Alice's context, and learns nothing from the answer.
	_, _, errBob := s.newTask("bob", a.contextID)
	_, _, errNone := s.newTask("bob", "ctx-none")
	assert.Equal(t, errNone, errBob)
	// Alice can continue it.
	b, fresh, err := s.newTask("alice", a.contextID)
	require.NoError(t, err)
	assert.False(t, fresh)
	assert.Equal(t, a.contextID, b.contextID)
}

func TestA2AStoreOneOpenTaskPerContext(t *testing.T) {
	s := newA2AStore(newFakeClock().Now)
	a, _, _ := s.newTask("alice", "")
	_, _, err := s.newTask("alice", a.contextID)
	assert.Equal(t, errContextBusy(), err)
	a.setStatus(a2aInputRequired, "wait", &a2aPending{confirmationID: "cf"}, nil)
	_, _, err = s.newTask("alice", a.contextID)
	assert.Equal(t, errContextBusy(), err, "input-required is still open")
	a.setStatus(a2aRejected, "no", nil, nil)
	_, _, err = s.newTask("alice", a.contextID)
	assert.NoError(t, err)
}

func TestA2AStoreBounds(t *testing.T) {
	clock := newFakeClock()
	s := newA2AStore(clock.Now)
	var open []*a2aTask
	for i := 0; i < maxA2AContexts; i++ {
		tk, _, err := s.newTask("alice", "")
		require.NoError(t, err)
		open = append(open, tk)
	}
	_, _, err := s.newTask("alice", "")
	assert.ErrorIs(t, err, errTooManyTasks, "every context has an open task")
	assert.Equal(t, rpcInternalError, rpcErrorOf(err).Code)

	clock.Advance(time.Second)
	open[3].setStatus(a2aCompleted, "", nil, nil)
	_, _, err = s.newTask("alice", "")
	assert.NoError(t, err, "an idle context is evicted to make room")
	assert.Nil(t, s.task("alice", open[3].id))

	// A finished context nobody touched expires.
	clock.Advance(a2aContextIdleTTL + time.Second)
	for _, tk := range open {
		tk.setStatus(a2aCompleted, "", nil, nil)
	}
	_, _, err = s.newTask("bob", "")
	require.NoError(t, err)
	assert.Nil(t, s.task("alice", open[5].id))
}

func TestA2AStoreKeepsBoundedHistoryPerContext(t *testing.T) {
	s := newA2AStore(newFakeClock().Now)
	first, _, _ := s.newTask("alice", "")
	ctx := first.contextID
	first.setStatus(a2aCompleted, "", nil, nil)
	for i := 0; i < maxA2ATasksPerContext+3; i++ {
		tk, _, err := s.newTask("alice", ctx)
		require.NoError(t, err)
		tk.setStatus(a2aCompleted, "", nil, nil)
	}
	assert.Nil(t, s.task("alice", first.id), "the oldest finished task was dropped")
}

func TestA2ATaskNeverLeavesAFinalState(t *testing.T) {
	tk := newA2ATask("alice", "ctx", newFakeClock().Now)
	assert.True(t, tk.setStatus(a2aCanceled, "x", nil, nil))
	assert.False(t, tk.setStatus(a2aCompleted, "", nil, nil))
	tk.addArtifact(a2aArtifact{ArtifactID: "a", Parts: []a2aPart{textPart("late")}})
	v := tk.view("")
	assert.Equal(t, a2aCanceled, v.Status.State)
	assert.Empty(t, v.Artifacts)
}

func TestA2ADropRemovesAFailedStart(t *testing.T) {
	s := newA2AStore(newFakeClock().Now)
	tk, fresh, _ := s.newTask("alice", "")
	s.drop(tk, fresh)
	assert.Nil(t, s.task("alice", tk.id))
	_, _, err := s.newTask("alice", tk.contextID)
	assert.Equal(t, errContextUnknown(), err, "the context created for the failed task is gone too")
}

func TestA2AErrorMapping(t *testing.T) {
	cases := map[string]struct {
		err  error
		code int
		tool string
		retr bool
	}{
		"session busy":      {ErrSessionBusy, rpcInternalError, CodeSessionBusy, true},
		"interrupt open":    {ErrInterruptOpen, rpcInternalError, CodeFailedPrecondition, false},
		"session full":      {ErrSessionFull, rpcInternalError, CodeFailedPrecondition, false},
		"too many sessions": {ErrTooManySessions, rpcInternalError, CodeResourceExhausted, true},
		"too many tasks":    {errTooManyTasks, rpcInternalError, CodeResourceExhausted, true},
		"message too large": {ErrMessageTooLarge, rpcInvalidParams, CodeSizeLimitExceeded, false},
		"session not found": {ErrSessionNotFound, a2aTaskNotFound, CodeNotFound, false},
		"anything else":     {assert.AnError, rpcInternalError, CodeInternal, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			re := rpcErrorOf(c.err)
			assert.Equal(t, c.code, re.Code)
			detail := re.Data[1].(map[string]any)["error"].(map[string]any)
			assert.Equal(t, c.tool, detail["code"])
			assert.Equal(t, c.retr, detail["retryable"])
			assert.Equal(t, "type.googleapis.com/google.rpc.ErrorInfo", re.Data[0].(map[string]any)["@type"])
			if name == "anything else" {
				assert.NotContains(t, re.Message, "assert.AnError")
			}
		})
	}
}

func TestA2AClassify(t *testing.T) {
	str := func(s string) *string { return &s }
	call, err := classify([]a2aPart{{Data: json.RawMessage(`{"tool":"pin_ls","arguments":{"confirm_token":"x","type":"recursive"}}`), MediaType: "application/json"}})
	require.Nil(t, err)
	assert.True(t, call.call)
	assert.NotContains(t, string(call.args), "confirm_token")
	_, err = classify([]a2aPart{{Data: json.RawMessage(`{"tool":"x","arguments":[]}`), MediaType: "application/json"}})
	assert.NotNil(t, err)
	_, err = classify([]a2aPart{{Data: json.RawMessage(`{"tool":"x"}`), MediaType: "text/plain"}})
	assert.Equal(t, a2aContentTypeNotSupport, err.Code)
	w, err := classify([]a2aPart{{Text: str("a")}, {Text: str("b")}})
	require.Nil(t, err)
	assert.Equal(t, "a\nb", w.text)
	_, err = classify([]a2aPart{{Text: str("  ")}})
	assert.NotNil(t, err)
	_, err = classify([]a2aPart{{}})
	assert.NotNil(t, err)
}

// TestA2ASkillsComeFromTheContract checks that every skill of the card is in the
// contract's skills file and that the file names no group the catalog lacks.
func TestA2ASkillsComeFromTheContract(t *testing.T) {
	sources, err := loadSkillSources()
	require.NoError(t, err)
	cat, err := contract.Load()
	require.NoError(t, err)
	for _, src := range sources {
		for _, g := range src.Groups {
			_, ok := cat.Group(g)
			assert.True(t, ok, "skill %s names unknown group %s", src.ID, g)
		}
	}
	raw, err := fs.ReadFile(contract.FS(), "agentic/a2a-skills.json")
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(raw), "ipfs-pinning"))
}
