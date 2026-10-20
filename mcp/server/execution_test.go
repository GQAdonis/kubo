package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executionOf(t *testing.T, s *Server, id string) *ExecutionRecord {
	t.Helper()
	p, _, ok := s.gate.broker.observed(id)
	require.True(t, ok)
	require.NotNil(t, p.Execution)
	validateConfirmationDef(t, "ExecutionRecord", p.Execution)
	return p.Execution
}

func currentOutcomeOf(t *testing.T, res *mcp.CallToolResult) ToolOutcomeMetadata {
	t.Helper()
	value, ok := res.Meta["ipfs-mcp"]
	require.True(t, ok, "winning current call includes safe outcome metadata")
	validateConfirmationDef(t, "ToolOutcomeMetadata", value)
	validateConfirmationDef(t, "ToolOutcomeResultMeta", res.Meta)
	var outcome ToolOutcomeMetadata
	require.NoError(t, json.Unmarshal(asJSON(t, value), &outcome))
	return outcome
}

func TestExecutionReceiptRequiresActualSuccessAndCannotReplay(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, nil, nil)
	conf := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired).Confirmation
	p, resolve := e.resolver.last(t)
	snap, _ := e.s.gate.broker.get(p.ID)
	assert.Nil(t, snap.Execution, "requesting approval is not execution")
	require.NoError(t, resolve(true))
	snap, _ = e.s.gate.broker.get(p.ID)
	assert.Nil(t, snap.Execution, "approval alone is not a successful operation")
	res := e.rm(t, cs, map[string]any{"confirm_token": conf.ConfirmToken})
	require.False(t, res.IsError)
	current := currentOutcomeOf(t, res)
	assert.Equal(t, p.ID, current.ConfirmationID)
	assert.Equal(t, conf.EffectDigest, current.EffectDigest)
	assert.Equal(t, ExecutionSucceeded, current.Execution.State)
	assert.Equal(t, "pin_rm", res.Meta[toolNameMetaKey])
	execution := executionOf(t, e.s, p.ID)
	require.Equal(t, ExecutionSucceeded, execution.State)
	receipt := execution.Receipt
	require.NotNil(t, receipt)
	assert.Equal(t, p.ID, receipt.DecisionID)
	assert.Equal(t, "pin_rm", receipt.Tool)
	assert.Equal(t, conf.EffectDigest, receipt.EffectDigest)
	assert.Equal(t, "node_handler", receipt.Provenance)
	assert.Equal(t, "operation_completed", receipt.Effect)
	assert.Equal(t, []ReceiptReference{{Kind: "cid", Value: e.target}}, receipt.ResultRefs)
	assert.Equal(t, *execution.CompletedAt, receipt.CompletedAt)
	assert.False(t, execution.CompletedAt.Before(*execution.StartedAt))

	// Detached observations cannot corrupt the retained successful fact.
	receipt.ResultRefs[0].Value = "tampered view"
	assert.Equal(t, e.target, executionOf(t, e.s, p.ID).Receipt.ResultRefs[0].Value)
	replayed := e.rm(t, cs, map[string]any{"confirm_token": conf.ConfirmToken})
	confirmationOf(t, replayed, CodeConfirmInvalid)
	assert.NotContains(t, replayed.Meta, "ipfs-mcp", "losing calls cannot observe a stale winning snapshot")
	assert.Equal(t, 1, e.d.count("pin/rm"))
}

func TestExecutionElicitationAndNativeGateShareOutcomeRecording(t *testing.T) {
	t.Run("elicitation", func(t *testing.T) {
		e := newGateEnv(t)
		ids := make(chan string, 1)
		cs := e.h.connect(t, elicitOptions(func(*mcp.ElicitRequest) *mcp.ElicitResult {
			pending := e.s.PendingConfirmations()
			if len(pending) == 1 {
				ids <- pending[0].ID
			}
			return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
		}), nil)
		require.False(t, e.rm(t, cs, nil).IsError)
		var id string
		select {
		case id = <-ids:
		default:
			t.Fatal("no elicitation decision was prepared")
		}
		execution := executionOf(t, e.s, id)
		assert.Equal(t, ExecutionSucceeded, execution.State)
		p, _, _ := e.s.gate.broker.observed(id)
		assert.Equal(t, ResolverElicitation, p.ResolvedBy)
	})
	t.Run("native required", func(t *testing.T) {
		native := newFakeResolver(ResolverMacOSLocalAuth)
		e := newGateEnv(t, native)
		cs := e.h.connect(t, nil, &Principal{ID: "chat", Kind: KindUI})
		conf := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired).Confirmation
		require.True(t, conf.RequiresNativePrompt)
		_, resolve := native.last(t)
		require.NoError(t, resolve(true))
		require.False(t, e.rm(t, cs, nil).IsError)
		assert.Equal(t, ExecutionSucceeded, executionOf(t, e.s, conf.ConfirmationID).State)
		p, _, _ := e.s.gate.broker.observed(conf.ConfirmationID)
		assert.Equal(t, ResolverMacOSLocalAuth, p.ResolvedBy)
	})
}

func TestExecutionDeniedOrExpiredHasNoInvocationRecord(t *testing.T) {
	for _, expired := range []bool{false, true} {
		e := newGateEnv(t)
		cs := e.h.connect(t, nil, nil)
		conf := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired).Confirmation
		_, resolve := e.resolver.last(t)
		code := CodeConfirmDeclined
		if expired {
			e.clock.Advance(confirmTTL + time.Second)
			code = CodeConfirmExpired
		} else {
			require.NoError(t, resolve(false))
		}
		confirmationOf(t, e.rm(t, cs, map[string]any{"confirm_token": conf.ConfirmToken}), code)
		p, _ := e.s.gate.broker.get(conf.ConfirmationID)
		assert.Nil(t, p.Execution)
		assert.Len(t, e.s.gate.broker.retained, 1, "resolved observation is retained without invocation evidence")
		e.assertNothingRan(t)
	}
}

func TestExecutionKnownFailureAndAmbiguityNeverGetReceipts(t *testing.T) {
	for name, fixture := range map[string]struct {
		err   error
		state string
	}{
		"daemon rejection":   {&RPCError{Command: "pin/rm", Message: "private upstream body"}, ExecutionFailed},
		"cancelled":          {context.Canceled, ExecutionUnknown},
		"deadline":           {context.DeadlineExceeded, ExecutionUnknown},
		"lost daemon":        {ErrDaemonUnreachable, ExecutionUnknown},
		"truncated response": {io.ErrUnexpectedEOF, ExecutionUnknown},
	} {
		t.Run(name, func(t *testing.T) {
			e := newGateEnv(t)
			e.d.callErr = map[string]error{"pin/rm": fixture.err}
			cs := e.h.connect(t, nil, nil)
			conf := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired).Confirmation
			p, resolve := e.resolver.last(t)
			require.NoError(t, resolve(true))
			res := e.rm(t, cs, map[string]any{"confirm_token": conf.ConfirmToken})
			require.True(t, res.IsError)
			assert.Equal(t, fixture.state, currentOutcomeOf(t, res).Execution.State)
			execution := executionOf(t, e.s, p.ID)
			assert.Equal(t, fixture.state, execution.State)
			assert.Nil(t, execution.Receipt)
			require.NotNil(t, execution.Error)
			assert.NotContains(t, execution.Error.Message, "private upstream body")
			assert.NotContains(t, execution.Error.Message, "Nothing changed")
			confirmationOf(t, e.rm(t, cs, map[string]any{"confirm_token": conf.ConfirmToken}), CodeConfirmInvalid)
			assert.Equal(t, 1, e.d.count("pin/rm"), "unknown outcomes are not retried")
		})
	}
}

func TestExecutionReadBackFailureAfterMutationIsUnknown(t *testing.T) {
	d := newContentFake(t)
	m := installMFS(d)
	d.handle["files/stat"] = func([]string, map[string]any) (any, error) {
		return nil, &RPCError{Command: "files/stat", Message: "read-back unavailable"}
	}
	r := newFakeResolver(ResolverLinuxPolkit)
	s := mustNew(t, d, Options{Tools: []string{"mfs"}, Resolvers: []Resolver{r}})
	s.gate.broker.setHistoryEnabled(true)
	cs := connect(t, s)
	a := map[string]any{"path": "/partial.txt", "content": "fixture bytes"}
	conf := confirmationOf(t, toolCall(t, cs, "mfs_write", a), CodeConfirmRequired).Confirmation
	_, resolve := r.last(t)
	require.NoError(t, resolve(true))
	a["confirm_token"] = conf.ConfirmToken
	require.True(t, toolCall(t, cs, "mfs_write", a).IsError)
	assert.True(t, m.has("/partial.txt"), "the write happened before its read-back failed")
	execution := executionOf(t, s, conf.ConfirmationID)
	assert.Equal(t, ExecutionUnknown, execution.State)
	assert.Nil(t, execution.Receipt)
}

func directExecutionHandler(t *testing.T, e *gateEnv, impl func(context.Context, json.RawMessage) (any, error)) (context.Context, mcp.ToolHandler, *mcp.CallToolRequest, Pending) {
	t.Helper()
	rt := e.s.registry["pin_rm"]
	handler := e.s.handler(rt.tool, rt.sc, toolImpl{Name: "pin_rm", Call: impl})
	ctx := context.WithValue(t.Context(), sessionIDKey{}, "execution-fixture")
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "pin_rm", Arguments: asJSON(t, map[string]any{"path": "/ipfs/" + e.target})}}
	first, err := handler(ctx, req)
	require.NoError(t, err)
	conf := confirmationOf(t, first, CodeConfirmRequired).Confirmation
	p, resolve := e.resolver.last(t)
	require.NoError(t, resolve(true))
	req.Params.Arguments = asJSON(t, map[string]any{"path": "/ipfs/" + e.target, "confirm_token": conf.ConfirmToken})
	return ctx, handler, req, p
}

func TestExecutionClaimIsConsumedWhileHandlerIsStillRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newGateEnv(t)
		started, release := make(chan struct{}), make(chan struct{})
		ctx, handler, req, p := directExecutionHandler(t, e, func(ctx context.Context, _ json.RawMessage) (any, error) {
			close(started)
			<-release
			var pins pinsResponse
			if err := e.s.daemon.Call(ctx, "pin/rm", []string{e.target}, nil, &pins); err != nil {
				return nil, err
			}
			return pinRmOutput{Unpinned: pins.Pins}, nil
		})
		done := make(chan *mcp.CallToolResult, 1)
		go func() { res, _ := handler(ctx, req); done <- res }()
		<-started
		snap, _ := e.s.gate.broker.get(p.ID)
		assert.Equal(t, StateExecuted, snap.State, "approval is already consumed before success")
		execution := executionOf(t, e.s, p.ID)
		assert.Equal(t, ExecutionRunning, execution.State)
		assert.Nil(t, execution.Receipt)
		replay, err := handler(ctx, req)
		require.NoError(t, err)
		require.True(t, replay.IsError)
		close(release)
		require.False(t, (<-done).IsError)
		assert.Equal(t, 1, e.d.count("pin/rm"))
		assert.Equal(t, ExecutionSucceeded, executionOf(t, e.s, p.ID).State)
	})
}

func TestExecutionConcurrentTokenClaimsHaveOneInvocationAndReceipt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newGateEnv(t)
		var calls atomic.Int32
		ctx, handler, req, p := directExecutionHandler(t, e, func(ctx context.Context, _ json.RawMessage) (any, error) {
			calls.Add(1)
			var pins pinsResponse
			if err := e.s.daemon.Call(ctx, "pin/rm", []string{e.target}, nil, &pins); err != nil {
				return nil, err
			}
			return pinRmOutput{Unpinned: pins.Pins}, nil
		})
		var successes atomic.Int32
		var wg sync.WaitGroup
		for range 16 {
			wg.Go(func() {
				res, err := handler(ctx, req)
				if err == nil && !res.IsError {
					successes.Add(1)
				}
			})
		}
		wg.Wait()
		assert.Equal(t, int32(1), calls.Load())
		assert.Equal(t, int32(1), successes.Load())
		assert.Equal(t, ExecutionSucceeded, executionOf(t, e.s, p.ID).State)
		assert.Equal(t, 1, e.d.count("pin/rm"))
	})
}

func TestExecutionSuccessSurvivesSerializationFailure(t *testing.T) {
	e := newGateEnv(t)
	ctx, handler, req, p := directExecutionHandler(t, e, func(ctx context.Context, _ json.RawMessage) (any, error) {
		var pins pinsResponse
		if err := e.s.daemon.Call(ctx, "pin/rm", []string{e.target}, nil, &pins); err != nil {
			return nil, err
		}
		return make(chan int), nil // successful operation, undeliverable output
	})
	res, err := handler(ctx, req)
	require.NoError(t, err)
	require.True(t, res.IsError)
	current := currentOutcomeOf(t, res)
	assert.Equal(t, ExecutionSucceeded, current.Execution.State, "delivery error cannot undo successful execution")
	assert.Equal(t, p.ID, current.Execution.Receipt.DecisionID)
	assert.Equal(t, "pin_rm", res.Meta[toolNameMetaKey], "other metadata is preserved")
	assert.False(t, e.pins.has(e.target))
	execution := executionOf(t, e.s, p.ID)
	assert.Equal(t, ExecutionSucceeded, execution.State)
	require.NotNil(t, execution.Receipt)
	assert.Empty(t, execution.Receipt.ResultRefs)
}

func TestExecutionAbnormalExitIsUnknownWithoutRetry(t *testing.T) {
	e := newGateEnv(t)
	ctx, handler, req, p := directExecutionHandler(t, e, func(context.Context, json.RawMessage) (any, error) { panic("fixture abnormal exit") })
	assert.Panics(t, func() { _, _ = handler(ctx, req) })
	execution := executionOf(t, e.s, p.ID)
	assert.Equal(t, ExecutionUnknown, execution.State)
	assert.Nil(t, execution.Receipt)
	assert.NotContains(t, execution.Error.Message, "fixture abnormal exit")
}

func TestExecutionOutcomeRefsAreReturnedIdentifiersOnly(t *testing.T) {
	id := uuid.NewString()
	state, effect, refs, err := successfulOutcome(bulkStartOutput{RunID: id})
	assert.Equal(t, ExecutionSucceeded, state)
	assert.Equal(t, "job_accepted", effect)
	assert.Equal(t, []ReceiptReference{{Kind: "job", Value: id}}, refs)
	assert.Nil(t, err)
	_, _, refs, _ = successfulOutcome(mfsMoveOutput{Source: "/source", Destination: "/echoed-input"})
	assert.Empty(t, refs, "input echoes are not returned node identifiers")
	state, _, refs, err = successfulOutcome(repoGcOutput{Errors: []string{"private stream error"}})
	assert.Equal(t, ExecutionUnknown, state)
	assert.Empty(t, refs)
	assert.NotContains(t, err.Message, "private stream error")
	state, _, _, _ = successfulOutcome(carImportOutput{Roots: []carImportRoot{{CID: testCID(t, 1), PinError: "failed"}}})
	assert.Equal(t, ExecutionUnknown, state)
}

func TestExecutionRetentionSurvivesTokenSweepAndRemainsBounded(t *testing.T) {
	clock := newFakeClock()
	b := newBroker(clock.Now, confirmTTL)
	var first string
	for i := range decisionHistoryLimit + 1 {
		p := b.create(newConfirmation{principal: Principal{ID: "owner", Kind: KindUI, sub: "private-subject"}, sid: "fixture", tool: "pin_rm", argsHash: "hash", summary: Summary{Tool: "pin_rm"}, effectDigest: "sha256:" + strings.Repeat("a", 64)})
		if i == 0 {
			first = p.ID
		}
		require.NoError(t, b.Resolve(p.ID, ResolverLinuxPolkit, true))
		claim, err := b.claimEffect(p.ID, "", nil)
		require.NoError(t, err)
		require.True(t, claim.start())
		clock.Advance(time.Second)
		claim.finish(ExecutionSucceeded, nil, "operation_completed", nil)
	}
	assert.Len(t, b.retained, decisionHistoryLimit)
	_, _, ok := b.observed(first)
	assert.False(t, ok, "oldest resolved evidence is evicted even during token grace")
	var kept string
	for id := range b.retained {
		kept = id
		break
	}
	clock.Advance(2*confirmTTL + time.Second)
	b.mu.Lock()
	b.sweep()
	b.mu.Unlock()
	_, ok = b.get(kept)
	assert.False(t, ok, "authorization/token record was swept")
	p, owner, ok := b.observed(kept)
	require.True(t, ok)
	assert.Empty(t, p.Principal.sub)
	assert.Equal(t, (Principal{ID: "owner", sub: "private-subject"}).BindingKey(), owner)
	assert.NotContains(t, string(asJSON(t, p.Execution)), "private-subject")
	assert.Equal(t, ExecutionSucceeded, p.Execution.State)
	require.ErrorIs(t, b.claim(kept, ""), ErrUnknownConfirmation, "retained evidence cannot authorize replay")
	clock.Advance(decisionHistoryTTL)
	_, _, ok = b.observed(kept)
	assert.False(t, ok)
	assert.True(t, errors.Is(b.claim(kept, ""), ErrUnknownConfirmation))
}
