//go:build !noagent

package server

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/ipfs/kubo/mcp/contract"
)

// Bounds of llm.uar.mcp_grant_ttl. The grant is the static bearer of the uar
// principal named by llm.uar.mcp_principal; the TTL is sent to UAR as
// expires_at_unix, which is advisory to UAR and not enforced by ipfs mcp. A
// leaked grant stays usable until that principal's secret is rotated, whatever
// the TTL says. The bounds only keep the advisory value sensible: never so
// short that a run outlives it before its first tool call.
const (
	minUARGrantTTL = 30 * time.Second
	maxUARGrantTTL = time.Hour
)

// loadUARPreflight is the contract's pin; tests replace it to cover the
// unpinned state.
var loadUARPreflight = contract.LoadUARPreflight

var uarNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// UARSettings is the resolved llm.uar section of a backend of kind uar. The
// UAR base URL, credential and model are the AgentSettings fields BaseURL,
// APIKey and Model.
type UARSettings struct {
	AgentID  string
	Provider string
	MCPName  string
	// MCPCredential is the bearer UAR presents to ipfs mcp (the credential of
	// llm.uar.mcp_principal); nil when no principal is named. It is read at
	// each run, so a rotated token takes effect without a restart.
	MCPCredential *SecretRef
	// MCPPrincipal names the principal behind MCPCredential, for messages.
	MCPPrincipal       string
	CredentialRevision string
	Scopes             []string
	GrantTTL           time.Duration
	SkillsDir          string
	// MinVersion and Pin come from the contract's uar-preflight.json. A backend
	// of kind uar is built only while a release is pinned.
	MinVersion string
	Pin        contract.UARPin
}

// resolveUAR builds the UARSettings of a backend of kind uar. It contacts
// nothing. It refuses the kind while the contract pins no published UAR
// release (spec "Published release only"), and every error names a key, never
// a value that could be a secret.
func (c *Config) resolveUAR(llm LLMConfig) (*UARSettings, error) {
	pre, err := loadUARPreflight()
	if err != nil {
		return nil, fmt.Errorf("llm.kind uar: %w", err)
	}
	if pre.MinVersion == nil || pre.PinnedFrom == nil {
		return nil, errors.New("llm.kind uar is refused: mcp/contract/agentic/uar-preflight.json pins no published UAR release")
	}
	u := &UARSettings{
		AgentID:    DefaultUARAgentID,
		MCPName:    DefaultUARMCPName,
		Scopes:     slices.Clone(DefaultUARScopes),
		GrantTTL:   DefaultUARGrantTTL,
		MinVersion: *pre.MinVersion,
		Pin:        *pre.PinnedFrom,
	}
	cfg := llm.UAR
	if cfg == nil {
		return u, nil
	}
	for _, v := range []struct{ key, val string }{{"llm.uar.agent_id", cfg.AgentID}, {"llm.uar.mcp_name", cfg.MCPName}} {
		if v.val != "" && !uarNamePattern.MatchString(v.val) {
			return nil, fmt.Errorf("%s must be 1 to 64 letters, digits, dot, dash or underscore", v.key)
		}
	}
	u.AgentID = firstNonEmpty(cfg.AgentID, u.AgentID)
	u.MCPName = firstNonEmpty(cfg.MCPName, u.MCPName)
	u.Provider = cfg.Provider
	if len(cfg.MCPScopes) > 0 {
		u.Scopes = slices.Clone(cfg.MCPScopes)
	}
	if cfg.MCPGrantTTL != "" {
		d, err := time.ParseDuration(cfg.MCPGrantTTL)
		if err != nil || d < minUARGrantTTL || d > maxUARGrantTTL {
			return nil, fmt.Errorf("llm.uar.mcp_grant_ttl must be a duration from %s to %s", minUARGrantTTL, maxUARGrantTTL)
		}
		u.GrantTTL = d
	}
	if cfg.SkillsDir != "" {
		if !filepath.IsAbs(cfg.SkillsDir) && !isWindowsAbs(cfg.SkillsDir) {
			return nil, errors.New("llm.uar.skills_dir must be an absolute path")
		}
		u.SkillsDir = cfg.SkillsDir
	}
	if cfg.MCPPrincipal == "" {
		return u, nil
	}
	i := slices.IndexFunc(c.Principals, func(p PrincipalConfig) bool { return p.Name == cfg.MCPPrincipal })
	switch {
	case i < 0:
		return nil, errors.New("llm.uar.mcp_principal names no principal in principals")
	case c.Principals[i].Kind != KindUAR:
		return nil, errors.New("llm.uar.mcp_principal must name a principal of kind uar")
	case cfg.MCPCredentialRevision == "":
		return nil, errors.New("llm.uar.mcp_credential_revision is required when llm.uar.mcp_principal is set")
	}
	cred := c.Principals[i].Credential
	u.MCPCredential = &cred
	u.MCPPrincipal = cfg.MCPPrincipal
	u.CredentialRevision = cfg.MCPCredentialRevision
	return u, nil
}

// digestText is a secret-free text of the UAR settings that decide the
// settings revision: a change to any of them changes the revision.
func (u *UARSettings) digestText() string {
	if u == nil {
		return ""
	}
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%v|%s|%s", u.AgentID, u.Provider, u.MCPName, u.MCPPrincipal, u.CredentialRevision,
		u.GrantTTL, u.Scopes, u.SkillsDir, u.Pin.Artifact)
}
