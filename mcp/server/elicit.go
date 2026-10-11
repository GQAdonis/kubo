package server

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// elicitKey is the key of the confirmation question in InputRequests and
// InputResponses.
const elicitKey = "confirm"

// Confirmation through elicitation uses the SDK's multi round-trip request
// pattern (SEP-2322), which works on every protocol version: the first
// invocation of the tool handler returns an input-required result that holds
// the elicitation request; the SDK (client side from protocol 2026-07-28,
// server side before) obtains the answer and invokes the handler again with
// the answer and the request state. The state is a signed token that binds the
// round trip to one confirmation, session, principal, tool and argument set.

// elicitRequest opens a pending confirmation and asks the client's user. The
// request state it returns is a binding handle, not consent: it only lets the
// answer in inputResponses resolve the confirmation it names.
func (g *gate) elicitRequest(c *call) (*mcp.CallToolResult, *ToolError) {
	p, err := g.open(c, false, MechanismElicitation)
	if err != nil {
		return nil, &ToolError{Code: CodeInternal, Message: err.Error()}
	}
	// An elicitation is answered in the same logical call; it needs no
	// repeat-call path.
	g.broker.forget(p.ID)
	state, err := g.mintFor(useRequestState, c, p)
	if err != nil {
		_ = g.broker.Resolve(p.ID, ResolverElicitation, false)
		return nil, &ToolError{Code: CodeInternal, Message: err.Error()}
	}
	return &mcp.CallToolResult{
		InputRequests: mcp.InputRequestMap{
			elicitKey: &mcp.ElicitParams{
				Mode:    "form",
				Message: p.Summary.Text,
				RequestedSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						elicitKey: map[string]any{"type": "boolean", "title": c.tool.Title},
					},
					"required": []string{elicitKey},
				},
			},
		},
		RequestState: state,
	}, nil
}

// elicitAnswer handles the second invocation (RequestStateAcceptance in the
// confirmation schema). The state check comes first: the handle must verify
// under the request-state key with use request_state, belong to this session,
// principal, tool and arguments, the session must have advertised elicitation,
// and the confirmation must exist, have been opened by elicitation and not be
// expired. Only then does the answer count, and only while the confirmation is
// pending: accept with confirm true approves; decline, cancel and anything
// else deny. A confirm token, a request state of another call or one from a
// session that cannot elicit never approves anything.
func (g *gate) elicitAnswer(c *call, state string, responses mcp.InputResponseMap) *ToolError {
	invalid := func(msg string) *ToolError {
		return &ToolError{Code: CodeConfirmInvalid, Message: msg + " Nothing changed.", Details: map[string]any{"status": "invalid"}}
	}
	claims, err := g.signer.verify(useRequestState, state)
	if err != nil {
		return invalid("The request state is malformed, was tampered with or is not a request state.")
	}
	switch {
	case claims.SID != c.sid, claims.PID != c.principal.ID, claims.Tool != c.tool.Name, claims.ArgsSHA256 != c.argsHash:
		return invalid("The request state belongs to another call.")
	case !mayElicit(c.principal, c.ss):
		return invalid("This session or caller cannot answer a confirmation through elicitation.")
	}
	p, ok := g.broker.get(claims.CID)
	switch {
	case !ok:
		return invalid("The confirmation behind this request state is unknown.")
	case p.Mechanism != MechanismElicitation:
		return invalid("The confirmation behind this request state was not opened by elicitation.")
	case g.now().Unix() >= claims.Exp:
		return &ToolError{Code: CodeConfirmExpired, Message: "The confirmation expired before the user answered. Nothing changed. Start a new call to ask again.", Details: map[string]any{"confirmation_id": claims.CID, "status": StateExpired}}
	case p.State != StatePending:
		return invalid("This confirmation was already answered or has expired.")
	}

	answer, _ := responses[elicitKey].(*mcp.ElicitResult)
	approved := answer != nil && answer.Action == "accept" && answer.Content["confirm"] == true
	if err := g.broker.Resolve(p.ID, ResolverElicitation, approved); err != nil {
		return invalid("This confirmation was already answered or has expired.")
	}
	if !approved {
		snap, _ := g.broker.get(p.ID)
		return g.outcomeError(c.tool.Name, snap, "declined")
	}
	if err := g.broker.claim(p.ID, claims.Nonce); err != nil {
		return invalid("This confirmation was already used.")
	}
	return nil
}
