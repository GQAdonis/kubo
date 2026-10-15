//go:build !noagent

package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func uarLLMConfig(llm string) string {
	return `{"principals":[{"name":"uar","kind":"uar","credential":{"env":"` + uarTestMCPEnv + `"},` +
		`"uar":{"card_url":"http://127.0.0.1:59990/.well-known/agent-card.json"}},` +
		`{"name":"chat","kind":"ui","credential":{"env":"` + uarTestMCPEnv + `"}}],"llm":` + llm + `}`
}

func TestResolveUARDefaultsAndRefusals(t *testing.T) {
	ok := func(t *testing.T, llm string) *AgentSettings {
		t.Helper()
		set, err := mustParse(t, uarLLMConfig(llm)).ResolveAgent(LLMOverrides{})
		require.NoError(t, err)
		require.True(t, set.IsUAR())
		return set
	}
	t.Run("a bare uar section takes the defaults", func(t *testing.T) {
		set := ok(t, `{"kind":"uar","base_url":"http://127.0.0.1:59990","model":"m"}`)
		assert.Equal(t, "m", set.Model)
		u := set.UAR
		assert.Equal(t, DefaultUARAgentID, u.AgentID)
		assert.Equal(t, DefaultUARMCPName, u.MCPName)
		assert.Equal(t, DefaultUARScopes, u.Scopes)
		assert.Equal(t, DefaultUARGrantTTL, u.GrantTTL)
		assert.Nil(t, u.MCPCredential, "no principal: no kubo tools")
		assert.Empty(t, u.SkillsDir, "no skills dir: no skill is uploaded")
		assert.Equal(t, "1.1.0", u.MinVersion)
		assert.Equal(t, "v1.1.0", u.Pin.Tag)
	})
	t.Run("the default kind is openai", func(t *testing.T) {
		set, err := mustParse(t, `{"llm":{"base_url":"http://127.0.0.1:59990/v1","model":"m"}}`).ResolveAgent(LLMOverrides{})
		require.NoError(t, err)
		assert.False(t, set.IsUAR())
		assert.Equal(t, BackendKindOpenAI, set.Kind)
		assert.Nil(t, set.UAR)
	})
	t.Run("a settings write may leave the model of a UAR agent empty", func(t *testing.T) {
		// The settings contract makes model optional for kind uar; only the
		// configuration file requires it.
		set, err := (&Config{LLM: &LLMConfig{Kind: BackendKindUAR, BaseURL: "http://127.0.0.1:59990"}}).ResolveAgent(LLMOverrides{})
		require.NoError(t, err)
		assert.Empty(t, set.Model)
		_, err = (&Config{LLM: &LLMConfig{BaseURL: "http://127.0.0.1:59990"}}).ResolveAgent(LLMOverrides{})
		require.Error(t, err, "an openai backend still needs a model")
	})
	t.Run("a full section", func(t *testing.T) {
		set := ok(t, `{"kind":"uar","base_url":"https://uar.example.test","model":"m","uar":{"agent_id":"a1","mcp_name":"n1","provider":"p",`+
			`"mcp_principal":"uar","mcp_credential_revision":"rev","mcp_scopes":["ipfs","x"],"mcp_grant_ttl":"90s","skills_dir":"/srv/skills"}}`)
		u := set.UAR
		assert.Equal(t, []string{"a1", "n1", "p", "uar", "rev", "/srv/skills"}, []string{u.AgentID, u.MCPName, u.Provider, u.MCPPrincipal, u.CredentialRevision, u.SkillsDir})
		assert.Equal(t, []string{"ipfs", "x"}, u.Scopes)
		require.NotNil(t, u.MCPCredential)
		assert.Equal(t, uarTestMCPEnv, u.MCPCredential.Env)
	})
	for name, c := range map[string]struct{ llm, want string }{
		"unknown principal":           {`{"kind":"uar","base_url":"http://127.0.0.1:1","model":"m","uar":{"mcp_principal":"nobody","mcp_credential_revision":"r"}}`, "names no principal"},
		"principal of the wrong kind": {`{"kind":"uar","base_url":"http://127.0.0.1:1","model":"m","uar":{"mcp_principal":"chat","mcp_credential_revision":"r"}}`, "of kind uar"},
		"principal without revision":  {`{"kind":"uar","base_url":"http://127.0.0.1:1","model":"m","uar":{"mcp_principal":"uar"}}`, "mcp_credential_revision"},
		"grant ttl too short":         {`{"kind":"uar","base_url":"http://127.0.0.1:1","model":"m","uar":{"mcp_grant_ttl":"1s"}}`, "mcp_grant_ttl"},
		"grant ttl too long":          {`{"kind":"uar","base_url":"http://127.0.0.1:1","model":"m","uar":{"mcp_grant_ttl":"2h"}}`, "mcp_grant_ttl"},
		"plain http key to a remote":  {`{"kind":"uar","base_url":"http://uar.example.test","model":"m","api_key":{"env":"` + uarTestCredEnv + `"}}`, "plain http"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := ParseConfig([]byte(uarLLMConfig(c.llm)))
			if err == nil {
				_, err = cfg.ResolveAgent(LLMOverrides{})
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
		})
	}
	for name, doc := range map[string]string{
		"a relative skills dir": `{"llm":{"kind":"uar","base_url":"http://127.0.0.1:1","model":"m","uar":{"skills_dir":"skills"}}}`,
		"an unknown kind":       `{"llm":{"kind":"gpt","base_url":"http://127.0.0.1:1","model":"m"}}`,
		"a bad agent id":        `{"llm":{"kind":"uar","base_url":"http://127.0.0.1:1","model":"m","uar":{"agent_id":"a b"}}}`,
		"an unknown uar key":    `{"llm":{"kind":"uar","base_url":"http://127.0.0.1:1","model":"m","uar":{"token":"x"}}}`,
	} {
		_, err := ParseConfig([]byte(doc))
		assert.Error(t, err, name)
	}
}

// Spec "Published release only": with no release pinned the kind is refused.
func TestResolveUARRefusedWhileNoReleaseIsPinned(t *testing.T) {
	orig := loadUARPreflight
	t.Cleanup(func() { loadUARPreflight = orig })
	loadUARPreflight = func() (*contract.UARPreflight, error) {
		return &contract.UARPreflight{RecheckInterval: "30s"}, nil
	}
	_, err := mustParse(t, `{"llm":{"kind":"uar","base_url":"http://127.0.0.1:1","model":"m"}}`).ResolveAgent(LLMOverrides{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pins no published UAR release")
}

// The settings API switches the kind, reports it, and keeps the revision rules.
func TestSettingsWriteSwitchesToAndFromUAR(t *testing.T) {
	fake := newFakeUARRelease(t, replay(t, "run-text.sse"))
	dir := t.TempDir()
	e := newSettingsEnvDir(t, settingsBackendJSON, dir)
	etag, before := getSettings(t, e)
	assert.Equal(t, "openai", before["backend"].(map[string]any)["kind"])
	require.NotNil(t, e.s.Agent())

	host := strings.TrimPrefix(fake.srv.URL, "http://")
	write := `{"backend":{"kind":"uar","base_url":"` + fake.srv.URL + `","model":"qwen3:8b","api_key":"` + fakeUARToken + `"},` +
		`"egress_confirmation":{"host":"` + host + `"}}`
	resp := e.do(t, http.MethodPut, tokSettingsAdmin, write, map[string]string{"If-Match": etag})
	respBody := bodyString(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, respBody)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(respBody), &doc))
	validateSettingsSchema(t, "settings/settings-read.schema.json", doc)
	backend := doc["backend"].(map[string]any)
	assert.Equal(t, "uar", backend["kind"])
	assert.Equal(t, fake.srv.URL, backend["base_url"])
	assert.Equal(t, "qwen3:8b", backend["model"])
	assert.Equal(t, map[string]any{"set": true, "source": "settings_api"}, backend["api_key"])
	assert.NotContains(t, bodyString(t, e.do(t, http.MethodGet, tokSettingsAdmin, "", nil)), fakeUARToken)
	assert.Nil(t, e.s.Agent(), "the loop no longer serves while the backend is a UAR agent")
	require.NotNil(t, e.s.UARBackend())
	assert.Zero(t, fake.runCount(), "applying a backend contacts no UAR instance")
	assert.Empty(t, fake.callLog())

	// The kind survives a restart through the overlay.
	ov, err := LoadSettingsOverlay(dir)
	require.NoError(t, err)
	require.NotNil(t, ov.Backend.Kind)
	assert.Equal(t, "uar", *ov.Backend.Kind)
	set, err := mustParse(t, settingsBackendJSON).ResolveAgentWithOverlay(LLMOverrides{}, ov)
	require.NoError(t, err)
	assert.True(t, set.IsUAR())

	// Switching back is a write too.
	etag2 := resp.Header.Get("ETag")
	back := `{"backend":{"kind":"openai","base_url":"http://127.0.0.1:59998/v1","model":"m-old","api_key":null},"egress_confirmation":{"host":"127.0.0.1:59998"}}`
	resp = e.do(t, http.MethodPut, tokSettingsAdmin, back, map[string]string{"If-Match": etag2})
	require.Equal(t, http.StatusOK, resp.StatusCode, bodyString(t, resp))
	assert.Nil(t, e.s.UARBackend())
	assert.NotNil(t, e.s.Agent())
}

func TestSettingsRejectsAnUnknownKind(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	etag, _ := getSettings(t, e)
	resp := e.do(t, http.MethodPut, tokSettingsAdmin, `{"backend":{"kind":"gpt"}}`, map[string]string{"If-Match": etag})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestProbeUARChecksTheAgentCard(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	fake := newFakeUARRelease(t, replay(t, "run-text.sse"))
	probe := func(url string) map[string]any {
		resp := e.doPath(t, http.MethodPost, RouteSettingsTestConnection, tokSettingsAdmin, `{"base_url":"`+url+`","kind":"uar"}`)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var out map[string]any
		require.NoError(t, json.Unmarshal([]byte(bodyString(t, resp)), &out))
		validateSettingsSchemaDef(t, "settings/test-connection.schema.json", "TestConnectionResult", out)
		return out
	}
	assert.Equal(t, true, probe(fake.srv.URL)["ok"])
	assert.Equal(t, []string{"GET " + uarCardPath}, fake.callLog(), "the probe reads the public card and nothing else")

	fake.edit(func(u *fakeUARRelease) { u.version = "1.0.0" })
	out := probe(fake.srv.URL)
	assert.Equal(t, false, out["ok"])
	assert.Contains(t, out["detail"], "older than the pinned minimum")

	list := e.doPath(t, http.MethodGet, RouteSettingsModels+"?kind=uar&base_url="+fake.srv.URL, tokSettingsAdmin, "")
	assert.Equal(t, http.StatusOK, list.StatusCode)
	assert.Contains(t, bodyString(t, list), "no model list")
}
