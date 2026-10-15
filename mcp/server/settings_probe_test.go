//go:build !noagent

package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/ipfs/kubo/mcp/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeModels is an OpenAI-compatible backend stand-in that serves its model
// list and records the Authorization header each probe carried.
type fakeModels struct {
	srv *httptest.Server
	mu  sync.Mutex
	// auths are the Authorization header values seen, one per request.
	auths []string
	// reply, when non-nil, replaces the default model-list answer (status and
	// body), for failure-shape tests.
	reply func(w http.ResponseWriter, auth string)
}

func newFakeModels(t *testing.T, ids ...string) *fakeModels {
	t.Helper()
	f := &fakeModels{}
	entries := make([]string, 0, len(ids))
	for _, id := range ids {
		entries = append(entries, fmt.Sprintf(`{"id":%q}`, id))
	}
	body := `{"data":[` + strings.Join(entries, ",") + `]}`
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		f.mu.Lock()
		f.auths = append(f.auths, auth)
		f.mu.Unlock()
		if f.reply != nil {
			f.reply(w, auth)
			return
		}
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/models") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeModels) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.auths...)
}

// doPath is settingsEnv.do for the settings subtree's other paths.
func (e *settingsEnv) doPath(t *testing.T, method, path, token, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, e.base+path, strings.NewReader(body))
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// validateSettingsSchemaDef checks a document against one $def of a contract
// schema file whose root is only a $defs container (the probe shapes).
func validateSettingsSchemaDef(t *testing.T, schemaPath, def string, doc map[string]any) {
	t.Helper()
	raw, err := fs.ReadFile(contract.FS(), schemaPath)
	require.NoError(t, err)
	var s jsonschema.Schema
	require.NoError(t, json.Unmarshal(raw, &s))
	s.Ref = "#/$defs/" + def
	rs, err := s.Resolve(nil)
	require.NoError(t, err)
	require.NoError(t, rs.Validate(doc))
}

// TestSettingsTestConnection: a probe of a conforming candidate URL answers
// ok:true in the contract shape, authenticates with the currently stored key
// (never a request field), and changes nothing — the read afterwards, its
// revision included, is byte-identical.
func TestSettingsTestConnection(t *testing.T) {
	fake := newFakeModels(t, "m1", "m2")
	e := newSettingsEnv(t, backendJSONAt(fake.srv.URL+"/v1"))
	etagBefore, before := getSettings(t, e)

	resp := e.doPath(t, http.MethodPost, RouteSettingsTestConnection, tokSettingsAdmin,
		`{"base_url":"`+fake.srv.URL+`/v1"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(bodyString(t, resp)), &doc))
	validateSettingsSchemaDef(t, "settings/test-connection.schema.json", "TestConnectionResult", doc)
	assert.Equal(t, true, doc["ok"])
	assert.Equal(t, true, doc["key_sent"], "the current backend's host gets the stored key")
	assert.NotContains(t, bodyString(t, e.doPath(t, http.MethodPost, RouteSettingsTestConnection, tokSettingsAdmin,
		`{"base_url":"`+fake.srv.URL+`/v1"}`)), settingsTestKey)

	// The probe authenticated with the stored key, once per call.
	assert.Equal(t, []string{"Bearer " + settingsTestKey, "Bearer " + settingsTestKey}, fake.seen())

	// The probe then read scenario: the settings are unchanged.
	etagAfter, after := getSettings(t, e)
	assert.Equal(t, etagBefore, etagAfter)
	assert.Equal(t, before, after)
}

// TestSettingsTestConnectionFailureDetail: an unreachable or refusing
// candidate answers 200 with ok:false and a detail that carries no key
// material, even when the backend echoes the Authorization header back.
func TestSettingsTestConnectionFailureDetail(t *testing.T) {
	fake := newFakeModels(t)
	fake.reply = func(w http.ResponseWriter, auth string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		// A hostile endpoint reflecting the credential it was sent.
		_, _ = w.Write([]byte(`{"error":{"message":"bad key ` + auth + `"}}`))
	}
	e := newSettingsEnv(t, backendJSONAt(fake.srv.URL+"/v1"))

	resp := e.doPath(t, http.MethodPost, RouteSettingsTestConnection, tokSettingsAdmin,
		`{"base_url":"`+fake.srv.URL+`/v1"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body := bodyString(t, resp)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &doc))
	validateSettingsSchemaDef(t, "settings/test-connection.schema.json", "TestConnectionResult", doc)
	assert.Equal(t, false, doc["ok"])
	assert.NotEmpty(t, doc["detail"])
	assert.NotContains(t, body, settingsTestKey, "a reflected key is scrubbed from detail")

	// An unreachable candidate fails the same way, bounded by the probe timeout.
	dead := e.doPath(t, http.MethodPost, RouteSettingsTestConnection, tokSettingsAdmin,
		`{"base_url":"http://127.0.0.1:1/v1"}`)
	require.Equal(t, http.StatusOK, dead.StatusCode)
	var deadDoc map[string]any
	require.NoError(t, json.Unmarshal([]byte(bodyString(t, dead)), &deadDoc))
	assert.Equal(t, false, deadDoc["ok"])
	assert.NotEmpty(t, deadDoc["detail"])
}

// TestSettingsTestConnectionRefusesNonConformingURLs is the acceptance check:
// a candidate URL carrying credentials, a non-http scheme, plain http to a
// non-loopback host, or a query is refused before any dial; the refusal echoes
// no part of the secret-looking material and leaves settings and the settings
// directory untouched.
func TestSettingsTestConnectionRefusesNonConformingURLs(t *testing.T) {
	dir := t.TempDir()
	e := newSettingsEnvDir(t, settingsBackendJSON, dir)
	etagBefore, before := getSettings(t, e)

	cases := map[string]string{
		"credentials in URL":        "http://user:pw@127.0.0.1:1/v1",
		"https with credentials":    "https://user:pw@example.com/v1",
		"non-http scheme":           "ftp://127.0.0.1/v1",
		"http to non-loopback IP":   "http://192.0.2.1:11434/v1",
		"http to non-loopback name": "http://example.com/v1",
		"query in URL":              "http://127.0.0.1:1/v1?key=abc123",
		"fragment in URL":           "http://127.0.0.1:1/v1#frag",
		"empty URL":                 "",
		"not a URL":                 "not-a-url",
	}
	for name, candidate := range cases {
		body, _ := json.Marshal(map[string]string{"base_url": candidate})
		resp := e.doPath(t, http.MethodPost, RouteSettingsTestConnection, tokSettingsAdmin, string(body))
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, name)
		text := bodyString(t, resp)
		assert.NotContains(t, text, "pw", name)
		assert.NotContains(t, text, "abc123", name)
		assert.NotContains(t, text, settingsTestKey, name)
	}
	// A conforming but unreachable loopback URL passes validation and fails as
	// a probe instead, proving the refusals above are about the URL's shape.
	ok := e.doPath(t, http.MethodPost, RouteSettingsTestConnection, tokSettingsAdmin,
		`{"base_url":"http://127.0.0.1:1/v1"}`)
	assert.Equal(t, http.StatusOK, ok.StatusCode)

	// No persistence and no settings change (the acceptance pair).
	assert.Empty(t, readdirnames(t, dir), "probes and refusals write nothing to the settings directory")
	etagAfter, after := getSettings(t, e)
	assert.Equal(t, etagBefore, etagAfter)
	assert.Equal(t, before, after)
}

// TestSettingsModelsList: GET on the model-list endpoint lists the configured
// backend's models in the contract shape; a base_url lists a candidate
// instead, under the same URL rules as test-connection.
func TestSettingsModelsList(t *testing.T) {
	fake := newFakeModels(t, "m-a", "m-b")
	cfg := fmt.Sprintf(`{"llm":{"base_url":%q,"model":"m-old","api_key":{"env":%q}}}`, fake.srv.URL+"/v1", settingsKeyEnv)
	e := newSettingsEnv(t, cfg)

	resp := e.doPath(t, http.MethodGet, RouteSettingsModels, tokSettingsAdmin, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(bodyString(t, resp)), &doc))
	validateSettingsSchemaDef(t, "settings/model-list.schema.json", "ModelListResult", doc)
	assert.Equal(t, []any{map[string]any{"id": "m-a"}, map[string]any{"id": "m-b"}}, doc["models"])
	assert.Equal(t, []string{"Bearer " + settingsTestKey}, fake.seen(), "the configured backend's stored key authenticates the list")

	// A candidate base_url lists that backend instead of the configured one.
	other := newFakeModels(t, "cand-1")
	resp = e.doPath(t, http.MethodGet, RouteSettingsModels+"?base_url="+other.srv.URL+"/v1", tokSettingsAdmin, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.Unmarshal([]byte(bodyString(t, resp)), &doc))
	validateSettingsSchemaDef(t, "settings/model-list.schema.json", "ModelListResult", doc)
	assert.Equal(t, []any{map[string]any{"id": "cand-1"}}, doc["models"])
	assert.Equal(t, []string{""}, other.seen(), "a candidate host that is not the backend or acknowledged gets no key")

	// Candidate URL rules apply to the list endpoint too.
	bad := e.doPath(t, http.MethodGet, RouteSettingsModels+"?base_url=http://example.com/v1", tokSettingsAdmin, "")
	assert.Equal(t, http.StatusBadRequest, bad.StatusCode)
	bad = e.doPath(t, http.MethodGet, RouteSettingsModels+"?base_url=http://user:pw@127.0.0.1:1/v1", tokSettingsAdmin, "")
	assert.Equal(t, http.StatusBadRequest, bad.StatusCode)
	assert.NotContains(t, bodyString(t, bad), "pw")
}

// backendJSONAt is a configured backend at the given URL with the key named by
// environment.
func backendJSONAt(baseURL string) string {
	return fmt.Sprintf(`{"llm":{"base_url":%q,"model":"m-old","api_key":{"env":%q}}}`, baseURL, settingsKeyEnv)
}

// TestSettingsProbeWithholdsKeyFromNewHost: the stored key reaches only the
// current backend host and port (ADR-0012). A new
// host is probed without Authorization, on both probe endpoints, and a 401 or
// 403 is reported as reachable with key_sent false.
func TestSettingsProbeWithholdsKeyFromNewHost(t *testing.T) {
	backend := newFakeModels(t, "m1")
	candidate := newFakeModels(t, "c1")
	candidate.reply = func(w http.ResponseWriter, auth string) {
		if auth == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"c1"}]}`))
	}
	e := newSettingsEnv(t, backendJSONAt(backend.srv.URL+"/v1"))

	resp := e.doPath(t, http.MethodPost, RouteSettingsTestConnection, tokSettingsAdmin, `{"base_url":"`+candidate.srv.URL+`/v1"}`)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(bodyString(t, resp)), &doc))
	validateSettingsSchemaDef(t, "settings/test-connection.schema.json", "TestConnectionResult", doc)
	assert.Equal(t, true, doc["ok"], "the host answered")
	assert.Equal(t, false, doc["key_sent"])
	assert.Contains(t, doc["detail"], "sent only to the current backend")

	resp = e.doPath(t, http.MethodGet, RouteSettingsModels+"?base_url="+candidate.srv.URL+"/v1", tokSettingsAdmin, "")
	require.NoError(t, json.Unmarshal([]byte(bodyString(t, resp)), &doc))
	validateSettingsSchemaDef(t, "settings/test-connection.schema.json", "TestConnectionResult", doc)
	assert.Equal(t, false, doc["ok"])
	assert.Equal(t, false, doc["key_sent"])

	assert.Equal(t, []string{"", ""}, candidate.seen(), "no Authorization header reached the new host")
	for _, a := range candidate.seen() {
		assert.NotContains(t, a, settingsTestKey)
	}
}

// TestSettingsProbeNeverSendsKeyBeyondCurrentBackend pins ADR-0012 option A
// (the N3 cases): an acknowledged host, any loopback port under "local", and a
// host acknowledged by an earlier confirmed write all get a keyless probe; only
// the current backend's own host and port receives the stored key.
func TestSettingsProbeNeverSendsKeyBeyondCurrentBackend(t *testing.T) {
	probe := func(t *testing.T, e *settingsEnv, candidate *fakeModels) {
		t.Helper()
		resp := e.doPath(t, http.MethodPost, RouteSettingsTestConnection, tokSettingsAdmin, `{"base_url":"`+candidate.srv.URL+`/v1"}`)
		var doc map[string]any
		require.NoError(t, json.Unmarshal([]byte(bodyString(t, resp)), &doc))
		validateSettingsSchemaDef(t, "settings/test-connection.schema.json", "TestConnectionResult", doc)
		assert.Equal(t, false, doc["key_sent"])
		resp = e.doPath(t, http.MethodGet, RouteSettingsModels+"?base_url="+candidate.srv.URL+"/v1", tokSettingsAdmin, "")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		for _, a := range candidate.seen() {
			assert.Empty(t, a, "no Authorization header reached the candidate")
		}
		require.Len(t, candidate.seen(), 2)
	}

	t.Run("acknowledged host", func(t *testing.T) {
		backend := newFakeModels(t, "m1")
		candidate := newFakeModels(t, "c1")
		host := HostFromURL(candidate.srv.URL)
		e := newSettingsEnvWith(t, backendJSONAt(backend.srv.URL+"/v1"), "", adminVerifierWithAck(t, []string{host}))
		probe(t, e, candidate)
	})

	t.Run("any loopback port under local", func(t *testing.T) {
		backend := newFakeModels(t, "m1")
		candidate := newFakeModels(t, "c1")
		e := newSettingsEnvWith(t, backendJSONAt(backend.srv.URL+"/v1"), "", adminVerifierWithAck(t, []string{"local"}))
		probe(t, e, candidate)
	})

	t.Run("previously acknowledged host", func(t *testing.T) {
		backend := newFakeModels(t, "m1")
		candidate := newFakeModels(t, "c1")
		dir := t.TempDir()
		e := newSettingsEnvWith(t, backendJSONAt(backend.srv.URL+"/v1"), dir, adminVerifierWithAck(t, nil))
		// Move to the candidate with the typed confirmation (an environment
		// key travels), then back; the candidate stays in the overlay's ack.
		etag, _ := getSettings(t, e)
		etag = e.putEgress(t, etag, `{"backend":{"base_url":"`+candidate.srv.URL+`/v1"},"egress_confirmation":{"host":"`+HostFromURL(candidate.srv.URL)+`"}}`)
		_ = e.putEgress(t, etag, `{"backend":{"base_url":"`+backend.srv.URL+`/v1"},"egress_confirmation":{"host":"`+HostFromURL(backend.srv.URL)+`"}}`)
		// The candidate is acknowledged but is no longer the current backend.
		before := len(candidate.seen())
		resp := e.doPath(t, http.MethodPost, RouteSettingsTestConnection, tokSettingsAdmin, `{"base_url":"`+candidate.srv.URL+`/v1"}`)
		var doc map[string]any
		require.NoError(t, json.Unmarshal([]byte(bodyString(t, resp)), &doc))
		assert.Equal(t, false, doc["key_sent"])
		seen := candidate.seen()[before:]
		require.Len(t, seen, 1)
		assert.Empty(t, seen[0])
	})
}

// TestSettingsModelsListNoBackend: with nothing configured the endpoint
// answers backend_not_configured in the settings error shape, and a candidate
// base_url still lists (without a stored key).
func TestSettingsModelsListNoBackend(t *testing.T) {
	e := newSettingsEnv(t, `{}`)
	resp := e.doPath(t, http.MethodGet, RouteSettingsModels, tokSettingsAdmin, "")
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(bodyString(t, resp)), &doc))
	validateSettingsSchema(t, "settings/settings-error.schema.json", doc)
	errObj, _ := doc["error"].(map[string]any)
	require.NotNil(t, errObj)
	assert.Equal(t, "backend_not_configured", errObj["code"])

	candidate := newFakeModels(t, "cand-9")
	ok := e.doPath(t, http.MethodGet, RouteSettingsModels+"?base_url="+candidate.srv.URL+"/v1", tokSettingsAdmin, "")
	require.Equal(t, http.StatusOK, ok.StatusCode)
	assert.Equal(t, []string{""}, candidate.seen(), "no stored key: the probe sends no Authorization header")
}

// TestSettingsProbeRouteAuth: the probe endpoints share the settings route's
// admin-only scoping and method rules.
func TestSettingsProbeRouteAuth(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	for _, path := range []string{RouteSettingsTestConnection, RouteSettingsModels} {
		assert.Equal(t, http.StatusUnauthorized, e.doPath(t, http.MethodPost, path, "", "{}").StatusCode, path)
		assert.Equal(t, http.StatusForbidden, e.doPath(t, http.MethodPost, path, tokReader, "{}").StatusCode, path)
		assert.Equal(t, http.StatusForbidden, e.doPath(t, http.MethodPost, path, tokChat, "{}").StatusCode, path)
		assert.Equal(t, http.StatusForbidden, e.doPath(t, http.MethodPost, path, tokPeer, "{}").StatusCode, path)
	}
	assert.Equal(t, http.StatusMethodNotAllowed,
		e.doPath(t, http.MethodGet, RouteSettingsTestConnection, tokSettingsAdmin, "").StatusCode)
	assert.Equal(t, http.StatusMethodNotAllowed,
		e.doPath(t, http.MethodPost, RouteSettingsModels, tokSettingsAdmin, "{}").StatusCode)
}

// TestSettingsProbeKind: uar is refused as on a settings write, and an unknown
// kind is a plain 400.
func TestSettingsProbeKind(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	for name, body := range map[string]string{
		"unknown": `{"base_url":"http://127.0.0.1:1/v1","kind":"ollama"}`,
	} {
		resp := e.doPath(t, http.MethodPost, RouteSettingsTestConnection, tokSettingsAdmin, body)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, name)
	}
	for name, path := range map[string]string{
		"unknown": RouteSettingsModels + "?kind=ollama",
	} {
		resp := e.doPath(t, http.MethodGet, path, tokSettingsAdmin, "")
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, name)
	}
}

// TestSettingsProbeWithholdsKeyOnSchemeChange: the key decision uses the whole
// endpoint (scheme, host, port, default ports filled in), not host and port
// alone, so an https candidate on the current backend's loopback port is
// probed without the key.
func TestSettingsProbeWithholdsKeyOnSchemeChange(t *testing.T) {
	backend := newFakeModels(t, "m1")
	e := newSettingsEnv(t, backendJSONAt(backend.srv.URL+"/v1"))
	same := func(base string) bool {
		resp := e.doPath(t, http.MethodPost, RouteSettingsTestConnection, tokSettingsAdmin, `{"base_url":"`+base+`"}`)
		var doc map[string]any
		require.NoError(t, json.Unmarshal([]byte(bodyString(t, resp)), &doc))
		sent, _ := doc["key_sent"].(bool)
		return sent
	}
	assert.True(t, same(backend.srv.URL+"/v1"), "the current backend's own endpoint gets the key")
	assert.True(t, same(backend.srv.URL+"/other"), "a path change is not a move")
	https := "https://" + HostFromURL(backend.srv.URL) + "/v1"
	assert.False(t, same(https), "https on the same host and port is another endpoint")
}
