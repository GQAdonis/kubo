//go:build !noagent

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Limits of the AG-UI input. IDs follow the loop's session ID bound.
const maxAGUIIDLen = maxSessionIDLen

// errAGUIInput is a request the endpoint refuses with 422 before any run. Its
// text is fixed per cause and never repeats what the client sent.
type errAGUIInput string

func (e errAGUIInput) Error() string { return string(e) }

// aguiInput is the part of RunAgentInput the endpoint reads. State, context,
// tools and forwardedProps are accepted and ignored: the server offers only the
// contract tools the principal may use and keeps no client state.
type aguiInput struct {
	ThreadID string
	RunID    string
	// Messages are the client's messages as sent, kept verbatim so the
	// snapshots can hand them back. Only the last one is ever read as input.
	Messages []json.RawMessage
	// Resume is nil when the field is absent or empty.
	Resume []aguiResume
}

// aguiResume is one validated resume entry.
type aguiResume struct {
	InterruptID string
	Status      string
	// Approved is the answer of a resolved entry that holds {approved}; Wait is
	// set instead for {wait: true}.
	Approved bool
	Wait     bool
}

type aguiInputWire struct {
	ThreadID string            `json:"threadId"`
	RunID    string            `json:"runId"`
	Messages []json.RawMessage `json:"messages"`
	Resume   []struct {
		InterruptID string          `json:"interruptId"`
		Status      string          `json:"status"`
		Payload     json.RawMessage `json:"payload"`
	} `json:"resume"`
}

// readAGUIInput decodes and validates a RunAgentInput. A body over the
// listener's limit is ErrMessageTooLarge, anything else wrong is errAGUIInput.
func readAGUIInput(body io.Reader) (*aguiInput, error) {
	var wire aguiInputWire
	if err := json.NewDecoder(body).Decode(&wire); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return nil, ErrMessageTooLarge
		}
		return nil, errAGUIInput("the body is not a RunAgentInput (JSON object expected)")
	}
	if err := validAGUIID(wire.ThreadID); err != nil {
		return nil, errAGUIInput("threadId must be 1 to 128 characters")
	}
	if err := validAGUIID(wire.RunID); err != nil {
		return nil, errAGUIInput("runId must be 1 to 128 characters")
	}
	in := &aguiInput{ThreadID: wire.ThreadID, RunID: wire.RunID, Messages: wire.Messages}
	if len(wire.Resume) == 0 {
		return in, nil
	}
	if len(wire.Resume) != 1 {
		return nil, errAGUIInput("resume must hold exactly one entry: a run carries at most one interrupt")
	}
	e := wire.Resume[0]
	if e.InterruptID == "" || len(e.InterruptID) > 2*maxAGUIIDLen {
		return nil, errAGUIInput("resume names no interrupt")
	}
	entry := aguiResume{InterruptID: e.InterruptID, Status: e.Status}
	hasPayload := len(bytes.TrimSpace(e.Payload)) > 0 && !bytes.Equal(bytes.TrimSpace(e.Payload), []byte("null"))
	switch e.Status {
	case ResumeCancelled:
		if hasPayload {
			return nil, errAGUIInput("a cancelled resume carries no payload")
		}
	case ResumeResolved:
		if !hasPayload {
			return nil, errAGUIInput("a resolved resume needs a payload: {approved: boolean} or {wait: true}")
		}
		if err := entry.decodePayload(e.Payload); err != nil {
			return nil, err
		}
	default:
		return nil, errAGUIInput(`resume status must be "resolved" or "cancelled"`)
	}
	in.Resume = []aguiResume{entry}
	return in, nil
}

func validAGUIID(id string) error {
	if id == "" || len(id) > maxAGUIIDLen {
		return errors.New("bad id")
	}
	return nil
}

// decodePayload accepts exactly {"approved": bool} or {"wait": true}.
func (e *aguiResume) decodePayload(raw json.RawMessage) error {
	bad := errAGUIInput("the resume payload must be {approved: boolean} or {wait: true}")
	var p map[string]json.RawMessage
	if err := json.Unmarshal(raw, &p); err != nil || len(p) != 1 {
		return bad
	}
	if v, ok := p["approved"]; ok {
		if json.Unmarshal(v, &e.Approved) != nil { // a non-boolean fails here
			return bad
		}
		return nil
	}
	if v, ok := p["wait"]; ok {
		var wait bool
		if json.Unmarshal(v, &wait) != nil || !wait {
			return bad
		}
		e.Wait = true
		return nil
	}
	return bad
}

// lastUserText returns the text of the last message, which must be a user
// message. Earlier messages, and any assistant or tool message the client
// sends, are never read: the server's own thread history is what the model
// sees, so a client cannot forge a tool result into it.
func (in *aguiInput) lastUserText() (string, error) {
	if len(in.Messages) == 0 {
		return "", errAGUIInput("messages is empty: send the user's message")
	}
	var m struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(in.Messages[len(in.Messages)-1], &m); err != nil || m.Role != "user" {
		return "", errAGUIInput("the last message must be a user message")
	}
	text := userText(m.Content)
	if strings.TrimSpace(text) == "" {
		return "", errAGUIInput("the user message has no text")
	}
	return text, nil
}

// userText reads a user message's content: a string, or the text parts of a
// multimodal array. Other parts (images, audio, files) are not sent to the
// model.
func userText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var texts []string
	for _, p := range parts {
		if p.Type == "text" && p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// aguiHTTPStatus maps an error that happens before the stream began to its
// HTTP status (design.md, "Errors before the stream").
func aguiHTTPStatus(err error) (status int, code string, retry bool) {
	var (
		input errAGUIInput
		te    *ToolError
		ub    *uarBackendError
	)
	switch {
	case errors.As(err, &ub):
		return http.StatusBadGateway, CodeInternal, true
	case errors.As(err, &te):
		return http.StatusForbidden, te.Code, false
	case errors.As(err, &input),
		errors.Is(err, ErrEmptyRun), errors.Is(err, ErrBadResume),
		errors.Is(err, ErrNoOpenInterrupt), errors.Is(err, ErrUnknownInterrupt),
		errors.Is(err, ErrNativePromptRequired):
		return http.StatusUnprocessableEntity, CodeInvalidArgument, false
	case errors.Is(err, ErrMessageTooLarge):
		return http.StatusRequestEntityTooLarge, CodeSizeLimitExceeded, false
	case errors.Is(err, ErrSessionNotFound):
		return http.StatusNotFound, CodeNotFound, false
	case errors.Is(err, ErrSessionBusy):
		return http.StatusConflict, CodeSessionBusy, true
	case errors.Is(err, ErrBackendChanged):
		return http.StatusConflict, CodeFailedPrecondition, false
	case errors.Is(err, ErrInterruptOpen), errors.Is(err, ErrSessionFull):
		return http.StatusConflict, CodeFailedPrecondition, false
	case errors.Is(err, ErrRateLimited):
		return http.StatusTooManyRequests, CodeResourceExhausted, true
	case errors.Is(err, ErrTooManySessions):
		return http.StatusServiceUnavailable, CodeResourceExhausted, true
	}
	return http.StatusInternalServerError, CodeInternal, false
}

// aguiPublicMessage is the text of an error body: the error's own fixed text
// for the known causes, a generic one otherwise.
func aguiPublicMessage(err error, status int) string {
	if status == http.StatusInternalServerError {
		return "the request failed"
	}
	return err.Error()
}

func writeAGUIError(w http.ResponseWriter, status int, code, msg string, retry bool) {
	writeAGUIToolError(w, status, &ToolError{Code: code, Message: msg, Retryable: retry})
}

func writeAGUIToolError(w http.ResponseWriter, status int, te *ToolError) {
	clean := *te
	clean.Confirmation = nil
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(toolErrorResult{Error: &clean})
}

// ---- events ----

// aguiEvent is one AG-UI event as it goes on the wire.
type aguiEvent = map[string]any

func evRunStarted(thread, run string) aguiEvent {
	return aguiEvent{"type": "RUN_STARTED", "threadId": thread, "runId": run}
}

func evRunFinished(thread, run string, outcome map[string]any) aguiEvent {
	return aguiEvent{"type": "RUN_FINISHED", "threadId": thread, "runId": run, "outcome": outcome}
}

func evRunError(msg, code string) aguiEvent {
	e := aguiEvent{"type": "RUN_ERROR", "message": msg}
	if code != "" {
		e["code"] = code
	}
	return e
}

// aguiToolCall is a tool call inside an assistant message.
func aguiToolCall(c CallInfo) map[string]any {
	return map[string]any{"id": c.ID, "type": "function", "function": map[string]any{"name": c.Tool, "arguments": c.Arguments}}
}

// aguiInterrupt maps the loop's interrupt (design.md, "Interrupt mapping").
// Resolvers are not sent.
func aguiInterrupt(in Interrupt) map[string]any {
	msg := in.Message
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"approved": map[string]any{"type": "boolean"}},
		"required":   []string{"approved"},
	}
	if in.RequiresNativePrompt {
		msg = strings.TrimRight(strings.TrimSpace(msg), ".") + ". Confirm in the system prompt; you can deny here."
		schema = map[string]any{"oneOf": []any{
			map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{"approved": map[string]any{"const": false}},
				"required":   []string{"approved"},
			},
			map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{"wait": map[string]any{"const": true}},
				"required":   []string{"wait"},
			},
		}}
	}
	out := map[string]any{
		"id":             in.ID,
		"reason":         "tool_call",
		"message":        msg,
		"toolCallId":     in.ToolCallID,
		"responseSchema": schema,
		"metadata": map[string]any{"ipfs-mcp": map[string]any{
			"confirmation_id":        in.ConfirmationID,
			"tool":                   in.Tool,
			"class":                  in.Class,
			"requires_native_prompt": in.RequiresNativePrompt,
		}},
	}
	if !in.ExpiresAt.IsZero() {
		out["expiresAt"] = in.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return out
}

// limitMessage words a RUN_ERROR for a run the loop stopped at a limit. AG-UI
// has no limit outcome.
func limitMessage(reason string) string {
	switch reason {
	case LimitMaxSteps:
		return "The run stopped: it reached the step limit."
	case LimitMaxTokens:
		return "The run stopped: it reached the token budget."
	case LimitWallClock:
		return "The run stopped: it reached the time limit."
	}
	return fmt.Sprintf("The run stopped: limit %s.", reason)
}
