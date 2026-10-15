package server

import (
	"strings"
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAckCoversAndLocal(t *testing.T) {
	for _, tc := range []struct {
		name string
		ack  []string
		host string
		want bool
	}{
		{"empty ack covers nothing", nil, "127.0.0.1", false},
		{"local covers loopback v4", []string{"local"}, "127.0.0.1:11434", true},
		{"local covers loopback v6", []string{"local"}, "[::1]:80", true},
		{"local covers localhost", []string{"local"}, "LocalHost", true},
		{"local does not cover a .localhost name (the resolver may send it to DNS)", []string{"local"}, "llm.localhost", false},
		{"local covers an IPv4-mapped loopback", []string{"local"}, "[::ffff:127.0.0.1]:80", true},
		{"local does not cover a remote host", []string{"local"}, "api.openai.com", false},
		{"local does not cover a private LAN address", []string{"local"}, "192.168.1.10", false},
		{"named host covers any port", []string{"llm.example.test"}, "llm.example.test:8443", true},
		{"named host with port covers that port only", []string{"llm.example.test:8443"}, "llm.example.test:9", false},
		{"named host with port covers itself", []string{"llm.example.test:8443"}, "llm.example.test:8443", true},
		{"case and trailing dot are ignored", []string{"LLM.Example.Test."}, "llm.example.test", true},
		{"another host is not covered", []string{"a.example.test"}, "b.example.test", false},
		{"empty host is never covered", []string{"local"}, "", false},
	} {
		assert.Equal(t, tc.want, AckCovers(tc.ack, tc.host), tc.name)
	}
	assert.False(t, IsLocalHost("local"), "the ack sentinel is not a loopback host name")
	assert.False(t, IsLocalHost(""))
	assert.False(t, IsLocalHost("0.0.0.0"), "the wildcard address is not loopback")
	assert.False(t, IsLocalHost("model.localhost"), "only localhost and literal loopback addresses are local")
	assert.False(t, IsLocalHost("a.b.localhost:8080"))
	assert.Equal(t, []string{"x.test"}, MissingAck([]string{"y.test"}, []string{"x.test", "x.test", "y.test"}))
}

func TestClientNameIsUAR(t *testing.T) {
	for name, want := range map[string]bool{
		"uar": true, "UAR": true, "uar-mcp-client": true, "universal-agent-runtime": true,
		"Universal Agent Runtime": true, "universal_agent_runtime/1.2": true,
		"claude-code": false, "guard": false, "quarantine": false, "uart-bridge": false, "": false,
		"universal agent": false,
	} {
		assert.Equal(t, want, nameIsUAR(name), name)
	}
}

const egressConfig = `{
  "principals": [
    {"name": "bare",    "kind": "human-mcp", "credential": {"env": "P_READER"}},
    {"name": "acked",   "kind": "human-mcp", "credential": {"env": "P_WRITER"}, "egress_ack": ["model.example.test"]},
    {"name": "locally", "kind": "human-mcp", "credential": {"env": "P_ADMIN"},  "egress_ack": ["local"]}
  ]
}`

func egressTokens() map[string]string {
	return map[string]string{"P_READER": pTokReader, "P_WRITER": pTokWriter, "P_ADMIN": pTokAdmin}
}

// contentEgressTools lists the registered tools the contract marks
// contentEgress, so the test follows the catalog rather than a copy of it.
func contentEgressTools(t *testing.T, s *Server) []string {
	t.Helper()
	cat, err := contract.Load()
	require.NoError(t, err)
	var out []string
	for _, tool := range cat.Tools {
		if tool.ContentEgress && s.toolRegistered(tool.Name) {
			out = append(out, tool.Name)
		}
	}
	return out
}

func TestEveryContentEgressToolIsRefusedWithoutAck(t *testing.T) {
	e := newPrincipalEnv(t, egressConfig, egressTokens(), Options{}, nil)
	tools := contentEgressTools(t, e.s)
	require.Contains(t, tools, "ipfs_cat")
	require.Greater(t, len(tools), 5, "the catalog marks many tools contentEgress")

	cs := e.client(t, pTokReader, nil)
	before := len(e.cf.log)
	for _, name := range tools {
		// Empty arguments: the refusal comes before anything else, so even an
		// invalid call is told about the missing acknowledgement.
		res := toolCall(t, cs, name, map[string]any{})
		assert.Equal(t, CodeEgressNotAcknowledged, errCode(t, res), name)
		d := errDetails(t, res)
		assert.Equal(t, ReasonNoEgressAck, d["reason"], name)
		assert.Equal(t, "egress_ack", d["missing_acknowledgement"], name)
		validateAgainstCommon(t, "ToolError", res.StructuredContent)
		require.NotEmpty(t, res.Content, name)
		assert.NotContains(t, res.Content[0].(*mcp.TextContent).Text, "SECRET-NODE-BYTES", name)
	}
	assert.Len(t, e.cf.log, before, "a refused call never reaches the daemon")
}

func TestContentEgressToolRunsWithAckAndLocalAck(t *testing.T) {
	e := newPrincipalEnv(t, egressConfig, egressTokens(), Options{}, nil)
	for _, token := range []string{pTokWriter, pTokAdmin} {
		cs := e.client(t, token, nil)
		res := toolCall(t, cs, "ipfs_cat", map[string]any{"path": "/ipfs/" + e.cid})
		require.False(t, res.IsError, "%+v", res.StructuredContent)
	}
	// Tools that return no node content need no acknowledgement.
	res := toolCall(t, e.client(t, pTokReader, nil), "ipfs_node_info", nil)
	require.False(t, res.IsError, "%+v", res.StructuredContent)
}

const backendEgressConfig = `{
  "llm": {"base_url": "https://llm.example.test/v1", "model": "m"},
  "embeddings": {"mode": "embedded", "embedded": {"surreal_binary": "/opt/surreal/bin/surreal", "data_dir": "/var/lib/ipfs-mcp/index"}, "embedder": {"base_url": "https://embed.example.test/v1", "model": "e", "dimensions": 8}},
  "principals": [
    {"name": "chat-none",   "kind": "ui", "credential": {"env": "P_READER"}, "egress_ack": ["llm.example.test"]},
    {"name": "chat-all",    "kind": "ui", "credential": {"env": "P_WRITER"}, "egress_ack": ["llm.example.test", "embed.example.test"]},
    {"name": "chat-local",  "kind": "ui", "credential": {"env": "P_ADMIN"},  "egress_ack": ["local"]},
    {"name": "host",        "kind": "human-mcp", "credential": {"env": "P_CHAT"}, "egress_ack": ["unrelated.example.test"]}
  ]
}`

func TestBackendHostsMustBeAcknowledged(t *testing.T) {
	t.Setenv("P_CHAT", pTokChat)
	e := newPrincipalEnv(t, backendEgressConfig, egressTokens(), Options{}, nil)
	call := func(token string) *mcp.CallToolResult {
		return toolCall(t, e.client(t, token, nil), "ipfs_cat", map[string]any{"path": "/ipfs/" + e.cid})
	}

	// The embedder host is missing from this ack.
	res := call(pTokReader)
	assert.Equal(t, CodeEgressNotAcknowledged, errCode(t, res))
	d := errDetails(t, res)
	assert.Equal(t, ReasonBackendHostNotAcked, d["reason"])
	assert.Equal(t, []any{"embed.example.test"}, d["missing_hosts"])

	// Both hosts acknowledged: allowed.
	require.False(t, call(pTokWriter).IsError)

	// "local" never covers a remote backend.
	res = call(pTokAdmin)
	assert.Equal(t, CodeEgressNotAcknowledged, errCode(t, res))
	assert.ElementsMatch(t, []any{"llm.example.test", "embed.example.test"}, errDetails(t, res)["missing_hosts"])

	// A human-mcp principal brings its own model: ipfs mcp's backends are not
	// in its path, so only its own acknowledgement is required.
	require.False(t, call(pTokChat).IsError)
}

func TestLocalBackendNeedsOnlyLocalAck(t *testing.T) {
	cfg := `{
	  "llm": {"base_url": "http://127.0.0.1:1234/v1", "model": "m"},
	  "principals": [{"name": "chat", "kind": "ui", "credential": {"env": "P_READER"}, "egress_ack": ["local"]}]
	}`
	e := newPrincipalEnv(t, cfg, egressTokens(), Options{}, nil)
	res := toolCall(t, e.client(t, pTokReader, nil), "ipfs_cat", map[string]any{"path": "/ipfs/" + e.cid})
	require.False(t, res.IsError, "%+v", res.StructuredContent)
}

func TestStdioClientIdentifyingAsUARIsRefusedContentEgress(t *testing.T) {
	d := catFake(t, map[string][]byte{"/ipfs/" + testCID(t, 7): []byte("SECRET-NODE-BYTES")})
	s := mustNew(t, d, Options{Tools: []string{"node", "content"}})

	call := func(clientName, tool string, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		ct, st := mcp.NewInMemoryTransports()
		ss, err := s.MCP().Connect(t.Context(), st, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = ss.Close() })
		cs, err := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: "0"}, nil).Connect(t.Context(), ct, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = cs.Close() })
		return toolCall(t, cs, tool, args)
	}
	path := map[string]any{"path": "/ipfs/" + testCID(t, 7)}

	res := call("universal-agent-runtime", "ipfs_cat", path)
	assert.Equal(t, CodeEgressNotAcknowledged, errCode(t, res))
	assert.Equal(t, ReasonStdioClientIsUAR, errDetails(t, res)["reason"])
	assert.Equal(t, CodeEgressNotAcknowledged, errCode(t, call("UAR", "ipfs_cat", path)))

	// Other clients, and tools that return no content, are untouched.
	require.False(t, call("claude-code", "ipfs_cat", path).IsError)
	require.False(t, call("uar", "ipfs_node_info", nil).IsError)
}

// The egress_ack sentinel "local" is not a host name: an endpoint called "local"
// is not loopback (R11 round 2).
func TestAckLocalDoesNotCoverAHostNamedLocal(t *testing.T) {
	ack := []string{EgressLocal}
	for _, h := range []string{"https://local", "local:443", "local", "https://Local./v1"} {
		host := h
		if strings.Contains(h, "://") {
			host = HostFromURL(h)
		}
		assert.False(t, IsLocalHost(host), h)
		assert.False(t, AckCovers(ack, host), h)
	}
	for _, h := range []string{"127.0.0.1", "127.0.0.1:11434", "localhost", "localhost:8080", "[::1]:443"} {
		assert.True(t, IsLocalHost(h), h)
		assert.True(t, AckCovers(ack, h), h)
	}
	assert.True(t, IsNoEgressHost("local"), "UAR provider_host label")
	assert.True(t, IsNoEgressHost("disabled"))
}
