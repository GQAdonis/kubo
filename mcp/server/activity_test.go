//go:build !noagent

package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func activityMetadata(t *testing.T, interrupt map[string]any) (string, string) {
	t.Helper()
	meta := interrupt["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)
	validateConfirmationDef(t, "ActivityId", meta["activity_id"])
	return meta["activity_id"].(string), meta["confirmation_id"].(string)
}

func decisionFromHTTP(t *testing.T, base, token, id string) decisionObservation {
	t.Helper()
	r, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+RouteDecisions+"/"+id, nil)
	require.NoError(t, err)
	r.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(r)
	require.NoError(t, err)
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode, string(data))
	validateDecisionRead(t, "ItemResponse", data)
	var out struct {
		Item decisionObservation `json:"item"`
	}
	require.NoError(t, json.Unmarshal(data, &out))
	return out.Item
}

func TestActivityBuiltinQueuedInterruptsAndResumeKeepOneServerIdentity(t *testing.T) {
	first := scriptedCall{ID: "tc-one", Name: "pin_add", Args: map[string]any{"path": "/ipfs/" + testCID(t, 5), "recursive": true}}
	second := scriptedCall{ID: "tc-two", Name: "pin_add", Args: map[string]any{"path": "/ipfs/" + testCID(t, 6)}}
	e := newAgentEnv(t, agentEnvOpts{}, callStep(10, first, second), sayStep("Done."), pinAddScript(t, "tc-new", 5))
	e.s.gate.broker.setHistoryEnabled(true)
	id := uiID(nil, "pin")
	// Even an in-process caller cannot choose the accepted activity's ID.
	ctx := context.WithValue(t.Context(), activityIDKey{}, "act_client_spoof")
	var events []Event
	initial, err := e.a.Run(ctx, id, RunInput{SessionID: "chosen-thread", Message: "pin twice"}, func(ev Event) { events = append(events, ev) })
	require.NoError(t, err)
	require.Equal(t, OutcomeInterrupt, initial.Outcome)
	validateConfirmationDef(t, "ActivityId", initial.ActivityID)
	assert.NotEqual(t, "act_client_spoof", initial.ActivityID)
	assert.NotEqual(t, initial.SessionID, initial.ActivityID)
	for _, ev := range events {
		assert.Equal(t, initial.ActivityID, ev.ActivityID)
	}
	assert.Equal(t, initial.ActivityID, initial.Interrupts[0].ActivityID)
	open, err := e.a.OpenInterrupt(id, initial.SessionID)
	require.NoError(t, err)
	assert.Equal(t, initial.ActivityID, open.ActivityID)
	p, _, ok := e.s.gate.broker.observed(open.ConfirmationID)
	require.True(t, ok)
	assert.Equal(t, initial.ActivityID, p.ActivityID)
	_, err = e.a.Run(t.Context(), id, RunInput{SessionID: initial.SessionID, Message: "cannot replace waiting activity"}, nil)
	require.ErrorIs(t, err, ErrInterruptOpen)
	openAgain, err := e.a.OpenInterrupt(id, initial.SessionID)
	require.NoError(t, err)
	assert.Equal(t, initial.ActivityID, openAgain.ActivityID)
	_, resolve := e.resolver.last(t)
	require.NoError(t, resolve(true))
	continued, _, err := e.resume(id, initial.SessionID, ResumeEntry{InterruptID: open.ID, Status: ResumeResolved, Approved: true})
	require.NoError(t, err)
	require.Equal(t, OutcomeInterrupt, continued.Outcome)
	assert.Equal(t, initial.ActivityID, continued.ActivityID)
	assert.Equal(t, initial.ActivityID, continued.Interrupts[0].ActivityID)
	assert.NotEqual(t, open.ConfirmationID, continued.Interrupts[0].ConfirmationID, "decisions stay independently bound")
	_, resolve = e.resolver.last(t)
	require.NoError(t, resolve(true))
	completed, _, err := e.resume(id, initial.SessionID, ResumeEntry{InterruptID: continued.Interrupts[0].ID, Status: ResumeResolved, Approved: true})
	require.NoError(t, err)
	assert.Equal(t, OutcomeSuccess, completed.Outcome)
	assert.Equal(t, initial.ActivityID, completed.ActivityID)
	newActivity, err := e.a.Run(t.Context(), id, RunInput{SessionID: initial.SessionID, Message: "another request"}, nil)
	require.NoError(t, err)
	assert.NotEqual(t, initial.ActivityID, newActivity.ActivityID, "new message on same conversation starts new activity")
	newPending, ok := e.s.gate.broker.get(newActivity.Interrupts[0].ConfirmationID)
	require.True(t, ok)
	assert.Equal(t, p.EffectDigest, newPending.EffectDigest, "activity is independent of the same canonical operation")
	assert.NotEqual(t, p.ID, newPending.ID, "activity correlation never substitutes for immutable decision identity")
}

func TestActivityBuiltinWrongOwnerOrInterruptCannotReplaceCorrelation(t *testing.T) {
	e := newAgentEnv(t, agentEnvOpts{}, pinAddScript(t, "tc-one", 5))
	id := uiID(nil, "pin")
	id.Principal.sub = "alice"
	initial, _ := e.run(id, "pin")
	other := id
	other.Principal.sub = "bob"
	_, _, err := e.resume(other, initial.SessionID, ResumeEntry{InterruptID: initial.Interrupts[0].ID, Status: ResumeResolved, Approved: true})
	assert.ErrorIs(t, err, ErrSessionNotFound)
	_, _, err = e.resume(id, initial.SessionID, ResumeEntry{InterruptID: "unknown", Status: ResumeResolved, Approved: true})
	assert.ErrorIs(t, err, ErrUnknownInterrupt)
	open, err := e.a.OpenInterrupt(id, initial.SessionID)
	require.NoError(t, err)
	assert.Equal(t, initial.ActivityID, open.ActivityID)
	assert.Zero(t, e.d.count("pin/add"))
}

func TestActivityAGUIInterruptApprovalAndObservationAgree(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc-one", 5), sayStep("Pinned."), pinAddScript(t, "tc-new", 6))
	body := aguiBody("activity-thread", "client-run", []any{userMsg("m1", "pin")})
	body["activity_id"] = "act_client_spoof"
	response := e.post(aguiTokChat, body, nil)
	require.Equal(t, 200, response.Status)
	interrupt := finishedInterrupt(t, response.Events)
	activity, decision := activityMetadata(t, interrupt)
	assert.NotEqual(t, "act_client_spoof", activity)
	assert.NotEqual(t, "client-run", activity)
	for _, event := range findEvents(response.Events, "RUN_STARTED") {
		assert.ElementsMatch(t, []string{"type", "threadId", "runId"}, keysOf(event), "standard AG-UI event has no extra activity field")
	}
	assert.Equal(t, activity, decisionFromHTTP(t, e.base, aguiTokChat, decision).ActivityID)
	r := e.post(aguiTokChat, aguiBody("activity-thread", "different-resume-run", nil, resumeEntry(interrupt["id"].(string), ResumeResolved, map[string]any{"approved": true})), nil)
	require.Equal(t, 200, r.Status)
	observed := decisionFromHTTP(t, e.base, aguiTokChat, decision)
	assert.Equal(t, activity, observed.ActivityID)
	require.NotNil(t, observed.Execution)
	assert.Equal(t, ExecutionSucceeded, observed.Execution.State)
	r = e.post(aguiTokChat, aguiBody("activity-thread", "new-message-run", []any{userMsg("m2", "pin another")}), nil)
	require.Equal(t, 200, r.Status)
	nextActivity, _ := activityMetadata(t, finishedInterrupt(t, r.Events))
	assert.NotEqual(t, activity, nextActivity)
}

func TestActivityInterruptProjectionDoesNotDependOnRunFactsEmission(t *testing.T) {
	// Both projection functions carry correlation directly on the interruption;
	// neither takes a facts emitter or fabricates an ipfs.run.facts event.
	wire := aguiInterrupt(Interrupt{ID: "int-fixture", ConfirmationID: "cf_fixture_00000000000001", Tool: "pin_add", ToolCallID: "tc", Message: "Pin content", Class: "write", ActivityID: "act_server_fixture"})
	activity, _ := activityMetadata(t, wire)
	assert.Equal(t, "act_server_fixture", activity)
	p, ok := classifyUARInterrupt(map[string]any{"id": "backend", "reason": uarReasonToolApproval}, nil)
	require.True(t, ok)
	p.activityID = "act_backend_owned"
	activity, _ = activityMetadata(t, p.aguiInterrupt())
	assert.Equal(t, "act_backend_owned", activity)
}

func TestActivityUARResumeAndObservationKeepOwnedCorrelationWithoutNodeOutcome(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-tool-approval-interrupt.sse", "run-resumed-executed.sse", "run-tool-approval-interrupt.sse", "run-tool-approval-interrupt.sse"), uarProxyOpts{})
	response := e.say("same-thread", "client-run", "pin")
	interrupt := finishedInterrupt(t, response.Events)
	activity, decision := activityMetadata(t, interrupt)
	assert.NotEqual(t, "client-run", activity)
	assert.Equal(t, activity, decisionFromHTTP(t, e.base, aguiTokChat, decision).ActivityID)
	foreign := e.post(aguiTokChat2, aguiBody("same-thread", "foreign-resume", nil, resumeEntry(interrupt["id"].(string), ResumeResolved, map[string]any{"approved": true})))
	assert.Equal(t, http.StatusNotFound, foreign.Status)
	assert.Equal(t, 1, e.fake.runCount())
	continued := e.answer("same-thread", "different-resume-run", interrupt["id"].(string), true)
	require.Equal(t, http.StatusOK, continued.Status)
	item := decisionFromHTTP(t, e.base, aguiTokChat, decision)
	assert.Equal(t, activity, item.ActivityID)
	assert.Equal(t, SourceKindBackendApproval, item.SourceKind)
	assert.Nil(t, item.Execution)
	assert.NotContains(t, string(asJSON(t, e.fake.run(1))), activity, "local activity is not a backend handshake or grant")
	newRun := e.say("same-thread", "client-run", "new user activity")
	nextActivity, _ := activityMetadata(t, finishedInterrupt(t, newRun.Events))
	assert.NotEqual(t, activity, nextActivity, "even reusing a client run ID cannot reuse the server activity")
	otherRun := e.post(aguiTokChat2, aguiBody("same-thread", "client-run", []any{userMsg("m-other", "same backend interrupt, different identity")}))
	require.Equal(t, http.StatusOK, otherRun.Status)
	otherActivity, _ := activityMetadata(t, finishedInterrupt(t, otherRun.Events))
	assert.NotEqual(t, activity, otherActivity)
	assert.NotEqual(t, nextActivity, otherActivity, "same thread/run/backend IDs never merge identities' activities")
}
