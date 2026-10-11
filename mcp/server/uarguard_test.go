package server

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uarPrincipalConfig has a reader and a kind uar principal with the pin write
// group. extra is spliced into the uar principal object.
func uarPrincipalConfig(u *fakeUAR, extra string) string {
	return fmt.Sprintf(`{
	  "principals": [
	    {"name": "reader", "kind": "human-mcp", "credential": {"env": "P_READER"}},
	    {"name": "runtime", "kind": "uar", "credential": {"env": "P_UAR"},
	     "write_groups": ["pin"], "egress_ack": ["local"],
	     "uar": {"card_url": %q, "admin_credential": {"env": %q}, "min_version": "1.4.0"}%s}
	  ]
	}`, u.cardURL(), fakeUARAdminEnv, extra)
}

func uarEnv(t *testing.T, u *fakeUAR, extra string, sopts Options) *principalEnv {
	t.Helper()
	sopts.uarOpts = append(sopts.uarOpts, WithSettingsPath(testUARSettingsPath))
	e := newPrincipalEnv(t, uarPrincipalConfig(u, extra), map[string]string{"P_READER": pTokReader, "P_UAR": pTokUAR}, sopts, nil)
	// The profile names the ipfs mcp that really serves this test.
	u.set(func(u *fakeUAR) {
		u.settings["mcp_servers"] = []any{map[string]any{"name": "ipfs", "transport": "http", "url": e.base + RouteMCP}}
	})
	return e
}

func TestUARPrincipalIsRecheckedPerSession(t *testing.T) {
	u := newFakeUAR(t, "profile")
	e := uarEnv(t, u, "", Options{})

	// A healthy UAR: tools list, writes are offered, and a configured uar
	// principal is not caught by the stdio guard.
	cs, err := e.clientNamed(t, pTokUAR, "uar")
	require.NoError(t, err)
	assert.Contains(t, listedNames(t, cs), "pin_add")
	res := toolCall(t, cs, "ipfs_cat", map[string]any{"path": "/ipfs/" + e.cid})
	require.False(t, res.IsError, "%+v", res.StructuredContent)

	// The effective model is switched to a remote provider after startup.
	u.set(func(u *fakeUAR) { u.field("llm")["provider_host"] = "api.remote.test" })

	// An open session keeps its cached verdict until the interval passes...
	res = toolCall(t, cs, "ipfs_node_info", nil)
	require.False(t, res.IsError, "within the interval the verdict is reused")

	// ...but the next session initialize is checked afresh and refused, for
	// that principal only.
	_, err = e.clientNamed(t, pTokUAR, "uar")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "check-uar")
	assert.Contains(t, err.Error(), FindingRemoteModel)
	_, err = e.clientNamed(t, pTokReader, "claude-code")
	require.NoError(t, err, "other principals keep running")

	// The refused initialize disabled the principal, so its open session is
	// refused too, with a structured error.
	res = toolCall(t, cs, "ipfs_cat", map[string]any{"path": "/ipfs/" + e.cid})
	assert.Equal(t, CodePermissionDenied, errCode(t, res))
	d := errDetails(t, res)
	assert.Equal(t, ReasonUARCheckFailed, d["reason"])
	assert.Contains(t, fmt.Sprint(d["findings"]), FindingRemoteModel)

	// Back to a local provider: a new session works again.
	u.set(func(u *fakeUAR) { u.field("llm")["provider_host"] = "127.0.0.1:11434" })
	_, err = e.clientNamed(t, pTokUAR, "uar")
	require.NoError(t, err)
}

func TestUAROpenSessionIsRecheckedAfterTheInterval(t *testing.T) {
	u := newFakeUAR(t, "profile")
	e := uarEnv(t, u, "", Options{})
	cs, err := e.clientNamed(t, pTokUAR, "uar")
	require.NoError(t, err)

	u.set(func(u *fakeUAR) { u.field("llm")["provider_host"] = "api.remote.test" })
	require.False(t, toolCall(t, cs, "ipfs_node_info", nil).IsError, "inside the interval")
	hits := u.total()

	e.clock.Advance(e.s.uar.checker.recheck() + time.Second)
	res := toolCall(t, cs, "ipfs_node_info", nil)
	assert.Equal(t, CodePermissionDenied, errCode(t, res), "a settings change is seen without a new session")
	assert.Greater(t, u.total(), hits, "the settings were read again")
}
func TestUARShellToolWithdrawsWriteGroups(t *testing.T) {
	u := newFakeUAR(t, "profile")
	e := uarEnv(t, u, "", Options{})
	u.set(func(u *fakeUAR) { u.field("native_tools")["terminal_exec_enabled"] = true })

	cs, err := e.clientNamed(t, pTokUAR, "uar")
	require.NoError(t, err, "the principal is downgraded, not disabled")
	names := listedNames(t, cs)
	assert.NotContains(t, names, "pin_add")
	assert.Contains(t, names, "pin_ls", "read tools stay")

	res := toolCall(t, cs, "pin_add", map[string]any{"path": "/ipfs/" + e.cid})
	assert.Equal(t, CodePermissionDenied, errCode(t, res))
	d := errDetails(t, res)
	assert.Equal(t, "uar_"+FindingShellToolEnabled, d["reason"])
	assert.NotNil(t, d["restrictions"])
	assert.Zero(t, e.d.count("pin/add"))

	// The operator turns terminal_exec off; the next session gets writes.
	u.set(func(u *fakeUAR) { u.field("native_tools")["terminal_exec_enabled"] = false })
	cs, err = e.clientNamed(t, pTokUAR, "uar")
	require.NoError(t, err)
	assert.Contains(t, listedNames(t, cs), "pin_add")
}

func TestUARUnreadableSettingsFailClosedUnlessAsserted(t *testing.T) {
	u := newFakeUAR(t, "profile")
	u.set(func(u *fakeUAR) { u.settingsStatus = 503 })
	e := uarEnv(t, u, "", Options{})
	_, err := e.clientNamed(t, pTokUAR, "uar")
	require.Error(t, err, "settings cannot be read, so the principal is disabled")
	assert.Contains(t, err.Error(), FindingSettingsUnreadable)

	u2 := newFakeUAR(t, "profile")
	u2.set(func(u *fakeUAR) { u.settingsStatus = 503 })
	e2 := uarEnv(t, u2, `, "check": "assert"`, Options{})
	cs, err := e2.clientNamed(t, pTokUAR, "uar")
	require.NoError(t, err, "check: assert records the operator's own verification")
	assert.Contains(t, listedNames(t, cs), "pin_add")
}

func TestUARVersionGate(t *testing.T) {
	u := newFakeUAR(t, "profile")
	u.set(func(u *fakeUAR) { u.version = "1.3.0" })
	e := uarEnv(t, u, "", Options{})
	_, err := e.clientNamed(t, pTokUAR, "uar")
	require.Error(t, err)
	assert.Contains(t, err.Error(), FindingVersionTooOld)
}

func TestUARStartupReport(t *testing.T) {
	u := newFakeUAR(t, "profile")
	e := uarEnv(t, u, "", Options{Resolvers: []Resolver{newFakeResolver(ResolverCLI)}})
	lines := e.hs.StartupReport(t.Context())
	joined := fmt.Sprint(lines)
	assert.Contains(t, joined, "principal runtime: check-uar ok")
	assert.Contains(t, joined, "write groups refused", "--confirm=cli without shell_capable: false is printed")
	assert.NotContains(t, joined, fakeUARAdminToken)

	u.set(func(u *fakeUAR) { u.field("llm")["provider_host"] = "api.remote.test" })
	lines = e.hs.StartupReport(t.Context())
	assert.Contains(t, fmt.Sprint(lines), "check-uar refuse")
	assertNoDestination(t, fmt.Sprint(lines), u)
}

// assertNoDestination fails when text carries the UAR card URL or its host: the
// startup report names the principal, never where it connects.
func assertNoDestination(t *testing.T, text string, u *fakeUAR) {
	t.Helper()
	parsed, err := url.Parse(u.cardURL())
	require.NoError(t, err)
	assert.NotContains(t, text, u.cardURL())
	assert.NotContains(t, text, parsed.Host)
	assert.NotContains(t, text, "api.remote.test", "a finding message can name a discovered host")
	assert.NotContains(t, text, "http://")
}

func TestNoUARContactWithoutAUARPrincipal(t *testing.T) {
	u := newFakeUAR(t, "profile")
	e := newPrincipalEnv(t, principalsConfig, principalTokens(), Options{}, nil)
	for _, tok := range []string{pTokReader, pTokWriter, pTokAdmin} {
		cs := e.client(t, tok, nil)
		_ = listedNames(t, cs)
	}
	assert.Empty(t, e.hs.StartupReport(t.Context()))
	assert.Zero(t, u.total(), "nothing is ever contacted unless a uar principal is configured")
}

func TestUARStartupReportPrintsTheAssertion(t *testing.T) {
	u := newFakeUAR(t, "profile")
	e := uarEnv(t, u, `, "check": "assert"`, Options{})
	joined := fmt.Sprint(e.hs.StartupReport(t.Context()))
	assert.Contains(t, joined, "principal runtime: check: assert is set")
	assert.Contains(t, joined, "the operator asserts")
	assertNoDestination(t, joined, u)
	assert.NotContains(t, joined, fakeUARAdminToken)

	u2 := newFakeUAR(t, "profile")
	e2 := uarEnv(t, u2, "", Options{})
	assert.NotContains(t, fmt.Sprint(e2.hs.StartupReport(t.Context())), "check: assert", "verify prints no assertion")
}
