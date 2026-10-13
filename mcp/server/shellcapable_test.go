package server

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const shellConfig = `{
  "principals": [
    {"name": "reader",     "kind": "human-mcp", "credential": {"env": "P_READER"}},
    {"name": "unasserted", "kind": "human-mcp", "credential": {"env": "P_WRITER"}, "write_groups": ["pin"]},
    {"name": "maybe",      "kind": "human-mcp", "credential": {"env": "P_ADMIN"},  "write_groups": ["pin"], "shell_capable": true},
    {"name": "no-shell",   "kind": "human-mcp", "credential": {"env": "P_CHAT"},   "write_groups": ["pin"], "shell_capable": false},
    {"name": "chat",       "kind": "ui",        "credential": {"env": "P_AGENT"},  "write_groups": ["pin"], "destructive_groups": ["pin"], "shell_capable": false}
  ]
}`

func shellTokens() map[string]string {
	return map[string]string{
		"P_READER": pTokReader, "P_WRITER": pTokWriter, "P_ADMIN": pTokAdmin, "P_CHAT": pTokChat, "P_AGENT": pTokAgent,
	}
}

func TestCLIConfirmationRequiresShellCapableFalse(t *testing.T) {
	e := newPrincipalEnv(t, shellConfig, shellTokens(), Options{Resolvers: []Resolver{newFakeResolver(ResolverCLI)}}, nil)

	for name, token := range map[string]string{"unasserted": pTokWriter, "shell_capable true": pTokAdmin} {
		cs := e.client(t, token, nil)
		assert.NotContains(t, listedNames(t, cs), "pin_add", name)
		res := toolCall(t, cs, "pin_add", map[string]any{"path": "/ipfs/" + e.cid})
		assert.Equal(t, CodePermissionDenied, errCode(t, res), name)
		d := errDetails(t, res)
		assert.Equal(t, ReasonShellCapableUnasserted, d["reason"], name)
		assert.Contains(t, fmt.Sprint(d["restrictions"]), "shell_capable", name)
	}
	assert.Zero(t, e.d.count("pin/add"), "a refused write principal never changed anything")

	// The assertion false keeps the write group; the call then reaches the
	// confirmation gate, where the CLI resolver holds it for a person.
	cs := e.client(t, pTokChat, nil)
	assert.Contains(t, listedNames(t, cs), "pin_add")
	res := toolCall(t, cs, "pin_add", map[string]any{"path": "/ipfs/" + e.cid})
	assert.Equal(t, CodeConfirmRequired, errCode(t, res))
	assert.Zero(t, e.d.count("pin/add"), "nothing runs before the human decision")

	// Read-only principals are unaffected.
	assert.Contains(t, listedNames(t, e.client(t, pTokReader, nil)), "ipfs_node_info")
}

func TestShellCapableNotRequiredWithoutCLIConfirmation(t *testing.T) {
	e := newPrincipalEnv(t, shellConfig, shellTokens(), Options{Resolvers: []Resolver{newFakeResolver(ResolverLinuxPolkit)}}, nil)
	for _, token := range []string{pTokWriter, pTokAdmin, pTokChat} {
		assert.Contains(t, listedNames(t, e.client(t, token, nil)), "pin_add")
	}
	assert.Empty(t, e.hs.StartupReport(t.Context()), "nothing to report when the CLI resolver is off")
}

func TestUIDestructiveNeedsAPresenceResolverNotCLI(t *testing.T) {
	// chat is shell_capable: false, so --confirm=cli leaves its destructive
	// group alone. The CLI has no presence factor, so a destructive call from
	// the ui principal still cannot be approved.
	e := newPrincipalEnv(t, shellConfig, shellTokens(), Options{Resolvers: []Resolver{newFakeResolver(ResolverCLI)}}, nil)
	cs := e.client(t, pTokAgent, nil)
	require.Contains(t, listedNames(t, cs), "pin_rm")
	res := toolCall(t, cs, "pin_rm", map[string]any{"path": "/ipfs/" + e.cid})
	assert.Equal(t, CodeConfirmUnavailable, errCode(t, res))
	assert.Zero(t, e.d.count("pin/rm"))
}

func TestStartupReportNamesShellCapablePrincipals(t *testing.T) {
	e := newPrincipalEnv(t, shellConfig, shellTokens(), Options{Resolvers: []Resolver{newFakeResolver(ResolverCLI)}}, nil)
	joined := fmt.Sprint(e.hs.StartupReport(t.Context()))
	assert.Contains(t, joined, "principal unasserted: write groups refused")
	assert.Contains(t, joined, "principal maybe: write groups refused")
	assert.NotContains(t, joined, "principal no-shell")
	assert.NotContains(t, joined, "principal chat")
}

func TestOperatorIdentityIsNotExemptUnderCLIConfirmation(t *testing.T) {
	s := mustNew(t, newFakeDaemon(t), Options{Version: "test", Resolvers: []Resolver{newFakeResolver(ResolverCLI)}})
	op, err := OperatorIdentity(false)
	require.NoError(t, err)

	r := s.shellRestriction(op)
	require.NotNil(t, r, "the --auth-token-* operator gets no exemption")
	assert.Equal(t, ReasonShellCapableUnasserted, r.Reason)
	for _, want := range []string{"human-mcp", "write_groups", "shell_capable: false", "--mcp-config"} {
		assert.Contains(t, r.Message, want)
	}

	lines := fmt.Sprint(s.StartupReport(t.Context(), []Identity{op}))
	assert.Contains(t, lines, "principal "+OperatorPrincipalID+": write groups refused")
	assert.Contains(t, lines, "human-mcp")
	assert.Contains(t, lines, "shell_capable: false")

	// Without the CLI resolver nothing is withdrawn.
	plain := mustNew(t, newFakeDaemon(t), Options{Version: "test"})
	assert.Nil(t, plain.shellRestriction(op))
}
