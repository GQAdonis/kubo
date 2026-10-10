package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// Tokens of the configured test principals. Credentials in the config files
// are env references; principalEnv sets the variables.
const (
	pTokReader = "p-reader-token-0123456789"
	pTokWriter = "p-writer-token-0123456789"
	pTokAdmin  = "p-admin-token-01234567890"
	pTokChat   = "p-chat-token-012345678901"
	pTokUAR    = "p-uar-token-0123456789012"
	pTokAgent  = "p-agent-token-01234567890"
)

// principalEnv is a running HTTP listener whose principals come from a config
// file, the way ipfs mcp builds them.
type principalEnv struct {
	*httpEnv
	cf    *contentFake
	clock *fakeClock
	cfg   *Config
	// file is the content ipfs_cat returns.
	cid string
}

// allGroupsAndDestructive selects every tool group plus the destructive
// selector.
func allGroupsAndDestructive(t testing.TB) []string {
	t.Helper()
	cat, err := contract.Load()
	require.NoError(t, err)
	sel := groupsFromCatalog(cat)
	return append(sel, cat.ToolSelection.DestructiveSelector)
}

func groupsFromCatalog(cat *contract.Catalog) []string {
	names := make([]string, 0, len(cat.Groups))
	for _, g := range cat.Groups {
		names = append(names, g.Name)
	}
	return names
}

// newPrincipalEnv parses cfgJSON, sets env, and serves every tool group over
// a content fake. sopts.Tools defaults to every group and destructive; the
// fake clock drives the UAR re-check interval.
func newPrincipalEnv(t *testing.T, cfgJSON string, env map[string]string, sopts Options, hmod func(*HTTPOptions)) *principalEnv {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
	cfg, err := ParseConfig([]byte(cfgJSON))
	require.NoError(t, err)
	creds, err := cfg.Credentials()
	require.NoError(t, err)
	verifier, err := NewStaticVerifier(creds)
	require.NoError(t, err)

	target := testCID(t, 7)
	d := catFake(t, map[string][]byte{"/ipfs/" + target: []byte("SECRET-NODE-BYTES")})
	installPins(d.fakeDaemon, map[string]fakePin{target: {Type: "recursive"}})
	clock := newFakeClock()
	if sopts.Tools == nil {
		sopts.Tools = allGroupsAndDestructive(t)
	}
	sopts.Transport = "http"
	sopts.now = clock.Now
	s := mustNew(t, d, sopts)

	opts := HTTPOptions{Listen: "127.0.0.1:0", Verifier: verifier}
	if hmod != nil {
		hmod(&opts)
	}
	hs, err := NewHTTPServer(s, opts)
	require.NoError(t, err)
	addr, err := hs.Listen()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- hs.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-done)
	})
	he := &httpEnv{s: s, hs: hs, base: "http://" + addr.String(), d: d.fakeDaemon, target: target}
	return &principalEnv{httpEnv: he, cf: d, clock: clock, cfg: cfg, cid: target}
}

// errCode returns the ToolError code of a result that must be an error.
func errCode(t testing.TB, res *mcp.CallToolResult) string {
	t.Helper()
	require.True(t, res.IsError, "want an error result, got %+v", res.StructuredContent)
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var out toolErrorResult
	require.NoError(t, json.Unmarshal(raw, &out))
	require.NotNil(t, out.Error)
	return out.Error.Code
}

// errDetails returns the details of the ToolError of an error result.
func errDetails(t testing.TB, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var out toolErrorResult
	require.NoError(t, json.Unmarshal(raw, &out))
	require.NotNil(t, out.Error)
	return out.Error.Details
}

// clientNamed opens an MCP session over HTTP whose clientInfo has the name.
func (e *principalEnv) clientNamed(t *testing.T, token, name string) (*mcp.ClientSession, error) {
	t.Helper()
	transport := &mcp.StreamableClientTransport{
		Endpoint:   e.base + RouteMCP,
		HTTPClient: &http.Client{Transport: bearerTransport{token: token}},
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: name, Version: "0"}, nil).Connect(t.Context(), transport, nil)
	if err == nil {
		t.Cleanup(func() { _ = cs.Close() })
	}
	return cs, err
}

// listedNames returns the tool names a session lists.
func listedNames(t testing.TB, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}
