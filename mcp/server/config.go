package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/ipfs/kubo/mcp/contract"
)

const configSchemaPath = "config/ipfs-mcp-config.schema.json"

// Config is the part of the `ipfs mcp --config` file that the transport and
// tool selection read. The file is validated against the contract's config
// schema before it is decoded, so the sections other changes own (llm, agent,
// embeddings) are checked here and interpreted by their owners.
type Config struct {
	HTTP       HTTPConfig        `json:"http"`
	Tools      ToolsConfig       `json:"tools"`
	Principals []PrincipalConfig `json:"principals"`
	// Auth is the auth section (auth.oidc: session-token verification, off by
	// default).
	Auth AuthConfig `json:"auth"`
	// Confirmation is the confirmation section (modes, resolvers, cli, ttl).
	Confirmation ConfirmationConfig `json:"confirmation"`
	// LLM is the model backend of the agent loop (nil: the agent is off) and
	// also names a host that content would be forwarded to (egress_ack).
	// Embeddings is read only for the hosts it names.
	LLM        *LLMConfig        `json:"llm"`
	Embeddings *EmbeddingsConfig `json:"embeddings"`
	// Agent holds the loop limits (agent.loop).
	Agent AgentConfig `json:"agent"`
	// Daemon is the daemon section (daemon.wait).
	Daemon DaemonConfig `json:"daemon"`
	// TrustedPeers is trusted_peers: peer IDs whose peer-supplied data is
	// marked trusted. Default empty.
	TrustedPeers []string `json:"trusted_peers"`
}

// ConfirmationConfig is the confirmation section. Modes is nil when the file
// does not set it, and an empty slice when it turns the gate resolvers off.
// Resolvers is nil when absent (all resolvers) and narrows the resolvers the
// modes enable otherwise. CLI is nil when absent; true enables the cli mode.
// TTL is empty when absent (DefaultConfirmTTL).
type ConfirmationConfig struct {
	Modes     *[]string `json:"modes"`
	Resolvers *[]string `json:"resolvers"`
	CLI       *bool     `json:"cli"`
	TTL       string    `json:"ttl"`
}

// Bounds of confirmation.ttl. The minimum is one second because confirm tokens
// carry whole-second expiries. DefaultConfirmTTL is how long a confirmation, and
// a token bound to it, lives when the config does not say.
const (
	DefaultConfirmTTL = 120 * time.Second
	MinConfirmTTL     = time.Second
	MaxConfirmTTL     = 10 * time.Minute
)

// TTLDuration is the confirmation lifetime: confirmation.ttl, or
// DefaultConfirmTTL when it is absent. It returns the validation error for a
// value ParseConfig would refuse, so an unvalidated config never falls back to
// the default silently.
func (c ConfirmationConfig) TTLDuration() (time.Duration, error) {
	if c.TTL == "" {
		return DefaultConfirmTTL, nil
	}
	return parseConfirmTTL(c.TTL)
}

// parseConfirmTTL parses and bounds a confirmation.ttl value.
func parseConfirmTTL(v string) (time.Duration, error) {
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("confirmation.ttl %q is not a valid duration", v)
	}
	if d < MinConfirmTTL || d > MaxConfirmTTL {
		return 0, fmt.Errorf("confirmation.ttl %q is out of range: it must be at least %s and at most %s", v, MinConfirmTTL, MaxConfirmTTL)
	}
	return d, nil
}

// validate checks what the schema cannot: the ttl bounds and a contradiction
// between confirmation.cli and confirmation.modes.
func (c ConfirmationConfig) validate() error {
	if c.TTL != "" {
		if _, err := parseConfirmTTL(c.TTL); err != nil {
			return err
		}
	}
	if c.CLI != nil && !*c.CLI && c.Modes != nil && slices.Contains(*c.Modes, "cli") {
		return errors.New(`confirmation.cli is false but confirmation.modes contains "cli"; remove "cli" from confirmation.modes or set confirmation.cli to true`)
	}
	return nil
}

// DaemonConfig is the daemon section. Wait is empty when absent
// (DefaultDaemonWait).
type DaemonConfig struct {
	Wait string `json:"wait"`
}

// Bounds of daemon.wait. DefaultDaemonWait, used only with --http, covers a
// sidecar that starts a few seconds to a minute before the daemon container is
// ready. Plain stdio defaults to no wait, because an MCP host has its own
// startup timeout; an explicit value, "0s" included, is always honored.
const (
	DefaultDaemonWait = 60 * time.Second
	MaxDaemonWait     = 10 * time.Minute
)

// WaitDuration is how long startup keeps retrying an unreachable daemon:
// daemon.wait when set; when absent DefaultDaemonWait with the HTTP transport
// (httpTransport) and zero for stdio. Zero means do not wait.
func (c DaemonConfig) WaitDuration(httpTransport bool) (time.Duration, error) {
	if c.Wait == "" {
		if httpTransport {
			return DefaultDaemonWait, nil
		}
		return 0, nil
	}
	d, err := time.ParseDuration(c.Wait)
	if err != nil {
		return 0, fmt.Errorf("daemon.wait %q is not a valid duration", c.Wait)
	}
	if d < 0 || d > MaxDaemonWait {
		return 0, fmt.Errorf("daemon.wait %q is out of range: it must be 0s or at most %s", c.Wait, MaxDaemonWait)
	}
	return d, nil
}

// HTTPConfig is the http section.
type HTTPConfig struct {
	Enabled        bool     `json:"enabled"`
	Listen         string   `json:"listen"`
	AllowedOrigins []string `json:"allowed_origins"`
	MaxBodyBytes   int64    `json:"max_body_bytes"`
	SessionTimeout string   `json:"session_timeout"`
}

// ToolsConfig is the tools section.
type ToolsConfig struct {
	Groups   []string `json:"groups"`
	ReadOnly bool     `json:"read_only"`
	Disabled []string `json:"disabled"`
	// AppViews is tools.app_views; nil means the default, true.
	AppViews *bool `json:"app_views"`
}

// AppViewsEnabled reports whether MCP App views are on (tools.app_views,
// default true).
func (t ToolsConfig) AppViewsEnabled() bool { return t.AppViews == nil || *t.AppViews }

// PrincipalConfig is one entry of principals. Credentials are references, never
// values.
type PrincipalConfig struct {
	Name              string     `json:"name"`
	Kind              string     `json:"kind"`
	Credential        SecretRef  `json:"credential"`
	Groups            []string   `json:"groups"`
	WriteGroups       []string   `json:"write_groups"`
	DestructiveGroups []string   `json:"destructive_groups"`
	EgressAck         []string   `json:"egress_ack"`
	ShellCapable      *bool      `json:"shell_capable"`
	Check             string     `json:"check"`
	UAR               *UARConfig `json:"uar"`
	// Subjects are the OpenID Connect `sub` values (identity IDs) that resolve
	// to this principal when auth.oidc is enabled. A principal needs a
	// credential, subjects, or both.
	Subjects []string `json:"subjects"`
	// RequireAMR nil is the kind default; an explicit [] accepts any amr.
	RequireAMR *[]string `json:"require_amr"`
	// MaxAuthAge is a duration string; empty is the kind default.
	MaxAuthAge string `json:"max_auth_age"`
}

// HasCredential reports whether the principal names a static credential.
func (p PrincipalConfig) HasCredential() bool {
	return p.Credential.Env != "" || p.Credential.File != ""
}

// LoadConfig reads and validates a config file. Validation errors name the
// offending key.
func LoadConfig(file string) (*Config, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg, err := ParseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", file, err)
	}
	return cfg, nil
}

// ParseConfig validates config JSON against the contract schema and decodes it,
// filling the contract defaults.
func ParseConfig(data []byte) (*Config, error) {
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("not valid JSON: %w", err)
	}
	schema, err := configValidator()
	if err != nil {
		return nil, err
	}
	if err := schema.Validate(doc); err != nil {
		// The validator quotes the offending value; a literal secret must not
		// reach a log or a terminal.
		return nil, fmt.Errorf("does not match the ipfs mcp config schema: %s", redactSecrets(err.Error(), doc))
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.Confirmation.validate(); err != nil {
		return nil, err
	}
	if _, err := cfg.Daemon.WaitDuration(false); err != nil {
		return nil, err
	}
	if _, err := parseTrustedPeers(cfg.TrustedPeers); err != nil {
		return nil, err
	}
	if err := cfg.validatePrincipals(); err != nil {
		return nil, err
	}
	if err := cfg.Auth.OIDC.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// secretKeys are the config keys whose values are credentials.
var secretKeys = []string{"credential", "api_key", "admin_credential", "token", "secret", "password"}

// redactSecrets replaces every string found under a credential key with
// "<redacted>" in msg.
func redactSecrets(msg string, doc any) string {
	var walk func(v any, secret bool)
	walk = func(v any, secret bool) {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				walk(child, secret || slices.Contains(secretKeys, k))
			}
		case []any:
			for _, child := range x {
				walk(child, secret)
			}
		case string:
			if secret && len(x) > 0 {
				msg = strings.ReplaceAll(msg, x, "<redacted>")
			}
		}
	}
	walk(doc, false)
	return msg
}

func configValidator() (*jsonschema.Resolved, error) {
	raw, err := fs.ReadFile(contract.FS(), configSchemaPath)
	if err != nil {
		return nil, fmt.Errorf("read config schema: %w", err)
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse config schema: %w", err)
	}
	r, err := s.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("resolve config schema: %w", err)
	}
	return r, nil
}

func (c *Config) applyDefaults() {
	if c.HTTP.Listen == "" {
		c.HTTP.Listen = DefaultHTTPListen
	}
	if c.HTTP.MaxBodyBytes == 0 {
		c.HTTP.MaxBodyBytes = DefaultMaxBodyBytes
	}
	if c.HTTP.SessionTimeout == "" {
		c.HTTP.SessionTimeout = DefaultSessionTimeout.String()
	}
	for i := range c.Principals {
		// groups defaults to every group (read-class tools only); write and
		// destructive grants default to none.
		if c.Principals[i].Groups == nil {
			c.Principals[i].Groups = []string{AllGroups}
		}
		if c.Principals[i].Check == "" {
			c.Principals[i].Check = CheckVerify
		}
	}
}

// SessionTimeoutDuration parses http.session_timeout.
func (c *Config) SessionTimeoutDuration() (time.Duration, error) {
	d, err := time.ParseDuration(c.HTTP.SessionTimeout)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("http.session_timeout %q is not a positive duration", c.HTTP.SessionTimeout)
	}
	return d, nil
}

// Credentials reads every principal's secret and returns the static
// credentials of the principals whose reference resolves, plus one warning per
// principal it disables. An absent credential — an unset or empty environment
// variable, a missing, unreadable or empty file — disables only that
// principal, with a warning (ADR-0009 section 3, CRITICAL-2): the principal
// authenticates nothing until the reference resolves. A warning names the
// principal and the reference, never a secret. Credentials does not stop
// startup; the HTTP caller does when principals are configured and none
// resolved (S5), so the listener never turns anonymous by accident.
func (c *Config) Credentials() (creds []StaticCredential, warnings []string) {
	for _, p := range c.Principals {
		if !p.HasCredential() && len(p.Subjects) > 0 {
			// A session-only principal: it has no static bearer to resolve.
			continue
		}
		token, err := p.Credential.Read()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("principal %s (%s) is disabled: %v; it authenticates nothing until the reference resolves", p.Name, p.Kind, err))
			continue
		}
		creds = append(creds, StaticCredential{
			Identity: c.configuredIdentity(p),
			Token:    token,
		})
	}
	return creds, warnings
}

// OperatorIdentity is the identity of a token given with --auth-token-env or
// --auth-token-file: the operator's own client. It reads every group and, as
// the token holder is the operator, writes in every group; destructive tools
// follow the server's destructive selector.
func OperatorIdentity(destructive bool) (Identity, error) {
	cat, err := contract.Load()
	if err != nil {
		return Identity{}, fmt.Errorf("load tool contract: %w", err)
	}
	groups := make([]string, 0, len(cat.Groups))
	for _, g := range cat.Groups {
		groups = append(groups, g.Name)
	}
	var destructiveGroups []string
	if destructive {
		destructiveGroups = groups
	}
	return NewIdentity(Principal{ID: OperatorPrincipalID, Kind: KindHumanMCP}, []string{AllGroups}, groups, destructiveGroups), nil
}

// OperatorPrincipalID names the principal of the --auth-token-* flags; a
// configured principal cannot use it.
const OperatorPrincipalID = "operator"

// GrantsDestructive reports whether any principal is granted a destructive
// group, which is when the HTTP server must register destructive tools so the
// grant has something to apply to (ADR-0003 section 2). An admin principal's
// tool-group keys are ignored, so they never register the tools.
func (c *Config) GrantsDestructive() bool {
	return slices.ContainsFunc(c.Principals, func(p PrincipalConfig) bool {
		return p.Kind != KindAdmin && len(p.DestructiveGroups) > 0
	})
}
