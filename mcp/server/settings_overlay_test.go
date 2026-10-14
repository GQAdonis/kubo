//go:build !noagent

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readdirnames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func writeOverlayFile(t *testing.T, dir, doc string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, settingsOverlayFileName), []byte(doc), 0o600))
}

// TestSettingsOverlayPrecedence is the acceptance check for the precedence
// rule: effective settings resolve flags > overlay > config (ADR-0009
// section 7).
func TestSettingsOverlayPrecedence(t *testing.T) {
	cfg := mustParse(t, `{"llm":{"base_url":"http://127.0.0.1:59990/v1","model":"m-config","api_key":{"env":"`+settingsKeyEnv+`"}},`+
		`"agent":{"loop":{"max_steps":3}}}`)
	baseURL, model := "http://127.0.0.1:59991/v1", "m-overlay"
	prompt := "Answer briefly."
	ov := &SettingsOverlay{
		Backend:      &OverlayBackend{BaseURL: &baseURL, Model: &model},
		SystemPrompt: &prompt,
	}

	// Overlay shadows the config file.
	set, err := cfg.ResolveAgentWithOverlay(LLMOverrides{}, ov)
	require.NoError(t, err)
	require.NotNil(t, set)
	assert.Equal(t, baseURL, set.BaseURL)
	assert.Equal(t, model, set.Model)
	assert.Equal(t, prompt, set.SystemPrompt)
	require.NotNil(t, set.APIKey, "a key the overlay does not mention stays")
	assert.Equal(t, settingsKeyEnv, set.APIKey.Env)
	assert.Equal(t, 3, set.Limits.MaxSteps, "config sections the overlay has no keys for stay")

	// Flags shadow the overlay.
	flagModel := "m-flag"
	set, err = cfg.ResolveAgentWithOverlay(LLMOverrides{Model: flagModel}, ov)
	require.NoError(t, err)
	assert.Equal(t, flagModel, set.Model)
	assert.Equal(t, baseURL, set.BaseURL, "the overlay still beats the config where no flag speaks")

	// A cleared key in the overlay shadows a configured key.
	cleared := &SettingsOverlay{Backend: &OverlayBackend{APIKey: json.RawMessage("null")}}
	set, err = cfg.ResolveAgentWithOverlay(LLMOverrides{}, cleared)
	require.NoError(t, err)
	require.NotNil(t, set)
	assert.Nil(t, set.APIKey)

	// Without an overlay the config alone rules.
	set, err = cfg.ResolveAgentWithOverlay(LLMOverrides{}, nil)
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:59990/v1", set.BaseURL)
	assert.Equal(t, "m-config", set.Model)
}

// TestSettingsOverlayLoad: an absent overlay file is no overlay and no error;
// a written one round-trips; a relative directory and an invalid file fail
// with the file or directory named.
func TestSettingsOverlayLoad(t *testing.T) {
	dir := t.TempDir()

	ov, err := LoadSettingsOverlay(dir)
	require.NoError(t, err)
	assert.Nil(t, ov, "an absent overlay file is no overlay, not an error")

	_, err = LoadSettingsOverlay("relative/dir")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an absolute path")

	writeOverlayFile(t, dir, `{"backend":{"base_url":"http://127.0.0.1:59992/v1"},"system_prompt":"Be terse."}`)
	ov, err = LoadSettingsOverlay(dir)
	require.NoError(t, err)
	require.NotNil(t, ov)
	require.NotNil(t, ov.Backend)
	assert.Equal(t, "http://127.0.0.1:59992/v1", *ov.Backend.BaseURL)
	require.NotNil(t, ov.SystemPrompt)
	assert.Equal(t, "Be terse.", *ov.SystemPrompt)

	writeOverlayFile(t, dir, `{"backend":`)
	_, err = LoadSettingsOverlay(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), settingsOverlayFileName, "a corrupt overlay fails with the file named")

	// The overlay never holds key material: a key reference that is not an
	// absolute file is refused.
	writeOverlayFile(t, dir, `{"backend":{"api_key":{"env":"SOME_KEY"}}}`)
	_, err = LoadSettingsOverlay(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute file reference")
}

// TestSettingsOverlaySurvivesRestart: a write through the API persists into
// the settings directory, and a fresh start on the same directory reports the
// written values (overlay over config), not the config file's.
func TestSettingsOverlaySurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	e := newSettingsEnvDir(t, settingsBackendJSON, dir)
	etag, doc := getSettings(t, e)
	backend, _ := doc["backend"].(map[string]any)
	require.NotNil(t, backend)
	assert.Equal(t, "m-old", backend["model"], "before any write the config file rules")

	resp := e.do(t, http.MethodPut, tokSettingsAdmin,
		`{"backend":{"model":"m-persisted"},"system_prompt":"Only facts."}`,
		map[string]string{"If-Match": etag})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = bodyString(t, resp)

	// A fresh server on the same directory (a restart) sees the write.
	e2 := newSettingsEnvDir(t, settingsBackendJSON, dir)
	_, doc2 := getSettings(t, e2)
	validateSettingsSchema(t, "settings/settings-read.schema.json", doc2)
	backend2, _ := doc2["backend"].(map[string]any)
	require.NotNil(t, backend2)
	assert.Equal(t, "m-persisted", backend2["model"], "the overlay beats the config file after a restart")
	assert.Equal(t, "http://127.0.0.1:59998/v1", backend2["base_url"], "values no write touched still come from the config file")
	prompt2, _ := doc2["system_prompt"].(map[string]any)
	require.NotNil(t, prompt2)
	assert.Equal(t, "Only facts.", prompt2["custom"])
}

// TestSettingsOverlayFilesAre0600 is the acceptance check for the stored key
// and the overlay file modes: a key written through the API becomes a 0600
// file referenced as a SecretRef file from the 0600 overlay, the read reports
// it as {set, source: settings_api}, and no key material leaves the route.
func TestSettingsOverlayFilesAre0600(t *testing.T) {
	dir := t.TempDir()
	e := newSettingsEnvDir(t, settingsBackendJSON, dir)
	etag, _ := getSettings(t, e)

	const newKey = "sk-overlay-0123456789abcdefSECRET"
	resp := e.do(t, http.MethodPut, tokSettingsAdmin,
		`{"backend":{"model":"m-keyed","api_key":"`+newKey+`"}}`,
		map[string]string{"If-Match": etag})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body := bodyString(t, resp)
	assert.NotContains(t, body, newKey, "the write response carries no key material")
	assert.NotContains(t, e.logs.String(), newKey, "the log carries no key material")

	// The key file holds exactly the key; the overlay references it as a file.
	keyPath := filepath.Join(dir, settingsKeyFileName)
	rawKey, err := os.ReadFile(keyPath)
	require.NoError(t, err)
	assert.Equal(t, newKey, string(rawKey))
	rawOverlay, err := os.ReadFile(filepath.Join(dir, settingsOverlayFileName))
	require.NoError(t, err)
	var ovDoc map[string]any
	require.NoError(t, json.Unmarshal(rawOverlay, &ovDoc))
	ovBackend, _ := ovDoc["backend"].(map[string]any)
	require.NotNil(t, ovBackend)
	assert.Equal(t, map[string]any{"file": keyPath}, ovBackend["api_key"])
	assert.Equal(t, "m-keyed", ovBackend["model"])
	assert.NotContains(t, string(rawOverlay), newKey, "the overlay itself carries no key material")

	if runtime.GOOS != "windows" {
		for _, name := range []string{settingsKeyFileName, settingsOverlayFileName} {
			fi, err := os.Stat(filepath.Join(dir, name))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm(), "%s must be mode 0600", name)
		}
	}

	// The read reports provenance, never material.
	_, doc := getSettings(t, e)
	backend, _ := doc["backend"].(map[string]any)
	require.NotNil(t, backend)
	assert.Equal(t, map[string]any{"set": true, "source": "settings_api"}, backend["api_key"])

	// A restart on the same directory still reports the API-stored key.
	e2 := newSettingsEnvDir(t, settingsBackendJSON, dir)
	_, doc2 := getSettings(t, e2)
	backend2, _ := doc2["backend"].(map[string]any)
	require.NotNil(t, backend2)
	assert.Equal(t, map[string]any{"set": true, "source": "settings_api"}, backend2["api_key"])
}

// TestSettingsWriteKeyClearRemovesKeyFile: api_key null clears the stored key,
// removes the key file, and the clear persists — after a restart the
// configured key stays shadowed by the overlay's explicit null.
func TestSettingsWriteKeyClearRemovesKeyFile(t *testing.T) {
	dir := t.TempDir()
	e := newSettingsEnvDir(t, settingsBackendJSON, dir)
	etag, _ := getSettings(t, e)
	resp := e.do(t, http.MethodPut, tokSettingsAdmin,
		`{"backend":{"api_key":"sk-clear-me-0123456789abcdef"}}`,
		map[string]string{"If-Match": etag})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	etag = resp.Header.Get("ETag")
	_ = bodyString(t, resp)
	require.FileExists(t, filepath.Join(dir, settingsKeyFileName))

	resp = e.do(t, http.MethodPut, tokSettingsAdmin, `{"backend":{"api_key":null}}`, map[string]string{"If-Match": etag})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = bodyString(t, resp)
	_, err := os.Stat(filepath.Join(dir, settingsKeyFileName))
	assert.True(t, os.IsNotExist(err), "a clear removes the key file")

	_, doc := getSettings(t, e)
	backend, _ := doc["backend"].(map[string]any)
	require.NotNil(t, backend)
	assert.Equal(t, map[string]any{"set": false}, backend["api_key"], "the key the config file named is cleared")

	rawOverlay, err := os.ReadFile(filepath.Join(dir, settingsOverlayFileName))
	require.NoError(t, err)
	assert.Contains(t, string(rawOverlay), `"api_key":null`)

	// The clear survives a restart: the overlay's explicit null shadows the
	// config file's key, which would otherwise resolve (the env var is set).
	e2 := newSettingsEnvDir(t, settingsBackendJSON, dir)
	_, doc2 := getSettings(t, e2)
	backend2, _ := doc2["backend"].(map[string]any)
	require.NotNil(t, backend2)
	assert.Equal(t, map[string]any{"set": false}, backend2["api_key"])
}

// TestAbsentLLMKeyFileYieldsApiKeySetFalse is the CRITICAL-2 acceptance check:
// a configured llm.api_key whose file is absent stops neither startup nor the
// settings route; the read reports api_key.set=false and the loop runs without
// a key until the reference resolves.
func TestAbsentLLMKeyFileYieldsApiKeySetFalse(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent-key-file")
	cfgJSON := fmt.Sprintf(`{"llm":{"base_url":"http://127.0.0.1:59994/v1","model":"m-old","api_key":{"file":%s}}}`, strconv.Quote(missing))

	// The whole startup path (resolve, EnableAgent, EnableAgentSettings,
	// listener) must succeed with the key file absent.
	e := newSettingsEnv(t, cfgJSON)
	require.NotNil(t, e.s.Agent(), "the agent loop installs even when the key file is absent")

	etag, doc := getSettings(t, e)
	validateSettingsSchema(t, "settings/settings-read.schema.json", doc)
	backend, _ := doc["backend"].(map[string]any)
	require.NotNil(t, backend, "the backend is configured and reported; only the key is missing")
	assert.Equal(t, "http://127.0.0.1:59994/v1", backend["base_url"])
	assert.Equal(t, map[string]any{"set": false}, backend["api_key"])
	raw, _ := json.Marshal(doc)
	assert.NotContains(t, string(raw), settingsTestKey)

	// The absent key does not freeze the revision: a model write still applies.
	resp := e.do(t, http.MethodPut, tokSettingsAdmin, `{"backend":{"model":"m-new"}}`, map[string]string{"If-Match": etag})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = bodyString(t, resp)
}
