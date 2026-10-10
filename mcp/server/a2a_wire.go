//go:build !noagent

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Wire types of the A2A 1.0 JSON-RPC binding, hand-written for the subset ipfs
// mcp serves (mcp/server/a2a/SOURCES.md). Field names are the ProtoJSON
// camelCase names of specification/a2a.proto; enums are their string names.

// Task states (TaskState). Only the values this server produces are named.
const (
	a2aSubmitted     = "TASK_STATE_SUBMITTED"
	a2aWorking       = "TASK_STATE_WORKING"
	a2aCompleted     = "TASK_STATE_COMPLETED"
	a2aFailed        = "TASK_STATE_FAILED"
	a2aCanceled      = "TASK_STATE_CANCELED"
	a2aInputRequired = "TASK_STATE_INPUT_REQUIRED"
	a2aRejected      = "TASK_STATE_REJECTED"
	a2aAuthRequired  = "TASK_STATE_AUTH_REQUIRED"
)

// Roles (Role).
const (
	a2aRoleUser  = "ROLE_USER"
	a2aRoleAgent = "ROLE_AGENT"
)

// a2aTerminal reports whether a task in the state never changes again.
func a2aTerminal(state string) bool {
	switch state {
	case a2aCompleted, a2aFailed, a2aCanceled, a2aRejected:
		return true
	}
	return false
}

// a2aMetaKey is the metadata key under which this server puts its own fields.
const a2aMetaKey = "ipfs-mcp"

// a2aPart is one Part. Exactly one of Text, Raw, URL and Data is set in a valid
// part; the server produces only text, data and url parts.
type a2aPart struct {
	Text      *string         `json:"text,omitempty"`
	Raw       *string         `json:"raw,omitempty"`
	URL       *string         `json:"url,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Filename  string          `json:"filename,omitempty"`
	MediaType string          `json:"mediaType,omitempty"`
	Metadata  map[string]any  `json:"metadata,omitempty"`
}

func textPart(s string) a2aPart { return a2aPart{Text: &s} }

func dataPart(raw json.RawMessage) a2aPart {
	return a2aPart{Data: raw, MediaType: "application/json"}
}

// a2aMessage is one Message.
type a2aMessage struct {
	MessageID        string         `json:"messageId"`
	ContextID        string         `json:"contextId,omitempty"`
	TaskID           string         `json:"taskId,omitempty"`
	Role             string         `json:"role"`
	Parts            []a2aPart      `json:"parts"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	Extensions       []string       `json:"extensions,omitempty"`
	ReferenceTaskIDs []string       `json:"referenceTaskIds,omitempty"`
}

// a2aStatus is a TaskStatus.
type a2aStatus struct {
	State     string      `json:"state"`
	Message   *a2aMessage `json:"message,omitempty"`
	Timestamp string      `json:"timestamp,omitempty"`
}

// a2aArtifact is an Artifact.
type a2aArtifact struct {
	ArtifactID  string         `json:"artifactId"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Parts       []a2aPart      `json:"parts"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// a2aTaskView is a Task as sent. History is never kept, so it is never sent.
type a2aTaskView struct {
	ID        string         `json:"id"`
	ContextID string         `json:"contextId"`
	Status    a2aStatus      `json:"status"`
	Artifacts []a2aArtifact  `json:"artifacts,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// a2aStatusUpdate is a TaskStatusUpdateEvent.
type a2aStatusUpdate struct {
	TaskID    string         `json:"taskId"`
	ContextID string         `json:"contextId"`
	Status    a2aStatus      `json:"status"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// a2aArtifactUpdate is a TaskArtifactUpdateEvent.
type a2aArtifactUpdate struct {
	TaskID    string      `json:"taskId"`
	ContextID string      `json:"contextId"`
	Artifact  a2aArtifact `json:"artifact"`
	Append    bool        `json:"append,omitempty"`
	LastChunk bool        `json:"lastChunk,omitempty"`
}

// a2aStreamResponse is a StreamResponse: one of its fields is set.
type a2aStreamResponse struct {
	Task           *a2aTaskView       `json:"task,omitempty"`
	Message        *a2aMessage        `json:"message,omitempty"`
	StatusUpdate   *a2aStatusUpdate   `json:"statusUpdate,omitempty"`
	ArtifactUpdate *a2aArtifactUpdate `json:"artifactUpdate,omitempty"`
}

// a2aSendResponse is a SendMessageResponse.
type a2aSendResponse struct {
	Task *a2aTaskView `json:"task"`
}

func a2aTimestamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

// agentMessage builds a server message with one text part.
func agentMessage(contextID, taskID, text string) *a2aMessage {
	return &a2aMessage{
		MessageID: "m-" + randomID(9),
		ContextID: contextID,
		TaskID:    taskID,
		Role:      a2aRoleAgent,
		Parts:     []a2aPart{textPart(text)},
	}
}

// JSON-RPC 2.0 and A2A error codes (A2A 1.0 section 5.4 and 9.5).
const (
	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
	rpcInternalError  = -32603

	a2aTaskNotFound          = -32001
	a2aTaskNotCancelable     = -32002
	a2aPushNotSupported      = -32003
	a2aUnsupportedOperation  = -32004
	a2aContentTypeNotSupport = -32005
	a2aExtendedCardMissing   = -32007
	a2aVersionNotSupported   = -32009
)

// a2aErrorDomain is the google.rpc.ErrorInfo domain of A2A errors.
const a2aErrorDomain = "a2a-protocol.org"

// rpcRequest is a JSON-RPC 2.0 request.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// rpcError is a JSON-RPC 2.0 error object. Data is the array of detail objects
// A2A 1.0 section 9.5 asks for (each with an "@type"): a google.rpc.ErrorInfo
// and an object whose "error" is the contract's ToolError, so a peer gets the
// same machine code and retryable flag as an MCP or /agui client.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    []any  `json:"data,omitempty"`
}

// Error implements error, so handlers can return an rpcError.
func (e *rpcError) Error() string { return fmt.Sprintf("a2a %d: %s", e.Code, e.Message) }

// rpcResponse is a JSON-RPC 2.0 response: exactly one of Result and Error.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// a2aErr builds an error. reason names the A2A-level cause, toolCode is the
// shared ErrorCode of data.error. The message is fixed text per cause: it never
// quotes the model backend or conversation content.
func a2aErr(code int, reason, toolCode string, retryable bool, msg string) *rpcError {
	return &rpcError{Code: code, Message: msg, Data: []any{
		map[string]any{
			"@type":  "type.googleapis.com/google.rpc.ErrorInfo",
			"reason": reason,
			"domain": a2aErrorDomain,
		},
		map[string]any{
			"@type": "type.googleapis.com/ipfs.mcp.ToolError",
			"error": map[string]any{"code": toolCode, "message": msg, "retryable": retryable},
		},
	}}
}

func errInvalidParams(msg string) *rpcError {
	return a2aErr(rpcInvalidParams, "INVALID_PARAMS", CodeInvalidArgument, false, msg)
}

// errTaskNotFound is the one answer for a task that does not exist and for a
// task that belongs to someone else (A2A 3.3.2: do not tell them apart).
func errTaskNotFound() *rpcError {
	return a2aErr(a2aTaskNotFound, "TASK_NOT_FOUND", CodeNotFound, false, "Task not found")
}

// errContextUnknown is the answer for a contextId that is not the caller's
// context, existing or not.
func errContextUnknown() *rpcError {
	return a2aErr(a2aTaskNotFound, "CONTEXT_NOT_FOUND", CodeNotFound, false, "Context not found; omit contextId to start a new one")
}

func errContextBusy() *rpcError {
	return a2aErr(rpcInternalError, "CONTEXT_BUSY", CodeSessionBusy, true,
		"The context has a task that is still running or waiting for approval; wait for it or cancel it")
}

// rpcErrorOf maps an error of the agent loop or the task store to a JSON-RPC
// error (design.md, "Errors"). Anything else is an internal error without
// detail: an error text could carry conversation content.
func rpcErrorOf(err error) *rpcError {
	var re *rpcError
	switch {
	case errors.As(err, &re):
		return re
	case errors.Is(err, ErrSessionNotFound):
		return errContextUnknown()
	case errors.Is(err, ErrMessageTooLarge):
		return a2aErr(rpcInvalidParams, "MESSAGE_TOO_LARGE", CodeSizeLimitExceeded, false, "Message is too large")
	case errors.Is(err, ErrEmptyRun):
		return errInvalidParams("Message has no text")
	case errors.Is(err, ErrSessionBusy):
		return errContextBusy()
	case errors.Is(err, ErrInterruptOpen):
		return a2aErr(rpcInternalError, "INTERRUPT_OPEN", CodeFailedPrecondition, false, "A confirmation is still open in this context")
	case errors.Is(err, ErrSessionFull):
		return a2aErr(rpcInternalError, "CONTEXT_FULL", CodeFailedPrecondition, false, "The context is full; start a new one")
	case errors.Is(err, ErrTooManySessions), errors.Is(err, errTooManyTasks):
		return a2aErr(rpcInternalError, "TOO_MANY_CONTEXTS", CodeResourceExhausted, true, "Too many open contexts; retry later")
	}
	return a2aErr(rpcInternalError, "INTERNAL", CodeInternal, false, "Internal error")
}
