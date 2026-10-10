//go:build !noagent

package server

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// callSessionPrefix marks the gate session IDs of transports that call tools
// through Server.CallTool, so their confirmations never share a session with an
// MCP client, the agent loop or another transport.
const callSessionPrefix = "call-"

// ErrNoSessionKey is returned by CallTool when the caller names no session.
var ErrNoSessionKey = errors.New("a tool call needs a session key")

// CallTool runs one contract tool for a transport that lives outside this
// package (the A2A data-part path). It goes through the handler MCP tools/call
// runs, so input validation, the principal's scope, the egress_ack rule and the
// confirmation gate apply exactly as they do there. The identity is checked
// against the tool's group and class before anything runs, and a tool outside
// the grants gets the same permission_denied result as on /mcp.
//
// A confirm_token in args is dropped: this path never accepts one, and a2a and
// uar principals are never issued one. A write or destructive call returns
// confirmation_required (or confirmation_pending on a repeat); it runs only
// when the same identity repeats the same call in the same session after a
// presence resolver approved it, once.
//
// sessionKey names the conversation the confirmations bind to; two sessions of
// the same principal do not share a confirmation. Failures of the tool are an
// isError result; the Go error is for a misuse of this function.
func (s *Server) CallTool(ctx context.Context, id Identity, sessionKey, name string, args json.RawMessage) (*mcp.CallToolResult, error) {
	if sessionKey == "" {
		return nil, ErrNoSessionKey
	}
	cleaned, err := stripConfirmToken(args)
	if err != nil {
		return errorResult(name, invalidArgument(err.Error())), nil
	}
	eff, refusal := s.applyPolicy(ctx, id, false)
	if refusal != nil {
		return errorResult(name, refusal), nil
	}
	t, known := s.cat.Tool(name)
	if !known {
		return errorResult(name, &ToolError{Code: CodeToolUnavailable, Message: "unknown tool " + name}), nil
	}
	if !eff.Allows(t) {
		return s.notGranted(t, eff), nil
	}
	rt, ok := s.registry[name]
	if !ok {
		return errorResult(name, &ToolError{Code: CodeToolUnavailable, Message: "tool " + name + " is not available on this server"}), nil
	}
	ctx = WithIdentity(WithPrincipal(ctx, eff.Principal), eff)
	ctx = context.WithValue(ctx, sessionIDKey{}, callSessionPrefix+sessionKey)
	return rt.handler(ctx, &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: name, Arguments: cleaned}})
}

// stripConfirmToken returns the arguments as a JSON object without
// confirm_token. Empty arguments are an empty object.
func stripConfirmToken(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage("{}"), nil
	}
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil || args == nil {
		return nil, errors.New("tool arguments must be a JSON object")
	}
	if _, has := args["confirm_token"]; !has {
		return raw, nil
	}
	delete(args, "confirm_token")
	return json.Marshal(args)
}
