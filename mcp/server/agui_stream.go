//go:build !noagent

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// aguiStream turns the loop's events into AG-UI events on one SSE response.
// Nothing is written, not even the headers, until the first event, so a request
// that fails before the run starts can still answer with an HTTP status.
type aguiStream struct {
	w  http.ResponseWriter
	rc *http.ResponseController

	// a2ui builds the A2UI surface of each tool result (nil: emit none);
	// egressAck is the principal's egress_ack, named on egress-rule results.
	a2ui      *a2uiEmitter
	egressAck []string

	threadID, runID string

	// started is true once the SSE headers went out. runStarted is true once
	// RUN_STARTED did.
	started    bool
	runStarted bool

	// history is what MESSAGES_SNAPSHOT carries: the client's messages as sent,
	// then this run's assistant replies and tool results. dirty says it changed
	// since the last snapshot.
	history []any
	dirty   bool

	// textID is the open assistant text message, text what it has said so far.
	textID string
	text   strings.Builder
}

func newAGUIStream(w http.ResponseWriter, in *aguiInput, a2ui *a2uiEmitter, egressAck []string) *aguiStream {
	history := make([]any, 0, len(in.Messages)+4)
	for _, m := range in.Messages {
		history = append(history, m)
	}
	return &aguiStream{w: w, rc: http.NewResponseController(w), a2ui: a2ui, egressAck: egressAck, threadID: in.ThreadID, runID: in.RunID, history: history}
}

// send writes one event. A write error means the client went away; the run's
// context ends with the request, so there is nothing more to do here.
func (s *aguiStream) send(e aguiEvent) {
	if !s.started {
		h := s.w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("X-Accel-Buffering", "no")
		s.w.WriteHeader(http.StatusOK)
		s.started = true
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", b); err != nil {
		return
	}
	_ = s.rc.Flush()
}

// keepalive writes an SSE comment so a proxy does not drop an idle stream.
func (s *aguiStream) keepalive() {
	if _, err := fmt.Fprint(s.w, ": keepalive\n\n"); err == nil {
		_ = s.rc.Flush()
	}
}

// begin sends RUN_STARTED once, then the one-per-run A2UI catalog
// advertisement when agent.a2ui is on (the emitter is nil when it is off).
func (s *aguiStream) begin() {
	if s.runStarted {
		return
	}
	s.runStarted = true
	s.send(evRunStarted(s.threadID, s.runID))
	if s.a2ui != nil {
		s.send(s.a2ui.catalogAdvertisement())
	}
}

// finishSuccess ends a run that did nothing but observe or record a decision.
func (s *aguiStream) finishSuccess() {
	s.send(evRunFinished(s.threadID, s.runID, map[string]any{"type": "success"}))
}

func newMessageID() string { return "msg-" + randomID(9) }

// handle is the loop's emit callback.
func (s *aguiStream) handle(ev Event) {
	switch ev.Type {
	case EventRunStarted:
		s.begin()
	case EventText:
		s.onText(ev.Text)
	case EventReply:
		s.onReply(ev.Calls)
	case EventToolResult:
		s.onToolResult(ev)
	case EventRunFinished:
		s.onFinished(ev.Result)
	}
	// EventToolCall repeats what EventReply carried, and EventInterrupt what
	// the final RunResult carries.
}

func (s *aguiStream) onText(delta string) {
	if delta == "" {
		return
	}
	if s.textID == "" {
		s.textID = newMessageID()
		s.send(aguiEvent{"type": "TEXT_MESSAGE_START", "messageId": s.textID, "role": "assistant"})
	}
	s.text.WriteString(delta)
	s.send(aguiEvent{"type": "TEXT_MESSAGE_CONTENT", "messageId": s.textID, "delta": delta})
}

// closeText ends the open text message and returns its ID and text.
func (s *aguiStream) closeText() (id, content string) {
	if s.textID == "" {
		return "", ""
	}
	id, content = s.textID, s.text.String()
	s.send(aguiEvent{"type": "TEXT_MESSAGE_END", "messageId": id})
	s.textID = ""
	s.text.Reset()
	return id, content
}

// onReply announces every call of a completed reply, in order, then snapshots
// the messages so the client holds the assistant message that the calls name as
// their parent.
func (s *aguiStream) onReply(calls []CallInfo) {
	msgID, content := s.closeText()
	if msgID == "" {
		msgID = newMessageID()
	}
	toolCalls := make([]any, 0, len(calls))
	for _, c := range calls {
		s.send(aguiEvent{"type": "TOOL_CALL_START", "toolCallId": c.ID, "toolCallName": c.Tool, "parentMessageId": msgID})
		if c.Arguments != "" {
			s.send(aguiEvent{"type": "TOOL_CALL_ARGS", "toolCallId": c.ID, "delta": c.Arguments})
		}
		s.send(aguiEvent{"type": "TOOL_CALL_END", "toolCallId": c.ID})
		toolCalls = append(toolCalls, aguiToolCall(c))
	}
	msg := map[string]any{"id": msgID, "role": "assistant", "toolCalls": toolCalls}
	if content != "" {
		msg["content"] = content
	}
	s.history = append(s.history, msg)
	s.dirty = true
	s.snapshot()
}

func (s *aguiStream) onToolResult(ev Event) {
	id := newMessageID()
	s.send(aguiEvent{"type": "TOOL_CALL_RESULT", "messageId": id, "toolCallId": ev.ToolCallID, "role": "tool", "content": ev.Content})
	s.history = append(s.history, map[string]any{"id": id, "role": "tool", "toolCallId": ev.ToolCallID, "content": ev.Content})
	s.dirty = true
	if s.a2ui != nil {
		for _, e := range s.a2ui.surfaceEvents(ev, s.egressAck) {
			s.send(e)
		}
	}
}

// snapshot sends MESSAGES_SNAPSHOT when the messages changed since the last one.
func (s *aguiStream) snapshot() {
	if !s.dirty {
		return
	}
	s.dirty = false
	s.send(aguiEvent{"type": "MESSAGES_SNAPSHOT", "messages": s.history})
}

func (s *aguiStream) onFinished(res *RunResult) {
	s.closeText()
	switch {
	case res == nil:
		s.runError(errors.New("run ended without a result"))
	case res.Outcome == OutcomeInterrupt && len(res.Interrupts) == 1:
		// State needed to resume goes out before the run that carries the
		// interrupt finishes.
		s.snapshot()
		s.send(evRunFinished(s.threadID, s.runID, map[string]any{
			"type":       "interrupt",
			"interrupts": []any{aguiInterrupt(res.Interrupts[0])},
		}))
	case res.Outcome == OutcomeLimit:
		s.send(evRunError(limitMessage(res.Reason), res.Reason))
	default:
		s.finishSuccess()
	}
}

// runError ends the stream with RUN_ERROR for a failure after the run began.
// The text is fixed per cause: an error from the model backend can quote the
// conversation, so it is not forwarded.
func (s *aguiStream) runError(err error) {
	s.closeText()
	var llm *LLMError
	switch {
	case errors.As(err, &llm):
		s.send(evRunError(fmt.Sprintf("The model backend answered HTTP %d.", llm.Status), "backend_error"))
	case errors.Is(err, ErrSessionFull):
		s.send(evRunError(ErrSessionFull.Error(), "session_full"))
	case errors.Is(err, ErrSessionBusy):
		s.send(evRunError(ErrSessionBusy.Error(), "session_busy"))
	case errors.Is(err, ErrInterruptOpen):
		s.send(evRunError(ErrInterruptOpen.Error(), "interrupt_open"))
	default:
		s.send(evRunError("The model backend could not be reached or failed.", "backend_error"))
	}
}
