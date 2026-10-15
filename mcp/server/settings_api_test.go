//go:build !noagent

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/ipfs/kubo/mcp/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settingsTestKey is the key material the redaction tests hunt for. It must
// appear in no response body and no log line.
const settingsTestKey = "sk-test-0123456789abcdefSECRET"

const settingsKeyEnv = "IPFS_MCP_TEST_SETTINGS_KEY"

// settingsEnv is an HTTP listener with the settings route mounted for the
// admin test principal, over a config of the test's choosing.
type settingsEnv struct {
	s    *Server
	base string
	logs *bytes.Buffer
}

func newSettingsEnv(t *testing.T, cfgJSON string) *settingsEnv {
	t.Helper()
	return newSettingsEnvDir(t, cfgJSON, "")
}

// newSettingsEnvDir is newSettingsEnv with a settings directory: an overlay in
// it is loaded and shadows the config file, exactly as startup does
// (flags > overlay > config; the env uses no flags).
func newSettingsEnvDir(t *testing.T, cfgJSON, dir string) *settingsEnv {
	t.Helper()
	return newSettingsEnvWith(t, cfgJSON, dir, adminVerifier(t))
}

// newSettingsEnvWith is newSettingsEnvDir with the caller's verifier, for
// tests that need a configured egress_ack on the admin principal (D4).
func newSettingsEnvWith(t *testing.T, cfgJSON, dir string, verifier *StaticVerifier) *settingsEnv {
	t.Helper()
	t.Setenv(settingsKeyEnv, settingsTestKey)
	cfg := mustParse(t, cfgJSON)
	var ov *SettingsOverlay
	if dir != "" {
		var err error
		ov, err = LoadSettingsOverlay(dir)
		require.NoError(t, err)
	}
	resolved, err := cfg.ResolveAgentWithOverlay(LLMOverrides{}, ov)
	require.NoError(t, err)

	d := newFakeDaemon(t)
	installPins(d, map[string]fakePin{testCID(t, 1): {Type: "recursive"}})
	logs := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s := mustNew(t, d, Options{
		Tools: []string{"node"}, Resolvers: []Resolver{newFakeResolver(ResolverLinuxPolkit)}, Transport: "http", Logger: log,
	})
	require.NoError(t, s.EnableAgent(resolved))
	require.NoError(t, s.EnableAgentSettings(AgentSettingsAPIConfig{Cfg: cfg, Dir: dir, Overlay: ov, Resolved: resolved}))

	hs, err := NewHTTPServer(s, HTTPOptions{Listen: "127.0.0.1:0", Verifier: verifier, Logger: log})
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
	return &settingsEnv{s: s, base: "http://" + addr.String(), logs: logs}
}

// settingsBackendJSON is a configured OpenAI-compatible backend on loopback,
// with the key named by environment (never a literal).
const settingsBackendJSON = `{"llm":{"base_url":"http://127.0.0.1:59998/v1","model":"m-old","api_key":{"env":"` + settingsKeyEnv + `"}}}`

func (e *settingsEnv) do(t *testing.T, method, token, body string, header map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, e.base+RouteSettings, strings.NewReader(body))
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func bodyString(t *testing.T, r *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	return string(b)
}

func getSettings(t *testing.T, e *settingsEnv) (string, map[string]any) {
	t.Helper()
	resp := e.do(t, http.MethodGet, tokSettingsAdmin, "", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	etag := resp.Header.Get("ETag")
	require.True(t, strings.HasPrefix(etag, `"`) && strings.HasSuffix(etag, `"`), "ETag is a quoted revision, got %q", etag)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(bodyString(t, resp)), &doc))
	return etag, doc
}

// validateSettingsSchema checks a document against one of the contract's
// settings schemas, so the wire shape is pinned to the contract, not to the
// test's own struct.
func validateSettingsSchema(t *testing.T, schemaPath string, doc map[string]any) {
	t.Helper()
	raw, err := fs.ReadFile(contract.FS(), schemaPath)
	require.NoError(t, err)
	var s jsonschema.Schema
	require.NoError(t, json.Unmarshal(raw, &s))
	rs, err := s.Resolve(nil)
	require.NoError(t, err)
	require.NoError(t, rs.Validate(doc))
}

// TestSettingsReadShapeAndRedaction: the read matches the contract schema,
// reports the key only as {set, source}, and carries no key material.
func TestSettingsReadShapeAndRedaction(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	etag, doc := getSettings(t, e)
	validateSettingsSchema(t, "settings/settings-read.schema.json", doc)

	assert.NotEmpty(t, doc["revision"])
	backend, _ := doc["backend"].(map[string]any)
	require.NotNil(t, backend)
	assert.Equal(t, "openai", backend["kind"])
	assert.Equal(t, "http://127.0.0.1:59998/v1", backend["base_url"])
	assert.Equal(t, "m-old", backend["model"])
	assert.Equal(t, map[string]any{"set": true, "source": "environment"}, backend["api_key"])
	prompt, _ := doc["system_prompt"].(map[string]any)
	require.NotNil(t, prompt)
	assert.Equal(t, systemPromptSafetyPrefix, prompt["safety_prefix"])
	assert.Equal(t, "", prompt["custom"])

	raw, _ := json.Marshal(doc)
	assert.NotContains(t, string(raw), settingsTestKey, "no key material in the read")
	assert.NotContains(t, etag, settingsTestKey)
}

// TestSettingsRouteAuth: the real route is scoped to the admin kind (the 1.1
// cross-path rule, now against the mounted route instead of a stand-in).
func TestSettingsRouteAuth(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	assert.Equal(t, http.StatusUnauthorized, e.do(t, http.MethodGet, "", "", nil).StatusCode)
	assert.Equal(t, http.StatusForbidden, e.do(t, http.MethodGet, tokReader, "", nil).StatusCode)
	assert.Equal(t, http.StatusForbidden, e.do(t, http.MethodGet, tokChat, "", nil).StatusCode)
	assert.Equal(t, http.StatusForbidden, e.do(t, http.MethodGet, tokPeer, "", nil).StatusCode)
	assert.Equal(t, http.StatusOK, e.do(t, http.MethodGet, tokSettingsAdmin, "", nil).StatusCode)
	assert.Equal(t, http.StatusMethodNotAllowed, e.do(t, http.MethodPost, tokSettingsAdmin, "{}", nil).StatusCode)
}

// TestSettingsWriteStaleRevisionIs412 is the acceptance check: a write with a
// stale or missing revision is refused with revision_conflict, and the running
// backend is unchanged.
func TestSettingsWriteStaleRevisionIs412(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	etag, before := getSettings(t, e)
	write := `{"backend":{"model":"m-new"}}`

	for name, header := range map[string]map[string]string{
		"stale revision":   {"If-Match": `"00000000000000000000000000000000"`},
		"missing If-Match": nil,
		"wildcard":         {"If-Match": "*"},
	} {
		resp := e.do(t, http.MethodPut, tokSettingsAdmin, write, header)
		assert.Equal(t, http.StatusPreconditionFailed, resp.StatusCode, name)
		var doc map[string]any
		require.NoError(t, json.Unmarshal([]byte(bodyString(t, resp)), &doc))
		validateSettingsSchema(t, "settings/settings-error.schema.json", doc)
		errObj, _ := doc["error"].(map[string]any)
		require.NotNil(t, errObj, name)
		assert.Equal(t, "revision_conflict", errObj["code"], name)
		details, _ := errObj["details"].(map[string]any)
		require.NotNil(t, details, name)
		assert.Equal(t, strings.Trim(etag, `"`), details["current_revision"], name)
	}

	// Nothing applied: the revision and the running backend are unchanged.
	etagAfter, after := getSettings(t, e)
	assert.Equal(t, etag, etagAfter)
	assert.Equal(t, before, after)
	require.NotNil(t, e.s.Agent())
	assert.Equal(t, "127.0.0.1:59998", e.s.Agent().BackendHost())
	assert.Equal(t, "m-old", e.s.Agent().snap.Load().llm.model)
}

// TestSettingsWriteAppliesWithoutRestart: a valid write with the current
// revision applies atomically through ApplyAgentSettings — the loop is swapped
// in place (same *Agent, sessions survive) and the answer carries the new
// revision as body and ETag. The write moves the backend host, so it carries
// the D4 typed confirmation of the new host.
func TestSettingsWriteAppliesWithoutRestart(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	etag, _ := getSettings(t, e)
	agentBefore := e.s.Agent()
	require.NotNil(t, agentBefore)

	write := `{"backend":{"base_url":"http://127.0.0.1:59997/v1","model":"m-new"},"system_prompt":"Be terse.",` +
		`"egress_confirmation":{"host":"127.0.0.1:59997"}}`
	resp := e.do(t, http.MethodPut, tokSettingsAdmin, write, map[string]string{"If-Match": etag})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	newETag := resp.Header.Get("ETag")
	assert.NotEqual(t, etag, newETag)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(bodyString(t, resp)), &doc))
	validateSettingsSchema(t, "settings/settings-read.schema.json", doc)
	assert.Equal(t, strings.Trim(newETag, `"`), doc["revision"])
	backend, _ := doc["backend"].(map[string]any)
	require.NotNil(t, backend)
	assert.Equal(t, "http://127.0.0.1:59997/v1", backend["base_url"])
	assert.Equal(t, "m-new", backend["model"])
	assert.Equal(t, map[string]any{"set": true, "source": "environment"}, backend["api_key"],
		"the key reference survives a write that does not touch it")
	prompt, _ := doc["system_prompt"].(map[string]any)
	require.NotNil(t, prompt)
	assert.Equal(t, "Be terse.", prompt["custom"])
	assert.Equal(t, systemPromptSafetyPrefix, prompt["safety_prefix"])

	// The apply was hot: the same loop object with the new backend snapshot.
	assert.Same(t, agentBefore, e.s.Agent(), "a settings apply swaps the snapshot, not the loop")
	assert.Equal(t, "127.0.0.1:59997", e.s.Agent().BackendHost())

	// A write presenting the previous revision is now stale.
	stale := e.do(t, http.MethodPut, tokSettingsAdmin, `{"backend":{"model":"m-third"}}`, map[string]string{"If-Match": etag})
	assert.Equal(t, http.StatusPreconditionFailed, stale.StatusCode)
}

// TestSettingsWriteInstallsBackendWithoutRestart: with no backend configured,
// a write that completes one installs the loop; a prompt-only write stores the
// value and dials nothing.
func TestSettingsWriteInstallsBackendWithoutRestart(t *testing.T) {
	e := newSettingsEnv(t, `{}`)
	etag, doc := getSettings(t, e)
	assert.Nil(t, doc["backend"])
	validateSettingsSchema(t, "settings/settings-read.schema.json", doc)
	assert.Nil(t, e.s.Agent())

	// A prompt-only write: valid, stored, still no backend.
	resp := e.do(t, http.MethodPut, tokSettingsAdmin, `{"system_prompt":"Only facts."}`, map[string]string{"If-Match": etag})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	etag2 := resp.Header.Get("ETag")
	assert.NotEqual(t, etag, etag2)
	assert.Nil(t, e.s.Agent(), "a prompt alone does not install the loop")

	// Completing the backend installs the loop without a restart. The first
	// backend host is newly chosen, so the write carries the D4 confirmation.
	resp = e.do(t, http.MethodPut, tokSettingsAdmin,
		`{"backend":{"base_url":"http://127.0.0.1:59996/v1","model":"m1"},"egress_confirmation":{"host":"127.0.0.1:59996"}}`,
		map[string]string{"If-Match": etag2})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, e.s.Agent(), "ApplyAgentSettings installs the loop around the new backend")
	assert.Equal(t, "127.0.0.1:59996", e.s.Agent().BackendHost())

	// Turning the agent off is a restart, not a write.
	etag3 := resp.Header.Get("ETag")
	off := e.do(t, http.MethodPut, tokSettingsAdmin, `{"backend":{"base_url":"","model":""}}`, map[string]string{"If-Match": etag3})
	assert.Equal(t, http.StatusBadRequest, off.StatusCode)
	assert.NotNil(t, e.s.Agent())
}

// TestSettingsWriteValidationSharedWithStartup: the write path refuses what
// ResolveAgent refuses, with the same messages and no echoed values.
func TestSettingsWriteValidationSharedWithStartup(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	etag, before := getSettings(t, e)

	cases := map[string]string{
		"credentials in URL":  `{"backend":{"base_url":"http://user:pw@127.0.0.1:1/v1"}}`,
		"non-http scheme":     `{"backend":{"base_url":"ftp://127.0.0.1/v1"}}`,
		"query in URL":        `{"backend":{"base_url":"http://127.0.0.1:1/v1?key=abc"}}`,
		"empty model":         `{"backend":{"model":""}}`,
		"unknown kind":        `{"backend":{"kind":"ollama"}}`,
		"unknown write field": `{"backendz":{}}`,
		"not JSON":            `{`,
	}
	for name, write := range cases {
		resp := e.do(t, http.MethodPut, tokSettingsAdmin, write, map[string]string{"If-Match": etag})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, name)
		body := bodyString(t, resp)
		assert.NotContains(t, body, "pw", name)
		assert.NotContains(t, body, settingsTestKey, name)
	}
	_, after := getSettings(t, e)
	assert.Equal(t, before, after, "no refused write changed the settings")
}

// TestSettingsWriteNeverLogsValues is the acceptance check for slog redaction:
// reads, a successful write and a refused write leave no value — key material,
// URLs, model names, prompts — in the server's log.
func TestSettingsWriteNeverLogsValues(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	etag, _ := getSettings(t, e)

	resp := e.do(t, http.MethodPut, tokSettingsAdmin,
		`{"backend":{"base_url":"http://127.0.0.1:59997/v1","model":"m-secret-new"},"system_prompt":"a private prompt",`+
			`"egress_confirmation":{"host":"127.0.0.1:59997"}}`,
		map[string]string{"If-Match": etag})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = bodyString(t, resp)
	// A stale write and a validation failure log their outcomes too.
	_ = bodyString(t, e.do(t, http.MethodPut, tokSettingsAdmin, `{"backend":{"model":"x"}}`, nil))
	_ = bodyString(t, e.do(t, http.MethodPut, tokSettingsAdmin, `{"backend":{"base_url":"http://user:pw2@127.0.0.1:1/v1"}}`, map[string]string{"If-Match": resp.Header.Get("ETag")}))

	logs := e.logs.String()
	// The audit line names the hosts (L2) but no key, model name, prompt or
	// credential.
	for _, leaked := range []string{settingsTestKey, "m-secret-new", "a private prompt", "pw2"} {
		assert.NotContains(t, logs, leaked, "log must not contain %q", leaked)
	}
	assert.Contains(t, logs, "agent settings applied", "the apply is logged by revision and field names")
	assert.Contains(t, logs, "new_host=127.0.0.1:59997", "the audit line names the new host")
}

// TestSettingsWriteWithSecretRefusedWhenAcceptSecretsFalse: the write-only key
// field never reaches the apply path — refused with secrets_not_accepted when
// accept_secrets is false (with or without a settings directory), and refused
// without a settings directory otherwise — and the refusal never echoes the
// key or writes a file.
func TestSettingsWriteWithSecretRefusedWhenAcceptSecretsFalse(t *testing.T) {
	write := `{"backend":{"api_key":"sk-new-0123456789abcdefSECRET"}}`

	e := newSettingsEnv(t, `{"agent":{"settings":{"accept_secrets":false}},`+settingsBackendJSON[1:])
	etag, before := getSettings(t, e)
	resp := e.do(t, http.MethodPut, tokSettingsAdmin, write, map[string]string{"If-Match": etag})
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(bodyString(t, resp)), &doc))
	validateSettingsSchema(t, "settings/settings-error.schema.json", doc)
	errObj, _ := doc["error"].(map[string]any)
	require.NotNil(t, errObj)
	assert.Equal(t, "secrets_not_accepted", errObj["code"])
	raw, _ := json.Marshal(doc)
	assert.NotContains(t, string(raw), "sk-new-0123456789abcdefSECRET")
	_, after := getSettings(t, e)
	assert.Equal(t, before, after)

	// accept_secrets false dominates even with a settings directory to store
	// the key in: refused, and no file is written.
	dir := t.TempDir()
	eDir := newSettingsEnvDir(t, `{"agent":{"settings":{"accept_secrets":false}},`+settingsBackendJSON[1:], dir)
	etagDir, _ := getSettings(t, eDir)
	respDir := eDir.do(t, http.MethodPut, tokSettingsAdmin, write, map[string]string{"If-Match": etagDir})
	assert.Equal(t, http.StatusForbidden, respDir.StatusCode)
	assert.NotContains(t, bodyString(t, respDir), "sk-new-0123456789abcdefSECRET")
	assert.Empty(t, readdirnames(t, dir), "a refused write leaves no file in the settings directory")

	// Default accept_secrets, but no settings directory to hold the key file.
	e2 := newSettingsEnv(t, settingsBackendJSON)
	etag2, _ := getSettings(t, e2)
	resp2 := e2.do(t, http.MethodPut, tokSettingsAdmin, write, map[string]string{"If-Match": etag2})
	assert.Equal(t, http.StatusBadRequest, resp2.StatusCode)
	assert.NotContains(t, bodyString(t, resp2), "sk-new-0123456789abcdefSECRET")
}

// TestSettingsRouteAbsentWhenNotEnabled pins the server half of the off
// switch (mcp-agent-settings-api 1.7): until EnableAgentSettings runs — the
// CLI calls it only with an admin principal configured and
// agent.settings.enabled not false — the settings subtree is never mounted
// and answers a plain 404, even to a valid admin token.
func TestSettingsRouteAbsentWhenNotEnabled(t *testing.T) {
	e := newHTTPEnv(t, func(o *HTTPOptions) { o.Verifier = adminVerifier(t) })
	for _, path := range []string{RouteSettings, RouteSettingsTestConnection, RouteSettingsModels, RouteSettingsEvents} {
		resp := e.do(t, rawReq{method: http.MethodGet, path: path, token: tokSettingsAdmin})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, path)
	}
	resp := e.do(t, rawReq{method: http.MethodPost, path: RouteMCP, token: tokReader, body: initializeBody})
	assert.Equal(t, http.StatusOK, resp.StatusCode, "the listener itself works")
}
