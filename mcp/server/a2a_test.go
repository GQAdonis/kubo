//go:build !noagent

package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func metaOf(task map[string]any) map[string]any {
	m, _ := task["metadata"].(map[string]any)
	info, _ := m[a2aMetaKey].(map[string]any)
	return info
}

func TestA2ADelegatedRead(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	status, resp := e.call(a2aTokPeer, "SendMessage", map[string]any{"message": userMessage(toolPart("pin_ls", map[string]any{"type": "recursive"}))})
	require.Equal(t, http.StatusOK, status)
	result := rpcResult(t, resp)
	task := result["task"].(map[string]any)
	assert.Equal(t, a2aCompleted, stateOf(task))
	assert.True(t, strings.HasPrefix(task["id"].(string), "task-"))
	assert.True(t, strings.HasPrefix(task["contextId"].(string), "ctx-"))
	arts := task["artifacts"].([]any)
	require.Len(t, arts, 1)
	art := arts[0].(map[string]any)
	assert.Equal(t, "pin_ls", art["name"])
	part := art["parts"].([]any)[0].(map[string]any)
	assert.Equal(t, "application/json", part["mediaType"])
	assert.Contains(t, part["data"].(map[string]any)["pins"].([]any)[0].(map[string]any)["cid"], testCID(t, 1)[:4])
	validateA2A(t, "SendMessageResponse", result)

	// GetTask shows the same task; the model was never asked.
	assert.Equal(t, a2aCompleted, stateOf(e.getTask(a2aTokPeer, task["id"].(string))))
	assert.Zero(t, e.model.hits())
}

func TestA2AWriteWaitsForThePersonAndApprovalCompletesIt(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	target := testCID(t, 77)
	task := taskOf(t, e.send(a2aTokPeer, toolPart("pin_add", map[string]any{"path": "/ipfs/" + target})))
	id := task["id"].(string)

	require.Equal(t, a2aInputRequired, stateOf(task))
	assert.False(t, e.pins.has(target), "nothing runs before the person decides")
	info := metaOf(task)
	assert.Equal(t, "pin_add", info["tool"])
	assert.Equal(t, "write", info["class"])
	assert.NotEmpty(t, info["confirmation_id"])
	msg := task["status"].(map[string]any)["message"].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"].(string)
	assert.Contains(t, msg, "Waiting for approval on the node's machine")
	validateA2A(t, "Task", task)
	pend, resolve := e.presence.last(t)
	assert.Equal(t, info["confirmation_id"], pend.ID)

	// The peer says "approve": that approves nothing, and the task keeps its
	// state.
	_, resp := e.call(a2aTokPeer, "SendMessage", map[string]any{"message": map[string]any{
		"messageId": "m-x", "role": "ROLE_USER", "taskId": id, "contextId": task["contextId"], "parts": []any{textPartJSON("approve")}}})
	again := taskOf(t, resp)
	assert.Equal(t, a2aInputRequired, stateOf(again))
	assert.Contains(t, again["status"].(map[string]any)["message"].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"], "Approval cannot be given over A2A")
	assert.False(t, e.pins.has(target))
	assert.Equal(t, a2aInputRequired, stateOf(e.getTask(a2aTokPeer, id)))
	// The stored task did not take the notice as its status.
	assert.Contains(t, e.getTask(a2aTokPeer, id)["status"].(map[string]any)["message"].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"], "Waiting for approval")

	// The person approves on the node's machine: the task completes with no
	// further request from the peer, and the pin exists exactly once.
	require.NoError(t, resolve(true))
	done := e.eventuallyState(a2aTokPeer, id, a2aCompleted)
	assert.True(t, e.pins.has(target))
	assert.Nil(t, done["metadata"], "the confirmation is no longer pending")
	require.Len(t, done["artifacts"], 1)
	validateA2A(t, "Task", done)
	assert.Empty(t, e.s.PendingConfirmations())
}

func TestA2AWriteDeniedAndExpiredAreRejected(t *testing.T) {
	t.Run("denied", func(t *testing.T) {
		e := newA2AEnv(t, a2aEnvOpts{})
		target := testCID(t, 78)
		task := taskOf(t, e.send(a2aTokPeer, toolPart("pin_add", map[string]any{"path": "/ipfs/" + target})))
		_, resolve := e.presence.last(t)
		require.NoError(t, resolve(false))
		done := e.eventuallyState(a2aTokPeer, task["id"].(string), a2aRejected)
		assert.False(t, e.pins.has(target))
		art := done["artifacts"].([]any)[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["data"].(map[string]any)
		assert.Equal(t, CodeConfirmDeclined, art["error"].(map[string]any)["code"])
		validateA2A(t, "Task", done)
	})
	t.Run("expired", func(t *testing.T) {
		e := newA2AEnv(t, a2aEnvOpts{})
		target := testCID(t, 79)
		task := taskOf(t, e.send(a2aTokPeer, toolPart("pin_add", map[string]any{"path": "/ipfs/" + target})))
		e.clock.Advance(confirmTTL + time.Second)
		done := e.eventuallyState(a2aTokPeer, task["id"].(string), a2aRejected)
		assert.False(t, e.pins.has(target))
		art := done["artifacts"].([]any)[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["data"].(map[string]any)
		assert.Equal(t, CodeConfirmExpired, art["error"].(map[string]any)["code"])
	})
}

func TestA2ACancelIsDenial(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	target := testCID(t, 80)
	task := taskOf(t, e.send(a2aTokPeer, toolPart("pin_add", map[string]any{"path": "/ipfs/" + target})))
	id := task["id"].(string)
	_, resolve := e.presence.last(t)

	_, resp := e.call(a2aTokPeer, "CancelTask", map[string]any{"id": id})
	assert.Equal(t, a2aCanceled, stateOf(rpcResult(t, resp)))
	assert.Empty(t, e.s.PendingConfirmations(), "the confirmation was denied")
	// A late approval finds nothing to approve and runs nothing.
	assert.Error(t, resolve(true))
	assert.False(t, e.pins.has(target))
	assert.Equal(t, a2aCanceled, stateOf(e.getTask(a2aTokPeer, id)))

	// A finished task cannot be cancelled again.
	_, resp = e.call(a2aTokPeer, "CancelTask", map[string]any{"id": id})
	code, tool := rpcErrorCode(t, resp)
	assert.Equal(t, a2aTaskNotCancelable, code)
	assert.Equal(t, CodeFailedPrecondition, tool)
}

func TestA2AOutOfScopeIsAuthRequiredAndRunsNothing(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	// reader has no write or destructive grant at all.
	task := taskOf(t, e.send(a2aTokReader, toolPart("pin_rm", map[string]any{"path": "/ipfs/" + testCID(t, 1)})))
	assert.Equal(t, a2aAuthRequired, stateOf(task))
	assert.Contains(t, task["status"].(map[string]any)["message"].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"], "pin_rm needs a destructive grant for group pin")
	assert.True(t, e.pins.has(testCID(t, 1)))
	task = taskOf(t, e.send(a2aTokReader, toolPart("pin_add", map[string]any{"path": "/ipfs/" + testCID(t, 5)})))
	assert.Equal(t, a2aAuthRequired, stateOf(task))
	assert.False(t, e.pins.has(testCID(t, 5)))
	assert.Empty(t, e.presence.requests, "no prompt was raised for a call outside the grants")
	validateA2A(t, "Task", task)
	// A tool in a group the principal has no read grant for.
	task = taskOf(t, e.send(a2aTokReader, toolPart("ipfs_cat", map[string]any{"path": "/ipfs/" + testCID(t, 1)})))
	assert.Equal(t, a2aAuthRequired, stateOf(task))
}

func TestA2ADestructiveNeedsThePresenceResolver(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	task := taskOf(t, e.send(a2aTokPeer, toolPart("pin_rm", map[string]any{"path": "/ipfs/" + testCID(t, 1)})))
	require.Equal(t, a2aInputRequired, stateOf(task))
	assert.Equal(t, true, metaOf(task)["requires_native_prompt"])
	// The chat-style approval resolver cannot approve it.
	assert.ErrorIs(t, e.s.ResolveConfirmation(metaOf(task)["confirmation_id"].(string), ResolverAGUIInterrupt, true), ErrNativePromptRequired)
	assert.ErrorIs(t, e.s.ResolveConfirmation(metaOf(task)["confirmation_id"].(string), ResolverCLI, true), ErrNativePromptRequired)
	assert.True(t, e.pins.has(testCID(t, 1)))
	_, resolve := e.presence.last(t)
	require.NoError(t, resolve(true))
	e.eventuallyState(a2aTokPeer, task["id"].(string), a2aCompleted)
	assert.False(t, e.pins.has(testCID(t, 1)))
}

func TestA2ADestructiveWithoutAPresenceResolverFailsClosed(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{noPresence: true})
	task := taskOf(t, e.send(a2aTokPeer, toolPart("pin_rm", map[string]any{"path": "/ipfs/" + testCID(t, 1)})))
	assert.Equal(t, a2aRejected, stateOf(task))
	assert.True(t, e.pins.has(testCID(t, 1)))
}

func TestA2AEgressAckAppliesToDirectCalls(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	task := taskOf(t, e.send(a2aTokNoAck, toolPart("ipfs_cat", map[string]any{"path": "/ipfs/" + testCID(t, 1)})))
	assert.Equal(t, a2aRejected, stateOf(task))
	art := task["artifacts"].([]any)[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["data"].(map[string]any)
	assert.Equal(t, CodeEgressNotAcknowledged, art["error"].(map[string]any)["code"])
}

func TestA2AOwnerChecks(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	task := taskOf(t, e.send(a2aTokPeer, toolPart("pin_ls", map[string]any{})))
	id, ctxID := task["id"].(string), task["contextId"].(string)
	missing := func() (int, string) {
		_, resp := e.call(a2aTokPeer2, "GetTask", map[string]any{"id": "task-nonexistent"})
		return rpcErrorCode(t, resp)
	}
	wantCode, wantTool := missing()
	require.Equal(t, a2aTaskNotFound, wantCode)

	// Another principal gets exactly the answer for a task that is not there.
	for name, call := range map[string]func() map[string]any{
		"GetTask":    func() map[string]any { _, r := e.call(a2aTokPeer2, "GetTask", map[string]any{"id": id}); return r },
		"CancelTask": func() map[string]any { _, r := e.call(a2aTokPeer2, "CancelTask", map[string]any{"id": id}); return r },
		"message to the task": func() map[string]any {
			_, r := e.call(a2aTokPeer2, "SendMessage", map[string]any{"message": map[string]any{
				"messageId": "m", "role": "ROLE_USER", "taskId": id, "parts": []any{textPartJSON("hi")}}})
			return r
		},
	} {
		code, tool := rpcErrorCode(t, call())
		assert.Equal(t, wantCode, code, name)
		assert.Equal(t, wantTool, tool, name)
	}
	// Another principal cannot use the context; the answer is the same for a
	// context that does not exist.
	for _, cid := range []string{ctxID, "ctx-nonexistent"} {
		_, resp := e.call(a2aTokPeer2, "SendMessage", map[string]any{"message": map[string]any{
			"messageId": "m", "role": "ROLE_USER", "contextId": cid, "parts": []any{toolPart("pin_ls", map[string]any{})}}})
		code, tool := rpcErrorCode(t, resp)
		assert.Equal(t, a2aTaskNotFound, code)
		assert.Equal(t, CodeNotFound, tool)
		assert.Equal(t, "Context not found; omit contextId to start a new one", resp["error"].(map[string]any)["message"])
	}
	// The owner is unaffected, and may continue its context.
	assert.Equal(t, a2aCompleted, stateOf(e.getTask(a2aTokPeer, id)))
	next := taskOf(t, e.send2(a2aTokPeer, ctxID, toolPart("pin_ls", map[string]any{})))
	assert.Equal(t, ctxID, next["contextId"])
	assert.NotEqual(t, id, next["id"])

	// A cancel by the other principal of a task that waits for approval leaves
	// the confirmation pending.
	w := taskOf(t, e.send(a2aTokPeer, toolPart("pin_add", map[string]any{"path": "/ipfs/" + testCID(t, 81)})))
	_, resp := e.call(a2aTokPeer2, "CancelTask", map[string]any{"id": w["id"]})
	assert.NotNil(t, resp["error"])
	assert.Len(t, e.s.PendingConfirmations(), 1)
	assert.Equal(t, a2aInputRequired, stateOf(e.getTask(a2aTokPeer, w["id"].(string))))
}

func (e *a2aEnv) send2(token, contextID string, parts ...any) map[string]any {
	e.t.Helper()
	m := userMessage(parts...)
	m["contextId"] = contextID
	_, resp := e.call(token, "SendMessage", map[string]any{"message": m})
	return resp
}

func TestA2AContextHasOneOpenTask(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	task := taskOf(t, e.send(a2aTokPeer, toolPart("pin_add", map[string]any{"path": "/ipfs/" + testCID(t, 82)})))
	resp := e.send2(a2aTokPeer, task["contextId"].(string), toolPart("pin_ls", map[string]any{}))
	code, tool := rpcErrorCode(t, resp)
	assert.Equal(t, rpcInternalError, code)
	assert.Equal(t, CodeSessionBusy, tool)
	assert.True(t, resp["error"].(map[string]any)["data"].([]any)[1].(map[string]any)["error"].(map[string]any)["retryable"].(bool))
	assert.Len(t, e.presence.requests, 1, "no second confirmation was opened")
	// A new context is independent.
	other := taskOf(t, e.send(a2aTokPeer, toolPart("pin_ls", map[string]any{})))
	assert.Equal(t, a2aCompleted, stateOf(other))
}

func TestA2AEndpointRejectsOtherPrincipalKinds(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	body := `{"jsonrpc":"2.0","id":1,"method":"GetTask","params":{"id":"x"}}`
	for name, tok := range map[string]string{"human-mcp": a2aTokHuman, "ui": a2aTokChat} {
		resp := e.openRaw(tok, []byte(body), nil)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, name)
	}
	assert.Equal(t, http.StatusUnauthorized, e.openRaw("", []byte(body), nil).StatusCode)
	assert.Equal(t, http.StatusUnauthorized, e.openRaw("wrong-token-0123456789012", []byte(body), nil).StatusCode)
	// And the a2a token opens neither /mcp nor /agui.
	for _, path := range []string{RouteMCP, RouteAGUI} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, e.base+path, strings.NewReader("{}"))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+a2aTokPeer)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, path)
	}
}

func TestA2ARequestErrors(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	raw := func(body string, hdr map[string]string) map[string]any {
		resp := e.openRaw(a2aTokPeer, []byte(body), hdr)
		var out map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		return out
	}
	cases := map[string]struct {
		resp map[string]any
		code int
		tool string
	}{
		"parse":           {raw(`{`, nil), rpcParseError, CodeInvalidArgument},
		"batch":           {raw(`[{"jsonrpc":"2.0","id":1,"method":"GetTask"}]`, nil), rpcInvalidRequest, CodeInvalidArgument},
		"not json-rpc":    {raw(`{"id":1,"method":"GetTask"}`, nil), rpcInvalidRequest, CodeInvalidArgument},
		"unknown method":  {raw(`{"jsonrpc":"2.0","id":1,"method":"message/send","params":{}}`, nil), rpcMethodNotFound, CodeInvalidArgument},
		"no params":       {raw(`{"jsonrpc":"2.0","id":1,"method":"SendMessage"}`, nil), rpcInvalidParams, CodeInvalidArgument},
		"no id param":     {raw(`{"jsonrpc":"2.0","id":1,"method":"GetTask","params":{}}`, nil), rpcInvalidParams, CodeInvalidArgument},
		"list":            {raw(`{"jsonrpc":"2.0","id":1,"method":"ListTasks","params":{}}`, nil), a2aUnsupportedOperation, CodeToolUnavailable},
		"push":            {raw(`{"jsonrpc":"2.0","id":1,"method":"CreateTaskPushNotificationConfig","params":{}}`, nil), a2aPushNotSupported, CodeToolUnavailable},
		"extended card":   {raw(`{"jsonrpc":"2.0","id":1,"method":"GetExtendedAgentCard"}`, nil), a2aExtendedCardMissing, CodeToolUnavailable},
		"old version":     {raw(`{"jsonrpc":"2.0","id":1,"method":"GetTask","params":{"id":"x"}}`, map[string]string{"A2A-Version": "0.3"}), a2aVersionNotSupported, CodeInvalidArgument},
		"unknown task":    {raw(`{"jsonrpc":"2.0","id":1,"method":"GetTask","params":{"id":"nope"}}`, nil), a2aTaskNotFound, CodeNotFound},
		"file part":       {raw(`{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{"message":{"messageId":"m","role":"ROLE_USER","parts":[{"url":"http://x/y","mediaType":"image/png"}]}}}`, nil), a2aContentTypeNotSupport, CodeInvalidArgument},
		"agent role":      {raw(`{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{"message":{"messageId":"m","role":"ROLE_AGENT","parts":[{"text":"x"}]}}}`, nil), rpcInvalidParams, CodeInvalidArgument},
		"mixed parts":     {raw(`{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{"message":{"messageId":"m","role":"ROLE_USER","parts":[{"text":"x"},{"data":{"tool":"pin_ls","arguments":{}},"mediaType":"application/json"}]}}}`, nil), rpcInvalidParams, CodeInvalidArgument},
		"bad data":        {raw(`{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{"message":{"messageId":"m","role":"ROLE_USER","parts":[{"data":{"cmd":"x"},"mediaType":"application/json"}]}}}`, nil), rpcInvalidParams, CodeInvalidArgument},
		"unknown context": {raw(`{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{"message":{"messageId":"m","role":"ROLE_USER","contextId":"ctx-zzz","parts":[{"text":"x"}]}}}`, nil), a2aTaskNotFound, CodeNotFound},
	}
	for name, c := range cases {
		code, tool := rpcErrorCode(t, c.resp)
		assert.Equal(t, c.code, code, name)
		assert.Equal(t, c.tool, tool, name)
	}
	// Only POST, only on the exact path.
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, e.base+RouteA2A, nil)
	req.Header.Set("Authorization", "Bearer "+a2aTokPeer)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	req, _ = http.NewRequestWithContext(t.Context(), http.MethodPost, e.base+RouteA2A+"/x", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+a2aTokPeer)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestA2AUnknownAndUngrantedToolsAreTaskStates(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	task := taskOf(t, e.send(a2aTokPeer, toolPart("rm_rf", map[string]any{})))
	assert.Equal(t, a2aFailed, stateOf(task))
	task = taskOf(t, e.send(a2aTokPeer, toolPart("pin_ls", map[string]any{"type": 42})))
	assert.Equal(t, a2aFailed, stateOf(task), "invalid arguments are rejected by the tool's schema")
	// A confirm_token in the arguments is dropped, never honoured.
	task = taskOf(t, e.send(a2aTokPeer, toolPart("pin_add", map[string]any{"path": "/ipfs/" + testCID(t, 90), "confirm_token": "forged"})))
	assert.Equal(t, a2aInputRequired, stateOf(task))
	assert.False(t, e.pins.has(testCID(t, 90)))
}

func TestA2ATextMessageRunsTheLoop(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{},
		callStep(40, scriptedCall{ID: "tc-1", Name: "pin_ls", Args: map[string]any{"type": "recursive"}}),
		sayStep("You have one pin."))
	task := taskOf(t, e.send(a2aTokPeer, textPartJSON("List my pins")))
	assert.Equal(t, a2aCompleted, stateOf(task))
	art := task["artifacts"].([]any)[0].(map[string]any)
	assert.Equal(t, "reply", art["name"])
	assert.Equal(t, "You have one pin.", art["parts"].([]any)[0].(map[string]any)["text"])
	validateA2A(t, "Task", task)
	// The model was offered only what the principal may use, and no confirm_token.
	reqs := e.model.requests()
	require.Len(t, reqs, 2)
	assert.Contains(t, toolNamesOf(reqs[0]), "pin_add")
	assert.NotContains(t, e.model.allBodies(), "confirm_token")
}

func TestA2ATextWriteWaitsAndApprovalResumesTheRun(t *testing.T) {
	target := testCID(t, 91)
	e := newA2AEnv(t, a2aEnvOpts{},
		callStep(40, scriptedCall{ID: "tc-1", Name: "pin_add", Args: map[string]any{"path": "/ipfs/" + target}}),
		sayStep("Pinned it."))
	task := taskOf(t, e.send(a2aTokPeer, textPartJSON("pin "+target)))
	id := task["id"].(string)
	require.Equal(t, a2aInputRequired, stateOf(task))
	assert.False(t, e.pins.has(target))
	assert.Equal(t, 1, e.model.hits(), "the model is not asked again while the call waits")

	// "approve" in a message changes nothing.
	_, resp := e.call(a2aTokPeer, "SendMessage", map[string]any{"message": map[string]any{
		"messageId": "m", "role": "ROLE_USER", "taskId": id, "parts": []any{textPartJSON("yes, approved")}}})
	assert.Equal(t, a2aInputRequired, stateOf(taskOf(t, resp)))
	assert.False(t, e.pins.has(target))
	assert.Equal(t, 1, e.model.hits())

	_, resolve := e.presence.last(t)
	require.NoError(t, resolve(true))
	done := e.eventuallyState(a2aTokPeer, id, a2aCompleted)
	assert.True(t, e.pins.has(target))
	assert.Equal(t, "Pinned it.", done["artifacts"].([]any)[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"])
}

func TestA2ATextWriteDeniedIsRejectedWithoutAskingTheModelAgain(t *testing.T) {
	target := testCID(t, 92)
	e := newA2AEnv(t, a2aEnvOpts{},
		callStep(40, scriptedCall{ID: "tc-1", Name: "pin_add", Args: map[string]any{"path": "/ipfs/" + target}}),
		sayStep("Next question answered."))
	task := taskOf(t, e.send(a2aTokPeer, textPartJSON("pin "+target)))
	_, resolve := e.presence.last(t)
	require.NoError(t, resolve(false))
	e.eventuallyState(a2aTokPeer, task["id"].(string), a2aRejected)
	assert.False(t, e.pins.has(target))
	assert.Equal(t, 1, e.model.hits())
	// The conversation continues in the same context.
	next := taskOf(t, e.send2(a2aTokPeer, task["contextId"].(string), textPartJSON("thanks")))
	assert.Equal(t, a2aCompleted, stateOf(next))
}

func TestA2ALimitIsFailed(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{limits: LoopLimits{MaxSteps: 1}},
		callStep(40, scriptedCall{ID: "tc-1", Name: "pin_ls", Args: map[string]any{}}))
	task := taskOf(t, e.send(a2aTokPeer, textPartJSON("loop")))
	assert.Equal(t, a2aFailed, stateOf(task))
	assert.Contains(t, task["status"].(map[string]any)["message"].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"], "step limit")
}

func TestA2ABackendFailureIsFailedWithoutDetail(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{}) // no scripted step: the model server answers 500
	task := taskOf(t, e.send(a2aTokPeer, textPartJSON("hello")))
	assert.Equal(t, a2aFailed, stateOf(task))
	text := task["status"].(map[string]any)["message"].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"].(string)
	assert.NotContains(t, text, "scripted")
	assert.NotContains(t, e.logs.String(), "hello", "message content is never logged")
}

func TestA2AReturnImmediately(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{}, sayStep("done"))
	_, resp := e.call(a2aTokPeer, "SendMessage", map[string]any{
		"message": userMessage(textPartJSON("hi")), "configuration": map[string]any{"returnImmediately": true}})
	task := taskOf(t, resp)
	id := task["id"].(string)
	assert.Contains(t, []string{a2aSubmitted, a2aWorking, a2aCompleted}, stateOf(task))
	e.eventuallyState(a2aTokPeer, id, a2aCompleted)
}

// readStream reads the JSON-RPC responses of an SSE body.
func readStream(t *testing.T, resp *http.Response) []map[string]any {
	t.Helper()
	var out []map[string]any
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
			var m map[string]any
			require.NoError(t, json.Unmarshal([]byte(data), &m))
			out = append(out, m)
		}
	}
	return out
}

func TestA2AStreamingRead(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	resp := e.open(a2aTokPeer, "SendStreamingMessage", map[string]any{"message": userMessage(toolPart("ipfs_node_info", map[string]any{}))}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	evs := readStream(t, resp)
	require.GreaterOrEqual(t, len(evs), 3)
	first := rpcResult(t, evs[0])
	require.Contains(t, first, "task", "a task stream begins with the Task")
	for _, ev := range evs {
		validateA2A(t, "StreamResponse", rpcResult(t, ev))
	}
	last := rpcResult(t, evs[len(evs)-1])["statusUpdate"].(map[string]any)
	assert.Equal(t, a2aCompleted, last["status"].(map[string]any)["state"])
	var sawArtifact bool
	for _, ev := range evs {
		sawArtifact = sawArtifact || rpcResult(t, ev)["artifactUpdate"] != nil
	}
	assert.True(t, sawArtifact)
}

func TestA2AStreamFollowsAnApprovalToTheEnd(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	target := testCID(t, 93)
	resp := e.open(a2aTokPeer, "SendStreamingMessage", map[string]any{"message": userMessage(toolPart("pin_add", map[string]any{"path": "/ipfs/" + target}))}, nil)
	var wg sync.WaitGroup
	var evs []map[string]any
	wg.Add(1)
	go func() {
		defer wg.Done()
		evs = readStream(t, resp)
	}()
	require.Eventually(t, func() bool { e.presence.mu.Lock(); defer e.presence.mu.Unlock(); return len(e.presence.requests) == 1 }, 5*time.Second, 5*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	assert.False(t, e.pins.has(target))
	_, resolve := e.presence.last(t)
	require.NoError(t, resolve(true))
	wg.Wait()

	var states []string
	for _, ev := range evs {
		r := rpcResult(t, ev)
		validateA2A(t, "StreamResponse", r)
		if su, ok := r["statusUpdate"].(map[string]any); ok {
			states = append(states, su["status"].(map[string]any)["state"].(string))
		}
	}
	assert.Contains(t, states, a2aInputRequired)
	assert.Equal(t, a2aCompleted, states[len(states)-1])
	assert.True(t, e.pins.has(target))
}

func TestA2AAgentCard(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{})
	get := func(hdr map[string]string) (*http.Response, []byte) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, e.base+RouteAgentCard, nil)
		require.NoError(t, err)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		var buf strings.Builder
		_, _ = bufio.NewReader(resp.Body).WriteTo(&buf)
		return resp, []byte(buf.String())
	}
	resp, body := get(nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "the card needs no credential")
	var card map[string]any
	require.NoError(t, json.Unmarshal(body, &card))
	validateA2A(t, "AgentCard", card)
	iface := card["supportedInterfaces"].([]any)[0].(map[string]any)
	assert.Equal(t, e.base+RouteA2A, iface["url"])
	assert.Equal(t, "JSONRPC", iface["protocolBinding"])
	assert.Equal(t, "1.0", iface["protocolVersion"])
	assert.Equal(t, "9.9.9", card["version"])
	sch := card["securitySchemes"].(map[string]any)["bearer"].(map[string]any)["httpAuthSecurityScheme"].(map[string]any)
	assert.Equal(t, "Bearer", sch["scheme"])
	assert.NotContains(t, string(body), a2aTokPeer)

	ids := func(card map[string]any) []string {
		var out []string
		for _, s := range card["skills"].([]any) {
			out = append(out, s.(map[string]any)["id"].(string))
		}
		return out
	}
	// Tools of node, content, pin and diag are registered, so those skills.
	assert.ElementsMatch(t, []string{"ipfs-node", "ipfs-content", "ipfs-pinning"}, ids(card))

	// Conditional requests.
	resp2, _ := get(map[string]string{"If-None-Match": resp.Header.Get("ETag")})
	assert.Equal(t, http.StatusNotModified, resp2.StatusCode)
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, e.base+RouteAgentCard, strings.NewReader("{}"))
	post, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = post.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, post.StatusCode)
}

func TestA2AAgentCardListsOnlyWhatThePeersMayUse(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{onlyReader: true})
	resp, err := http.Get(e.base + RouteAgentCard)
	require.NoError(t, err)
	defer resp.Body.Close()
	var card map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&card))
	validateA2A(t, "AgentCard", card)
	var ids []string
	for _, s := range card["skills"].([]any) {
		ids = append(ids, s.(map[string]any)["id"].(string))
	}
	// reader may read the node and pin groups only.
	assert.ElementsMatch(t, []string{"ipfs-node", "ipfs-pinning"}, ids)
}

func TestA2ADisabledServesNothing(t *testing.T) {
	for name, opts := range map[string]a2aEnvOpts{
		"not enabled": {noA2A: true},
	} {
		t.Run(name, func(t *testing.T) {
			e := newA2AEnv(t, opts)
			resp, err := http.Get(e.base + RouteAgentCard)
			require.NoError(t, err)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
			for _, tok := range []string{a2aTokPeer, ""} {
				assert.Equal(t, http.StatusNotFound, e.openRaw(tok, []byte("{}"), nil).StatusCode)
			}
			assert.Zero(t, e.model.hits())
		})
	}
	t.Run("enabling needs a backend", func(t *testing.T) {
		s := mustNew(t, catFake(t, nil), Options{Tools: []string{"node"}})
		assert.Error(t, s.EnableA2A())
	})
	t.Run("a backend alone mounts nothing", func(t *testing.T) {
		e := newA2AEnv(t, a2aEnvOpts{noA2A: true})
		resp, err := http.Get(e.base + RouteAgentCard)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// A text message for a context whose loop session is gone is rejected, not
// restarted under the same contextId (A2A 1.0.0 section 3.4).
func TestA2AContextWithoutItsSessionIsRejected(t *testing.T) {
	textTo := func(contextID, text string) map[string]any {
		m := userMessage(textPartJSON(text))
		if contextID != "" {
			m["contextId"] = contextID
		}
		return map[string]any{"message": m}
	}
	evict := func(e *a2aEnv, contextID string) {
		a := e.s.Agent()
		a.mu.Lock()
		defer a.mu.Unlock()
		require.Contains(t, a.sessions, contextID)
		delete(a.sessions, contextID) // what the idle TTL or the session cap does
	}

	t.Run("evicted session", func(t *testing.T) {
		e := newA2AEnv(t, a2aEnvOpts{}, sayStep("first answer"), sayStep("must never be asked"))
		_, resp := e.call(a2aTokPeer, "SendMessage", textTo("", "hello"))
		first := taskOf(t, resp)
		require.Equal(t, a2aCompleted, stateOf(first))
		ctxID, firstID := first["contextId"].(string), first["id"].(string)
		require.Equal(t, 1, e.model.hits())

		evict(e, ctxID)
		_, resp = e.call(a2aTokPeer, "SendMessage", textTo(ctxID, "and again"))
		code, tool := rpcErrorCode(t, resp)
		assert.Equal(t, a2aTaskNotFound, code)
		assert.Equal(t, CodeNotFound, tool)
		assert.Equal(t, 1, e.model.hits(), "no model call is made for a conversation that is gone")

		// The A2A context went with it: its earlier task is gone too.
		_, resp = e.call(a2aTokPeer, "GetTask", map[string]any{"id": firstID})
		code, tool = rpcErrorCode(t, resp)
		assert.Equal(t, a2aTaskNotFound, code)
		assert.Equal(t, CodeNotFound, tool)
		// And the contextId is unknown from now on.
		_, resp = e.call(a2aTokPeer, "SendMessage", textTo(ctxID, "still there?"))
		code, _ = rpcErrorCode(t, resp)
		assert.Equal(t, a2aTaskNotFound, code)
	})

	t.Run("a new context starts after the rejection", func(t *testing.T) {
		e := newA2AEnv(t, a2aEnvOpts{}, sayStep("one"), sayStep("two"))
		_, resp := e.call(a2aTokPeer, "SendMessage", textTo("", "hello"))
		ctxID := taskOf(t, resp)["contextId"].(string)
		evict(e, ctxID)
		_, resp = e.call(a2aTokPeer, "SendMessage", textTo(ctxID, "again"))
		rpcErrorCode(t, resp)
		_, resp = e.call(a2aTokPeer, "SendMessage", textTo("", "fresh start"))
		fresh := taskOf(t, resp)
		assert.Equal(t, a2aCompleted, stateOf(fresh))
		assert.NotEqual(t, ctxID, fresh["contextId"])
	})

	t.Run("a context that only ran data-part tasks still creates its session", func(t *testing.T) {
		e := newA2AEnv(t, a2aEnvOpts{}, sayStep("created"))
		task := taskOf(t, e.send(a2aTokPeer, toolPart("pin_ls", map[string]any{"type": "recursive"})))
		require.Equal(t, a2aCompleted, stateOf(task))
		ctxID := task["contextId"].(string)
		_, resp := e.call(a2aTokPeer, "SendMessage", textTo(ctxID, "now a question"))
		second := taskOf(t, resp)
		assert.Equal(t, a2aCompleted, stateOf(second))
		assert.Equal(t, ctxID, second["contextId"])
		assert.Equal(t, 1, e.model.hits())
		// With the session now existing, a third text message continues it.
		assert.Contains(t, e.s.Agent().sessions, ctxID)
	})
}

// With only the headless cli resolver, a destructive call from an a2a
// principal is refused before anything is asked of anyone.
func TestA2ADestructiveWithOnlyTheCLIResolverIsRejected(t *testing.T) {
	e := newA2AEnv(t, a2aEnvOpts{cliOnly: true})
	task := taskOf(t, e.send(a2aTokPeer, toolPart("pin_rm", map[string]any{"path": "/ipfs/" + testCID(t, 1)})))
	require.Equal(t, a2aRejected, stateOf(task))
	art := task["artifacts"].([]any)[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["data"].(map[string]any)
	assert.Equal(t, CodeConfirmUnavailable, art["error"].(map[string]any)["code"])
	assert.Empty(t, e.s.PendingConfirmations(), "no confirmation was opened")
	assert.Empty(t, e.cli.requests, "the cli resolver was never asked")
	assert.True(t, e.pins.has(testCID(t, 1)), "nothing ran")
	validateA2A(t, "Task", task)
}
