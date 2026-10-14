package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"unicode/utf8"

	cmds "github.com/ipfs/go-ipfs-cmds"
)

// Error codes from the contract's shared ErrorCode list that this package
// produces.
const (
	CodeInvalidArgument       = "invalid_argument"
	CodeNotFound              = "not_found"
	CodePermissionDenied      = "permission_denied"
	CodeEgressNotAcknowledged = "egress_not_acknowledged"
	CodeToolUnavailable       = "tool_unavailable"
	CodeConfirmRequired       = "confirmation_required"
	CodeConfirmPending        = "confirmation_pending"
	CodeConfirmDeclined       = "confirmation_declined"
	CodeConfirmExpired        = "confirmation_expired"
	CodeConfirmInvalid        = "confirmation_invalid"
	CodeConfirmUnavailable    = "confirmation_unavailable"
	CodeDaemonUnreachable     = "daemon_unreachable"
	CodeDaemonVersionUnsupp   = "daemon_version_unsupported"
	CodeSizeLimitExceeded     = "size_limit_exceeded"
	CodeSessionBusy           = "session_busy"
	CodeFailedPrecondition    = "failed_precondition"
	CodeResourceExhausted     = "resource_exhausted"
	CodeTimeout               = "timeout"
	CodeCancelled             = "cancelled"
	CodeIndexUnavailable      = "index_unavailable"
	CodeRPCError              = "rpc_error"
	CodeInternal              = "internal"

	maxErrorMessageRunes = 2000
	fallbackErrorMessage = "unknown error"
)

// CodeBackendNotConfigured is the settings error schema's code for "no backend
// is configured" (mcp/contract/settings/settings-error.schema.json), which the
// schema also assigns to /agui when the route is mounted without a backend. It
// is deliberately not in the shared ErrorCode list above: the agent surfaces,
// not the tools, produce it.
const CodeBackendNotConfigured = "backend_not_configured"

// ErrDaemonUnreachable is wrapped by every error caused by failing to reach
// the daemon's RPC API.
var ErrDaemonUnreachable = errors.New("ipfs daemon API is not reachable")

// RPCError is an error the daemon's RPC API returned for a command.
type RPCError struct {
	Command string
	Message string
	// Code is the go-ipfs-cmds error type of the response.
	Code cmds.ErrorType
}

// Error implements error.
func (e *RPCError) Error() string {
	return fmt.Sprintf("rpc %s: %s", e.Command, e.Message)
}

// VersionError reports a daemon older than the contract minimum.
type VersionError struct {
	Found   string
	Minimum string
}

// Error implements error.
func (e *VersionError) Error() string {
	return fmt.Sprintf("ipfs daemon version %s is older than the minimum supported version %s; upgrade the daemon", e.Found, e.Minimum)
}

// ToolError is the contract's ToolError.error object.
type ToolError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Tool      string         `json:"tool,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	// Confirmation is set for confirmation_required and confirmation_pending.
	Confirmation *ConfirmationRequired `json:"confirmation,omitempty"`
}

// Error implements error.
func (e *ToolError) Error() string { return e.Code + ": " + e.Message }

// toolErrorResult is the structuredContent of every result with isError true.
type toolErrorResult struct {
	Error *ToolError `json:"error"`
}

func invalidArgument(msg string) *ToolError {
	return &ToolError{Code: CodeInvalidArgument, Message: msg}
}

// toToolError maps any error to the contract error model.
func toToolError(tool string, err error) *ToolError {
	var te *ToolError
	if errors.As(err, &te) {
		out := *te
		out.Tool = tool
		out.Message = clipMessage(out.Message)
		return &out
	}
	out := &ToolError{Tool: tool, Code: CodeInternal}
	var rpcErr *RPCError
	var verErr *VersionError
	var netErr net.Error
	switch {
	case errors.Is(err, context.Canceled):
		out.Code = CodeCancelled
	case errors.Is(err, context.DeadlineExceeded):
		out.Code, out.Retryable = CodeTimeout, true
	case errors.As(err, &verErr):
		out.Code = CodeDaemonVersionUnsupp
	case errors.Is(err, ErrDaemonUnreachable), errors.As(err, &netErr):
		out.Code, out.Retryable = CodeDaemonUnreachable, true
	case errors.As(err, &rpcErr):
		out.Code = rpcCode(rpcErr)
		out.Details = map[string]any{"rpc_command": rpcErr.Command}
	}
	out.Message = clipMessage(err.Error())
	return out
}

func rpcCode(e *RPCError) string {
	if e.Code == cmds.ErrForbidden {
		return CodePermissionDenied
	}
	if strings.Contains(e.Message, "not found") {
		return CodeNotFound
	}
	return CodeRPCError
}

func clipMessage(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallbackErrorMessage
	}
	if utf8.RuneCountInString(s) <= maxErrorMessageRunes {
		return s
	}
	return string([]rune(s)[:maxErrorMessageRunes])
}
