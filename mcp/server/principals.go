package server

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/ipfs/kubo/mcp/server/embeddings"
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
	// Settings is agent.settings: the runtime settings API switches.
	Settings SettingsConfig `json:"settings"`
}

// SettingsConfig is agent.settings. The settings route mounts only when an
// admin principal is configured and Enabled is not false (ADR-0009 section 3).
type SettingsConfig struct {
	// Enabled is agent.settings.enabled (nil means the default, true).
	Enabled *bool `json:"enabled"`
	// AcceptSecrets is agent.settings.accept_secrets: whether a settings write
	// may carry the write-only key field (nil means the default, true).
	AcceptSecrets *bool `json:"accept_secrets"`
	// Dir is agent.settings.dir: where the settings overlay lives (empty: no
	// overlay is read or written).
	Dir string `json:"dir"`
}

// SettingsEnabled reports whether the settings route may mount
// (agent.settings.enabled, default true). An admin principal is still
// required.
func (c AgentConfig) SettingsEnabled() bool { return c.Settings.Enabled == nil || *c.Settings.Enabled }

// SettingsAcceptSecrets reports whether the settings API accepts write-only
// secret fields (agent.settings.accept_secrets, default true).
func (c AgentConfig) SettingsAcceptSecrets() bool {
	return c.Settings.AcceptSecrets == nil || *c.Settings.AcceptSecrets
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
	// Extractor turns documents, code and data into Markdown through xberg; nil
	// means no extractor process or request exists.
	Extractor *ExtractorConfig `json:"extractor"`
	// Describer describes images (and, if listed, videos) through an
	// OpenAI-compatible chat model; nil means no description request exists.
	Describer *DescriberConfig `json:"describer"`
	// InsecureHTTPHosts are hosts (or host:port) that may be reached over plain
	// http by the remote extractor, the describer and a keyed embedder; empty
	// refuses plain http to every non-loopback host. egress_ack still applies.
	InsecureHTTPHosts []string `json:"insecure_http_hosts"`
	// Bulk holds the bulk indexing caps and its off switch (bulk.enabled).
	Bulk BulkConfig `json:"bulk"`
	// SearchRoute is search_route: the HTTP search routes' off switch.
	SearchRoute struct {
		Enabled *bool `json:"enabled"`
	} `json:"search_route"`
}

// DescriberConfig is embeddings.describer. Pointer and slice fields tell an
// absent key (the contract default) from an explicit zero or empty value.
type DescriberConfig struct {
	BaseURL string     `json:"base_url"`
	Model   string     `json:"model"`
	APIKey  *SecretRef `json:"api_key"`
	// Kinds nil is the default ["image"]; [] describes nothing.
	Kinds           []string `json:"kinds"`
	MaxOutputTokens int      `json:"max_output_tokens"`
	ImageMaxBytes   int      `json:"image_max_bytes"`
	VideoMaxBytes   int      `json:"video_max_bytes"`
	VideoFPS        float64  `json:"video_fps"`
	// MaxRequestsPerHour nil is the default 120; 0 disables the describer.
	MaxRequestsPerHour *int `json:"max_requests_per_hour"`
	TimeoutSeconds     int  `json:"timeout_seconds"`
}

// Enabled reports whether the describer can describe anything: a kind is
// listed and its hourly limit is not 0.
func (c *DescriberConfig) Enabled() bool {
	if c == nil || (c.MaxRequestsPerHour != nil && *c.MaxRequestsPerHour == 0) {
		return false
	}
	return c.Kinds == nil || len(c.Kinds) > 0
}

// Bulk defaults, the contract's defaults of embeddings.bulk.
const (
	DefaultBulkMFSRoot        = "/"
	DefaultBulkMaxFiles       = 1000
	DefaultBulkMaxBytes       = int64(1) << 30
	DefaultBulkFilesPerMinute = 30
)

// BulkConfig is embeddings.bulk. Zero values take the contract defaults.
type BulkConfig struct {
	// Enabled nil is true; false hides the three bulk tools.
	Enabled *bool `json:"enabled"`
	// Sources nil is ["pins","mfs"]; [] makes a run index nothing.
	Sources        []string `json:"sources"`
	MFSRoot        string   `json:"mfs_root"`
	MaxFilesPerRun int      `json:"max_files_per_run"`
	MaxBytesPerRun int64    `json:"max_bytes_per_run"`
	FilesPerMinute int      `json:"files_per_minute"`
}

// IsEnabled reports whether the bulk tools are offered (bulk.enabled, default
// true).
func (b BulkConfig) IsEnabled() bool { return b.Enabled == nil || *b.Enabled }

// EffectiveSources is the configured sources, or both when the key is absent.
func (b BulkConfig) EffectiveSources() []string {
	if b.Sources == nil {
		return []string{"pins", "mfs"}
	}
	return slices.Clone(b.Sources)
}

// EffectiveMFSRoot is the configured MFS root, or "/".
func (b BulkConfig) EffectiveMFSRoot() string {
	if b.MFSRoot == "" {
		return DefaultBulkMFSRoot
	}
	return b.MFSRoot
}

// Caps returns the configured caps with defaults filled in.
func (b BulkConfig) Caps() embeddings.RunCaps {
	c := embeddings.RunCaps{MaxFiles: b.MaxFilesPerRun, MaxBytes: b.MaxBytesPerRun, FilesPerMinute: b.FilesPerMinute}
	if c.MaxFiles == 0 {
		c.MaxFiles = DefaultBulkMaxFiles
	}
	if c.MaxBytes == 0 {
		c.MaxBytes = DefaultBulkMaxBytes
	}
	if c.FilesPerMinute == 0 {
		c.FilesPerMinute = DefaultBulkFilesPerMinute
	}
	return c
}

// SearchRouteEnabled reports whether the HTTP search routes may mount
// (embeddings.search_route.enabled, default true).
func (c *EmbeddingsConfig) SearchRouteEnabled() bool {
	return c != nil && (c.SearchRoute.Enabled == nil || *c.SearchRoute.Enabled)
}

// ExtractorConfig is embeddings.extractor.
type ExtractorConfig struct {
	// Mode is "embedded" or "remote".
	Mode     string `json:"mode"`
	Embedded *struct {
		XbergBinary         string `json:"xberg_binary"`
		CacheDir            string `json:"cache_dir"`
		MinVersion          string `json:"min_version"`
		AllowModelDownloads bool   `json:"allow_model_downloads"`
	} `json:"embedded"`
	Remote *struct {
		BaseURL    string     `json:"base_url"`
		Credential *SecretRef `json:"credential"`
	} `json:"remote"`
	TimeoutSeconds int `json:"timeout_seconds"`
	MaxBytes       int `json:"max_bytes"`
	// MaxTextBytes bounds the Markdown taken from one extraction; 0 is the
	// default (embeddings.DefaultExtractorMaxTextBytes).
	MaxTextBytes int  `json:"max_text_bytes"`
	OCR          bool `json:"ocr"`
	Transcribe   bool `json:"transcribe"`
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
		// An embedded extractor is a child on this machine; only a remote one
		// is a host content is sent to.
		if x := c.Embeddings.Extractor; x != nil && x.Remote != nil {
			add(x.Remote.BaseURL)
		}
		// The describer receives media bytes; a disabled one receives nothing.
		if d := c.Embeddings.Describer; d.Enabled() {
			add(d.BaseURL)
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
	if p.Kind == KindAdmin {
		// The tool-group keys grant an admin principal nothing (contract config
		// schema): its scope is the settings route only.
		id.Groups, id.WriteGroups, id.DestructiveGroups = nil, nil, nil
		id.Scopes = []string{ScopeRead}
	}
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
