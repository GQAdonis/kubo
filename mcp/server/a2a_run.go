//go:build !noagent

package server

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// drive does the work of one task on its own goroutine: it runs the message
// through the agent loop (text) or the named tool (data part), and when a write
// waits for a person it stays to see how the person decided.
func (h *a2aHandler) drive(t *a2aTask, id Identity, w a2aWork) {
	defer h.recoverTask(t)
	var waiting bool
	if w.call {
		t.markStarted(nil)
		t.setStatus(a2aWorking, "", nil, nil)
		waiting = h.callTool(t, id, w)
	} else {
		waiting = h.runAgent(t, id, RunInput{SessionID: t.contextID, Message: w.text, ContinueOnly: w.continueOnly})
	}
	if waiting {
		h.awaitApproval(t, id, w)
	}
}

// errBackendChanged ends a task whose agent loop is gone: a settings write
// moved the backend to a UAR agent after the task began (U6). It is never sent
// to a peer as text.
var errBackendChanged = errors.New("the agent loop is no longer the model backend")

// runAgent runs one step of the agent loop for the task. The loop is looked up
// at each step because a settings write can replace it with a UAR backend
// between steps; a task that finds no loop ends as failed instead of
// dereferencing nil. It does not run an old loop it no longer owns: that would
// send content to a model host the operator has since moved away from.
func (h *a2aHandler) runAgent(t *a2aTask, id Identity, in RunInput) bool {
	ag := h.srv.Agent()
	if ag == nil {
		return h.settleRun(t, nil, errBackendChanged)
	}
	res, err := ag.Run(t.ctx, id, in, h.emitter(t))
	return h.settleRun(t, res, err)
}

// recoverTask turns a panic on a task goroutine into a failed task, so one bad
// task cannot end the node's process. It logs the panic's type only.
func (h *a2aHandler) recoverTask(t *a2aTask) {
	r := recover()
	if r == nil {
		return
	}
	h.log.Error("a2a task panicked", "panic_type", fmt.Sprintf("%T", r))
	t.markStarted(errors.New("the task failed unexpectedly"))
	t.setStatus(a2aFailed, "The agent could not finish this request.", nil, nil)
}

// emitter turns loop events into task updates. It carries no model text and no
// tool arguments or results.
func (h *a2aHandler) emitter(t *a2aTask) func(Event) {
	return func(ev Event) {
		switch ev.Type {
		case EventRunStarted:
			t.markStarted(nil)
			t.setStatus(a2aWorking, "", nil, nil)
		case EventToolCall:
			t.setStatus(a2aWorking, "Calling "+ev.Tool+".", nil, nil)
		}
	}
}

// settleRun records how a run ended. It reports whether the task now waits for
// approval.
func (h *a2aHandler) settleRun(t *a2aTask, res *RunResult, err error) bool {
	if t.ctx.Err() != nil {
		return false // cancelled: the state is already final
	}
	if err != nil {
		if t.markStarted(err) {
			return false // before the run began: begin reports it
		}
		var te *ToolError
		if errors.As(err, &te) && te.Code == CodePermissionDenied {
			t.setStatus(a2aAuthRequired, te.Message, nil, nil)
		} else {
			// The type only: an error text could carry conversation content.
			h.log.Warn("a2a run failed", "error_type", errTypeName(err))
			t.setStatus(a2aFailed, "The agent could not finish this request.", nil, nil)
		}
		return false
	}
	switch res.Outcome {
	case OutcomeSuccess:
		t.addArtifact(a2aArtifact{ArtifactID: "art-" + randomID(6), Name: "reply", Parts: []a2aPart{textPart(res.Final)}})
		t.setStatus(a2aCompleted, "", nil, nil)
	case OutcomeInterrupt:
		iv := res.Interrupts[0]
		return h.inputRequired(t, &a2aPending{confirmationID: iv.ConfirmationID, tool: iv.Tool, text: true, interruptID: iv.ID},
			iv.Message, iv.Class, iv.RequiresNativePrompt, iv.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"))
	default:
		t.setStatus(a2aFailed, "The agent stopped before finishing: "+limitText(res.Reason)+". Nothing further was done.", nil, nil)
	}
	return false
}

func limitText(reason string) string {
	switch reason {
	case LimitMaxSteps:
		return "it reached the step limit"
	case LimitMaxTokens:
		return "it reached the token limit"
	case LimitWallClock:
		return "it ran out of time"
	}
	return "a limit was reached"
}

func errTypeName(err error) string { return fmt.Sprintf("%T", err) }

// inputRequired puts the task in TASK_STATE_INPUT_REQUIRED. It never resolves
// the confirmation.
func (h *a2aHandler) inputRequired(t *a2aTask, p *a2aPending, summary, class string, native bool, expires string) bool {
	info := map[string]any{"confirmation_id": p.confirmationID, "tool": p.tool, "class": class, "expires_at": expires}
	if native {
		info["requires_native_prompt"] = true
	}
	return t.setStatus(a2aInputRequired, "Waiting for approval on the node's machine: "+summary, p, map[string]any{a2aMetaKey: info})
}

// callTool makes the data-part call and records the result. It reports whether
// the task now waits for approval.
func (h *a2aHandler) callTool(t *a2aTask, id Identity, w a2aWork) bool {
	res, err := h.srv.CallTool(t.ctx, id, t.contextID, w.tool, w.args)
	if t.ctx.Err() != nil {
		return false
	}
	if err != nil {
		h.log.Warn("a2a call failed", "error_type", errTypeName(err))
		t.setStatus(a2aFailed, "The call could not be made.", nil, nil)
		return false
	}
	if !res.IsError {
		t.addArtifact(resultArtifact(w.tool, res))
		t.setStatus(a2aCompleted, "", nil, nil)
		return false
	}
	te := errorOf(res)
	if te == nil {
		t.setStatus(a2aFailed, w.tool+" failed.", nil, nil)
		return false
	}
	return h.settleError(t, w, te)
}

// settleError maps a tool error to a task state.
func (h *a2aHandler) settleError(t *a2aTask, w a2aWork, te *ToolError) bool {
	switch te.Code {
	case CodePermissionDenied:
		t.setStatus(a2aAuthRequired, h.notGrantedText(w.tool, te), nil, nil)
		return false
	case CodeConfirmRequired, CodeConfirmPending:
		if c := te.Confirmation; c != nil {
			return h.inputRequired(t, &a2aPending{confirmationID: c.ConfirmationID, tool: w.tool, args: w.args},
				c.Summary.Text, c.Summary.Class, c.RequiresNativePrompt, c.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"))
		}
	case CodeConfirmDeclined, CodeConfirmExpired, CodeConfirmInvalid, CodeConfirmUnavailable, CodeEgressNotAcknowledged:
		t.addArtifact(errorArtifact(w.tool, te))
		t.setStatus(a2aRejected, te.Message, nil, nil)
		return false
	}
	t.addArtifact(errorArtifact(w.tool, te))
	t.setStatus(a2aFailed, te.Message, nil, nil)
	return false
}

// notGrantedText says what grant is missing, without naming the principal.
func (h *a2aHandler) notGrantedText(tool string, te *ToolError) string {
	if te.Details["reason"] != ReasonPrincipalNotGranted {
		return te.Message
	}
	t, ok := h.srv.cat.Tool(tool)
	if !ok {
		return te.Message
	}
	class := map[string]string{contract.ClassNone: "read", contract.ClassWrite: "write", contract.ClassDestructive: "destructive"}[t.Confirmation]
	return tool + " needs a " + class + " grant for group " + t.Group + ", which this principal does not have."
}

// awaitApproval waits for the person's decision on the confirmation the task
// holds and then finishes the work, or rejects it. It decides nothing itself.
func (h *a2aHandler) awaitApproval(t *a2aTask, id Identity, w a2aWork) {
	for {
		p := t.pendingCopy()
		if p == nil {
			return
		}
		state, err := h.srv.AwaitConfirmation(t.ctx, p.confirmationID)
		if t.ctx.Err() != nil {
			return
		}
		if err != nil {
			state = StateExpired // unknown to the broker: gone, which is denial
		}
		if state != StateApproved {
			h.reject(t, p, state)
			return
		}
		var again bool
		if p.text {
			again = h.runAgent(t, id, RunInput{SessionID: t.contextID, Resume: []ResumeEntry{
				{InterruptID: p.interruptID, Status: ResumeResolved, Approved: true},
			}})
		} else {
			t.setStatus(a2aWorking, "", nil, nil)
			again = h.callTool(t, id, a2aWork{call: true, tool: p.tool, args: p.args})
		}
		if !again {
			return
		}
	}
}

// reject ends a task whose confirmation was declined, expired or lost. Nothing
// ran.
func (h *a2aHandler) reject(t *a2aTask, p *a2aPending, state string) {
	pend, ok := h.srv.gate.broker.get(p.confirmationID)
	if !ok {
		pend = Pending{ID: p.confirmationID}
	}
	te := h.srv.gate.outcomeError(p.tool, pend, state)
	h.srv.gate.broker.forget(p.confirmationID)
	t.addArtifact(errorArtifact(p.tool, te))
	t.setStatus(a2aRejected, te.Message, nil, nil)
}

// errorArtifact carries a ToolError, without a confirmation object, so the peer
// has the machine code.
func errorArtifact(tool string, te *ToolError) a2aArtifact {
	clean := *te
	clean.Confirmation = nil
	raw, _ := json.Marshal(toolErrorResult{Error: &clean})
	return a2aArtifact{ArtifactID: "art-" + randomID(6), Name: tool, Parts: []a2aPart{dataPart(raw)}}
}

// resultArtifact turns a tool result into an artifact: the structured result as
// a data part, resource links as url parts (large content stays out of the
// message), and a marker when the content comes from the network.
func resultArtifact(tool string, res *mcp.CallToolResult) a2aArtifact {
	art := a2aArtifact{ArtifactID: "art-" + randomID(6), Name: tool}
	if res.StructuredContent != nil {
		if raw, err := json.Marshal(res.StructuredContent); err == nil {
			art.Parts = append(art.Parts, dataPart(raw))
		}
	}
	for _, c := range res.Content {
		switch v := c.(type) {
		case *mcp.ResourceLink:
			uri := v.URI
			art.Parts = append(art.Parts, a2aPart{URL: &uri, MediaType: v.MIMEType, Filename: v.Name})
		case *mcp.TextContent:
			if res.StructuredContent == nil {
				art.Parts = append(art.Parts, textPart(v.Text))
			}
		}
	}
	if untrusted, _ := res.Meta[contract.UntrustedContentMetaKey].(bool); untrusted {
		art.Metadata = map[string]any{a2aMetaKey: map[string]any{"untrusted_content": true}}
	}
	if len(art.Parts) == 0 {
		art.Parts = []a2aPart{textPart("")}
	}
	return art
}
