//go:build !noagent

package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const kindMoveHost = "127.0.0.1:59998"

// kindMoveConfig is an OpenAI backend plus a uar principal named as the run
// grant, so a move to kind uar sends the grant.
func kindMoveConfig() string {
	return `{"principals":[{"name":"uar","kind":"uar","credential":{"env":"` + uarTestMCPEnv + `"},` +
		`"uar":{"card_url":"http://127.0.0.1:59990/.well-known/agent-card.json"}}],` +
		`"llm":{"base_url":"http://` + kindMoveHost + `/v1","model":"m-old",` +
		`"uar":{"provider":"p","mcp_principal":"uar","mcp_credential_revision":"r1"}}}`
}

func (e *settingsEnv) putRaw(t *testing.T, etag, body string) (int, map[string]any) {
	t.Helper()
	resp := e.do(t, http.MethodPut, tokSettingsAdmin, body, map[string]string{"If-Match": etag})
	raw := bodyString(t, resp)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &doc), raw)
	return resp.StatusCode, doc
}

// U2: a kind change alone is a move: it needs the typed confirmation even
// though the host is unchanged, and the D4 text names the grant.
func TestKindChangeAloneNeedsTheTypedConfirmation(t *testing.T) {
	e, _, etag := keyMoveEnv(t, kindMoveConfig())

	status, doc := e.putRaw(t, etag, `{"backend":{"kind":"uar"}}`)
	require.Equal(t, http.StatusConflict, status)
	validateSettingsSchema(t, "settings/settings-error.schema.json", doc)
	errObj := doc["error"].(map[string]any)
	assert.Equal(t, "egress_confirmation_required", errObj["code"])
	msg := errObj["message"].(string)
	assert.Contains(t, msg, "kind of the model backend")
	assert.Contains(t, msg, "kubo MCP grant")
	assert.Contains(t, msg, "llm.uar.mcp_principal uar")
	assert.Nil(t, e.s.UARBackend(), "nothing applied")
	assert.NotNil(t, e.s.Agent())

	// With the confirmation for the unchanged host it applies.
	status, doc = e.putRaw(t, etag, `{"backend":{"kind":"uar"},"egress_confirmation":{"host":"`+kindMoveHost+`"}}`)
	require.Equal(t, http.StatusOK, status, doc)
	assert.Equal(t, "uar", doc["backend"].(map[string]any)["kind"])
	assert.NotNil(t, e.s.UARBackend())
	assert.Nil(t, e.s.Agent())
}

// A write that sets only api_key and kind, with no base_url, cannot switch the
// kind without the confirmation, and the D4 text says the supplied key goes to
// the host.
func TestKindChangeWithOnlyAKeyNeedsTheConfirmation(t *testing.T) {
	e, dir, etag := keyMoveEnv(t, kindMoveConfig())

	status, doc := e.putRaw(t, etag, `{"backend":{"kind":"uar","api_key":"`+keyMoveNewKey+`"}}`)
	require.Equal(t, http.StatusConflict, status)
	msg := doc["error"].(map[string]any)["message"].(string)
	assert.Equal(t, "egress_confirmation_required", doc["error"].(map[string]any)["code"])
	assert.Contains(t, msg, "API key supplied in this write")
	assert.NotContains(t, msg, keyMoveNewKey)
	assert.NoFileExists(t, filepath.Join(dir, settingsKeyFileName), "a refused write stores nothing")
	assert.Nil(t, e.s.UARBackend())
}

// A key stored through the API does not follow a change of kind.
func TestKindChangeDoesNotCarryTheStoredKey(t *testing.T) {
	e, dir, etag := keyMoveEnv(t, kindMoveConfig())
	etag = storeAPIKey(t, e, etag)
	keyFile := filepath.Join(dir, settingsKeyFileName)
	require.FileExists(t, keyFile)

	doc, _ := e.putChecked(t, etag, `{"backend":{"kind":"uar"},"egress_confirmation":{"host":"`+kindMoveHost+`"}}`)
	assert.Equal(t, map[string]any{"set": false}, apiKeyOf(t, doc))
	assert.Equal(t, map[string]any{"reason": "host_changed", "cleared": true}, doc["key_not_carried"])
	_, err := os.Stat(keyFile)
	assert.ErrorIs(t, err, os.ErrNotExist, "the stored key file is removed")
	assert.NotNil(t, e.s.UARBackend())
}

// A key referenced from the config file does not follow a change of kind
// either; a key supplied by the same write does.
func TestKindChangeDoesNotCarryAReferencedKey(t *testing.T) {
	cfg := `{"principals":[{"name":"uar","kind":"uar","credential":{"env":"` + uarTestMCPEnv + `"},` +
		`"uar":{"card_url":"http://127.0.0.1:59990/.well-known/agent-card.json"}}],` +
		`"llm":{"base_url":"http://` + kindMoveHost + `/v1","model":"m-old","api_key":{"env":"` + settingsKeyEnv + `"},` +
		`"uar":{"provider":"p","mcp_principal":"uar","mcp_credential_revision":"r1"}}}`
	e, _, etag := keyMoveEnv(t, cfg)
	_, before := getSettings(t, e)
	assert.Equal(t, map[string]any{"set": true, "source": "environment"}, apiKeyOf(t, before))

	doc, _ := e.putChecked(t, etag, `{"backend":{"kind":"uar"},"egress_confirmation":{"host":"`+kindMoveHost+`"}}`)
	assert.Equal(t, map[string]any{"set": false}, apiKeyOf(t, doc), "the referenced key does not follow to the new kind")
	// Round 8 N3: the drop is reported, with the same reason as a host move.
	assert.Equal(t, map[string]any{"reason": "host_changed", "cleared": true}, doc["key_not_carried"])
}

// U5: the run grant is refused over plain http to a non-loopback host that the
// operator did not list, from the config and from the settings API.
func TestPlainHTTPRefusesTheGrant(t *testing.T) {
	principal := `"principals":[{"name":"uar","kind":"uar","credential":{"env":"` + uarTestMCPEnv + `"},` +
		`"uar":{"card_url":"http://127.0.0.1:59990/.well-known/agent-card.json"}}]`
	grant := `"uar":{"provider":"p","mcp_principal":"uar","mcp_credential_revision":"r1"}`

	t.Run("config", func(t *testing.T) {
		cfg := mustParse(t, `{`+principal+`,"llm":{"kind":"uar","base_url":"http://uar.example.test","model":"m",`+grant+`}}`)
		_, err := cfg.ResolveAgent(LLMOverrides{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "grant over plain http")

		listed := mustParse(t, `{`+principal+`,"llm":{"kind":"uar","base_url":"http://uar.example.test","model":"m",`+grant+`}}`)
		listed.Embeddings = &EmbeddingsConfig{InsecureHTTPHosts: []string{"uar.example.test"}}
		_, err = listed.ResolveAgent(LLMOverrides{})
		require.NoError(t, err, "a listed host may be reached over http")

		loop := mustParse(t, `{`+principal+`,"llm":{"kind":"uar","base_url":"http://127.0.0.1:59990","model":"m",`+grant+`}}`)
		_, err = loop.ResolveAgent(LLMOverrides{})
		require.NoError(t, err, "loopback may use http")
	})

	t.Run("settings API", func(t *testing.T) {
		// The openai backend is plain http on a host the operator listed; the
		// unlisted-host case is a write of base_url, refused by the probe rule,
		// so the reachable path is a kind change on an http backend that the
		// config (not the API) put there.
		cfg := `{` + principal + `,"llm":{"base_url":"http://uar.example.test/v1","model":"m-old",` + grant + `}}`
		e, _, etag := keyMoveEnv(t, cfg)
		resp := e.do(t, http.MethodPut, tokSettingsAdmin, `{"backend":{"kind":"uar"},"egress_confirmation":{"host":"uar.example.test"}}`,
			map[string]string{"If-Match": etag})
		body := bodyString(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, body)
		assert.Contains(t, body, "grant over plain http")
		assert.Nil(t, e.s.UARBackend())
	})
}

// R8-09: llm.kind is validated before the "nothing configured" return.
func TestLLMKindIsValidatedBeforeNothingConfigured(t *testing.T) {
	_, err := ParseConfig([]byte(`{"llm":{"kind":"gpt"}}`))
	require.Error(t, err, "an invalid kind fails the parse")
	_, err = (&Config{LLM: &LLMConfig{Kind: "gpt"}}).ResolveAgent(LLMOverrides{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "llm.kind")

	_, err = (&Config{LLM: &LLMConfig{Kind: BackendKindUAR}}).ResolveAgent(LLMOverrides{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "llm.kind uar needs llm.base_url")

	set, err := (&Config{LLM: &LLMConfig{Kind: BackendKindOpenAI}}).ResolveAgent(LLMOverrides{})
	require.NoError(t, err)
	assert.Nil(t, set, "a valid kind with nothing else configured still leaves the agent off")
}
