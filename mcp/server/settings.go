//go:build !noagent

package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// RouteSettings is the runtime settings route (GET reads, PUT writes), pinned
// here by mcp-agent-settings-api (the client contract takes the path from this
// change). It mounts only when EnableAgentSettings ran and accepts only an
// admin principal (MountOptions Kinds), so it exists on no path by default.
const RouteSettings = "/agent/settings"

// Bounds of a settings write, from the contract's settings-write schema.
const (
	maxSettingsPromptBytes = 32768
	maxSettingsURLBytes    = 2048
	maxSettingsModelBytes  = 256
)

// AgentSettingsAPIConfig is the input of EnableAgentSettings: the startup
// configuration a runtime write extends, and the flag overrides that keep
// winning over it (flags > overlay > config file, ADR-0009 section 7).
type AgentSettingsAPIConfig struct {
	// Over are the --llm-* flags of this run. A write never replaces a value a
	// flag sets.
	Over LLMOverrides
	// Cfg is the --mcp-config file; its llm and agent.loop sections are the
	// base a write merges into.
	Cfg *Config
	// Dir is the settings directory (--settings-dir, else agent.settings.dir).
	// Empty: writes apply to the running process but are not persisted. The
	// overlay and the key file live here, both mode 0600.
	Dir string
	// Overlay is the persisted writes loaded from Dir at startup by
	// LoadSettingsOverlay (nil: none). Its present fields shadow Cfg.
	Overlay *SettingsOverlay
	// Resolved is the backend startup resolved from Cfg, Overlay and Over, nil
	// when no backend is configured. It is the state the first GET reports.
	Resolved *AgentSettings
}

// llmValues are the writable backend fields (the llm section as the config
// file plus applied writes left it). The flag overrides are not part of them;
// they are applied at every resolve.
type llmValues struct {
	baseURL string
	model   string
	key     *SecretRef
	timeout string
}

// settingsState is the state behind the settings route: the writable values,
// the effective resolved backend, the persisted overlay and its revision. One
// mutex serializes reads and writes, so an If-Match check and the apply it
// guards cannot interleave with another write.
type settingsState struct {
	mu            sync.Mutex
	base          llmValues
	prompt        string
	loop          LoopConfig
	over          LLMOverrides
	acceptSecrets bool
	dir           string
	overlay       *SettingsOverlay
	current       *AgentSettings
	revision      string
	// revisionAtomic mirrors revision for the change hook, which runs inside
	// ApplyAgentSettings while the write that triggered it holds st.mu.
	revisionAtomic atomic.Pointer[string]
	// events is the change stream's broadcaster (RouteSettingsEvents).
	events *settingsEvents
}

// EnableAgentSettings turns the runtime settings route on. The CLI calls it
// only when an admin principal is configured and agent.settings.enabled is not
// false; without it the route is a plain 404 and nothing is written or dialed.
// Call it before NewHTTPServer; the route is mounted by NewHTTPServer and is
// therefore never served over stdio.
func (s *Server) EnableAgentSettings(c AgentSettingsAPIConfig) error {
	st := &settingsState{over: c.Over, acceptSecrets: true, dir: c.Dir, overlay: c.Overlay, current: c.Resolved}
	if st.overlay == nil {
		st.overlay = &SettingsOverlay{}
	}
	if c.Cfg != nil {
		// The overlay shadows the config file's llm section; the flags are not
		// part of base and win at every resolve.
		merged := c.Cfg.WithSettingsOverlay(c.Overlay)
		st.loop = merged.Agent.Loop
		st.acceptSecrets = c.Cfg.Agent.SettingsAcceptSecrets()
		if merged.LLM != nil {
			st.base = llmValues{
				baseURL: merged.LLM.BaseURL,
				model:   merged.LLM.Model,
				key:     merged.LLM.APIKey,
				timeout: merged.LLM.Timeout,
			}
		}
	}
	// The system prompt has no config key: it comes from the overlay (which a
	// Resolved built by ResolveAgentWithOverlay already carries).
	switch {
	case c.Overlay != nil && c.Overlay.SystemPrompt != nil:
		st.prompt = *c.Overlay.SystemPrompt
	case c.Resolved != nil:
		st.prompt = c.Resolved.SystemPrompt
	}
	st.events = newSettingsEvents()
	st.setRevision(revisionOf(st.current, st.prompt, st.dir))
	s.agentMu.Lock()
	s.settingsExt = st
	s.agentMu.Unlock()
	// The change stream is fed by the hot-reload hook: it runs once after every
	// applied write and carries no payload, so it re-reads the revision's
	// lock-free mirror.
	s.OnAgentChange(func() { st.events.publish(st.currentRevision()) })
	return nil
}

func init() { RegisterRouteProvider(mountAgentSettings) }

// mountAgentSettings mounts the settings route when EnableAgentSettings ran,
// scoped to the admin kind: no other principal kind reaches it, and it reaches
// no other route (ADR-0009 section 3).
func mountAgentSettings(s *Server, m RouteMounter) error {
	s.agentMu.RLock()
	st, _ := s.settingsExt.(*settingsState)
	s.agentMu.RUnlock()
	if st == nil {
		return nil
	}
	return m.Mount(RouteSettings, &settingsHandler{srv: s, st: st, log: s.log}, MountOptions{Kinds: []string{KindAdmin}})
}

// resolve builds the effective backend the startup path would build from the
// same values: it runs the candidate through Config.ResolveAgent with the run's
// flag overrides, so a write is validated by exactly the rules startup uses and
// a flag always wins over a written value. The system prompt has no config key;
// it is attached after the resolve.
func (st *settingsState) resolve() (*AgentSettings, error) {
	cfg := &Config{Agent: AgentConfig{Loop: st.loop}}
	if st.base.baseURL != "" || st.base.model != "" || st.base.key != nil || st.base.timeout != "" {
		cfg.LLM = &LLMConfig{BaseURL: st.base.baseURL, Model: st.base.model, APIKey: st.base.key, Timeout: st.base.timeout}
	}
	set, err := cfg.ResolveAgent(st.over)
	if err != nil {
		return nil, err
	}
	if set != nil {
		set.SystemPrompt = st.prompt
	}
	return set, nil
}

// keyStatus is the read shape's api_key for the current backend.
func (st *settingsState) keyStatus() apiKeyStatusBody {
	return keyStatusOf(st.current, st.dir)
}

// keyFilePath is where a key written through the API is stored.
func (st *settingsState) keyFilePath() string {
	if st.dir == "" {
		return ""
	}
	return filepath.Join(st.dir, settingsKeyFileName)
}

// keyStatusOf is the read shape's api_key: presence and provenance of the
// effective key reference (flag overrides included), never material. A
// reference that does not resolve (an unset variable, an absent file) reports
// set=false, so a missing key file fails nothing (CRITICAL-2).
func keyStatusOf(set *AgentSettings, dir string) apiKeyStatusBody {
	if set == nil || set.APIKey == nil {
		return apiKeyStatusBody{}
	}
	ref := set.APIKey
	if _, err := ref.Read(); err != nil {
		return apiKeyStatusBody{}
	}
	if ref.Env != "" {
		return apiKeyStatusBody{Set: true, Source: "environment"}
	}
	if dir != "" && ref.File == filepath.Join(dir, settingsKeyFileName) {
		return apiKeyStatusBody{Set: true, Source: "settings_api"}
	}
	return apiKeyStatusBody{Set: true, Source: "file"}
}

// revisionOf hashes the effective, secret-free settings: any change to them
// changes the revision, and no key material feeds the hash (only whether a key
// resolves and where it comes from).
func revisionOf(set *AgentSettings, prompt, dir string) string {
	doc := struct {
		BaseURL   string     `json:"base_url,omitempty"`
		Model     string     `json:"model,omitempty"`
		KeySet    bool       `json:"key_set"`
		KeySource string     `json:"key_source,omitempty"`
		Timeout   string     `json:"timeout,omitempty"`
		Limits    LoopLimits `json:"limits"`
		Prompt    string     `json:"prompt"`
	}{Prompt: prompt}
	if set != nil {
		doc.BaseURL, doc.Model, doc.Timeout, doc.Limits = set.BaseURL, set.Model, set.Timeout.String(), set.Limits.withDefaults()
		if ks := keyStatusOf(set, dir); ks.Set {
			doc.KeySet, doc.KeySource = true, ks.Source
		}
	}
	b, _ := json.Marshal(doc)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:16])
}

// setRevision stores the revision and its lock-free mirror. Call with the
// state lock held (or during construction).
func (st *settingsState) setRevision(revision string) {
	st.revision = revision
	st.revisionAtomic.Store(&revision)
}

// currentRevision reads the revision without the state lock: the change hook
// and the change stream's first event. EnableAgentSettings stores it before
// the route can mount, so the pointer is never nil here.
func (st *settingsState) currentRevision() string {
	return *st.revisionAtomic.Load()
}

// Wire shapes of the settings route, matching the contract's
// settings-read/settings-write/settings-error schemas. No field of a response
// can carry key material.
type settingsReadBody struct {
	Revision     string               `json:"revision"`
	Backend      *backendReadBody     `json:"backend"`
	SystemPrompt systemPromptReadBody `json:"system_prompt"`
	// KeyNotCarried is set only in the response to a PUT whose backend move
	// withheld a key stored through this API (ADR-0012).
	KeyNotCarried *keyNotCarriedBody `json:"key_not_carried,omitempty"`
}

type keyNotCarriedBody struct {
	Reason  string `json:"reason"`
	Cleared bool   `json:"cleared"`
}

type backendReadBody struct {
	Kind    string           `json:"kind"`
	BaseURL string           `json:"base_url"`
	Model   string           `json:"model"`
	APIKey  apiKeyStatusBody `json:"api_key"`
}

type apiKeyStatusBody struct {
	Set    bool   `json:"set"`
	Source string `json:"source,omitempty"`
}

type systemPromptReadBody struct {
	SafetyPrefix string `json:"safety_prefix"`
	Custom       string `json:"custom"`
}

type settingsWriteBody struct {
	Backend      *backendWriteBody `json:"backend"`
	SystemPrompt *string           `json:"system_prompt"`
	// EgressConfirmation is the D4 typed host confirmation: a write that moves
	// the model backend to a host the calling principal's egress_ack does not
	// cover must name exactly that host here, and extends the egress_ack with
	// that host only. Anything else is refused with
	// egress_confirmation_required.
	EgressConfirmation *struct {
		Host string `json:"host"`
	} `json:"egress_confirmation"`
	Schema string `json:"$schema"`
}

type backendWriteBody struct {
	Kind    string          `json:"kind"`
	BaseURL *string         `json:"base_url"`
	Model   *string         `json:"model"`
	APIKey  json.RawMessage `json:"api_key"`
}

type settingsErrorBody struct {
	Error settingsErrorDetail `json:"error"`
}

type settingsErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// settingsHandler serves GET and PUT on the settings route.
type settingsHandler struct {
	srv *Server
	st  *settingsState
	log *slog.Logger
}

func (h *settingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The listener maps the subtree here; the exact paths below are all that
	// exists.
	switch r.URL.Path {
	case RouteSettings:
		switch r.Method {
		case http.MethodGet:
			h.get(w)
		case http.MethodPut:
			h.put(w, r)
		default:
			w.Header().Set("Allow", "GET, PUT")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case RouteSettingsTestConnection:
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.testConnection(w, r)
	case RouteSettingsModels:
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.listModels(w, r)
	case RouteSettingsEvents:
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.events(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *settingsHandler) get(w http.ResponseWriter) {
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
	h.writeRead(w)
}

// writeRead answers with the current read shape and its ETag. Call with the
// state lock held.
func (h *settingsHandler) writeRead(w http.ResponseWriter) {
	h.writeReadWithNotice(w, nil)
}

// writeReadWithNotice answers with the read shape plus, for a PUT, the
// key_not_carried notice.
func (h *settingsHandler) writeReadWithNotice(w http.ResponseWriter, notice *keyNotCarriedBody) {
	st := h.st
	body := settingsReadBody{
		KeyNotCarried: notice,
		Revision:      st.revision,
		SystemPrompt: systemPromptReadBody{
			SafetyPrefix: systemPromptSafetyPrefix,
			Custom:       st.prompt,
		},
	}
	if st.current != nil {
		body.Backend = &backendReadBody{
			Kind:    "openai",
			BaseURL: st.current.BaseURL,
			Model:   st.current.Model,
			APIKey:  st.keyStatus(),
		}
	}
	writeSettingsJSON(w, http.StatusOK, st.revision, body)
}

func (h *settingsHandler) put(w http.ResponseWriter, r *http.Request) {
	var wr settingsWriteBody
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&wr); err != nil {
		http.Error(w, "the settings write is not a valid settings document", http.StatusBadRequest)
		return
	}
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
	if !matchRevision(r.Header.Get("If-Match"), h.st.revision) {
		h.writeError(w, http.StatusPreconditionFailed, "revision_conflict",
			"If-Match must name the current settings revision; read the settings and retry",
			map[string]string{"current_revision": h.st.revision})
		return
	}
	h.apply(w, r, wr)
}

// callerIdentity returns the authenticated identity of a settings request. The
// route's kind gate guarantees an admin principal; a missing identity leaves
// the egress check below with an empty egress_ack, which fails closed.
func callerIdentity(r *http.Request) Identity {
	info := auth.TokenInfoFromContext(r.Context())
	if info == nil {
		return Identity{}
	}
	id, _ := info.Extra[identityExtraKey].(Identity)
	return id
}

// effectiveAck is the calling principal's egress_ack as the settings API knows
// it: the configured list plus every host a confirmed write added (D4), which
// the overlay persists. Call with the state lock held.
func (st *settingsState) effectiveAck(id Identity) []string {
	return append(slices.Clone(id.EgressAck), st.overlay.EgressAck[id.ID]...)
}

// matchRevision compares an If-Match header with the current revision. The
// header must name exactly the current revision; a missing header, a list, a
// weak or wildcard value and a stale revision all fail.
func matchRevision(ifMatch, current string) bool {
	got := strings.Trim(strings.TrimSpace(ifMatch), `"`)
	return got != "" && got == current
}

// apply validates and installs a write, or answers with the failure. On any
// error the running backend, the stored values and the persisted files are
// untouched. Call with the state lock held.
func (h *settingsHandler) apply(w http.ResponseWriter, r *http.Request, wr settingsWriteBody) {
	st := h.st
	next := st.base
	prompt := st.prompt
	var fields []string
	// keySetTo is the key material to store in the settings directory; keyClear
	// is a write that removes the stored key. Both stay out of every response
	// and log line.
	var keySetTo string
	var keyClear bool
	if b := wr.Backend; b != nil {
		switch b.Kind {
		case "", "openai":
		case "uar":
			http.Error(w, "backend kind uar is not served by this build", http.StatusBadRequest)
			return
		default:
			http.Error(w, "backend kind must be openai or uar", http.StatusBadRequest)
			return
		}
		if len(b.APIKey) > 0 {
			// The key field is write-only and stored as a file; refuse before
			// anything else so no secret reaches the apply path.
			if !st.acceptSecrets {
				h.writeError(w, http.StatusForbidden, "secrets_not_accepted",
					"this server does not accept secret fields (agent.settings.accept_secrets is false); set the key with llm.api_key or the --llm-api-key-* flags", nil)
				return
			}
			if st.dir == "" {
				http.Error(w, "api_key is stored as a file in the settings directory, and no settings directory is configured (agent.settings.dir, --settings-dir)", http.StatusBadRequest)
				return
			}
			if bytes.Equal(bytes.TrimSpace(b.APIKey), []byte("null")) {
				next.key = nil
				keyClear = true
			} else {
				var key string
				if err := json.Unmarshal(b.APIKey, &key); err != nil || key == "" || len(key) > maxSettingsKeyBytes {
					http.Error(w, "backend.api_key must be a key string or null to clear the stored key", http.StatusBadRequest)
					return
				}
				next.key = &SecretRef{File: st.keyFilePath()}
				keySetTo = key
			}
			fields = append(fields, "api_key")
		}
		if b.BaseURL != nil {
			if len(*b.BaseURL) > maxSettingsURLBytes || strings.ContainsAny(*b.BaseURL, " \t\r\n") {
				http.Error(w, "backend.base_url is not a valid http(s) URL", http.StatusBadRequest)
				return
			}
			next.baseURL = *b.BaseURL
			fields = append(fields, "base_url")
		}
		if b.Model != nil {
			if *b.Model == "" || len(*b.Model) > maxSettingsModelBytes {
				http.Error(w, "backend.model must be a non-empty model name", http.StatusBadRequest)
				return
			}
			next.model = *b.Model
			fields = append(fields, "model")
		}
	}
	if wr.SystemPrompt != nil {
		if len(*wr.SystemPrompt) > maxSettingsPromptBytes {
			http.Error(w, "system_prompt is too large", http.StatusBadRequest)
			return
		}
		prompt = *wr.SystemPrompt
		fields = append(fields, "system_prompt")
	}

	// Validate the candidate with the same function startup uses: the written
	// values as the config base, the run's flags still winning over them.
	cand := settingsState{base: next, prompt: prompt, loop: st.loop, over: st.over}
	set, err := cand.resolve()
	if err != nil {
		// ResolveAgent errors name the offending key, never a value.
		http.Error(w, "the settings are not valid: "+err.Error(), http.StatusBadRequest)
		return
	}
	if set == nil && st.current != nil {
		http.Error(w, "a settings write cannot turn the agent off; restart without a backend instead", http.StatusBadRequest)
		return
	}

	// ADR-0012: a change of backend scheme, host or port does not carry a key
	// stored through this API. Unless the same write supplies api_key (a new key
	// or null), the stored key is cleared; a key supplied by reference (file or
	// environment variable) is the operator's and stays as configured.
	var notice *keyNotCarriedBody
	carriesKey := wr.Backend != nil && len(wr.Backend.APIKey) > 0
	if !carriesKey && set != nil && st.current != nil && backendMoved(st.current.BaseURL, set.BaseURL) &&
		st.dir != "" && st.current.APIKey != nil && st.current.APIKey.File == st.keyFilePath() {
		wasSet := keyStatusOf(st.current, st.dir).Set
		next.key = nil
		keyClear = true
		cand.base = next
		if set, err = cand.resolve(); err != nil {
			http.Error(w, "the settings are not valid: "+err.Error(), http.StatusBadRequest)
			return
		}
		if wasSet {
			notice = &keyNotCarriedBody{Reason: "host_changed", Cleared: true}
		}
	}

	// D4: a write that moves the effective model backend to a host the calling
	// principal's egress_ack does not cover needs the typed host confirmation,
	// and extends that egress_ack with exactly that host and no other. The
	// hosts compared are the resolved (flag-overridden) ones: a write a flag
	// overrides changes no egress destination and needs no confirmation.
	id := callerIdentity(r)
	var extendHost string
	var newHost, oldHost string
	if set != nil {
		newHost = HostFromURL(set.BaseURL)
	}
	if st.current != nil {
		oldHost = HostFromURL(st.current.BaseURL)
	}
	hostChanged := newHost != "" && !sameHostPort(newHost, oldHost)
	switch {
	case wr.EgressConfirmation != nil && !hostChanged:
		h.writeError(w, http.StatusConflict, "egress_confirmation_required",
			"egress_confirmation may name only the newly chosen model host, and this write changes no model host", nil)
		return
	case wr.EgressConfirmation != nil && !sameHostPort(wr.EgressConfirmation.Host, newHost):
		h.writeError(w, http.StatusConflict, "egress_confirmation_required",
			"egress_confirmation.host must be exactly the newly chosen model host "+newHost+"; no other host can be acknowledged through a settings write",
			map[string]string{"host": newHost})
		return
	case hostChanged && !AckCovers(st.effectiveAck(id), newHost):
		// id.ID empty means the request carried no authenticated identity (the
		// kind gate makes that unreachable today); the extension could not be
		// attributed to a principal, so it cannot be confirmed.
		if wr.EgressConfirmation == nil || id.ID == "" {
			msg := "the write moves the model backend to " + newHost + ", which the calling principal's egress_ack does not cover; " +
				"resend with egress_confirmation {\"host\": \"" + newHost + "\"} to acknowledge that node content will be sent to that host"
			if referencedKey(set, st.keyFilePath()) {
				// Operator ruling 2026-10-15: a key supplied by reference follows
				// a host move, so the confirmation names it.
				msg += "; the configured model key (supplied by an environment variable, file or flag, not stored through this API) " +
					"will be sent to that host too"
			}
			h.writeError(w, http.StatusConflict, "egress_confirmation_required", msg, map[string]string{"host": newHost})
			return
		}
		extendHost = newHost
		fields = append(fields, "egress_ack")
	}

	// Persist before applying: the running backend must never reference a key
	// file that does not exist, and a restart must never load an overlay whose
	// write did not apply. The key file first, then the overlay that
	// references it; a clear removes the file after the overlay records it.
	nextOverlay := st.overlay.withWrite(wr, keySetTo != "", keyClear, st.keyFilePath(), id.ID, extendHost)
	// The prior files, so a failed apply puts them back byte for byte.
	var undo persistedUndo
	if st.dir != "" {
		undo = snapshotPersisted(st.dir)
		if keySetTo != "" {
			if err := writeFile0600(st.dir, settingsKeyFileName, []byte(keySetTo)); err != nil {
				h.log.Warn("the settings key file could not be written", "error", err)
				http.Error(w, "the settings could not be persisted: the key file could not be written", http.StatusInternalServerError)
				return
			}
		}
		if err := nextOverlay.save(st.dir); err != nil {
			undo.restore(h.log, st.dir)
			h.log.Warn("the settings overlay could not be written", "error", err)
			http.Error(w, "the settings could not be persisted: the overlay could not be written", http.StatusInternalServerError)
			return
		}
	}

	// The change hook runs inside ApplyAgentSettings, before the state below is
	// stored, so the new revision must be visible first; a failed apply restores
	// the old one and publishes nothing.
	oldRevision := st.revision
	st.setRevision(revisionOf(set, prompt, st.dir))
	if set != nil {
		if err := h.srv.ApplyAgentSettings(set); err != nil {
			st.setRevision(oldRevision)
			undo.restore(h.log, st.dir)
			http.Error(w, "the settings did not apply: "+err.Error(), http.StatusInternalServerError)
			return
		}
		h.removeClearedKeyFile(st, keyClear)
		if set.APIKey != nil {
			if _, err := set.APIKey.Read(); err != nil {
				// CRITICAL-2: an unreadable key reference never fails a write;
				// the backend runs without a key and the read reports
				// api_key.set=false.
				h.log.Warn("the configured LLM API key does not resolve; the backend sends no key until the reference resolves", "error", err)
			}
		}
	} else {
		// A write that installs no backend (a prompt-only write with none
		// configured) never runs ApplyAgentSettings, so the hook does not fire;
		// publish the change here.
		h.removeClearedKeyFile(st, keyClear)
		st.events.publish(st.revision)
	}
	st.base, st.prompt, st.current, st.overlay = next, prompt, set, nextOverlay
	// The revision and the changed field names are safe to log; values never are.
	h.log.Debug("agent settings applied", "revision", st.revision, "fields", strings.Join(fields, ","))
	h.writeReadWithNotice(w, notice)
}

// persistedUndo holds the overlay and key files as they were before a write,
// so a write that fails after persisting leaves the directory as it found it.
type persistedUndo struct {
	files [2]undoFile
}

type undoFile struct {
	name    string
	data    []byte
	existed bool
}

// snapshotPersisted records the overlay and key files of dir. A file that
// cannot be read is treated as absent: the restore then removes whatever the
// write created, which is what an absent file asks for.
func snapshotPersisted(dir string) persistedUndo {
	var u persistedUndo
	for i, name := range [...]string{settingsOverlayFileName, settingsKeyFileName} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		u.files[i] = undoFile{name: name, data: data, existed: err == nil}
	}
	return u
}

// restore puts the snapshotted files back and logs, without failing, any file
// it cannot restore: the caller is already answering with the write's error.
func (u persistedUndo) restore(log *slog.Logger, dir string) {
	if dir == "" {
		return
	}
	for _, f := range u.files {
		if f.name == "" {
			continue
		}
		var err error
		if f.existed {
			err = writeFile0600(dir, f.name, f.data)
		} else if err = os.Remove(filepath.Join(dir, f.name)); errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
		if err != nil {
			log.Warn("the previous settings file could not be restored", "file", f.name, "error", err)
		}
	}
}

// removeClearedKeyFile deletes the stored key file once a write that cleared
// the key has applied. It runs only after a successful ApplyAgentSettings: the
// running backend still reads the file until the new settings are in place, so
// a failed apply leaves the old backend keyed (the contract's failed-apply
// rule). A leftover file is unreferenced once the overlay records the clear and
// is overwritten by the next set, so a removal failure does not fail the write.
func (h *settingsHandler) removeClearedKeyFile(st *settingsState, cleared bool) {
	if !cleared || st.dir == "" {
		return
	}
	if err := os.Remove(st.keyFilePath()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		h.log.Warn("the old settings key file could not be removed", "error", err)
	}
}

// referencedKey reports whether the resolved backend carries a key supplied by
// reference (environment variable, file or flag) rather than the key file this
// API stores. Such a key follows a backend move (operator ruling 2026-10-15).
func referencedKey(set *AgentSettings, apiKeyFile string) bool {
	return set != nil && set.APIKey != nil && (apiKeyFile == "" || set.APIKey.File != apiKeyFile)
}

// backendMoved reports whether two backend base URLs differ in scheme, host or
// port (ADR-0012); a change of path alone is not a move. Default ports are
// filled in so https://h and https://h:443 are the same endpoint. An
// unparsable URL counts as moved, which withholds rather than carries a key.
func backendMoved(oldURL, newURL string) bool {
	a, errA := url.Parse(oldURL)
	b, errB := url.Parse(newURL)
	if errA != nil || errB != nil {
		return true
	}
	return endpointOf(a) != endpointOf(b)
}

func endpointOf(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	host, port := normalizeHost(u.Host)
	if port == "" {
		switch scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	return scheme + "://" + net.JoinHostPort(host, port)
}

// writeError answers with the contract's settings error shape.
func (h *settingsHandler) writeError(w http.ResponseWriter, status int, code, message string, details any) {
	var body settingsErrorBody
	body.Error = settingsErrorDetail{Code: code, Message: message, Details: details}
	writeSettingsJSON(w, status, "", body)
}

func writeSettingsJSON(w http.ResponseWriter, status int, revision string, body any) {
	hd := w.Header()
	hd.Set("Content-Type", "application/json")
	if revision != "" {
		hd.Set("ETag", `"`+revision+`"`)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
