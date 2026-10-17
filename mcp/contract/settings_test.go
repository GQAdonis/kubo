package contract

import (
	"slices"
	"testing"
)

// validSettingsRead is a minimal conforming read document (openai backend).
func validSettingsRead() map[string]any {
	return map[string]any{
		"revision": "sha256-9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		"backend": map[string]any{
			"kind":     "openai",
			"base_url": "https://llm.example.com/v1",
			"model":    "gpt-5",
			"api_key":  map[string]any{"set": true, "source": "settings_api"},
		},
		"system_prompt": map[string]any{"safety_prefix": "FIXED", "custom": ""},
	}
}

// TestSettingsReadCarriesNoKeyMaterial pins ADR-0009 section 5: the read shape
// reports a key only as {set, source}, so no field can carry key material, and
// source is present exactly when set is true.
func TestSettingsReadCarriesNoKeyMaterial(t *testing.T) {
	rs := resolveRef(t, "settings/settings-read.schema.json")
	if err := rs.Validate(validSettingsRead()); err != nil {
		t.Fatalf("valid read rejected: %v", err)
	}

	withKeyMaterial := validSettingsRead()
	withKeyMaterial["backend"].(map[string]any)["api_key"] = map[string]any{"set": true, "source": "settings_api", "value": "sk-live"}
	if err := rs.Validate(withKeyMaterial); err == nil {
		t.Error("a read whose api_key carries a value must not validate")
	}

	noSource := validSettingsRead()
	noSource["backend"].(map[string]any)["api_key"] = map[string]any{"set": true}
	if err := rs.Validate(noSource); err == nil {
		t.Error("api_key set=true without source must not validate")
	}

	straySource := validSettingsRead()
	straySource["backend"].(map[string]any)["api_key"] = map[string]any{"set": false, "source": "file"}
	if err := rs.Validate(straySource); err == nil {
		t.Error("api_key set=false with a source must not validate")
	}

	notConfigured := validSettingsRead()
	notConfigured["backend"] = nil
	if err := rs.Validate(notConfigured); err != nil {
		t.Errorf("backend null (not configured) must validate: %v", err)
	}

	openaiIncomplete := validSettingsRead()
	openaiIncomplete["backend"] = map[string]any{"kind": "openai"}
	if err := rs.Validate(openaiIncomplete); err == nil {
		t.Error("kind openai without base_url, model and api_key must not validate")
	}

	uar := validSettingsRead()
	uar["backend"] = map[string]any{"kind": "uar", "base_url": "https://uar.example.com"}
	if err := rs.Validate(uar); err != nil {
		t.Errorf("kind uar must validate: %v", err)
	}

	unknownKind := validSettingsRead()
	unknownKind["backend"] = map[string]any{"kind": "ollama"}
	if err := rs.Validate(unknownKind); err == nil {
		t.Error("backend.kind outside openai|uar must not validate")
	}
}

// TestSettingsWriteShape pins the write rules: secret fields are write-only
// strings (or null to clear), unknown fields are rejected, and an egress
// confirmation must name the host.
func TestSettingsWriteShape(t *testing.T) {
	rs := resolveRef(t, "settings/settings-write.schema.json")
	valid := []map[string]any{
		{"backend": map[string]any{"api_key": "sk-live"}},
		{"backend": map[string]any{"api_key": nil}},
		{"backend": map[string]any{"kind": "uar"}},
		{"system_prompt": ""},
		{
			"backend":             map[string]any{"base_url": "https://llm2.example.com/v1"},
			"egress_confirmation": map[string]any{"host": "llm2.example.com"},
		},
	}
	for _, doc := range valid {
		if err := rs.Validate(doc); err != nil {
			t.Errorf("valid write %v rejected: %v", doc, err)
		}
	}
	invalid := []map[string]any{
		{"unknown_field": 1},
		{"backend": map[string]any{"api_key": ""}},
		{"backend": map[string]any{}},
		{"egress_confirmation": map[string]any{}},
		{"egress_confirmation": map[string]any{"host": "llm2.example.com", "also": "x"}},
		{"backend": map[string]any{"kind": "ollama"}},
	}
	for _, doc := range invalid {
		if err := rs.Validate(doc); err == nil {
			t.Errorf("invalid write %v must not validate", doc)
		}
	}
}

// TestSettingsErrorCodesAreStable pins the closed set of stable error codes:
// exactly the five the settings API defines (step_up_required added by
// console-session-auth, ADR-0013 section 5), and no tool error code leaks in.
func TestSettingsErrorCodesAreStable(t *testing.T) {
	var doc map[string]any
	readInto(t, "settings/settings-error.schema.json", &doc)
	errProps := doc["properties"].(map[string]any)["error"].(map[string]any)["properties"].(map[string]any)
	raw := errProps["code"].(map[string]any)["enum"].([]any)
	var codes []string
	for _, c := range raw {
		codes = append(codes, c.(string))
	}
	want := []string{"backend_not_configured", "revision_conflict", "egress_confirmation_required", "secrets_not_accepted", "step_up_required"}
	if !slices.Equal(codes, want) {
		t.Fatalf("error code enum = %v, want %v", codes, want)
	}

	rs := resolveRef(t, "settings/settings-error.schema.json")
	for _, code := range want {
		if err := rs.Validate(map[string]any{"error": map[string]any{"code": code, "message": "x"}}); err != nil {
			t.Errorf("code %s must validate: %v", code, err)
		}
	}
	// A tool error code (schemas/common.json ErrorCode) is not a settings code.
	if err := rs.Validate(map[string]any{"error": map[string]any{"code": "egress_not_acknowledged", "message": "x"}}); err == nil {
		t.Error("a code outside the stable set must not validate")
	}
}

// TestSettingsEventCarriesOnlyRevision pins mcp-agent-settings-api 1.5: the
// change event names the new revision and nothing else.
func TestSettingsEventCarriesOnlyRevision(t *testing.T) {
	rs := resolveRef(t, "settings/settings-event.schema.json")
	if err := rs.Validate(map[string]any{"type": "settings_changed", "revision": "r2"}); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}
	for _, doc := range []map[string]any{
		{"type": "settings_changed", "revision": "r2", "backend": map[string]any{}},
		{"type": "settings_changed"},
		{"type": "settings_updated", "revision": "r2"},
	} {
		if err := rs.Validate(doc); err == nil {
			t.Errorf("event %v must not validate", doc)
		}
	}
}

// TestCandidateUrlRules pins the test-connection and model-list URL rules
// (mcp-agent-settings-api 1.4): https anywhere, http only on loopback, and no
// credentials in the URL.
func TestCandidateUrlRules(t *testing.T) {
	tc := resolveRef(t, "settings/test-connection.schema.json#/$defs/TestConnectionRequest")
	accept := []string{
		"https://api.example.com/v1",
		"https://host.example.com:8443",
		"http://localhost:8080/v1",
		"http://127.0.0.1:11434",
		"http://[::1]:9000",
	}
	for _, u := range accept {
		if err := tc.Validate(map[string]any{"base_url": u}); err != nil {
			t.Errorf("candidate URL %q rejected: %v", u, err)
		}
	}
	reject := []string{
		"http://api.example.com/v1",
		"http://192.0.2.10:8080",
		"https://user:pass@api.example.com/v1",
		"ftp://api.example.com",
		"api.example.com",
	}
	for _, u := range reject {
		if err := tc.Validate(map[string]any{"base_url": u}); err == nil {
			t.Errorf("candidate URL %q must not validate", u)
		}
	}

	mlReq := resolveRef(t, "settings/model-list.schema.json#/$defs/ModelListRequest")
	if err := mlReq.Validate(map[string]any{}); err != nil {
		t.Errorf("empty model-list request (use the configured backend) must validate: %v", err)
	}
	mlRes := resolveRef(t, "settings/model-list.schema.json#/$defs/ModelListResult")
	if err := mlRes.Validate(map[string]any{"models": []any{map[string]any{"id": "gpt-5", "owned_by": "example"}}}); err != nil {
		t.Errorf("model list with extra entry fields must validate: %v", err)
	}
	if err := mlRes.Validate(map[string]any{"models": []any{map[string]any{"name": "gpt-5"}}}); err == nil {
		t.Error("a model entry without id must not validate")
	}
}
