package server

import (
	"context"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"unicode"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// EgressLocal is the egress_ack value that stands for every loopback host.
const EgressLocal = "local"

// Reasons carried in ToolError.details.reason by the egress refusals.
const (
	// ReasonNoEgressAck: the principal's config has no egress_ack.
	ReasonNoEgressAck = "no_egress_ack"
	// ReasonBackendHostNotAcked: a model or embedder host that content would
	// reach is not covered by the principal's egress_ack.
	ReasonBackendHostNotAcked = "backend_host_not_acknowledged"
	// ReasonStdioClientIsUAR: the MCP client identifies itself as UAR on a
	// session that is not a configured uar principal.
	ReasonStdioClientIsUAR = "client_identifies_as_uar"
)

// normalizeHost lowercases a host or host:port and strips brackets and a
// trailing dot, so "LocalHost.", "[::1]:80" and "::1" compare predictably.
func normalizeHost(h string) (host, port string) {
	h = strings.ToLower(strings.TrimSpace(h))
	if hp, p, err := net.SplitHostPort(h); err == nil {
		h, port = hp, p
	}
	h = strings.TrimSuffix(strings.Trim(h, "[]"), ".")
	return h, port
}

// IsLocalHost reports whether a host names this machine: the literal "local",
// localhost, a *.localhost name or a loopback address.
func IsLocalHost(h string) bool {
	host, _ := normalizeHost(h)
	if host == "" {
		return false
	}
	if host == EgressLocal || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}

// EgressDisabled is the provider_host of a provider that is configured but not
// used: it forwards nothing anywhere.
const EgressDisabled = "disabled"

// IsNoEgressHost reports whether a provider host sends content nowhere off
// this machine: a local host, or the literal "disabled".
func IsNoEgressHost(h string) bool {
	return strings.EqualFold(strings.TrimSpace(h), EgressDisabled) || IsLocalHost(h)
}

// HostFromURL returns the host:port of an endpoint URL, or "" when it has
// none.
func HostFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

// AckCovers reports whether egress_ack covers the host. "local" covers every
// loopback host and nothing else. A listed name covers that host on any port;
// a listed name with a port covers only that port.
func AckCovers(ack []string, host string) bool {
	h, port := normalizeHost(host)
	if h == "" {
		return false
	}
	if IsLocalHost(h) && slices.ContainsFunc(ack, func(a string) bool {
		return strings.EqualFold(strings.TrimSpace(a), EgressLocal)
	}) {
		return true
	}
	for _, a := range ack {
		ah, ap := normalizeHost(a)
		if ah == EgressLocal || ah != h {
			continue
		}
		if ap == "" || ap == port {
			return true
		}
	}
	return false
}

// MissingAck returns the hosts that egress_ack does not cover, without
// duplicates.
func MissingAck(ack, hosts []string) []string {
	var missing []string
	for _, h := range hosts {
		if !AckCovers(ack, h) && !slices.Contains(missing, h) {
			missing = append(missing, h)
		}
	}
	return missing
}

// egressRefusal decides whether a contentEgress tool call is refused. It
// returns nil when the call may run.
//
// Configured principals need an egress_ack, and the model and embedder hosts
// that ipfs mcp itself would forward their content to must be covered by it.
// The operator's own clients (stdio, --auth-token-*, the anonymous loopback
// listener) have no principal config and are outside the guarantee; so is the
// model their MCP host talks to, which Kubo cannot see.
//
// Independently, a session whose MCP clientInfo says UAR is refused unless it
// is a configured uar principal: stdio-spawned UAR cannot be checked, and
// clientInfo is self-reported, so this is a misconfiguration guard and not a
// security boundary.
func (s *Server) egressRefusal(ctx context.Context, req *mcp.CallToolRequest, t contract.Tool) *ToolError {
	if !t.ContentEgress {
		return nil
	}
	id, authed := IdentityFromContext(ctx)
	if clientIsUAR(req) && !(authed && id.Kind == KindUAR) {
		return &ToolError{
			Code: CodeEgressNotAcknowledged,
			Message: t.Name + " returns node content and the MCP client identifies as the Universal Agent Runtime, " +
				"which is only supported as a configured principal of kind uar over HTTP (stdio-spawned UAR cannot be " +
				"checked); run UAR against 'ipfs mcp --http' with a uar principal in the ipfs mcp config",
			Details: map[string]any{"reason": ReasonStdioClientIsUAR},
		}
	}
	if !authed || !id.Configured {
		return nil
	}
	if len(id.EgressAck) == 0 {
		return &ToolError{
			Code: CodeEgressNotAcknowledged,
			Message: t.Name + " returns node content, and principal " + id.ID + " has no egress_ack. Add the model and " +
				"embedder hosts it may forward content to (or \"local\") to its egress_ack in the ipfs mcp config",
			Details: map[string]any{"reason": ReasonNoEgressAck, "principal": id.ID, "missing_acknowledgement": "egress_ack"},
		}
	}
	if missing := MissingAck(id.EgressAck, id.BackendHosts); len(missing) > 0 {
		return &ToolError{
			Code: CodeEgressNotAcknowledged,
			Message: t.Name + " returns node content that ipfs mcp would forward to " + strings.Join(missing, ", ") +
				", which is not in the egress_ack of principal " + id.ID,
			Details: map[string]any{"reason": ReasonBackendHostNotAcked, "principal": id.ID, "missing_hosts": missing},
		}
	}
	return nil
}

// clientIsUAR reports whether the MCP clientInfo names the Universal Agent
// Runtime: a "uar" word, or the words "universal agent runtime" in a row, in
// the name or title.
func clientIsUAR(req *mcp.CallToolRequest) bool {
	if req == nil || req.Session == nil {
		return false
	}
	p := req.Session.InitializeParams()
	if p == nil || p.ClientInfo == nil {
		return false
	}
	return nameIsUAR(p.ClientInfo.Name) || nameIsUAR(p.ClientInfo.Title)
}

func nameIsUAR(name string) bool {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if slices.Contains(words, "uar") {
		return true
	}
	for i := 0; i+2 < len(words); i++ {
		if words[i] == "universal" && words[i+1] == "agent" && words[i+2] == "runtime" {
			return true
		}
	}
	return false
}
