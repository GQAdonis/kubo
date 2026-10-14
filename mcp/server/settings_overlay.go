//go:build !noagent

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// File names inside the settings directory (agent.settings.dir,
// --settings-dir). The overlay is the persisted, secret-free half of the
// settings writes; a key written through the API lives in its own 0600 file
// and is referenced from the overlay as a SecretRef file (ADR-0009 section 7).
const (
	settingsOverlayFileName = "overlay.json"
	settingsKeyFileName     = "llm-api-key"
	// maxSettingsKeyBytes is the write schema's bound on the api_key string.
	maxSettingsKeyBytes = 512
)

// SettingsOverlay is the persisted form of the settings writes, held in
// <dir>/overlay.json. It is sparse: a field is present only when a settings
// write set it, so config-file values no write touched still take effect
// (effective settings resolve flags > overlay > config). It never carries key
// material: the write-only key is a SecretRef file naming the key file.
type SettingsOverlay struct {
	Backend      *OverlayBackend `json:"backend,omitempty"`
	SystemPrompt *string         `json:"system_prompt,omitempty"`
	// EgressAck records, per principal name, the hosts a confirmed settings
	// write added to that principal's egress_ack (D4): only ever the newly
	// chosen backend host of the write that carried the typed confirmation.
	// The settings route's check reads it beside the configured egress_ack;
	// it grants no tool access by itself.
	EgressAck map[string][]string `json:"egress_ack,omitempty"`
}

// OverlayBackend is the overlay's backend section.
type OverlayBackend struct {
	BaseURL *string `json:"base_url,omitempty"`
	Model   *string `json:"model,omitempty"`
	// APIKey is absent when no write touched the key, "null" when a write
	// cleared it, and a SecretRef object (always a file) after a write set it.
	APIKey json.RawMessage `json:"api_key,omitempty"`
}

// apiKey decodes the overlay's key field: present is false when no write
// touched it, cleared is true when a write cleared it. A file loaded through
// LoadSettingsOverlay was validated, so the decode cannot fail here.
func (b *OverlayBackend) apiKey() (ref *SecretRef, cleared, present bool) {
	if b == nil || len(b.APIKey) == 0 {
		return nil, false, false
	}
	if bytes.Equal(bytes.TrimSpace(b.APIKey), []byte("null")) {
		return nil, true, true
	}
	var r SecretRef
	if err := json.Unmarshal(b.APIKey, &r); err != nil {
		return nil, false, false
	}
	return &r, false, true
}

// LoadSettingsOverlay reads <dir>/overlay.json. An absent overlay is not an
// error: nothing was persisted yet. A present but invalid one is an error
// naming the file: what the written settings were is then ambiguous, and
// silently falling back to the config file would hide that. The directory must
// be an absolute path, like every path this package takes. The overlay carries
// no secrets, so an error can quote no key material.
func LoadSettingsOverlay(dir string) (*SettingsOverlay, error) {
	if !filepath.IsAbs(dir) && !isWindowsAbs(dir) {
		return nil, fmt.Errorf("settings directory %q is not an absolute path (agent.settings.dir, --settings-dir)", dir)
	}
	path := filepath.Join(dir, settingsOverlayFileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read settings overlay: %w", err)
	}
	var ov SettingsOverlay
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ov); err != nil {
		return nil, fmt.Errorf("settings overlay %s is not a valid overlay: %v", path, err)
	}
	if ref, cleared, present := ov.Backend.apiKey(); present && !cleared {
		if ref.Env != "" || !filepath.IsAbs(ref.File) && !isWindowsAbs(ref.File) {
			return nil, fmt.Errorf("settings overlay %s: api_key must be an absolute file reference", path)
		}
	}
	for name, hosts := range ov.EgressAck {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("settings overlay %s: egress_ack has an empty principal name", path)
		}
		for _, h := range hosts {
			if host, _ := normalizeHost(h); host == "" {
				return nil, fmt.Errorf("settings overlay %s: egress_ack entry %q is not a host", path, h)
			}
		}
	}
	return &ov, nil
}

// WithSettingsOverlay returns a copy of the config with the overlay's present
// fields shadowing the llm section (overlay > config; the --llm-* flags still
// win over both at ResolveAgent). A nil overlay, or one without a backend
// section, returns the config unchanged.
func (c *Config) WithSettingsOverlay(ov *SettingsOverlay) *Config {
	if ov == nil || ov.Backend == nil {
		return c
	}
	cp := *c
	var llm LLMConfig
	if c.LLM != nil {
		llm = *c.LLM
	}
	b := ov.Backend
	if b.BaseURL != nil {
		llm.BaseURL = *b.BaseURL
	}
	if b.Model != nil {
		llm.Model = *b.Model
	}
	if ref, cleared, present := b.apiKey(); present {
		if cleared {
			llm.APIKey = nil
		} else {
			r := *ref
			llm.APIKey = &r
		}
	}
	cp.LLM = &llm
	return &cp
}

// ResolveAgentWithOverlay is ResolveAgent with the overlay between the flags
// and the config file, plus the overlay's system prompt (which has no config
// key). An overlay whose key file is absent resolves like any other
// configuration with an unreadable key reference: it is not a startup failure
// (CRITICAL-2); the read shape reports api_key.set=false.
func (c *Config) ResolveAgentWithOverlay(o LLMOverrides, ov *SettingsOverlay) (*AgentSettings, error) {
	set, err := c.WithSettingsOverlay(ov).ResolveAgent(o)
	if err != nil || set == nil {
		return set, err
	}
	if ov != nil && ov.SystemPrompt != nil {
		set.SystemPrompt = *ov.SystemPrompt
	}
	return set, nil
}

// withWrite returns a copy of the overlay with the write's fields recorded.
// The overlay stays sparse: only the fields the write carried change. A
// non-empty egressHost records the D4 extension of the calling principal's
// egress_ack with exactly that host.
func (ov *SettingsOverlay) withWrite(wr settingsWriteBody, keySet bool, keyClear bool, keyPath, principal, egressHost string) *SettingsOverlay {
	out := &SettingsOverlay{}
	if ov != nil {
		if ov.Backend != nil {
			b := *ov.Backend
			out.Backend = &b
		}
		if ov.SystemPrompt != nil {
			p := *ov.SystemPrompt
			out.SystemPrompt = &p
		}
		if len(ov.EgressAck) > 0 {
			out.EgressAck = make(map[string][]string, len(ov.EgressAck))
			for name, hosts := range ov.EgressAck {
				out.EgressAck[name] = slices.Clone(hosts)
			}
		}
	}
	if b := wr.Backend; b != nil {
		if out.Backend == nil {
			out.Backend = &OverlayBackend{}
		}
		if b.BaseURL != nil {
			v := *b.BaseURL
			out.Backend.BaseURL = &v
		}
		if b.Model != nil {
			v := *b.Model
			out.Backend.Model = &v
		}
		switch {
		case keySet:
			raw, _ := json.Marshal(SecretRef{File: keyPath})
			out.Backend.APIKey = raw
		case keyClear:
			out.Backend.APIKey = json.RawMessage("null")
		}
	}
	if wr.SystemPrompt != nil {
		v := *wr.SystemPrompt
		out.SystemPrompt = &v
	}
	if egressHost != "" && principal != "" {
		if out.EgressAck == nil {
			out.EgressAck = make(map[string][]string, 1)
		}
		if !slices.ContainsFunc(out.EgressAck[principal], func(h string) bool { return sameHostPort(h, egressHost) }) {
			out.EgressAck[principal] = append(out.EgressAck[principal], egressHost)
		}
	}
	return out
}

// save writes the overlay atomically (write-then-rename) with mode 0600, so a
// crashed write never leaves a half-written settings file. The overlay carries
// no key material.
func (ov *SettingsOverlay) save(dir string) error {
	data, err := json.Marshal(ov)
	if err != nil {
		return fmt.Errorf("encode settings overlay: %w", err)
	}
	return writeFile0600(dir, settingsOverlayFileName, append(data, '\n'))
}

// writeFile0600 writes data to <dir>/<name> with mode 0600 through a temporary
// file and rename. The directory is created with mode 0700 when absent.
func writeFile0600(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, name+".tmp-*") // CreateTemp files are 0600
	if err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	tmpName := tmp.Name()
	if _, werr := tmp.Write(data); werr != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write %s: %w", name, werr)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("write %s: %w", name, err)
	}
	target := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		// Windows refuses to rename over an existing file.
		_ = os.Remove(target)
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace %s: %w", name, err)
	}
	return nil
}
