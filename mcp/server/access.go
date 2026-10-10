package server

import (
	"context"
	"slices"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Scopes of the HTTP transports (ADR-0003 section 5). They map to the
// contract's confirmation classes: ipfs:read to none, ipfs:write to write and
// ipfs:admin to destructive.
const (
	ScopeRead  = "ipfs:read"
	ScopeWrite = "ipfs:write"
	ScopeAdmin = "ipfs:admin"
)

// ReasonPrincipalNotGranted is the ipfs_node_status reason enum value for a
// tool outside the calling principal's grants.
const ReasonPrincipalNotGranted = "principal_not_granted"

// AllGroups in a grant means every tool group.
const AllGroups = "*"

// identityExtraKey is the TokenInfo.Extra key that carries the Identity.
const identityExtraKey = "identity"

// Identity is the authenticated caller of an HTTP route: the principal the
// confirmation gate binds tokens to, plus what it may use. A verifier produces
// it; tool handlers never see the credential behind it.
type Identity struct {
	Principal
	// Scopes are the OAuth-style scopes of the credential.
	Scopes []string
	// Groups are the tool groups whose read-class (confirmation none) tools
	// the identity may call; "*" means every group. Empty grants no tool.
	Groups []string
	// WriteGroups are the groups whose write-class tools it may call. Empty
	// (the default) makes the identity read-only.
	WriteGroups []string
	// DestructiveGroups are the groups whose destructive-class tools it may
	// call. Empty (the default) offers none (ADR-0003 section 2).
	DestructiveGroups []string

	// Configured is true for an identity built from a principal in the ipfs mcp
	// config file. Only configured principals are subject to egress_ack: the
	// operator's own clients (--auth-token-*, the anonymous loopback listener,
	// stdio) have no principal config.
	Configured bool
	// EgressAck lists the model and embedder hosts the principal's content may
	// be forwarded to, or "local". Empty refuses every contentEgress tool.
	EgressAck []string
	// BackendHosts are the hosts of the model and embedder backends that ipfs
	// mcp itself forwards this principal's content to (llm and embeddings
	// sections); each must be covered by EgressAck.
	BackendHosts []string
	// ShellCapable is the operator assertion shell_capable. Under
	// --confirm=cli a write principal needs false.
	ShellCapable *bool
	// Check is "verify" or "assert" for kind uar.
	Check string
	// UAR is the UAR settings of a kind uar principal.
	UAR *UARConfig
	// Restrictions are grants withdrawn by policy for this request. It is set
	// only on the effective identity, never on the configured one.
	Restrictions []Restriction
}

// Restriction records why a grant was withdrawn from an identity.
type Restriction struct {
	// Reason is a stable machine-readable code.
	Reason string `json:"reason"`
	// Message tells the operator what to change.
	Message string `json:"message"`
}

// NewIdentity builds an identity from configured grants and derives its
// scopes: ipfs:read always, ipfs:write when a write group is granted and
// ipfs:admin when a destructive group is granted.
func NewIdentity(p Principal, groups, writeGroups, destructiveGroups []string) Identity {
	id := Identity{
		Principal:         p,
		Groups:            slices.Clone(groups),
		WriteGroups:       slices.Clone(writeGroups),
		DestructiveGroups: slices.Clone(destructiveGroups),
		Scopes:            []string{ScopeRead},
	}
	if len(writeGroups) > 0 {
		id.Scopes = append(id.Scopes, ScopeWrite)
	}
	if len(destructiveGroups) > 0 {
		id.Scopes = append(id.Scopes, ScopeAdmin)
	}
	return id
}

// Allows reports whether the identity may use the tool: its scope for the
// tool's confirmation class and a grant for the tool's group.
func (i Identity) Allows(t contract.Tool) bool {
	switch t.Confirmation {
	case contract.ClassNone:
		return slices.Contains(i.Scopes, ScopeRead) && grants(i.Groups, t.Group)
	case contract.ClassWrite:
		return slices.Contains(i.Scopes, ScopeWrite) && grants(i.WriteGroups, t.Group)
	case contract.ClassDestructive:
		return slices.Contains(i.Scopes, ScopeAdmin) && grants(i.DestructiveGroups, t.Group)
	}
	return false
}

func grants(groups []string, group string) bool {
	return slices.Contains(groups, AllGroups) || slices.Contains(groups, group)
}

// anonymousIdentity is who a request is when a loopback listener runs without
// a verifier: a read-only caller with no write or destructive grant.
func anonymousIdentity() Identity {
	return NewIdentity(Principal{ID: "anonymous-loopback", Kind: KindHumanMCP}, []string{AllGroups}, nil, nil)
}

// identityOf returns the identity the HTTP layer attached to a request.
func identityOf(extra *mcp.RequestExtra) (Identity, bool) {
	if extra == nil || extra.TokenInfo == nil {
		return Identity{}, false
	}
	id, ok := extra.TokenInfo.Extra[identityExtraKey].(Identity)
	return id, ok
}

func init() { registerBuildHook(func(s *Server) { s.installAccess() }) }

// installAccess makes the SDK server enforce per-identity access. Requests
// without an identity (stdio, in-process) are untouched. With one, policy is
// applied first (UAR preflight, shell_capable rule), the principal and
// effective identity reach the handlers through the context, tool calls
// outside the identity's grants are refused before any handler runs, and
// tools/list shows only what the identity may call.
func (s *Server) installAccess() {
	s.mcp.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			configured, ok := identityOf(req.GetExtra())
			if !ok {
				return next(ctx, method, req)
			}
			id, refusal := s.applyPolicy(ctx, configured, method == methodInitialize)
			if refusal != nil {
				return s.refuse(method, req, refusal)
			}
			ctx = WithIdentity(WithPrincipal(ctx, id.Principal), id)
			switch method {
			case "tools/call":
				if name := calledTool(req.GetParams()); name != "" {
					if t, known := s.cat.Tool(name); known && !id.Allows(t) {
						return s.notGranted(t, id), nil
					}
				}
			case "tools/list":
				res, err := next(ctx, method, req)
				if lr, isList := res.(*mcp.ListToolsResult); isList && err == nil {
					lr.Tools = slices.DeleteFunc(slices.Clone(lr.Tools), func(t *mcp.Tool) bool {
						entry, known := s.cat.Tool(t.Name)
						return known && !id.Allows(entry)
					})
				}
				return res, err
			}
			return next(ctx, method, req)
		}
	})
}

// methodInitialize is the MCP method that opens a session.
const methodInitialize = "initialize"

// refuse answers a request of a principal that policy disabled. A tool call
// gets the structured isError result; any other method (initialize included)
// gets a JSON-RPC error, so the session is not established.
func (s *Server) refuse(method string, req mcp.Request, te *ToolError) (mcp.Result, error) {
	if method == "tools/call" {
		name := calledTool(req.GetParams())
		return errorResult(name, te), nil
	}
	return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidRequest, Message: te.Code + ": " + te.Message}
}

func calledTool(p mcp.Params) string {
	switch v := p.(type) {
	case *mcp.CallToolParamsRaw:
		return v.Name
	case *mcp.CallToolParams:
		return v.Name
	}
	return ""
}

// notGranted is the isError result for a tool outside the caller's grants.
// When policy withdrew the grant (not the operator's config), the reason and
// the restrictions say so.
func (s *Server) notGranted(t contract.Tool, id Identity) *mcp.CallToolResult {
	reason := ReasonPrincipalNotGranted
	msg := t.Name + " is not granted to principal " + id.ID + " (" + t.Confirmation +
		"-class tool in group " + t.Group + "); ask the operator to grant the group in the ipfs mcp config"
	details := map[string]any{"reason": reason}
	if len(id.Restrictions) > 0 && t.Confirmation != contract.ClassNone {
		reason = id.Restrictions[0].Reason
		details["reason"] = reason
		details["restrictions"] = id.Restrictions
		msg = t.Name + " is not available to principal " + id.ID + " because of ipfs mcp policy: " + id.Restrictions[0].Message
	}
	te := &ToolError{Code: CodePermissionDenied, Message: msg, Tool: t.Name, Details: details}
	return errorResult(t.Name, te)
}
