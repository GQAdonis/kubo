//go:build !noagent

package server

import (
	"io"
	"net/http"
	"testing"

	"github.com/ipfs/kubo/mcp/ui/catalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of mcp-agui-a2ui-registry 1.1: the embedded catalog is
// served at the contract route behind the same protections as /agui, and the
// run stream advertises it once, immediately after RUN_STARTED. agent.a2ui is
// the off switch of both.

// request sends one raw request to the env's listener.
func (e *aguiEnv) request(t *testing.T, method, path, token string, hdr map[string]string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, e.base+path, nil)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, resp.Header, b
}

// TestAGUICatalogServesTheEmbeddedBytes: the route answers the generated
// catalog byte for byte, as application/json — the embedded file is the only
// copy the server has.
func TestAGUICatalogServesTheEmbeddedBytes(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{origins: []string{"webui://localhost"}})
	status, hdr, body := e.request(t, http.MethodGet, RouteAGUICatalog, aguiTokChat, nil)
	require.Equal(t, http.StatusOK, status, "%s", body)
	assert.Equal(t, "application/json", hdr.Get("Content-Type"))
	assert.Equal(t, catalog.Bytes(), body, "the route serves the embedded catalog unchanged")
}

// TestAGUICatalogProtection: the route passes the same mount chain as /agui —
// bearer authentication, the ui principal kind, the strict origin policy — and
// answers only GET.
func TestAGUICatalogProtection(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{origins: []string{"webui://localhost"}})

	status, _, _ := e.request(t, http.MethodGet, RouteAGUICatalog, "", nil)
	assert.Equal(t, http.StatusUnauthorized, status, "no token")
	for name, tok := range map[string]string{"human-mcp": aguiTokHuman, "a2a": aguiTokPeer} {
		status, _, _ := e.request(t, http.MethodGet, RouteAGUICatalog, tok, nil)
		assert.Equal(t, http.StatusForbidden, status, "%s principal", name)
	}

	status, hdr, _ := e.request(t, http.MethodPost, RouteAGUICatalog, aguiTokChat, nil)
	assert.Equal(t, http.StatusMethodNotAllowed, status)
	assert.Equal(t, http.MethodGet, hdr.Get("Allow"))

	status, _, _ = e.request(t, http.MethodGet, RouteAGUICatalog, aguiTokChat, map[string]string{"Origin": "https://evil.example"})
	assert.Equal(t, http.StatusForbidden, status, "an origin outside agent.origin_allowlist")

	status, hdr, _ = e.request(t, http.MethodGet, RouteAGUICatalog, aguiTokChat, map[string]string{"Origin": "webui://localhost"})
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "webui://localhost", hdr.Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, OPTIONS", hdr.Get("Access-Control-Allow-Methods"), "the catalog's strict CORS answer names GET")
	assert.Empty(t, hdr.Get("Access-Control-Allow-Credentials"))
}

// TestAGUICatalogOffSwitch: agent.a2ui off unmounts the route (a plain 404,
// like /agui with --agui off), and without /agui nothing is mounted at all.
func TestAGUICatalogOffSwitch(t *testing.T) {
	off := false
	e := newAGUIEnv(t, aguiEnvOpts{a2ui: &off})
	status, _, _ := e.request(t, http.MethodGet, RouteAGUICatalog, aguiTokChat, nil)
	assert.Equal(t, http.StatusNotFound, status, "agent.a2ui: false disables the catalog route")

	noAGUI := newAGUIEnv(t, aguiEnvOpts{noAGUI: true})
	status, _, _ = noAGUI.request(t, http.MethodGet, RouteAGUICatalog, aguiTokChat, nil)
	assert.Equal(t, http.StatusNotFound, status, "no /agui, no catalog route")
}

// TestAGUIRunAdvertisesTheCatalog: one CUSTOM event named by the contract
// (aguiEvents.catalog), immediately after RUN_STARTED, carrying the catalog ID
// and the route.
func TestAGUIRunAdvertisesTheCatalog(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, sayStep("Done."))
	r := e.post(aguiTokChat, offerA2UI(t, aguiBody("thread-cat", "r1", []any{userMsg("m1", "hi")})), nil)
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	require.GreaterOrEqual(t, len(r.Events), 2, "%v", r.types())
	require.Equal(t, "RUN_STARTED", r.Events[0]["type"])

	ad := r.Events[1]
	assert.Equal(t, "CUSTOM", ad["type"], "the advertisement follows RUN_STARTED immediately: %v", r.types())
	f := a2uiDerivationsForTest(t)
	assert.Equal(t, f.AguiEvents.Catalog, ad["name"])
	assert.Equal(t, map[string]any{"catalogId": f.CatalogID, "catalogVersion": f.CatalogVersion, "protocolVersion": f.ProtocolVersion, "catalogUrl": RouteAGUICatalog}, ad["value"])

	var ads int
	for _, ev := range r.Events {
		if ev["type"] == "CUSTOM" && ev["name"] == f.AguiEvents.Catalog {
			ads++
		}
	}
	assert.Equal(t, 1, ads, "exactly one catalog advertisement per run")
}
