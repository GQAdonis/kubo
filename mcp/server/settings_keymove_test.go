//go:build !noagent

package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	keyMoveBackendJSON = `{"llm":{"base_url":"http://127.0.0.1:59998/v1","model":"m-old"}}`
	keyMoveNewKey      = "sk-new-0123456789abcdefSECRET"
)

// putChecked applies one write expected to succeed and returns the decoded,
// contract-validated response and the new ETag.
func (e *settingsEnv) putChecked(t *testing.T, etag, body string) (map[string]any, string) {
	t.Helper()
	resp := e.do(t, http.MethodPut, tokSettingsAdmin, body, map[string]string{"If-Match": etag})
	raw := bodyString(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, raw)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &doc))
	validateSettingsSchema(t, "settings/settings-read.schema.json", doc)
	assert.NotContains(t, raw, settingsTestKey)
	assert.NotContains(t, raw, keyMoveNewKey)
	return doc, resp.Header.Get("ETag")
}

// keyMoveEnv is an env with a settings directory, "local" acknowledged (so a
// loopback move needs no typed confirmation) and an API-stored key.
func keyMoveEnv(t *testing.T, cfg string) (*settingsEnv, string, string) {
	t.Helper()
	dir := t.TempDir()
	e := newSettingsEnvWith(t, cfg, dir, adminVerifierWithAck(t, []string{"local"}))
	etag, _ := getSettings(t, e)
	return e, dir, etag
}

func apiKeyOf(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	backend, _ := doc["backend"].(map[string]any)
	require.NotNil(t, backend)
	k, _ := backend["api_key"].(map[string]any)
	return k
}

func storeAPIKey(t *testing.T, e *settingsEnv, etag string) string {
	t.Helper()
	doc, etag := e.putChecked(t, etag, `{"backend":{"api_key":"`+keyMoveNewKey+`"}}`)
	assert.Equal(t, map[string]any{"set": true, "source": "settings_api"}, apiKeyOf(t, doc))
	return etag
}

func TestSettingsMoveClearsAPIStoredKey(t *testing.T) {
	e, dir, etag := keyMoveEnv(t, keyMoveBackendJSON)
	etag = storeAPIKey(t, e, etag)
	keyFile := filepath.Join(dir, settingsKeyFileName)
	require.FileExists(t, keyFile)

	doc, etag := e.putChecked(t, etag, `{"backend":{"base_url":"http://127.0.0.1:59997/v1"}}`)
	assert.Equal(t, map[string]any{"set": false}, apiKeyOf(t, doc))
	assert.Equal(t, map[string]any{"reason": "host_changed", "cleared": true}, doc["key_not_carried"])
	_, err := os.Stat(keyFile)
	assert.ErrorIs(t, err, os.ErrNotExist, "the stored key file is removed")

	// GET never carries the notice, and a move back does not restore the key.
	_, got := getSettings(t, e)
	assert.NotContains(t, got, "key_not_carried")
	assert.Equal(t, map[string]any{"set": false}, apiKeyOf(t, got))
	doc, _ = e.putChecked(t, etag, `{"backend":{"base_url":"http://127.0.0.1:59998/v1"}}`)
	assert.Equal(t, map[string]any{"set": false}, apiKeyOf(t, doc))
}

func TestSettingsMoveWithAPIKeyKeepsNewKey(t *testing.T) {
	e, dir, etag := keyMoveEnv(t, keyMoveBackendJSON)
	etag = storeAPIKey(t, e, etag)

	doc, etag := e.putChecked(t, etag,
		`{"backend":{"base_url":"http://127.0.0.1:59997/v1","api_key":"sk-other-0123456789abcdefSECRET"}}`)
	assert.Equal(t, map[string]any{"set": true, "source": "settings_api"}, apiKeyOf(t, doc))
	assert.NotContains(t, doc, "key_not_carried")
	raw, err := os.ReadFile(filepath.Join(dir, settingsKeyFileName))
	require.NoError(t, err)
	assert.Equal(t, "sk-other-0123456789abcdefSECRET", string(raw))

	// api_key null on a move is an explicit clear, not a withheld key.
	doc, _ = e.putChecked(t, etag, `{"backend":{"base_url":"http://127.0.0.1:59998/v1","api_key":null}}`)
	assert.Equal(t, map[string]any{"set": false}, apiKeyOf(t, doc))
	assert.NotContains(t, doc, "key_not_carried")
}

// TestSettingsMoveKeepsReferencedKey pins the operator ruling of 2026-10-15
// ("let it follow, document it"): a model key supplied by reference (an
// environment variable, a file or a --llm-api-key-* flag) follows a backend
// host move; only a key stored through this API is withheld. After the move the
// key is still set, no key_not_carried notice is answered, and the probe sends
// that same key to the new backend.
func TestSettingsMoveKeepsReferencedKey(t *testing.T) {
	dir := t.TempDir()
	e := newSettingsEnvWith(t, settingsBackendJSON, dir, adminVerifierWithAck(t, []string{"local"}))
	etag, _ := getSettings(t, e)
	next := newFakeModels(t, "m-new")
	doc, _ := e.putChecked(t, etag, `{"backend":{"base_url":"`+next.srv.URL+`/v1"}}`)
	assert.Equal(t, map[string]any{"set": true, "source": "environment"}, apiKeyOf(t, doc))
	assert.NotContains(t, doc, "key_not_carried")

	resp := e.doPath(t, http.MethodPost, RouteSettingsTestConnection, tokSettingsAdmin, `{"base_url":"`+next.srv.URL+`/v1"}`)
	var probe map[string]any
	require.NoError(t, json.Unmarshal([]byte(bodyString(t, resp)), &probe))
	assert.Equal(t, true, probe["key_sent"], "the referenced key followed the move to the new backend")
	require.Len(t, next.seen(), 1)
	assert.Equal(t, "Bearer "+settingsTestKey, next.seen()[0])
}

// TestSettingsMoveWithAcceptSecretsOffKeepsReferencedKey is the same ruling on a
// node that does not accept API-stored keys (accept_secrets false), where a
// mounted key reference is the only kind of key there is.
func TestSettingsMoveWithAcceptSecretsOffKeepsReferencedKey(t *testing.T) {
	cfg := `{"agent":{"settings":{"accept_secrets":false}},"llm":{"base_url":"http://127.0.0.1:59998/v1","model":"m-old","api_key":{"env":"` + settingsKeyEnv + `"}}}`
	e, _, etag := keyMoveEnv(t, cfg)
	doc, _ := e.putChecked(t, etag, `{"backend":{"base_url":"http://127.0.0.1:59997/v1"}}`)
	assert.Equal(t, map[string]any{"set": true, "source": "environment"}, apiKeyOf(t, doc))
	assert.NotContains(t, doc, "key_not_carried")
}

// TestSettingsMoveConfirmationNamesReferencedKey: the typed host confirmation
// text says the configured key will be sent to the new host when the key is a
// reference, and does not when the key is API-stored (a move clears that one).
func TestSettingsMoveConfirmationNamesReferencedKey(t *testing.T) {
	const move = `{"backend":{"base_url":"http://127.0.0.1:59997/v1"}}`
	confirmationMessage := func(t *testing.T, e *settingsEnv, etag string) string {
		t.Helper()
		resp := e.do(t, http.MethodPut, tokSettingsAdmin, move, map[string]string{"If-Match": etag})
		raw := bodyString(t, resp)
		require.Equal(t, http.StatusConflict, resp.StatusCode, raw)
		var doc settingsErrorBody
		require.NoError(t, json.Unmarshal([]byte(raw), &doc))
		require.Equal(t, "egress_confirmation_required", doc.Error.Code)
		return doc.Error.Message
	}

	t.Run("referenced key", func(t *testing.T) {
		e := newSettingsEnvWith(t, settingsBackendJSON, "", adminVerifierWithAck(t, nil))
		etag, _ := getSettings(t, e)
		msg := confirmationMessage(t, e, etag)
		assert.Contains(t, msg, "configured model key")
		assert.Contains(t, msg, "will be sent to that host too")
		assert.NotContains(t, msg, settingsTestKey)
	})

	t.Run("api-stored key", func(t *testing.T) {
		dir := t.TempDir()
		e := newSettingsEnvWith(t, keyMoveBackendJSON, dir, adminVerifierWithAck(t, nil))
		etag, _ := getSettings(t, e)
		etag = storeAPIKey(t, e, etag)
		assert.NotContains(t, confirmationMessage(t, e, etag), "configured model key")
	})

	t.Run("no key", func(t *testing.T) {
		e := newSettingsEnvWith(t, keyMoveBackendJSON, "", adminVerifierWithAck(t, nil))
		etag, _ := getSettings(t, e)
		assert.NotContains(t, confirmationMessage(t, e, etag), "configured model key")
	})
}

// breakApply makes the next ApplyAgentSettings fail: with no agent loop
// installed it builds one, and a tool whose input schema cannot be parsed makes
// that build error.
func breakApply(e *settingsEnv) {
	e.s.agentMu.Lock()
	defer e.s.agentMu.Unlock()
	e.s.agentExt = nil
	for _, rt := range e.s.registry {
		rt.sc.rawIn = json.RawMessage("{")
	}
}

// TestSettingsFailedApplyKeepsStoredKey: a move that clears the API-stored key
// removes the key file only after the new settings applied. A failed apply
// leaves the key file, the overlay file, the old backend and its key as they
// were, in memory and on disk.
func TestSettingsFailedApplyKeepsStoredKey(t *testing.T) {
	t.Run("a move that clears the key", func(t *testing.T) {
		e, dir, etag := keyMoveEnv(t, keyMoveBackendJSON)
		etag = storeAPIKey(t, e, etag)
		keyFile := filepath.Join(dir, settingsKeyFileName)
		overlayBefore, err := os.ReadFile(filepath.Join(dir, settingsOverlayFileName))
		require.NoError(t, err)
		before, err := LoadSettingsOverlay(dir)
		require.NoError(t, err)

		breakApply(e)
		resp := e.do(t, http.MethodPut, tokSettingsAdmin, `{"backend":{"base_url":"http://127.0.0.1:59997/v1"}}`,
			map[string]string{"If-Match": etag})
		raw := bodyString(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.StatusCode, raw)
		assert.Contains(t, raw, "did not apply")
		assert.FileExists(t, keyFile, "the key file stays until the new settings applied")
		keyData, err := os.ReadFile(keyFile)
		require.NoError(t, err)
		assert.Equal(t, keyMoveNewKey, string(keyData))

		after, err := LoadSettingsOverlay(dir)
		require.NoError(t, err)
		assert.Equal(t, before, after, "the overlay on disk still names the old backend")
		assert.Equal(t, before.Backend.BaseURL, after.Backend.BaseURL)
		overlayAfter, err := os.ReadFile(filepath.Join(dir, settingsOverlayFileName))
		require.NoError(t, err)
		assert.Equal(t, string(overlayBefore), string(overlayAfter))
		assert.NotContains(t, string(overlayAfter), `"api_key":null`, "no recorded key clear")
		assert.NotContains(t, string(overlayAfter), "59997", "the new base_url is not persisted")

		_, got := getSettings(t, e)
		assert.Equal(t, map[string]any{"set": true, "source": "settings_api"}, apiKeyOf(t, got), "the old backend is still keyed")
	})

	t.Run("a write that supplies a new key", func(t *testing.T) {
		e, dir, etag := keyMoveEnv(t, keyMoveBackendJSON)
		etag = storeAPIKey(t, e, etag)
		keyFile := filepath.Join(dir, settingsKeyFileName)
		overlayBefore, err := os.ReadFile(filepath.Join(dir, settingsOverlayFileName))
		require.NoError(t, err)

		breakApply(e)
		resp := e.do(t, http.MethodPut, tokSettingsAdmin, `{"backend":{"api_key":"replacement-key-value"}}`,
			map[string]string{"If-Match": etag})
		raw := bodyString(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.StatusCode, raw)
		assert.Contains(t, raw, "did not apply")
		assert.NotContains(t, raw, "replacement-key-value")

		keyData, err := os.ReadFile(keyFile)
		require.NoError(t, err)
		assert.Equal(t, keyMoveNewKey, string(keyData), "the old key file content is restored")
		overlayAfter, err := os.ReadFile(filepath.Join(dir, settingsOverlayFileName))
		require.NoError(t, err)
		assert.Equal(t, string(overlayBefore), string(overlayAfter))
	})

	t.Run("a first key write leaves no key or overlay file", func(t *testing.T) {
		e, dir, etag := keyMoveEnv(t, keyMoveBackendJSON)
		breakApply(e)
		resp := e.do(t, http.MethodPut, tokSettingsAdmin, `{"backend":{"api_key":"replacement-key-value"}}`,
			map[string]string{"If-Match": etag})
		require.Equal(t, http.StatusInternalServerError, resp.StatusCode, bodyString(t, resp))
		assert.NoFileExists(t, filepath.Join(dir, settingsKeyFileName))
		assert.NoFileExists(t, filepath.Join(dir, settingsOverlayFileName))
	})
}

// TestProbeKeyReadsKeyAndBackendTogether: probeKey takes the key and the current
// backend in one critical section, so a move that clears the key and installs
// the new backend is seen whole, never as the old key with the new host.
func TestProbeKeyReadsKeyAndBackendTogether(t *testing.T) {
	e := newSettingsEnv(t, backendJSONAt("http://127.0.0.1:59998/v1"))
	e.s.agentMu.RLock()
	st, _ := e.s.settingsExt.(*settingsState)
	e.s.agentMu.RUnlock()
	require.NotNil(t, st)
	h := &settingsHandler{srv: e.s, st: st, log: e.s.log}

	st.mu.Lock()
	type result struct {
		key  secret
		sent bool
	}
	done := make(chan result)
	go func() {
		// The new backend's own host: with the key and backend read together
		// this is either (old key, old backend: not sent) or (no key); it is
		// never the old key sent to the new host.
		k, sent, _ := h.probeKey("http://127.0.0.1:59997/v1")
		done <- result{k, sent}
	}()
	select {
	case <-done:
		st.mu.Unlock()
		t.Fatal("probeKey read state without taking the settings lock")
	case <-time.After(50 * time.Millisecond):
	}
	// The move: the new backend and the cleared key land together.
	moved := *st.current
	moved.BaseURL = "http://127.0.0.1:59997/v1"
	st.current = &moved
	st.base.key = nil
	st.mu.Unlock()

	r := <-done
	assert.Empty(t, string(r.key), "the cleared key is not sent to the new host")
	assert.False(t, r.sent)
}

func TestSettingsMoveWithoutAPIStoredKeyHasNoNotice(t *testing.T) {
	// accept_secrets false: no API-stored key can exist.
	cfg := `{"agent":{"settings":{"accept_secrets":false}},"llm":{"base_url":"http://127.0.0.1:59998/v1","model":"m-old"}}`
	e, _, etag := keyMoveEnv(t, cfg)
	doc, _ := e.putChecked(t, etag, `{"backend":{"base_url":"http://127.0.0.1:59997/v1"}}`)
	assert.Equal(t, map[string]any{"set": false}, apiKeyOf(t, doc))
	assert.NotContains(t, doc, "key_not_carried")

	// A move with no key stored at all behaves the same.
	e2, _, etag2 := keyMoveEnv(t, keyMoveBackendJSON)
	doc, _ = e2.putChecked(t, etag2, `{"backend":{"base_url":"http://127.0.0.1:59997/v1"}}`)
	assert.NotContains(t, doc, "key_not_carried")
}

func TestSettingsNonMovingWriteKeepsAPIStoredKey(t *testing.T) {
	e, dir, etag := keyMoveEnv(t, keyMoveBackendJSON)
	etag = storeAPIKey(t, e, etag)
	// Model, path and default-port spellings are not a move.
	for _, body := range []string{
		`{"backend":{"model":"m-new"}}`,
		`{"backend":{"base_url":"http://127.0.0.1:59998/other"}}`,
	} {
		var doc map[string]any
		doc, etag = e.putChecked(t, etag, body)
		assert.Equal(t, map[string]any{"set": true, "source": "settings_api"}, apiKeyOf(t, doc), body)
		assert.NotContains(t, doc, "key_not_carried", body)
	}
	assert.FileExists(t, filepath.Join(dir, settingsKeyFileName))
}

func TestBackendMoved(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		moved bool
	}{
		{"http://h:1/v1", "http://h:1/other", false},
		{"https://h/v1", "https://h:443/v1", false},
		{"http://H:1/v1", "http://h:1/v1", false},
		{"http://h:1/v1", "http://h:2/v1", true},
		{"http://h:1/v1", "http://g:1/v1", true},
		{"http://h/v1", "https://h/v1", true},
		{"http://localhost/v1", "https://localhost/v1", true},
		{"http://h:1/v1", "://bad", true},
	} {
		assert.Equal(t, tc.moved, backendMoved(tc.a, tc.b), "%s -> %s", tc.a, tc.b)
	}
}
