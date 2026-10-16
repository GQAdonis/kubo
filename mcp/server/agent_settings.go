//go:build !noagent

package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Loop limits when agent.loop leaves a value out. They are deliberately small:
// a run is a short conversation with a handful of tool calls, not a
// background job.
const (
	DefaultMaxSteps  = 8
	DefaultMaxTokens = 32768
	DefaultWallClock = 120 * time.Second
	// DefaultLLMTimeout is the per-request model timeout (llm.timeout).
	DefaultLLMTimeout = 60 * time.Second
)

// envNamePattern is the contract's SecretRef env pattern. A value that does not
// match it (a key such as "sk-abc") cannot be an environment variable name, so
// the flag's value is refused without being echoed.
var envNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)

// secret holds a credential read from the environment or a file. It prints as
// "<redacted>" in every formatting verb, in JSON and in slog, so a log line or
// an error built from a struct that contains it cannot leak it.
type secret string

func (secret) String() string               { return "<redacted>" }
func (secret) GoString() string             { return `"<redacted>"` }
func (secret) MarshalJSON() ([]byte, error) { return []byte(`"<redacted>"`), nil }
func (secret) LogValue() slog.Value         { return slog.StringValue("<redacted>") }

// reveal returns the credential for the one place that sends it.
func (s secret) reveal() string { return string(s) }

// LoopLimits bound one run (and one resume) of the agent loop. A zero field
// takes its default.
type LoopLimits struct {
	// MaxSteps is the number of model round trips.
	MaxSteps int
	// MaxTokens is the token budget across the run's model calls, counted from
	// the backend's usage report, or estimated from the bytes exchanged when
	// the backend reports none.
	MaxTokens int
	// WallClock is the time budget of the whole run.
	WallClock time.Duration
}

func (l LoopLimits) withDefaults() LoopLimits {
	if l.MaxSteps <= 0 {
		l.MaxSteps = DefaultMaxSteps
	}
	if l.MaxTokens <= 0 {
		l.MaxTokens = DefaultMaxTokens
	}
	if l.WallClock <= 0 {
		l.WallClock = DefaultWallClock
	}
	return l
}

// AgentSettings configure the agent loop and its model backend. There is no
// default endpoint or model: ResolveAgent returns nil settings, and nothing is
// mounted or dialed, until an operator names both.
type AgentSettings struct {
	// BaseURL is the OpenAI-compatible base, for example http://127.0.0.1:5051/v1;
	// "/chat/completions" is appended.
	BaseURL string
	Model   string
	// APIKey names where the key is read from (environment variable or
	// file); nil sends no Authorization header (local servers).
	APIKey *SecretRef
	// Timeout is the per-request model timeout.
	Timeout time.Duration
	Limits  LoopLimits

	// SystemPrompt is the operator-configured part of the system prompt (the
	// settings write's system_prompt). It only ever extends the fixed safety
	// prefix, never replaces it; empty keeps the default instructions.
	SystemPrompt string

	// HTTPClient replaces the default client; tests use it to observe dialing.
	HTTPClient *http.Client

	// Kind is BackendKindOpenAI (also the empty value) or BackendKindUAR. For
	// uar, BaseURL is the UAR instance, APIKey the credential kubo presents to
	// it, Model the agent's default model (optional) and UAR the rest.
	Kind string
	UAR  *UARSettings
}

// IsUAR reports whether the backend is a UAR agent.
func (a *AgentSettings) IsUAR() bool { return a != nil && a.Kind == BackendKindUAR }

// LLMOverrides are the --llm-* flags. A non-empty value replaces the config
// file's.
type LLMOverrides struct {
	BaseURL string
	Model   string
	// KeyEnv (--llm-api-key-env) is the NAME of an environment variable, never
	// a key. KeyFile (--llm-api-key-file) is an absolute path.
	KeyEnv  string
	KeyFile string
}

func (o LLMOverrides) keyGiven() bool { return o.KeyEnv != "" || o.KeyFile != "" }

// ResolveAgent combines the llm and agent sections with the --llm-* flags.
// It returns nil settings when no backend is named: the agent is off. It
// refuses a literal key, a half-configured backend and a base URL that carries
// credentials; errors never echo a value that could be a secret.
func (c *Config) ResolveAgent(o LLMOverrides) (*AgentSettings, error) {
	var llm LLMConfig
	if c.LLM != nil {
		llm = *c.LLM
	}
	base := firstNonEmpty(o.BaseURL, llm.BaseURL)
	model := firstNonEmpty(o.Model, llm.Model)
	// The kind is validated before the "nothing configured" return, so an
	// invalid kind, or kind uar without a base URL, fails instead of silently
	// leaving the agent off.
	kind := llm.Kind
	switch kind {
	case "":
		kind = BackendKindOpenAI
	case BackendKindOpenAI, BackendKindUAR:
	default:
		return nil, errors.New("llm.kind must be openai or uar")
	}
	if kind == BackendKindUAR && base == "" {
		return nil, errors.New("llm.kind uar needs llm.base_url")
	}
	if base == "" && model == "" {
		if o.keyGiven() {
			return nil, errors.New("--llm-api-key-env or --llm-api-key-file is set but no model backend is configured; name --llm-base-url and --llm-model, or remove the key flag")
		}
		return nil, nil
	}
	switch {
	case base == "":
		return nil, errors.New("llm.model is set without llm.base_url (--llm-base-url): the agent has no default endpoint")
	case model == "" && kind == BackendKindOpenAI:
		return nil, errors.New("llm.base_url is set without llm.model (--llm-model): the agent has no default model")
	}
	if err := validateBackendURL(base); err != nil {
		return nil, err
	}

	key := llm.APIKey
	if o.keyGiven() {
		ref, err := keyRefFromFlags(o)
		if err != nil {
			return nil, err
		}
		key = &ref
	}
	if key != nil && strings.EqualFold(schemeOf(base), "http") && !IsLocalHost(HostFromURL(base)) {
		return nil, errors.New("refusing to send an API key over plain http to a non-loopback host; use https or a loopback backend")
	}

	timeout := DefaultLLMTimeout
	if llm.Timeout != "" {
		d, err := time.ParseDuration(llm.Timeout)
		if err != nil || d <= 0 {
			return nil, errors.New("llm.timeout is not a positive duration")
		}
		timeout = d
	}
	limits := LoopLimits{MaxSteps: c.Agent.Loop.MaxSteps, MaxTokens: c.Agent.Loop.MaxTokens}
	if w := c.Agent.Loop.WallClock; w != "" {
		d, err := time.ParseDuration(w)
		if err != nil || d <= 0 {
			return nil, errors.New("agent.loop.wall_clock is not a positive duration")
		}
		limits.WallClock = d
	}
	set := &AgentSettings{BaseURL: base, Model: model, APIKey: key, Timeout: timeout, Limits: limits.withDefaults(), Kind: kind}
	if kind == BackendKindUAR {
		u, err := c.resolveUAR(llm)
		if err != nil {
			return nil, err
		}
		if _, _, err := uarProviderModel(&AgentSettings{Model: model, UAR: u}); err != nil {
			return nil, fmt.Errorf("llm.kind uar: %w", err)
		}
		set.UAR = u
		// The run grant is the principal's bearer; it must not cross the
		// network in clear text either. Plain http is allowed to loopback or
		// to a host the operator listed in embeddings.insecure_http_hosts.
		if u.MCPCredential != nil && strings.EqualFold(schemeOf(base), "http") && !IsLocalHost(HostFromURL(base)) {
			var listed []string
			if c.Embeddings != nil {
				listed = c.Embeddings.InsecureHTTPHosts
			}
			if pu, perr := url.Parse(base); perr != nil || !insecureHostListed(pu, listed) {
				return nil, errors.New("refusing to send the llm.uar.mcp_principal grant over plain http to a non-loopback host; use https, a loopback backend, or list the host in embeddings.insecure_http_hosts")
			}
		}
	}
	return set, nil
}

func keyRefFromFlags(o LLMOverrides) (SecretRef, error) {
	if o.KeyEnv != "" && o.KeyFile != "" {
		return SecretRef{}, errors.New("use only one of --llm-api-key-env and --llm-api-key-file")
	}
	if o.KeyEnv != "" {
		if !envNamePattern.MatchString(o.KeyEnv) {
			return SecretRef{}, errors.New("--llm-api-key-env takes the NAME of an environment variable (upper case letters, digits and underscores), not a key; a literal key is never accepted")
		}
		return SecretRef{Env: o.KeyEnv}, nil
	}
	if !strings.HasPrefix(o.KeyFile, "/") && !isWindowsAbs(o.KeyFile) {
		return SecretRef{}, errors.New("--llm-api-key-file takes the absolute path of a file that holds the key, not a key")
	}
	return SecretRef{File: o.KeyFile}, nil
}

func isWindowsAbs(p string) bool {
	return len(p) > 2 && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

// validateBackendURL checks the shape of llm.base_url. The error does not
// repeat the value, which could contain credentials.
func validateBackendURL(raw string) error {
	u, err := url.Parse(raw)
	switch {
	case err != nil || u.Host == "":
		return errors.New("llm.base_url is not a valid http(s) URL")
	case u.Scheme != "http" && u.Scheme != "https":
		return errors.New("llm.base_url must use http or https")
	case u.User != nil:
		return errors.New("llm.base_url must not contain credentials; name the key with --llm-api-key-env or --llm-api-key-file")
	case u.RawQuery != "" || u.Fragment != "":
		return errors.New("llm.base_url must not contain a query or fragment (keys do not belong in URLs)")
	}
	return nil
}

func schemeOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Scheme
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// Describe is the one-line, secret-free summary ipfs mcp prints at startup.
func (a *AgentSettings) Describe() string {
	key := "no api key"
	if a.APIKey != nil {
		key = "api key from " + a.APIKey.where()
	}
	if a.IsUAR() {
		return fmt.Sprintf("agent backend UAR %s, agent %s, %s", HostFromURL(a.BaseURL), a.UAR.AgentID, key)
	}
	return fmt.Sprintf("agent backend %s, model %s, %s", HostFromURL(a.BaseURL), a.Model, key)
}

// where names the source of a credential for a startup line. It never reads
// the credential.
func (r SecretRef) where() string {
	if r.Env != "" {
		return "$" + r.Env
	}
	return r.File
}
