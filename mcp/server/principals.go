package server

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// Values of a principal's check key.
const (
	// CheckVerify (default) fails closed when UAR settings are unreadable.
	CheckVerify = "verify"
	// CheckAssert records that the operator verified what the settings API
	// does not expose.
	CheckAssert = "assert"
)

// LLMConfig is the llm section: the OpenAI-compatible model backend of the
// agent loop. The section is absent by default, and then nothing is mounted
// and no model call is made. APIKey is a reference to an environment variable
// or a file, never the key.
type LLMConfig struct {
	BaseURL string     `json:"base_url"`
	Model   string     `json:"model"`
	APIKey  *SecretRef `json:"api_key"`
	// Timeout is the per-request limit as a duration string ("60s" by default).
	Timeout string `json:"timeout"`
}

// AgentConfig is the agent section: the loop limits, the AG-UI and A2A switches
// and the AG-UI browser origin allowlist.
type AgentConfig struct {
	// AGUI is agent.agui: serve POST /agui (off by default).
	AGUI bool `json:"agui"`
	// A2UI is agent.a2ui: emit A2UI surface messages on /agui (on by default
	// wherever /agui is on; nil means the default).
	A2UI *bool `json:"a2ui"`
	// A2A is agent.a2a: serve POST /a2a and the agent card (off by default).
	A2A bool `json:"a2a"`
	// OriginAllowlist is agent.origin_allowlist: page origins allowed to call
	// /agui from a browser (empty by default: none).
	OriginAllowlist []string   `json:"origin_allowlist"`
	Loop            LoopConfig `json:"loop"`
}

// A2UIEnabled reports whether /agui emits server-built A2UI surfaces
// (agent.a2ui, default true). It has no effect when agent.agui is false.
func (c AgentConfig) A2UIEnabled() bool { return c.A2UI == nil || *c.A2UI }

// LoopConfig is agent.loop: the limits every run enforces. Zero values take the
// contract defaults.
type LoopConfig struct {
	MaxSteps  int    `json:"max_steps"`
	MaxTokens int    `json:"max_tokens"`
	WallClock string `json:"wall_clock"`
}

// EmbeddingsConfig is the embeddings section. It is absent by default, and
// then no store is opened, no process is started and no embedder is contacted.
// Credentials are references, never values. The policy keys (auto, MIME
// allowlist, size cap, chunking) belong to the index lifecycle and are not
// read here.
type EmbeddingsConfig struct {
	// Mode is "embedded" or "remote".
	Mode     string `json:"mode"`
	Embedded *struct {
		SurrealBinary string `json:"surreal_binary"`
		DataDir       string `json:"data_dir"`
		MinVersion    string `json:"min_version"`
	} `json:"embedded"`
	Remote *struct {
		Endpoint   string     `json:"endpoint"`
		Namespace  string     `json:"namespace"`
		Database   string     `json:"database"`
		Credential *SecretRef `json:"credential"`
	} `json:"remote"`
	Embedder *struct {
		BaseURL    string     `json:"base_url"`
		Model      string     `json:"model"`
		Dimensions int        `json:"dimensions"`
		APIKey     *SecretRef `json:"api_key"`
	} `json:"embedder"`
	// Policy says what is indexed; absent means the contract defaults and no
	// automatic indexing.
	Policy *struct {
		Auto          bool     `json:"auto"`
		MIMEAllowlist []string `json:"mime_allowlist"`
		MaxBytes      int      `json:"max_bytes"`
		ChunkChars    int      `json:"chunk_chars"`
	} `json:"policy"`
}

// BackendHosts returns the hosts ipfs mcp itself would forward node content
// to: the llm endpoint, the embedder endpoint and a remote vector store. Each
// must be covered by the egress_ack of a principal whose content flows there.
func (c *Config) BackendHosts() []string {
	var hosts []string
	add := func(raw string) {
		if h := HostFromURL(raw); h != "" && !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	if c.LLM != nil {
		add(c.LLM.BaseURL)
	}
	if c.Embeddings != nil {
		if c.Embeddings.Embedder != nil {
			add(c.Embeddings.Embedder.BaseURL)
		}
		if c.Embeddings.Remote != nil {
			add(c.Embeddings.Remote.Endpoint)
		}
	}
	return hosts
}

// usesBackend reports whether a principal kind is served by ipfs mcp's own
// agent surface, so its content reaches the llm and embedder hosts. A
// human-mcp host and UAR bring their own model, which egress_ack and the UAR
// preflight cover instead.
func usesBackend(kind string) bool { return kind == KindUI || kind == KindA2A }

// configuredIdentity builds the identity of a configured principal: its
// grants, egress acknowledgement and assertions.
func (c *Config) configuredIdentity(p PrincipalConfig) Identity {
	id := NewIdentity(Principal{ID: p.Name, Kind: p.Kind}, p.Groups, p.WriteGroups, p.DestructiveGroups)
	id.Configured = true
	id.EgressAck = slices.Clone(p.EgressAck)
	id.ShellCapable = p.ShellCapable
	id.Check = p.Check
	if p.UAR != nil {
		u := *p.UAR
		id.UAR = &u
	}
	if usesBackend(p.Kind) {
		id.BackendHosts = c.BackendHosts()
	}
	return id
}

// validatePrincipals enforces the rules the JSON schema cannot express.
func (c *Config) validatePrincipals() error {
	seen := make(map[string]struct{}, len(c.Principals))
	for i, p := range c.Principals {
		at := fmt.Sprintf("principals[%d] (%s)", i, p.Name)
		if p.Name == OperatorPrincipalID {
			return fmt.Errorf("%s: the name %q is reserved for the --auth-token-* operator", at, OperatorPrincipalID)
		}
		if _, dup := seen[p.Name]; dup {
			return fmt.Errorf("%s: the name is used twice", at)
		}
		seen[p.Name] = struct{}{}
		if p.Kind == KindUAR {
			if p.UAR == nil || p.UAR.CardURL == "" {
				return fmt.Errorf("%s: a principal of kind uar needs uar.card_url, the address the check-uar preflight may contact", at)
			}
			if u, err := url.Parse(p.UAR.CardURL); err != nil || u.Host == "" || strings.ContainsAny(u.Host, " ") {
				return fmt.Errorf("%s: uar.card_url is not a valid URL", at)
			}
			continue
		}
		if p.UAR != nil {
			return fmt.Errorf("%s: the uar block is only valid for kind uar", at)
		}
		if p.Check == CheckAssert {
			return fmt.Errorf("%s: check: assert is only valid for kind uar", at)
		}
	}
	return nil
}
