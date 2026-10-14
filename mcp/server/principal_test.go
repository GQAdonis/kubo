package server

import (
	"net/http"
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const principalsConfig = `{
  "principals": [
    {"name": "reader", "kind": "human-mcp", "credential": {"env": "P_READER"}},
    {"name": "writer", "kind": "human-mcp", "credential": {"env": "P_WRITER"}, "write_groups": ["pin"]},
    {"name": "admin",  "kind": "human-mcp", "credential": {"env": "P_ADMIN"},
     "write_groups": ["pin"], "destructive_groups": ["pin"]},
    {"name": "chat",   "kind": "ui", "credential": {"env": "P_CHAT"}}
  ]
}`

func principalTokens() map[string]string {
	return map[string]string{"P_READER": pTokReader, "P_WRITER": pTokWriter, "P_ADMIN": pTokAdmin, "P_CHAT": pTokChat}
}

func TestUnknownTokenGetsNothing(t *testing.T) {
	e := newPrincipalEnv(t, principalsConfig, principalTokens(), Options{}, nil)

	// A well-formed token that no principal owns is 401 on every route, with
	// and without a body, and no session is established.
	for _, path := range []string{RouteMCP, RouteMCP + "/anything"} {
		resp := e.do(t, rawReq{path: path, token: "valid-format-but-unowned-token-0123", body: initializeBody})
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, path)
		assert.Contains(t, resp.Header.Get("WWW-Authenticate"), "Bearer")
	}
	_, err := e.connect(t, "valid-format-but-unowned-token-0123", nil)
	require.Error(t, err, "an unowned token cannot start a session, so no tool is listed")

	// A configured principal still works: the rejection is per token.
	cs := e.client(t, pTokReader, nil)
	assert.NotEmpty(t, listedNames(t, cs))
	assert.Zero(t, e.d.count("pin/add"), "no rejected request ran a tool")
}

func TestReadOnlyPrincipalListsNoWriteTool(t *testing.T) {
	e := newPrincipalEnv(t, principalsConfig, principalTokens(), Options{}, nil)
	cat, err := contract.Load()
	require.NoError(t, err)

	classes := func(cs interface { /* listedNames */
	}) {
	}
	_ = classes

	reader := listedNames(t, e.client(t, pTokReader, nil))
	require.NotEmpty(t, reader)
	for _, n := range reader {
		tool, ok := cat.Tool(n)
		require.True(t, ok, n)
		assert.Equal(t, contract.ClassNone, tool.Confirmation, "%s is not read-only but is listed for a read-only principal", n)
	}
	assert.NotContains(t, reader, "pin_add")

	// Calling a write tool anyway is a structured refusal and changes nothing.
	res := toolCall(t, e.client(t, pTokReader, nil), "pin_add", map[string]any{"path": "/ipfs/" + e.cid})
	assert.Equal(t, CodePermissionDenied, errCode(t, res))
	assert.Equal(t, ReasonPrincipalNotGranted, errDetails(t, res)["reason"])
	assert.Zero(t, e.d.count("pin/add"))

	// Grants are by name: a write group adds write tools, a destructive group
	// adds destructive ones, and neither implies the other.
	writer := listedNames(t, e.client(t, pTokWriter, nil))
	assert.Contains(t, writer, "pin_add")
	assert.NotContains(t, writer, "pin_rm", "write_groups does not grant destructive tools")
	admin := listedNames(t, e.client(t, pTokAdmin, nil))
	assert.Contains(t, admin, "pin_rm")
}

func TestPrincipalKindsAreConfigurable(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{"principals": [
	  {"name": "host", "kind": "human-mcp", "credential": {"env": "P_K1"}},
	  {"name": "chat", "kind": "ui", "credential": {"env": "P_K2"}},
	  {"name": "peer", "kind": "a2a", "credential": {"env": "P_K3"}},
	  {"name": "runtime", "kind": "uar", "credential": {"env": "P_K4"},
	   "uar": {"card_url": "http://127.0.0.1:9/.well-known/agent-card.json"}}
	]}`))
	require.NoError(t, err)
	t.Setenv("P_K1", "kind-token-0000000001")
	t.Setenv("P_K2", "kind-token-0000000002")
	t.Setenv("P_K3", "kind-token-0000000003")
	t.Setenv("P_K4", "kind-token-0000000004")
	creds, warnings := cfg.Credentials()
	require.Empty(t, warnings)
	var kinds []string
	for _, c := range creds {
		kinds = append(kinds, c.Identity.Kind)
		assert.Equal(t, []string{ScopeRead}, c.Identity.Scopes, "%s is read-only by default", c.Identity.ID)
		assert.Empty(t, c.Identity.WriteGroups)
		assert.Empty(t, c.Identity.DestructiveGroups)
		assert.True(t, c.Identity.Configured)
		assert.Empty(t, c.Identity.EgressAck, "no egress acknowledgement unless configured")
	}
	assert.Equal(t, []string{KindHumanMCP, KindUI, KindA2A, KindUAR}, kinds)
}

func TestPrincipalConfigRules(t *testing.T) {
	const cred = `"credential": {"env": "P_X"}`
	for name, tc := range map[string]struct {
		cfg     string
		wantErr string
	}{
		"uar needs its block":         {`{"principals":[{"name":"u","kind":"uar",` + cred + `}]}`, "uar.card_url"},
		"uar block on another kind":   {`{"principals":[{"name":"u","kind":"ui",` + cred + `,"uar":{"card_url":"http://127.0.0.1:9/x.json"}}]}`, "only valid for kind uar"},
		"assert on another kind":      {`{"principals":[{"name":"u","kind":"a2a",` + cred + `,"check":"assert"}]}`, "only valid for kind uar"},
		"operator name is reserved":   {`{"principals":[{"name":"operator","kind":"human-mcp",` + cred + `}]}`, "reserved"},
		"duplicate names":             {`{"principals":[{"name":"a","kind":"ui",` + cred + `},{"name":"a","kind":"ui",` + cred + `}]}`, "twice"},
		"unknown kind":                {`{"principals":[{"name":"a","kind":"root",` + cred + `}]}`, "kind"},
		"literal credential":          {`{"principals":[{"name":"a","kind":"ui","credential":"hunter2-hunter2-hunter2"}]}`, "credential"},
		"unknown group is rejected":   {`{"principals":[{"name":"a","kind":"ui",` + cred + `,"write_groups":["nope"]}]}`, "write_groups"},
		"destructive wildcard barred": {`{"principals":[{"name":"a","kind":"ui",` + cred + `,"destructive_groups":["*"]}]}`, "destructive_groups"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseConfig([]byte(tc.cfg))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.NotContains(t, err.Error(), "hunter2", "a literal secret is never echoed")
		})
	}
}

func TestAnonymousLoopbackPrincipalIsReadOnly(t *testing.T) {
	e := newHTTPEnv(t, func(o *HTTPOptions) { o.Verifier = nil })
	cs := e.client(t, "", nil)
	cat, err := contract.Load()
	require.NoError(t, err)
	names := httpToolNames(t, cs)
	require.NotEmpty(t, names)
	for _, n := range names {
		tool, _ := cat.Tool(n)
		assert.Equal(t, contract.ClassNone, tool.Confirmation, n)
	}
	res := toolCall(t, cs, "pin_add", map[string]any{"path": "/ipfs/" + e.target})
	assert.Equal(t, CodePermissionDenied, errCode(t, res))
}
