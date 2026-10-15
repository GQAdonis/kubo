package contract

import (
	"io/fs"
	"strings"
	"testing"
	"time"
)

func TestUARPreflightLoads(t *testing.T) {
	p, err := LoadUARPreflight()
	if err != nil {
		t.Fatal(err)
	}
	if err := resolveRef(t, "agentic/uar-preflight.schema.json").Validate(readJSON(t, UARPreflightFile)); err != nil {
		t.Fatalf("%s: %v", UARPreflightFile, err)
	}
	if d, _ := p.Recheck(); d != 30*time.Second {
		t.Errorf("recheck interval %s, want 30s (ADR-0007)", d)
	}
	// v1.1.0 is pinned (agent-backend-uar 1.4): version, settings path and the
	// evidence travel together, and nothing in the file is null.
	if p.MinVersion == nil || *p.MinVersion != "1.1.0" {
		t.Errorf("minVersion %v, want 1.1.0", p.MinVersion)
	}
	if p.SettingsPath == nil || *p.SettingsPath != "/api/uar/settings/effective" {
		t.Errorf("settingsPath %v, want /api/uar/settings/effective", p.SettingsPath)
	}
	want := UARPin{
		Tag:      "v1.1.0",
		Artifact: "sha256:064f5004fb3ba76506abce5cf81285b2a8aa3e54fa5a22e2c2bfc97b1211e43b",
		Report:   "mcp/uar/spike/REPORT.md",
	}
	if p.PinnedFrom == nil || *p.PinnedFrom != want {
		t.Errorf("pinnedFrom %+v, want %+v", p.PinnedFrom, want)
	}
	if strings.Contains(string(mustRead(t, UARPreflightFile)), "null") {
		t.Errorf("%s contains null", UARPreflightFile)
	}
}

func TestUARPreflightRules(t *testing.T) {
	v, path := "1.4.0", "/api/uar/effective-settings"
	pin := &UARPin{Tag: "v1.4.0", Artifact: "sha256:" + strings.Repeat("0", 64), Report: "mcp/uar/spike/REPORT.md"}
	for name, p := range map[string]UARPreflight{
		"version without path":  {MinVersion: &v, PinnedFrom: pin, RecheckInterval: "30s"},
		"path without version":  {SettingsPath: &path, RecheckInterval: "30s"},
		"pin without evidence":  {MinVersion: &v, SettingsPath: &path, RecheckInterval: "30s"},
		"zero recheck":          {RecheckInterval: "0s"},
		"recheck over a minute": {RecheckInterval: "61s"},
		"recheck not seconds":   {RecheckInterval: "1m"},
	} {
		if p.check() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	ok := UARPreflight{MinVersion: &v, SettingsPath: &path, PinnedFrom: pin, RecheckInterval: "30s"}
	if err := ok.check(); err != nil {
		t.Errorf("complete pin rejected: %v", err)
	}
}

func TestUARSettingsFixtures(t *testing.T) {
	rs := resolveRef(t, UARSettingsSchemaFile)
	for _, f := range []string{
		"fixtures/uar/settings-shipped-profile.json",
		"fixtures/uar/settings-uar-defaults.json",
		"fixtures/uar/settings-shell-enabled.json",
		"fixtures/uar/settings-no-inventory.json",
	} {
		if err := rs.Validate(readJSON(t, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	// The record list UAR serves today must not pass as effective settings.
	if rs.Validate(readJSON(t, "fixtures/uar/observed-records-a7cb9729.json")) == nil {
		t.Error("observed UAR settings records validated as effective settings")
	}
	// Missing core fields and a URL instead of a host are refused.
	for name, doc := range map[string]map[string]any{
		"no revision": {
			"llm": map[string]any{"provider_host": "local"}, "knowledge_base_embedder": map[string]any{"provider_host": "local"},
			"memory_embedder": map[string]any{"provider_host": "local"},
		},
		"url as host": {
			"revision": "r", "llm": map[string]any{"provider_host": "https://api.example.com/v1"},
			"knowledge_base_embedder": map[string]any{"provider_host": "local"}, "memory_embedder": map[string]any{"provider_host": "local"},
		},
	} {
		if rs.Validate(doc) == nil {
			t.Errorf("%s: validated", name)
		}
	}
}

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	data, err := fs.ReadFile(files, name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return data
}
