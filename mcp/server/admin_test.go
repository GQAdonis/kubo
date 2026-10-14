package server

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tokSettingsAdmin is the token of the admin test principal. The admin kind is
// scoped to the settings route only (ADR-0009 section 3).
const tokSettingsAdmin = "settings-admin-token-01234"

// settingsTestRoute stands in for the settings route, which a later task of
// mcp-agent-settings-api mounts; its only accepted kind is KindAdmin.
const settingsTestRoute = "/settings-test"

// adminVerifier authenticates one principal per kind, including an admin.
func adminVerifier(t *testing.T) *StaticVerifier {
	t.Helper()
	mk := func(id, kind string) Identity {
		return NewIdentity(Principal{ID: id, Kind: kind}, []string{AllGroups}, nil, nil)
	}
	v, err := NewStaticVerifier([]StaticCredential{
		{Identity: mk("reader", KindHumanMCP), Token: tokReader},
		{Identity: mk("chat", KindUI), Token: tokChat},
		{Identity: mk("peer", KindA2A), Token: tokPeer},
		{Identity: mk("settings-admin", KindAdmin), Token: tokSettingsAdmin},
	})
	require.NoError(t, err)
	return v
}

// TestAdminPrincipalScopedToSettingsRoute is the G9-4 cross-path check for the
// admin kind: its token is refused on /mcp, /agui and /a2a, and a tool
// principal's token is refused on the settings route.
func TestAdminPrincipalScopedToSettingsRoute(t *testing.T) {
	e := newHTTPEnv(t, func(o *HTTPOptions) { o.Verifier = adminVerifier(t) })
	require.NoError(t, e.hs.Mount(RouteA2A, echoRoute(), MountOptions{}))
	require.NoError(t, e.hs.Mount(RouteAGUI, echoRoute(), MountOptions{}))
	require.NoError(t, e.hs.Mount(settingsTestRoute, echoRoute(), MountOptions{Kinds: []string{KindAdmin}}))
	require.NoError(t, e.hs.Mount("/other", echoRoute(), MountOptions{}))

	matrix := []struct {
		route, token string
		want         int
	}{
		// The admin token opens only the settings route.
		{settingsTestRoute, tokSettingsAdmin, http.StatusOK},
		{RouteMCP, tokSettingsAdmin, http.StatusForbidden},
		{RouteAGUI, tokSettingsAdmin, http.StatusForbidden},
		{RouteA2A, tokSettingsAdmin, http.StatusForbidden},
		{"/other", tokSettingsAdmin, http.StatusForbidden},
		// A tool principal's token is refused on the settings route.
		{settingsTestRoute, tokReader, http.StatusForbidden},
		{settingsTestRoute, tokChat, http.StatusForbidden},
		{settingsTestRoute, tokPeer, http.StatusForbidden},
		{settingsTestRoute, "", http.StatusUnauthorized},
		// The tool tokens still open their own routes.
		{RouteMCP, tokReader, http.StatusOK},
		{RouteAGUI, tokChat, http.StatusOK},
		{RouteA2A, tokPeer, http.StatusOK},
	}
	for _, m := range matrix {
		method, body := http.MethodGet, ""
		if m.route == RouteMCP {
			method, body = http.MethodPost, initializeBody
		}
		resp := e.do(t, rawReq{method: method, path: m.route, token: m.token, body: body})
		assert.Equal(t, m.want, resp.StatusCode, "%s with %q", m.route, m.token)
	}
}

// TestAbsentCredentialDisablesOnlyThatPrincipal: the server starts with one
// principal's token environment variable unset; that principal authenticates
// nothing while the surviving principal works.
func TestAbsentCredentialDisablesOnlyThatPrincipal(t *testing.T) {
	t.Setenv("IPFS_MCP_TEST_ABSENT_READER", tokReader)
	// IPFS_MCP_TEST_ABSENT_ADMIN is deliberately never set.
	cfg, err := ParseConfig([]byte(`{"principals":[
		{"name":"reader","kind":"human-mcp","credential":{"env":"IPFS_MCP_TEST_ABSENT_READER"}},
		{"name":"settings-admin","kind":"admin","credential":{"env":"IPFS_MCP_TEST_ABSENT_ADMIN"}}
	]}`))
	require.NoError(t, err)
	creds, warnings := cfg.Credentials()
	require.Len(t, creds, 1)
	require.Equal(t, "reader", creds[0].Identity.ID)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "settings-admin")
	verifier, err := NewStaticVerifier(creds)
	require.NoError(t, err)

	e := newHTTPEnv(t, func(o *HTTPOptions) { o.Verifier = verifier })
	require.NoError(t, e.hs.Mount(settingsTestRoute, echoRoute(), MountOptions{Kinds: []string{KindAdmin}}))

	// The surviving principal is served.
	ok := e.do(t, rawReq{method: http.MethodPost, path: RouteMCP, token: tokReader, body: initializeBody})
	assert.Equal(t, http.StatusOK, ok.StatusCode)
	// The disabled principal authenticates nothing: no token is its token.
	assert.Equal(t, http.StatusUnauthorized, e.do(t, rawReq{path: settingsTestRoute, token: tokSettingsAdmin}).StatusCode)
	assert.Equal(t, http.StatusUnauthorized, e.do(t, rawReq{path: settingsTestRoute}).StatusCode)
	// The surviving principal's token still does not open the settings route.
	assert.Equal(t, http.StatusForbidden, e.do(t, rawReq{path: settingsTestRoute, token: tokReader}).StatusCode)
}

// TestAllCredentialsAbsentServerStarts: with every configured credential
// absent the server still starts, and the empty verifier authenticates no
// token at all.
func TestAllCredentialsAbsentServerStarts(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{"principals":[
		{"name":"settings-admin","kind":"admin","credential":{"file":"/nonexistent/ipfs-mcp-test/admin.token"}}
	]}`))
	require.NoError(t, err)
	creds, warnings := cfg.Credentials()
	require.Empty(t, creds)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "settings-admin")
	verifier, err := NewStaticVerifier(creds)
	require.NoError(t, err, "an empty verifier is valid: every credential was absent")

	e := newHTTPEnv(t, func(o *HTTPOptions) { o.Verifier = verifier })
	require.NoError(t, e.hs.Mount(settingsTestRoute, echoRoute(), MountOptions{Kinds: []string{KindAdmin}}))
	assert.Equal(t, http.StatusUnauthorized, e.do(t, rawReq{method: http.MethodPost, path: RouteMCP, token: tokReader, body: initializeBody}).StatusCode)
	assert.Equal(t, http.StatusUnauthorized, e.do(t, rawReq{path: settingsTestRoute, token: tokSettingsAdmin}).StatusCode)
}

// TestAdminPrincipalHasNoToolGrants pins the contract rule that the tool-group
// keys are ignored for an admin principal: none reach its identity, and none
// register destructive tools.
func TestAdminPrincipalHasNoToolGrants(t *testing.T) {
	t.Setenv("IPFS_MCP_TEST_ADMIN_GRANTS", tokSettingsAdmin)
	cfg, err := ParseConfig([]byte(`{"principals":[
		{"name":"settings-admin","kind":"admin","credential":{"env":"IPFS_MCP_TEST_ADMIN_GRANTS"},
		 "groups":["*"],"write_groups":["pin"],"destructive_groups":["pin"],"egress_ack":["models.example"]}
	]}`))
	require.NoError(t, err)
	assert.False(t, cfg.GrantsDestructive(), "an admin's destructive_groups register no tools")

	creds, warnings := cfg.Credentials()
	require.Empty(t, warnings)
	require.Len(t, creds, 1)
	id := creds[0].Identity
	assert.Equal(t, KindAdmin, id.Kind)
	assert.Empty(t, id.Groups)
	assert.Empty(t, id.WriteGroups)
	assert.Empty(t, id.DestructiveGroups)
	assert.Equal(t, []string{ScopeRead}, id.Scopes)
	assert.Equal(t, []string{"models.example"}, id.EgressAck, "egress_ack still applies to settings writes (D4)")
}
