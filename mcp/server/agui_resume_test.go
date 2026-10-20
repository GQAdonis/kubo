//go:build !noagent

package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pinAddScript is a model reply asking for pin_add on test CID n, with id.
func pinAddScript(t *testing.T, id string, n int) func(modelRequest) []string {
	return callStep(10, scriptedCall{ID: id, Name: "pin_add", Args: map[string]any{"path": "/ipfs/" + testCID(t, n), "recursive": true}})
}

// startWrite runs a thread up to its write interrupt and returns the interrupt.
func (e *aguiEnv) startWrite(token, thread string) map[string]any {
	e.t.Helper()
	r := e.post(token, aguiBody(thread, "run-1", []any{userMsg("m1", "pin it")}), nil)
	require.Equal(e.t, http.StatusOK, r.Status, r.Body)
	return finishedInterrupt(e.t, r.Events)
}

func (e *aguiEnv) resumeReq(token, thread, interruptID, status string, payload any) *aguiResponse {
	e.t.Helper()
	return e.post(token, aguiBody(thread, "run-2", nil, resumeEntry(interruptID, status, payload)), nil)
}

// ---- writes: the chat's approval counts ----

func TestAGUIWriteWaitsThenRunsOnceAfterTheChatApproves(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc-1", 5), sayStep("Pinned."))
	in := e.startWrite(aguiTokChat, "thread-w")
	assert.False(t, e.pins.has(testCID(t, 5)), "nothing is pinned while the call waits")
	assert.Zero(t, e.d.count("pin/add"))
	meta := in["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)
	assert.Equal(t, "pin_add", meta["tool"])
	assert.Equal(t, "write", meta["class"])
	assert.Equal(t, false, meta["requires_native_prompt"])
	assert.Equal(t, "int-"+meta["confirmation_id"].(string), in["id"])
	assert.Contains(t, in["message"], testCID(t, 5))
	assert.Equal(t, 1, e.model.hits(), "the model is not asked again while the call waits")

	r := e.resumeReq(aguiTokChat, "thread-w", in["id"].(string), "resolved", map[string]any{"approved": true})
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	assert.NotContains(t, r.types(), "TOOL_CALL_START", "a resumed call is not announced again")
	assert.Contains(t, resultContent(t, r.Events, "tc-1"), testCID(t, 5))
	assert.True(t, e.pins.has(testCID(t, 5)))
	assert.Equal(t, 1, e.d.count("pin/add"), "exactly once")

	// The same resume cannot run it a second time.
	again := e.resumeReq(aguiTokChat, "thread-w", in["id"].(string), "resolved", map[string]any{"approved": true})
	assert.Equal(t, http.StatusUnprocessableEntity, again.Status)
	assert.Equal(t, 1, e.d.count("pin/add"))
}

func TestAGUIDeniedAndCancelledWritesNeverReachTheNode(t *testing.T) {
	for name, send := range map[string]func(e *aguiEnv, id string) *aguiResponse{
		"denied": func(e *aguiEnv, id string) *aguiResponse {
			return e.resumeReq(aguiTokChat, "thread-d", id, "resolved", map[string]any{"approved": false})
		},
		"cancelled": func(e *aguiEnv, id string) *aguiResponse {
			return e.resumeReq(aguiTokChat, "thread-d", id, "cancelled", nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc-1", 5), sayStep("Not pinned."))
			in := e.startWrite(aguiTokChat, "thread-d")
			r := send(e, in["id"].(string))
			require.Equal(t, http.StatusOK, r.Status, r.Body)
			assert.Contains(t, resultContent(t, r.Events, "tc-1"), CodeConfirmDeclined)
			assert.Zero(t, e.d.count("pin/add"))
			assert.False(t, e.pins.has(testCID(t, 5)))
			assert.Empty(t, e.s.PendingConfirmations())
		})
	}
}

func TestAGUIExpiryIsDenial(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc-1", 5), sayStep("It expired."), sayStep("Sure."))
	in := e.startWrite(aguiTokChat, "thread-x")
	e.clock.Advance(confirmTTL + time.Second)

	// A late approval does not run the call; the run says it expired.
	r := e.resumeReq(aguiTokChat, "thread-x", in["id"].(string), "resolved", map[string]any{"approved": true})
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	assert.Contains(t, resultContent(t, r.Events, "tc-1"), CodeConfirmExpired)
	assert.Zero(t, e.d.count("pin/add"))

	// Once nothing is open, a new message is accepted.
	next := e.post(aguiTokChat, aguiBody("thread-x", "run-3", []any{userMsg("m2", "thanks")}), nil)
	assert.Equal(t, http.StatusOK, next.Status)
}

func TestAGUINewMessageWaitsForTheOpenInterrupt(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc-1", 5), sayStep("Fresh start."))
	e.startWrite(aguiTokChat, "thread-n")
	r := e.post(aguiTokChat, aguiBody("thread-n", "run-2", []any{userMsg("m2", "never mind")}), nil)
	assert.Equal(t, http.StatusConflict, r.Status)
	assert.Equal(t, 1, e.model.hits())
	e.clock.Advance(confirmTTL + time.Second)
	assert.Equal(t, http.StatusOK, e.post(aguiTokChat, aguiBody("thread-n", "run-3", []any{userMsg("m2", "never mind")}), nil).Status,
		"expiry is denial, so the thread is free again")
}

func TestAGUIReadOnlyPrincipalCannotWrite(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc-1", 5), sayStep("I cannot pin."))
	r := e.post(aguiTokReadUI, aguiBody("thread-ro", "run-1", []any{userMsg("m1", "pin it")}), nil)
	assert.Contains(t, resultContent(t, r.Events, "tc-1"), CodeToolUnavailable)
	assert.Zero(t, e.d.count("pin/add"))
	assert.Empty(t, e.s.PendingConfirmations())
}

// ---- destructive: the native prompt decides ----

func destructiveEnv(t *testing.T, steps ...func(modelRequest) []string) *aguiEnv {
	t.Helper()
	all := append([]func(modelRequest) []string{
		callStep(10, scriptedCall{ID: "tc-rm", Name: "pin_rm", Args: map[string]any{"path": "/ipfs/" + testCID(t, 1)}}),
	}, steps...)
	return newAGUIEnv(t, aguiEnvOpts{}, all...)
}

func TestAGUIDestructiveCallApprovedOnlyInChatDoesNotRun(t *testing.T) {
	e := destructiveEnv(t, sayStep("The pin stays."))
	in := e.startWrite(aguiTokChat, "thread-rm")
	meta := in["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)
	require.Equal(t, true, meta["requires_native_prompt"])
	require.Equal(t, "destructive", meta["class"])
	assert.Contains(t, in["responseSchema"].(map[string]any), "oneOf", "approved:true is not offered")
	assert.Contains(t, in["message"], "system prompt")

	r := e.resumeReq(aguiTokChat, "thread-rm", in["id"].(string), "resolved", map[string]any{"approved": true})
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status, "a chat approval is refused for a call that needs the native prompt")
	assert.True(t, e.pins.has(testCID(t, 1)), "the pin remains")
	assert.Zero(t, e.d.count("pin/rm"))
	require.Len(t, e.s.PendingConfirmations(), 1, "the confirmation is still pending: nothing was resolved")

	// The native prompt is denied: the pin remains.
	waiter := e.openWait(aguiTokChat, "thread-rm", in["id"].(string))
	first, ok := waiter.sse.next()
	require.True(t, ok)
	assert.Equal(t, "RUN_STARTED", first["type"])
	_, resolve := e.presence.last(t)
	require.NoError(t, resolve(false))
	rest := waiter.sse.all()
	assert.Contains(t, resultContent(t, rest, "tc-rm"), CodeConfirmDeclined)
	assert.True(t, e.pins.has(testCID(t, 1)))
	assert.Zero(t, e.d.count("pin/rm"))
}

func TestAGUIWaitObservesTheSystemPromptApproving(t *testing.T) {
	e := destructiveEnv(t, sayStep("Removed."))
	in := e.startWrite(aguiTokChat, "thread-rm")
	waiter := e.openWait(aguiTokChat, "thread-rm", in["id"].(string))
	first, _ := waiter.sse.next()
	assert.Equal(t, "RUN_STARTED", first["type"])
	_, resolve := e.presence.last(t)
	require.NoError(t, resolve(true))
	rest := waiter.sse.all()
	assert.Equal(t, 1, e.d.count("pin/rm"), "runs once after the system prompt approved")
	assert.False(t, e.pins.has(testCID(t, 1)))
	assert.NotContains(t, resultContent(t, rest, "tc-rm"), "error")
	assert.Equal(t, "RUN_FINISHED", rest[len(rest)-1]["type"])
	assert.Equal(t, 1, len(findEvents(rest, "RUN_FINISHED")), "the loop's own RUN_STARTED is dropped; the stream has one")
	assert.Empty(t, findEvents(rest, "RUN_STARTED"))
}

func TestAGUIWaitOnAWriteInterruptIsRefused(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc-1", 5))
	in := e.startWrite(aguiTokChat, "thread-w")
	r := e.resumeReq(aguiTokChat, "thread-w", in["id"].(string), "resolved", map[string]any{"wait": true})
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status)
	assert.Len(t, e.s.PendingConfirmations(), 1)
}

func TestAGUIDenyWhileWaitingEndsTheWatchWithDeclined(t *testing.T) {
	e := destructiveEnv(t, sayStep("Understood."))
	in := e.startWrite(aguiTokChat, "thread-rm")
	waiter := e.openWait(aguiTokChat, "thread-rm", in["id"].(string))
	first, _ := waiter.sse.next()
	require.Equal(t, "RUN_STARTED", first["type"])

	deny := e.resumeReq(aguiTokChat, "thread-rm", in["id"].(string), "resolved", map[string]any{"approved": false})
	require.Equal(t, http.StatusOK, deny.Status)
	assert.Equal(t, []string{"RUN_STARTED", "RUN_FINISHED"}, deny.types(), "the deny request only records the decision; omitted capabilities advertise no catalog")
	assert.Equal(t, map[string]any{"type": "success"}, deny.Events[1]["outcome"])

	rest := waiter.sse.all()
	assert.Contains(t, resultContent(t, rest, "tc-rm"), CodeConfirmDeclined, "the open watch reports it")
	assert.True(t, e.pins.has(testCID(t, 1)))
	assert.Zero(t, e.d.count("pin/rm"))

	// A late approval from the system prompt cannot run a denied call.
	_, resolve := e.presence.last(t)
	assert.ErrorIs(t, resolve(true), ErrConfirmationResolved)
}

func TestAGUIDroppedWatchLeavesTheConfirmationAndANewWaitPicksItUp(t *testing.T) {
	e := destructiveEnv(t, sayStep("Removed."))
	in := e.startWrite(aguiTokChat, "thread-rm")

	ctx, cancel := context.WithCancel(t.Context())
	resp := e.open(ctx, aguiTokChat, aguiBody("thread-rm", "run-2", nil, resumeEntry(in["id"].(string), "resolved", map[string]any{"wait": true})), nil)
	stream := newSSE(resp.Body)
	first, _ := stream.next()
	require.Equal(t, "RUN_STARTED", first["type"])
	cancel() // the client goes away
	require.Eventually(t, func() bool {
		_, err := e.s.Agent().OpenInterrupt(aguiIdentity("chat", KindUI, nil, nil, nil), "thread-rm")
		return err == nil
	}, 5*time.Second, 5*time.Millisecond)
	require.Len(t, e.s.PendingConfirmations(), 1, "the confirmation stays pending")
	assert.Zero(t, e.d.count("pin/rm"))

	again := e.openWait(aguiTokChat, "thread-rm", in["id"].(string))
	f2, _ := again.sse.next()
	require.Equal(t, "RUN_STARTED", f2["type"])
	_, resolve := e.presence.last(t)
	require.NoError(t, resolve(true))
	rest := again.sse.all()
	assert.NotContains(t, resultContent(t, rest, "tc-rm"), "error")
	assert.Equal(t, 1, e.d.count("pin/rm"), "once, by the second watch")
}

func TestAGUINewestWatchTakesOver(t *testing.T) {
	e := destructiveEnv(t, sayStep("Removed."))
	in := e.startWrite(aguiTokChat, "thread-rm")
	first := e.openWait(aguiTokChat, "thread-rm", in["id"].(string))
	f1, _ := first.sse.next()
	require.Equal(t, "RUN_STARTED", f1["type"])
	second := e.openWait(aguiTokChat, "thread-rm", in["id"].(string))
	f2, _ := second.sse.next()
	require.Equal(t, "RUN_STARTED", f2["type"])

	// No catalog capability was offered; replacement ends the older watch
	// without a presentation event or operation.
	assert.Empty(t, first.sse.all())
	_, resolve := e.presence.last(t)
	require.NoError(t, resolve(true))
	rest := second.sse.all()
	assert.NotContains(t, resultContent(t, rest, "tc-rm"), "error")
	assert.Equal(t, 1, e.d.count("pin/rm"))
}

// waiter is an open {wait: true} stream.
type waiter struct{ sse *sse }

func (e *aguiEnv) openWait(token, thread, interruptID string) *waiter {
	e.t.Helper()
	resp := e.open(e.t.Context(), token, aguiBody(thread, "run-w", nil, resumeEntry(interruptID, "resolved", map[string]any{"wait": true})), nil)
	require.Equal(e.t, http.StatusOK, resp.StatusCode)
	return &waiter{sse: newSSE(resp.Body)}
}

// ---- L2: nobody resolves what is not theirs ----

func TestAGUIAnotherPrincipalCannotAnswerAThreadsInterrupt(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc-1", 5), sayStep("Pinned."))
	in := e.startWrite(aguiTokChat, "thread-own")
	id := in["id"].(string)

	for name, payload := range map[string]struct {
		status  string
		payload any
	}{
		"approve": {"resolved", map[string]any{"approved": true}},
		"deny":    {"resolved", map[string]any{"approved": false}},
		"cancel":  {"cancelled", nil},
		"wait":    {"resolved", map[string]any{"wait": true}},
	} {
		r := e.resumeReq(aguiTokChat2, "thread-own", id, payload.status, payload.payload)
		assert.Equal(t, http.StatusNotFound, r.Status, name)
		assert.Equal(t, CodeNotFound, r.errCode(t), name)
	}
	pending := e.s.PendingConfirmations()
	require.Len(t, pending, 1, "nothing was resolved by the other principal")
	assert.Equal(t, StatePending, pending[0].State)
	assert.Zero(t, e.d.count("pin/add"))

	// The owner still can.
	r := e.resumeReq(aguiTokChat, "thread-own", id, "resolved", map[string]any{"approved": true})
	require.Equal(t, http.StatusOK, r.Status)
	assert.Equal(t, 1, e.d.count("pin/add"))
}

func TestAGUIAnotherThreadsInterruptCannotBeNamed(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc-a", 5), pinAddScript(t, "tc-b", 6), sayStep("ok"), sayStep("ok"))
	a := e.startWrite(aguiTokChat, "thread-a")
	b := e.startWrite(aguiTokChat, "thread-b")
	require.NotEqual(t, a["id"], b["id"])
	require.Len(t, e.s.PendingConfirmations(), 2)

	// The same principal names thread B's interrupt on thread A.
	r := e.resumeReq(aguiTokChat, "thread-a", b["id"].(string), "resolved", map[string]any{"approved": true})
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status)
	assert.Len(t, e.s.PendingConfirmations(), 2, "resolves nothing")
	assert.Zero(t, e.d.count("pin/add"))
	for _, id := range []string{"int-cf_does-not-exist", ""} {
		r = e.resumeReq(aguiTokChat, "thread-a", id, "resolved", map[string]any{"approved": true})
		assert.Equal(t, http.StatusUnprocessableEntity, r.Status, id)
	}
	assert.Zero(t, e.d.count("pin/add"))

	// Each thread answers its own.
	require.Equal(t, http.StatusOK, e.resumeReq(aguiTokChat, "thread-b", b["id"].(string), "resolved", map[string]any{"approved": true}).Status)
	assert.True(t, e.pins.has(testCID(t, 6)))
	assert.False(t, e.pins.has(testCID(t, 5)))
}

func TestAGUICannotApproveAConfirmationOfAnMCPSession(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc-a", 5), sayStep("ok"))
	// The chat principal also calls /mcp. Its client cannot elicit, so the write
	// waits for the system prompt.
	transport := &mcp.StreamableClientTransport{Endpoint: e.base + RouteMCP, HTTPClient: &http.Client{Transport: bearerTransport{token: aguiTokChat}}}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "mcp-ui", Version: "0"}, nil).Connect(t.Context(), transport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "pin_add", Arguments: map[string]any{"path": "/ipfs/" + testCID(t, 77)}})
	require.NoError(t, err)
	mcpConf := confirmationOf(t, res, CodeConfirmRequired).Confirmation
	assert.NotContains(t, mcpConf.Resolvers, ResolverAGUIInterrupt, "no chat is waiting on an MCP session's call... ")

	in := e.startWrite(aguiTokChat, "thread-chat")
	r := e.resumeReq(aguiTokChat, "thread-chat", "int-"+mcpConf.ConfirmationID, "resolved", map[string]any{"approved": true})
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status)
	for _, p := range e.s.PendingConfirmations() {
		assert.Equal(t, StatePending, p.State, "the MCP session's confirmation is untouched")
	}
	assert.Len(t, e.s.PendingConfirmations(), 2)
	assert.Zero(t, e.d.count("pin/add"))
	_ = in
}

func TestAGUIResumeNeedsTheChatPrincipal(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc-1", 5))
	in := e.startWrite(aguiTokChat, "thread-w")
	for _, tok := range []string{aguiTokHuman, aguiTokPeer} {
		r := e.resumeReq(tok, "thread-w", in["id"].(string), "resolved", map[string]any{"approved": true})
		assert.Equal(t, http.StatusForbidden, r.Status)
	}
	assert.Len(t, e.s.PendingConfirmations(), 1)
	assert.Zero(t, e.d.count("pin/add"))
}

// ---- the chat resolver in the gate ----

func TestAGUIChatResolverIsListedForTheChatPrincipalOnly(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc-1", 5))
	in := e.startWrite(aguiTokChat, "thread-l")
	assert.NotEmpty(t, in)
	var chatPending Pending
	for _, p := range e.s.PendingConfirmations() {
		chatPending = p
	}
	require.NotEmpty(t, chatPending.ID)

	// A human-mcp principal on /mcp is never offered the chat as a resolver.
	transport := &mcp.StreamableClientTransport{Endpoint: e.base + RouteMCP, HTTPClient: &http.Client{Transport: bearerTransport{token: aguiTokHuman}}}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "human", Version: "0"}, nil).Connect(t.Context(), transport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "pin_add", Arguments: map[string]any{"path": "/ipfs/" + testCID(t, 78)}})
	require.NoError(t, err)
	conf := confirmationOf(t, res, CodeConfirmRequired).Confirmation
	assert.Equal(t, []string{ResolverLinuxPolkit}, conf.Resolvers)
}

func TestAGUIMountDoesNotMakeNonChatWritesWaitForNothing(t *testing.T) {
	// No native resolver at all: the chat resolver must not turn a refusal into a
	// confirmation that nobody can answer, for a principal that is not the chat
	// in a chat thread.
	e := newAGUIEnv(t, aguiEnvOpts{noPresence: true}, pinAddScript(t, "tc-1", 5))
	for name, tok := range map[string]string{"human-mcp": aguiTokHuman, "ui over MCP": aguiTokChat} {
		transport := &mcp.StreamableClientTransport{Endpoint: e.base + RouteMCP, HTTPClient: &http.Client{Transport: bearerTransport{token: tok}}}
		cs, err := mcp.NewClient(&mcp.Implementation{Name: name, Version: "0"}, nil).Connect(t.Context(), transport, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = cs.Close() })
		res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "pin_add", Arguments: map[string]any{"path": "/ipfs/" + testCID(t, 79)}})
		require.NoError(t, err)
		te := decodeToolError(t, res)
		assert.Equal(t, CodeConfirmUnavailable, te.Code, name)
	}
	assert.Empty(t, e.s.PendingConfirmations())

	// In the chat itself the interrupt works without any native prompt.
	in := e.startWrite(aguiTokChat, "thread-chat-only")
	assert.NotEmpty(t, in)
	assert.Len(t, e.s.PendingConfirmations(), 1)
}
