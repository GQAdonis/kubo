package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/ipfs/kubo/mcp/contract"
	"github.com/ipfs/kubo/mcp/server/embeddings"
)

const (
	uarCardSuffix  = "/.well-known/agent-card.json"
	uarHTTPTimeout = 10 * time.Second
	uarMaxBody     = 1 << 20
)

// Verdicts of a UAR check.
const (
	// VerdictOK: nothing found; the principal keeps its grants.
	VerdictOK = "ok"
	// VerdictReadOnly: write and destructive grants are withdrawn.
	VerdictReadOnly = "read_only"
	// VerdictRefuse: the principal is disabled; its session is refused.
	VerdictRefuse = "refuse"
)

// Severities of a Finding.
const (
	SeverityInfo     = "info"
	SeverityReadOnly = "read_only"
	SeverityRefuse   = "refuse"
)

// Finding codes.
const (
	FindingCardUnreadable        = "card_unreadable"
	FindingVersionNotPublished   = "version_not_published_release"
	FindingVersionTooOld         = "version_too_old"
	FindingNoMinVersion          = "min_version_not_set"
	FindingAuthNotRequired       = "auth_not_required"
	FindingAuthProbeInconclusive = "auth_probe_inconclusive"
	FindingSettingsUnreadable    = "settings_unreadable"
	FindingSettingsAsserted      = "settings_asserted"
	FindingRemoteModel           = "remote_model_not_acknowledged"
	FindingRemoteEmbedder        = "remote_embedder_not_acknowledged"
	FindingGovernanceDisabled    = "governance_disabled"
	FindingShellToolEnabled      = "shell_tool_enabled"
	FindingNonKuboMCPServer      = "non_kubo_mcp_server"
	FindingInventoryUnreadable   = "tool_inventory_unreadable"
)

// UARConfig is the uar block of a kind uar principal.
type UARConfig struct {
	// CardURL is the operator-configured URL of the UAR agent card. It is the
	// only address check-uar contacts, together with the settings API under
	// the same base.
	CardURL string `json:"card_url"`
	// AdminCredential names where the admin token for the settings API is
	// read from. Nil means settings cannot be read.
	AdminCredential *SecretRef `json:"admin_credential"`
	// MinVersion overrides the minimum pinned in the contract
	// (agentic/uar-preflight.json).
	MinVersion *string `json:"min_version"`
}

// Finding is one result of a UAR check.
type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	// Host is the provider host involved, for the egress findings.
	Host string `json:"host,omitempty"`
}

// UARReport is the outcome of one check.
type UARReport struct {
	CardURL   string    `json:"card_url"`
	Version   string    `json:"version,omitempty"`
	Revision  string    `json:"revision,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
	Findings  []Finding `json:"findings"`
}

// Verdict is the strictest severity among the findings.
func (r UARReport) Verdict() string {
	verdict := VerdictOK
	for _, f := range r.Findings {
		switch f.Severity {
		case SeverityRefuse:
			return VerdictRefuse
		case SeverityReadOnly:
			verdict = VerdictReadOnly
		}
	}
	return verdict
}

// Restrictions turns the read_only findings into the restrictions recorded on
// the effective identity.
func (r UARReport) Restrictions() []Restriction {
	var out []Restriction
	for _, f := range r.Findings {
		if f.Severity == SeverityReadOnly {
			out = append(out, Restriction{Reason: "uar_" + f.Code, Message: f.Message})
		}
	}
	return out
}

// Summary is a one-line account of the findings that matter.
// Codes lists the codes of the findings that are not informational, without
// their messages. A message can name a configured or discovered endpoint, so
// startup output and logs carry the codes only.
func (r UARReport) Codes() string {
	var codes []string
	for _, f := range r.Findings {
		if f.Severity != SeverityInfo {
			codes = append(codes, f.Code)
		}
	}
	return strings.Join(codes, ", ")
}

func (r UARReport) Summary() string {
	var parts []string
	for _, f := range r.Findings {
		if f.Severity != SeverityInfo {
			parts = append(parts, f.Code+": "+f.Message)
		}
	}
	return strings.Join(parts, "; ")
}

// UARCheckInput is what one check needs. Everything in it comes from the
// operator: the principal's config or the check-uar command line.
type UARCheckInput struct {
	UAR UARConfig
	// Check is "verify" (default) or "assert". Assert waives only what
	// Kubo cannot read; whatever the settings API does return is still
	// checked.
	Check string
	// EgressAck is the principal's egress_ack.
	EgressAck []string
	// KuboListen is the address ipfs mcp serves HTTP on, used to tell UAR's
	// Kubo MCP server entry from other servers. Empty means the default.
	KuboListen string
}

// UARChecker runs the check-uar preflight. The zero value is not usable; use
// NewUARChecker.
type UARChecker struct {
	client *http.Client
	now    func() time.Time
	// pre holds the values the contract owns: the pinned minimum version, the
	// settings path and the recheck interval. preErr is set when the embedded
	// contract file cannot be read, which makes every check refuse.
	pre    contract.UARPreflight
	preErr error
}

// UARCheckerOption customizes a checker. Production code passes none: the
// pinned values come from the contract.
type UARCheckerOption func(*UARChecker)

// WithSettingsPath replaces the pinned settings path. It exists for tests of
// the settings reading, which the contract keeps null until a published UAR
// release documents an endpoint; there is no default path.
func WithSettingsPath(path string) UARCheckerOption {
	return func(c *UARChecker) { c.pre.SettingsPath = &path }
}

// NewUARChecker returns a checker. A nil client selects one with a short
// timeout, no proxy and no redirects, so a credential is only ever sent to
// the configured address. A nil clock selects time.Now. The minimum version,
// settings path and recheck interval come from contract.LoadUARPreflight.
func NewUARChecker(client *http.Client, now func() time.Time, opts ...UARCheckerOption) *UARChecker {
	if client == nil {
		client = &http.Client{
			Timeout:       uarHTTPTimeout,
			Transport:     embeddings.DirectTransport(),
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	if now == nil {
		now = time.Now
	}
	c := &UARChecker{client: client, now: now}
	pre, err := contract.LoadUARPreflight()
	if err != nil {
		c.preErr = err
	} else {
		c.pre = *pre
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// recheck is how long a verdict is reused. It is 0 (never reuse) when the
// contract cannot be read.
func (c *UARChecker) recheck() time.Duration {
	if c.preErr != nil {
		return 0
	}
	d, err := c.pre.Recheck()
	if err != nil {
		return 0
	}
	return d
}

// settingsPath is the pinned settings path, or "" when none is pinned.
func (c *UARChecker) settingsPath() string {
	if c.pre.SettingsPath == nil {
		return ""
	}
	return *c.pre.SettingsPath
}

// pinnedMinVersion is the contract's minimum UAR version, or "".
func (c *UARChecker) pinnedMinVersion() string {
	if c.pre.MinVersion == nil {
		return ""
	}
	return *c.pre.MinVersion
}

var publishedVersion = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

// parsePublishedVersion accepts only plain release versions. A pre-release or
// build suffix (1.4.0-rc1, 1.4.0+sha) is a development build, not a published
// release, and does not count against the minimum.
func parsePublishedVersion(s string) ([3]int, bool) {
	m := publishedVersion.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return [3]int{}, false
	}
	var v [3]int
	for i := range v {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return [3]int{}, false
		}
		v[i] = n
	}
	return v, true
}

func versionLess(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// uarSettings is the part of the settings that the check reads. It has
// already been validated against the contract schema, so the required fields
// are present; pointers tell a missing optional field from a false one.
type uarSettings struct {
	Revision          uarRevision     `json:"revision"`
	LLM               *uarProvider    `json:"llm"`
	KnowledgeEmbedder *uarProvider    `json:"knowledge_base_embedder"`
	MemoryEmbedder    *uarProvider    `json:"memory_embedder"`
	Governance        *uarGovernance  `json:"governance"`
	NativeTools       *uarNativeTools `json:"native_tools"`
	MCPServers        *[]uarMCPServer `json:"mcp_servers"`
}

// uarRevision is the settings revision: a string or a non-negative integer,
// kept as text.
type uarRevision string

// UnmarshalJSON implements json.Unmarshaler.
func (r *uarRevision) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*r = uarRevision(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("revision is neither a string nor an integer: %w", err)
	}
	*r = uarRevision(n.String())
	return nil
}

type uarProvider struct {
	Model        string `json:"model"`
	ProviderHost string `json:"provider_host"`
}

type uarGovernance struct {
	Enabled *bool `json:"enabled"`
}

type uarNativeTools struct {
	TerminalExecEnabled *bool    `json:"terminal_exec_enabled"`
	ShellTools          []string `json:"shell_tools"`
}

type uarMCPServer struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
	URL       string `json:"url"`
	Command   string `json:"command"`
}

// Check runs the preflight. It always returns a report; any failure to learn
// something is a finding, so the caller never has to guess whether a missing
// finding means "fine" or "not checked".
func (c *UARChecker) Check(ctx context.Context, in UARCheckInput) UARReport {
	rep := UARReport{CardURL: in.UAR.CardURL, CheckedAt: c.now().UTC()}
	add := func(code, severity, msg string) {
		rep.Findings = append(rep.Findings, Finding{Code: code, Severity: severity, Message: msg})
	}
	if c.preErr != nil {
		add(FindingSettingsUnreadable, SeverityRefuse, "the check-uar contract values cannot be read: "+c.preErr.Error())
		return rep
	}
	base, err := uarBase(in.UAR.CardURL)
	if err != nil {
		add(FindingCardUnreadable, SeverityRefuse, err.Error())
		return rep
	}

	version, err := c.fetchVersion(ctx, in.UAR.CardURL)
	if err != nil {
		add(FindingCardUnreadable, SeverityRefuse, "the UAR agent card could not be read: "+err.Error())
		return rep
	}
	rep.Version = version
	c.checkVersion(&rep, in, version)

	assert := in.Check == CheckAssert
	path := c.settingsPath()
	if path == "" {
		// No published UAR release documents a settings endpoint (ADR-0007),
		// so there is nothing to read and no path to probe.
		sev := SeverityRefuse
		if assert {
			sev = SeverityInfo
		}
		add(FindingSettingsUnreadable, sev, "no UAR settings endpoint is pinned in the contract, so the effective model, "+
			"embedders, governance and tool inventory cannot be read"+uarAssertHint(assert))
		add(FindingAuthProbeInconclusive, sev, "no UAR settings endpoint is pinned in the contract, so the unauthenticated-access probe has no target"+
			uarAssertHint(assert))
		return rep
	}
	settingsURL := base + path
	c.probeAuth(ctx, &rep, settingsURL, assert)

	s, unreadable := c.readSettings(ctx, in, settingsURL)
	if s == nil {
		if assert {
			add(FindingSettingsAsserted, SeverityInfo, "UAR settings could not be read ("+unreadable+
				"); check: assert records the operator's own verification of the model, embedders, governance and tool inventory")
			return rep
		}
		add(FindingSettingsUnreadable, SeverityRefuse, "UAR settings could not be read ("+unreadable+
			"), so the effective model and embedders cannot be verified; fix the admin credential or record check: assert")
		return rep
	}
	rep.Revision = string(s.Revision)
	c.checkSettings(&rep, in, s, assert)
	return rep
}

func uarAssertHint(assert bool) string {
	if assert {
		return "; asserted by the operator (check: assert)"
	}
	return "; record check: assert only if you verified this yourself"
}

// uarBase returns the base URL the settings API hangs off: the card URL
// without its well-known suffix, or its origin when the suffix is absent.
func uarBase(card string) (string, error) {
	u, err := url.Parse(card)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("the UAR card URL must be an absolute http or https URL")
	}
	path := strings.TrimRight(u.Path, "/")
	switch {
	case strings.HasSuffix(path, uarCardSuffix):
		path = strings.TrimSuffix(path, uarCardSuffix)
	case strings.HasSuffix(path, ".json"):
		path = ""
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: path}).String(), nil
}

// get performs one GET without following redirects. token, when not empty, is
// sent as a bearer credential.
func (c *UARChecker) get(ctx context.Context, rawURL, adminKey string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	if adminKey != "" {
		// UAR's settings API authenticates with X-UAR-Admin-Key, not a bearer
		// (src/uar/api/settings.rs require_admin_key at v1.1.0).
		req.Header.Set("X-UAR-Admin-Key", adminKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		// The error text carries the URL, never a header.
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, uarMaxBody))
	return resp.StatusCode, body, err
}

func (c *UARChecker) fetchVersion(ctx context.Context, card string) (string, error) {
	status, body, err := c.get(ctx, card, "")
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", status)
	}
	var doc struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("not a JSON agent card: %w", err)
	}
	return doc.Version, nil
}

func (c *UARChecker) checkVersion(rep *UARReport, in UARCheckInput, version string) {
	add := func(code, msg string) {
		rep.Findings = append(rep.Findings, Finding{Code: code, Severity: SeverityRefuse, Message: msg})
	}
	min := c.pinnedMinVersion()
	if in.UAR.MinVersion != nil && *in.UAR.MinVersion != "" {
		min = *in.UAR.MinVersion
	}
	if min == "" {
		add(FindingNoMinVersion, "no minimum UAR version is pinned; set uar.min_version to the published release you tested")
	}
	got, ok := parsePublishedVersion(version)
	if !ok {
		add(FindingVersionNotPublished, fmt.Sprintf("the agent card reports version %q, which is not a published release version (x.y.z); "+
			"only published UAR releases count towards the minimum", version))
		return
	}
	if min == "" {
		return
	}
	want, ok := parsePublishedVersion(min)
	if !ok {
		add(FindingNoMinVersion, fmt.Sprintf("the minimum version %q is not a published release version (x.y.z)", min))
		return
	}
	if versionLess(got, want) {
		add(FindingVersionTooOld, fmt.Sprintf("UAR %s is older than the minimum %s", version, min))
	}
}

// probeAuth checks that the instance refuses an unauthenticated request. The
// admin settings path is the probe: it must answer 401 or 403 without a
// credential.
func (c *UARChecker) probeAuth(ctx context.Context, rep *UARReport, settingsURL string, assert bool) {
	status, _, err := c.get(ctx, settingsURL, "")
	switch {
	case err != nil:
		rep.Findings = append(rep.Findings, Finding{Code: FindingAuthProbeInconclusive, Severity: SeverityRefuse,
			Message: "the unauthenticated probe failed: " + err.Error()})
	case status == http.StatusOK:
		rep.Findings = append(rep.Findings, Finding{Code: FindingAuthNotRequired, Severity: SeverityRefuse,
			Message: "UAR answered an unauthenticated request with 200; it must require authentication"})
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
	default:
		sev := SeverityRefuse
		if assert {
			sev = SeverityInfo
		}
		rep.Findings = append(rep.Findings, Finding{Code: FindingAuthProbeInconclusive, Severity: sev,
			Message: fmt.Sprintf("the unauthenticated probe returned HTTP %d, neither 200 nor 401/403, so it proves nothing", status)})
	}
}

// readSettings reads the settings API with the admin credential. It returns
// nil and a reason when the settings cannot be used. The credential is sent
// only over https or to a loopback host.
func (c *UARChecker) readSettings(ctx context.Context, in UARCheckInput, settingsURL string) (*uarSettings, string) {
	if in.UAR.AdminCredential == nil {
		return nil, "no uar.admin_credential is configured"
	}
	token, err := in.UAR.AdminCredential.Read()
	if err != nil {
		return nil, err.Error()
	}
	u, err := url.Parse(settingsURL)
	if err != nil {
		return nil, "invalid settings URL"
	}
	if u.Scheme != "https" && !IsLocalHost(u.Hostname()) {
		return nil, "the admin credential is not sent over plain http to a non-loopback host"
	}
	status, body, err := c.get(ctx, settingsURL, token)
	if err != nil {
		return nil, "request failed: " + err.Error()
	}
	if status != http.StatusOK {
		return nil, fmt.Sprintf("HTTP %d", status)
	}
	return decodeUARSettings(body)
}

// decodeUARSettings validates a settings response against the contract schema
// and decodes it. A response that does not validate, including a top-level
// array, is unreadable (ADR-0007).
func decodeUARSettings(body []byte) (*uarSettings, string) {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, "unexpected response: " + err.Error()
	}
	schema, err := uarSettingsValidator()
	if err != nil {
		return nil, err.Error()
	}
	if err := schema.Validate(doc); err != nil {
		return nil, "the response does not match the contract settings schema: " + err.Error()
	}
	var s uarSettings
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, "unexpected response: " + err.Error()
	}
	return &s, ""
}

func uarSettingsValidator() (*jsonschema.Resolved, error) {
	raw, err := fs.ReadFile(contract.FS(), contract.UARSettingsSchemaFile)
	if err != nil {
		return nil, fmt.Errorf("read UAR settings schema: %w", err)
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse UAR settings schema: %w", err)
	}
	r, err := s.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("resolve UAR settings schema: %w", err)
	}
	return r, nil
}

func (c *UARChecker) checkSettings(rep *UARReport, in UARCheckInput, s *uarSettings, assert bool) {
	var inventoryMissing []string
	// The schema requires llm and both embedders, so they are present here.
	provider := func(label, code string, p *uarProvider) {
		if p == nil || IsNoEgressHost(p.ProviderHost) || AckCovers(in.EgressAck, p.ProviderHost) {
			return
		}
		rep.Findings = append(rep.Findings, Finding{
			Code: code, Severity: SeverityRefuse, Host: p.ProviderHost,
			Message: label + " uses " + p.ProviderHost + ", which is remote and not in this principal's egress_ack",
		})
	}
	provider("the chat model provider", FindingRemoteModel, s.LLM)
	provider("the knowledge-base embedder", FindingRemoteEmbedder, s.KnowledgeEmbedder)
	provider("the memory embedder", FindingRemoteEmbedder, s.MemoryEmbedder)

	switch {
	case s.Governance == nil || s.Governance.Enabled == nil:
		inventoryMissing = append(inventoryMissing, "governance state")
	case !*s.Governance.Enabled:
		rep.Findings = append(rep.Findings, Finding{Code: FindingGovernanceDisabled, Severity: SeverityReadOnly,
			Message: "UAR governance is effectively disabled, so write groups are withdrawn"})
	}
	switch {
	case s.NativeTools == nil || s.NativeTools.TerminalExecEnabled == nil:
		inventoryMissing = append(inventoryMissing, "terminal_exec state")
	case *s.NativeTools.TerminalExecEnabled:
		rep.Findings = append(rep.Findings, Finding{Code: FindingShellToolEnabled, Severity: SeverityReadOnly,
			Message: "UAR has terminal_exec enabled; an agent with a shell can reach any same-user surface, so write groups are withdrawn"})
	}
	if s.NativeTools != nil && len(s.NativeTools.ShellTools) > 0 {
		rep.Findings = append(rep.Findings, Finding{Code: FindingShellToolEnabled, Severity: SeverityReadOnly,
			Message: "UAR has shell-capable native tools enabled (" + strings.Join(s.NativeTools.ShellTools, ", ") +
				"); an agent with a shell can reach any same-user surface, so write groups are withdrawn"})
	}
	if s.MCPServers == nil {
		inventoryMissing = append(inventoryMissing, "MCP server list")
	} else {
		for _, srv := range *s.MCPServers {
			if !isKuboMCPServer(srv, in.KuboListen) {
				rep.Findings = append(rep.Findings, Finding{Code: FindingNonKuboMCPServer, Severity: SeverityReadOnly,
					Message: "UAR has the MCP server " + strconv.Quote(srv.Name) + " configured besides ipfs mcp, so write groups are withdrawn"})
			}
		}
	}

	if len(inventoryMissing) > 0 {
		c.unreadable(rep, FindingInventoryUnreadable, SeverityReadOnly, assert, inventoryMissing)
	}
}

// unreadable records settings the API did not expose. Under check: assert the
// operator vouches for them and the finding is informational.
func (c *UARChecker) unreadable(rep *UARReport, code, severity string, assert bool, missing []string) {
	msg := "the settings API does not expose: " + strings.Join(missing, ", ")
	if assert {
		rep.Findings = append(rep.Findings, Finding{Code: FindingSettingsAsserted, Severity: SeverityInfo,
			Message: msg + "; asserted by the operator (check: assert)"})
		return
	}
	if severity == SeverityReadOnly {
		msg += ", so write groups are withdrawn"
	}
	rep.Findings = append(rep.Findings, Finding{Code: code, Severity: severity, Message: msg})
}

// isKuboMCPServer reports whether a UAR MCP server entry is this ipfs mcp: an
// http entry whose URL is the /mcp route of the listen address.
func isKuboMCPServer(s uarMCPServer, listen string) bool {
	if s.URL == "" {
		return false
	}
	u, err := url.Parse(s.URL)
	if err != nil || strings.TrimRight(u.Path, "/") != RouteMCP {
		return false
	}
	if listen == "" {
		listen = DefaultHTTPListen
	}
	lh, lp, err := net.SplitHostPort(listen)
	if err != nil || lp == "" || lp == "0" {
		return false
	}
	up := u.Port()
	if up == "" {
		up = map[string]string{"http": "80", "https": "443"}[u.Scheme]
	}
	if up != lp {
		return false
	}
	return strings.EqualFold(u.Hostname(), strings.Trim(lh, "[]")) || (IsLocalHost(u.Hostname()) && IsLocalHost(lh))
}
