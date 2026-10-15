package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	routeConsole = "https://console.example.test"

	tokUI       = "ui-index-token-0123456789"
	tokHuman    = "human-index-token-012345"
	tokNoIndex  = "ui-noindex-token-0123456"
	tokNoAck    = "ui-noack-token-012345678"
	tokWrongAck = "ui-wrongack-token-01234"
	tokRAdmin   = "admin-route-token-012345"
	tokRPeer    = "a2a-route-token-0123456"
)

// routeEnv is a running --http listener over a media environment, with the
// search routes enabled.
type routeEnv struct {
	*mediaEnv
	base string
}

type routeOpts struct {
	media mediaOpts
	// disabled leaves EnableIndexRoutes uncalled: embeddings.search_route.enabled
	// is false.
	disabled bool
	// noEmbeddings leaves the embeddings feature unconfigured.
	noEmbeddings bool
	modify       func(*HTTPOptions)
}

func newRouteEnv(t *testing.T, o routeOpts) *routeEnv {
	t.Helper()
	if o.media.policy.MIMEAllowlist == nil {
		o.media.policy = embeddings.Policy{MIMEAllowlist: []string{"text/plain", "image/png"}}
	}
	var m *mediaEnv
	if o.noEmbeddings {
		m = &mediaEnv{indexEnv: newIndexEnv(t, indexEnvOptions{noEmbeddings: true})}
	} else {
		m = newMediaEnv(t, o.media)
	}
	if !o.disabled {
		require.NoError(t, m.s.EnableIndexRoutes([]string{routeConsole}))
	}
	ack := []string{m.embed.host()}
	mk := func(id, kind string, groups []string, ack []string) Identity {
		i := NewIdentity(Principal{ID: id, Kind: kind}, groups, nil, nil)
		i.Configured = true
		i.EgressAck = ack
		return i
	}
	creds := []StaticCredential{
		{Identity: mk("chat", KindUI, []string{"index"}, ack), Token: tokUI},
		{Identity: mk("human", KindHumanMCP, []string{AllGroups}, []string{"local"}), Token: tokHuman},
		{Identity: mk("noindex", KindUI, []string{"node"}, ack), Token: tokNoIndex},
		{Identity: mk("noack", KindUI, []string{"index"}, nil), Token: tokNoAck},
		{Identity: mk("wrongack", KindUI, []string{"index"}, []string{"elsewhere.example.test"}), Token: tokWrongAck},
		{Identity: mk("admin", KindAdmin, []string{AllGroups}, ack), Token: tokRAdmin},
		{Identity: mk("peer", KindA2A, []string{AllGroups}, ack), Token: tokRPeer},
	}
	v, err := NewStaticVerifier(creds)
	require.NoError(t, err)
	opts := HTTPOptions{Listen: "127.0.0.1:0", Verifier: v}
	if o.modify != nil {
		o.modify(&opts)
	}
	hs, err := NewHTTPServer(m.s, opts)
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
	return &routeEnv{mediaEnv: m, base: "http://" + addr.String()}
}

type routeResp struct {
	status int
	header http.Header
	body   string
}

func (e *routeEnv) do(t *testing.T, method, path, token, body string, header map[string]string) routeResp {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, e.base+path, strings.NewReader(body))
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return routeResp{status: resp.StatusCode, header: resp.Header, body: string(b)}
}

func (r routeResp) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.body), &m), r.body)
	return m
}

func (r routeResp) errorCode(t *testing.T) string {
	t.Helper()
	e, _ := r.json(t)["error"].(map[string]any)
	require.NotNil(t, e, r.body)
	return e["code"].(string)
}

const searchBody = `{"query":"cats","top_k":3}`

// seedIndex indexes a text file and a described image.
func (e *routeEnv) seedIndex(t *testing.T) {
	t.Helper()
	e.files["/ipfs/bafyText"] = []byte("cats purr in the sun")
	e.index(indexIdentity(e.embed.host()), "bafyText", false)
}

// --- 1.5: mounting ----------------------------------------------------------------------------

func TestIndexRoutesAnswer404WhenSwitchedOffAndReadNothing(t *testing.T) {
	for name, o := range map[string]routeOpts{
		"search_route.enabled false": {disabled: true},
		"no embeddings section":      {noEmbeddings: true},
	} {
		t.Run(name, func(t *testing.T) {
			e := newRouteEnv(t, o)
			calls := e.d.count("cat") + e.d.count("pin/ls")
			for _, p := range []struct{ method, path, body string }{
				{http.MethodPost, RouteIndexSearch, searchBody},
				{http.MethodGet, RouteIndexStatus, ""},
			} {
				resp := e.do(t, p.method, p.path, tokUI, p.body, nil)
				assert.Equal(t, http.StatusNotFound, resp.status, "%s %s", p.method, p.path)
			}
			assert.Equal(t, calls, e.d.count("cat")+e.d.count("pin/ls"), "nothing was read from the node")
			assert.Zero(t, e.embed.requests.Load(), "nothing was dialed")
		})
	}
}

func TestIndexRoutesRequireAKnownBearer(t *testing.T) {
	e := newRouteEnv(t, routeOpts{})
	for _, token := range []string{"", "not-a-token-0123456789012"} {
		assert.Equal(t, http.StatusUnauthorized, e.do(t, http.MethodPost, RouteIndexSearch, token, searchBody, nil).status)
		assert.Equal(t, http.StatusUnauthorized, e.do(t, http.MethodGet, RouteIndexStatus, token, "", nil).status)
	}
	assert.Zero(t, e.embed.requests.Load())
}

func TestIndexRoutesRefuseKindsAndMissingGroups(t *testing.T) {
	e := newRouteEnv(t, routeOpts{})
	for name, tok := range map[string]string{"admin": tokRAdmin, "a2a peer": tokRPeer} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, http.StatusForbidden, e.do(t, http.MethodPost, RouteIndexSearch, tok, searchBody, nil).status)
			assert.Equal(t, http.StatusForbidden, e.do(t, http.MethodGet, RouteIndexStatus, tok, "", nil).status)
		})
	}
	t.Run("a ui principal without the index group", func(t *testing.T) {
		resp := e.do(t, http.MethodPost, RouteIndexSearch, tokNoIndex, searchBody, nil)
		assert.Equal(t, http.StatusForbidden, resp.status)
		assert.Equal(t, CodePermissionDenied, resp.errorCode(t))
		assert.Equal(t, http.StatusForbidden, e.do(t, http.MethodGet, RouteIndexStatus, tokNoIndex, "", nil).status)
	})
	assert.Zero(t, e.embed.requests.Load(), "no embedder request was made for a refused caller")
}

func TestIndexRoutesApplyTheToolsEgressRule(t *testing.T) {
	e := newRouteEnv(t, routeOpts{})
	t.Run("no egress_ack at all", func(t *testing.T) {
		resp := e.do(t, http.MethodPost, RouteIndexSearch, tokNoAck, searchBody, nil)
		assert.Equal(t, http.StatusForbidden, resp.status)
		assert.Equal(t, CodeEgressNotAcknowledged, resp.errorCode(t))
	})
	t.Run("the embedder host is not acknowledged", func(t *testing.T) {
		resp := e.do(t, http.MethodPost, RouteIndexSearch, tokWrongAck, searchBody, nil)
		assert.Equal(t, http.StatusForbidden, resp.status)
		assert.Equal(t, CodeEgressNotAcknowledged, resp.errorCode(t))
		details := resp.json(t)["error"].(map[string]any)["details"].(map[string]any)
		assert.Equal(t, []any{e.embed.host()}, details["missing_hosts"], "the response names the host")
	})
	assert.Zero(t, e.embed.requests.Load(), "nothing was sent to the embedder")
}

// --- 1.5: the shapes -------------------------------------------------------------------------------

func TestIndexSearchRouteReturnsTheToolsOutput(t *testing.T) {
	e := newRouteEnv(t, routeOpts{media: mediaOpts{
		policy:    embeddings.Policy{MIMEAllowlist: []string{"text/plain", "image/png"}},
		describer: func(*embeddings.DescriberSettings) {},
	}})
	e.seedIndex(t)
	e.files["/ipfs/bafyPNG"] = pngBytes
	e.index(e.principal(), "bafyPNG", false)

	resp := e.do(t, http.MethodPost, RouteIndexSearch, tokUI, searchBody, nil)
	require.Equal(t, http.StatusOK, resp.status, resp.body)
	assert.Equal(t, "application/json", resp.header.Get("Content-Type"))
	assert.Equal(t, "no-store", resp.header.Get("Cache-Control"))
	out := resp.json(t)
	validateOutput(t, "ipfs_index_search", out)
	results := out["results"].([]any)
	require.Len(t, results, 2)
	bySource := map[string]map[string]any{}
	for _, r := range results {
		hit := r.(map[string]any)
		bySource[hit["source"].(string)] = hit
	}
	assert.Equal(t, "vision-test", bySource["description"]["described_by"], "descriptions are labelled with the model on the route too")
	assert.NotContains(t, bySource["text"], "described_by")
}

func TestIndexSearchRouteRejectsBodiesThatFailTheToolSchema(t *testing.T) {
	e := newRouteEnv(t, routeOpts{})
	for name, body := range map[string]string{
		"unknown field":  `{"query":"cats","extra":1}`,
		"top_k too high": `{"query":"cats","top_k":51}`,
		"no query":       `{}`,
		"not json":       `cats`,
	} {
		t.Run(name, func(t *testing.T) {
			resp := e.do(t, http.MethodPost, RouteIndexSearch, tokUI, body, nil)
			assert.Equal(t, http.StatusBadRequest, resp.status)
			assert.Equal(t, CodeInvalidArgument, resp.errorCode(t))
		})
	}
	assert.Zero(t, e.embed.requests.Load())
}

func TestIndexSearchRouteBodyLimit(t *testing.T) {
	e := newRouteEnv(t, routeOpts{modify: func(o *HTTPOptions) { o.MaxBodyBytes = 64 }})
	resp := e.do(t, http.MethodPost, RouteIndexSearch, tokUI, `{"query":"`+strings.Repeat("c", 200)+`"}`, nil)
	assert.GreaterOrEqual(t, resp.status, 400)
	assert.Less(t, resp.status, 500, "the listener's body limit applies")
	assert.Zero(t, e.embed.requests.Load())
}

func TestIndexSearchRouteStoreDownIsIndexUnavailable(t *testing.T) {
	e := newRouteEnv(t, routeOpts{})
	e.embed.down.Store(true)
	resp := e.do(t, http.MethodPost, RouteIndexSearch, tokUI, searchBody, nil)
	assert.Equal(t, http.StatusServiceUnavailable, resp.status)
	assert.Equal(t, CodeIndexUnavailable, resp.errorCode(t))
	assert.NotEmpty(t, resp.header.Get("Retry-After"))
}

func TestIndexRoutesMethods(t *testing.T) {
	e := newRouteEnv(t, routeOpts{})
	resp := e.do(t, http.MethodGet, RouteIndexSearch, tokUI, "", nil)
	assert.Equal(t, http.StatusMethodNotAllowed, resp.status)
	assert.Equal(t, http.MethodPost, resp.header.Get("Allow"))
	resp = e.do(t, http.MethodPost, RouteIndexStatus, tokUI, "{}", nil)
	assert.Equal(t, http.StatusMethodNotAllowed, resp.status)
	assert.Equal(t, http.MethodGet, resp.header.Get("Allow"))
}

func TestIndexStatusRouteAddsTheBulkSummary(t *testing.T) {
	e := newRouteEnv(t, routeOpts{})
	e.seedIndex(t)

	resp := e.do(t, http.MethodGet, RouteIndexStatus, tokUI, "", nil)
	require.Equal(t, http.StatusOK, resp.status, resp.body)
	out := resp.json(t)
	validateOutput(t, "ipfs_index_status", withoutKey(out, "bulk_run"))
	assert.Equal(t, true, out["enabled"])
	assert.EqualValues(t, 1, out["indexed_cids"])
	assert.NotContains(t, out, "bulk_run", "no run has ever been started")

	require.NoError(t, e.store.SaveRun(t.Context(), embeddings.Run{
		ID: "r1", State: embeddings.RunPaused, StartedBy: "chat", Sources: []string{"pins"},
		Caps: embeddings.RunCaps{MaxFiles: 10, MaxBytes: 1000, FilesPerMinute: 60}, FilesIndexed: 2,
	}))
	out = e.do(t, http.MethodGet, RouteIndexStatus, tokUI, "", nil).json(t)
	validateOutput(t, "ipfs_index_status", withoutKey(out, "bulk_run"))
	run, ok := out["bulk_run"].(map[string]any)
	require.True(t, ok, "%v", out)
	validateOutput(t, "ipfs_index_bulk_status", map[string]any{"enabled": true, "run": run})
	assert.Equal(t, "paused", run["state"])

	out = e.do(t, http.MethodGet, RouteIndexStatus+"?cid=bafyText", tokUI, "", nil).json(t)
	assert.Equal(t, true, out["cid_indexed"])
	assert.Equal(t, http.StatusBadRequest, e.do(t, http.MethodGet, RouteIndexStatus+"?cid=not%20a%20cid", tokUI, "", nil).status)
}

func TestIndexStatusRouteContactsNoEmbedder(t *testing.T) {
	e := newRouteEnv(t, routeOpts{})
	resp := e.do(t, http.MethodGet, RouteIndexStatus, tokWrongAck, "", nil)
	assert.Equal(t, http.StatusOK, resp.status, "status carries no content egress, so a principal without the embedder host may read it")
	assert.Zero(t, e.embed.requests.Load())
}

func withoutKey(m map[string]any, key string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if k != key {
			out[k] = v
		}
	}
	return out
}

// --- CORS --------------------------------------------------------------------------------------------

func TestIndexRoutesCORSFollowsTheOriginAllowlist(t *testing.T) {
	e := newRouteEnv(t, routeOpts{})
	listed := map[string]string{"Origin": routeConsole}

	resp := e.do(t, http.MethodPost, RouteIndexSearch, tokUI, searchBody, listed)
	require.Equal(t, http.StatusOK, resp.status, resp.body)
	assert.Equal(t, routeConsole, resp.header.Get("Access-Control-Allow-Origin"), "the origin is echoed, never *")
	assert.Equal(t, "Origin", resp.header.Get("Vary"))
	assert.Empty(t, resp.header.Get("Access-Control-Allow-Credentials"), "the bearer is the only credential")

	pre := e.do(t, http.MethodOptions, RouteIndexSearch, "", "", map[string]string{
		"Origin": routeConsole, "Access-Control-Request-Method": "POST", "Access-Control-Request-Headers": "authorization,content-type",
	})
	assert.Equal(t, http.StatusNoContent, pre.status)
	assert.Equal(t, "GET, POST, OPTIONS", pre.header.Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "authorization, content-type", pre.header.Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "Retry-After", pre.header.Get("Access-Control-Expose-Headers"))

	bad := e.do(t, http.MethodPost, RouteIndexSearch, tokUI, searchBody, map[string]string{"Origin": "https://evil.example.test"})
	assert.Equal(t, http.StatusForbidden, bad.status)
	assert.Empty(t, bad.header.Get("Access-Control-Allow-Origin"), "an unlisted origin gets no CORS headers")
}

func TestIndexRoutesAreNotServedOnTheMCPSession(t *testing.T) {
	// The routes add no tool: the MCP surface of the same server is unchanged.
	e := newRouteEnv(t, routeOpts{})
	id := indexIdentity(e.embed.host())
	cs := e.connect(&id)
	tools := toolNames(t, cs)
	for _, name := range indexToolNames {
		assert.Contains(t, tools, name)
	}
}
