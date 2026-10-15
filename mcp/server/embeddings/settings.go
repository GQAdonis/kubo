package embeddings

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Mode selects where the SurrealDB store lives.
type Mode string

const (
	// ModeEmbedded supervises a SurrealDB process started from an operator
	// supplied binary, bound to a loopback ephemeral port.
	ModeEmbedded Mode = "embedded"
	// ModeRemote connects to a SurrealDB server at an operator supplied URL.
	ModeRemote Mode = "remote"
)

// DefaultMinVersion is the oldest SurrealDB the embedded mode accepts when the
// operator names none (the contract default of embedded.min_version).
const DefaultMinVersion = "3.0.0"

// MaxDimensions is the largest embedding dimension the contract allows.
const MaxDimensions = 8192

// Secret is a credential value. It prints as "<redacted>" everywhere a value
// could reach a log, a JSON document or an error message; Reveal returns it.
type Secret string

// Reveal returns the secret itself. Call it only where it goes on the wire.
func (s Secret) Reveal() string { return string(s) }

// String implements fmt.Stringer.
func (s Secret) String() string { return redacted(s) }

// GoString implements fmt.GoStringer.
func (s Secret) GoString() string { return redacted(s) }

// MarshalText implements encoding.TextMarshaler.
func (s Secret) MarshalText() ([]byte, error) { return []byte(redacted(s)), nil }

// LogValue implements slog.LogValuer.
func (s Secret) LogValue() slog.Value { return slog.StringValue(redacted(s)) }

func redacted(s Secret) string {
	if s == "" {
		return ""
	}
	return "<redacted>"
}

// Settings is a resolved embeddings configuration: credentials are values, read
// from their env or file reference by the caller.
type Settings struct {
	Mode     Mode
	Embedded *EmbeddedSettings
	Remote   *RemoteSettings
	Embedder EmbedderSettings
	// Policy says what is indexed (embeddings.policy).
	Policy Policy
	// Extractor, if set, turns documents, code and data into Markdown through
	// xberg (embeddings.extractor). Nil means no extractor process or request.
	Extractor *ExtractorSettings
	// Describer, if set, describes images (and, if listed, videos) through an
	// OpenAI-compatible chat model (embeddings.describer). Nil means no
	// description request exists.
	Describer *DescriberSettings
}

// EmbeddedSettings configures the supervised SurrealDB process.
type EmbeddedSettings struct {
	// Binary is the absolute path of the surreal executable. It is never
	// looked up on PATH and never downloaded.
	Binary string
	// DataDir is the absolute directory that holds the surrealkv store and the
	// supervisor's state.
	DataDir string
	// MinVersion is the oldest accepted surreal version; empty means
	// DefaultMinVersion.
	MinVersion string
	// AvoidPorts are ports the ephemeral bind must not use (ports of other
	// local services). It is set by the caller, not by the operator.
	AvoidPorts []int
}

// RemoteSettings configures the connection to a SurrealDB server.
type RemoteSettings struct {
	Endpoint  string
	Namespace string
	Database  string
	// Credential is "user:password" (HTTP basic, a root or scoped system user)
	// or a bearer token; empty means an unauthenticated connection.
	Credential Secret
}

// EmbedderSettings configures the OpenAI-compatible embeddings endpoint.
type EmbedderSettings struct {
	BaseURL    string
	Model      string
	Dimensions int
	// APIKey is sent as a bearer token; empty sends no Authorization header.
	APIKey Secret
	// InsecureHTTPHosts is embeddings.insecure_http_hosts: hosts that may be
	// reached over plain http while APIKey is set.
	InsecureHTTPHosts []string
}

// identName restricts namespace and database names to characters that are safe
// in a DEFINE statement and in an HTTP header.
var identName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// Validate checks the settings without touching the network, the file system
// or any process. Errors name the configuration key.
func (s Settings) Validate() error {
	var errs []error
	switch s.Mode {
	case ModeEmbedded:
		errs = append(errs, s.validateEmbedded()...)
	case ModeRemote:
		errs = append(errs, s.validateRemote()...)
	default:
		errs = append(errs, fmt.Errorf("embeddings.mode %q is not embedded or remote", s.Mode))
	}
	errs = append(errs, s.Embedder.validate()...)
	errs = append(errs, validateInsecureHTTPHosts(s.Embedder.InsecureHTTPHosts)...)
	errs = append(errs, s.Policy.validate()...)
	if s.Extractor != nil {
		errs = append(errs, s.Extractor.Validate())
	}
	if s.Describer != nil {
		errs = append(errs, s.Describer.Validate())
	}
	return errors.Join(errs...)
}

// insecureHostPattern is the contract's pattern for an
// embeddings.insecure_http_hosts entry: a host name or IPv4 address with an
// optional :port, nothing else.
var insecureHostPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?(:[0-9]{1,5})?$`)

// validateInsecureHTTPHosts rejects entries the matcher could never use or
// would match more than intended (a scheme, a path, a wildcard, a bare IPv6
// such as fd00::1:8000 whose port cannot be told from an address group).
func validateInsecureHTTPHosts(hosts []string) []error {
	var errs []error
	for _, h := range hosts {
		if len(h) <= 262 && insecureHostPattern.MatchString(h) {
			if _, port, ok := strings.Cut(h, ":"); !ok || validPort(port) {
				continue
			}
		}
		errs = append(errs, fmt.Errorf("embeddings.insecure_http_hosts entry %q must be a host name or IPv4 address with an optional :port (1-65535); no scheme, path, wildcard or IPv6", h))
	}
	return errs
}

func validPort(p string) bool {
	n, err := strconv.Atoi(p)
	return err == nil && n >= 1 && n <= 65535
}

func (s Settings) validateEmbedded() []error {
	e := s.Embedded
	if e == nil {
		return []error{errors.New("embeddings.embedded is required when embeddings.mode is embedded")}
	}
	var errs []error
	if !filepath.IsAbs(e.Binary) {
		errs = append(errs, fmt.Errorf("embeddings.embedded.surreal_binary %q must be an absolute path (it is never looked up on PATH)", e.Binary))
	}
	if !filepath.IsAbs(e.DataDir) {
		errs = append(errs, fmt.Errorf("embeddings.embedded.data_dir %q must be an absolute path", e.DataDir))
	}
	if e.MinVersion != "" {
		if _, err := parseVersion(e.MinVersion); err != nil {
			errs = append(errs, fmt.Errorf("embeddings.embedded.min_version: %w", err))
		}
	}
	if s.Remote != nil {
		errs = append(errs, errors.New("embeddings.remote is set but embeddings.mode is embedded"))
	}
	return errs
}

func (s Settings) validateRemote() []error {
	r := s.Remote
	if r == nil {
		return []error{errors.New("embeddings.remote.endpoint is required when embeddings.mode is remote: there is no default SurrealDB URL")}
	}
	var errs []error
	if err := checkEndpoint(r.Endpoint); err != nil {
		errs = append(errs, fmt.Errorf("embeddings.remote.endpoint: %w", err))
	}
	if !identName.MatchString(r.Namespace) {
		errs = append(errs, fmt.Errorf("embeddings.remote.namespace %q must be 1-128 letters, digits, '_' or '-'", r.Namespace))
	}
	if !identName.MatchString(r.Database) {
		errs = append(errs, fmt.Errorf("embeddings.remote.database %q must be 1-128 letters, digits, '_' or '-'", r.Database))
	}
	if s.Embedded != nil {
		errs = append(errs, errors.New("embeddings.embedded is set but embeddings.mode is remote"))
	}
	return errs
}

func (e EmbedderSettings) validate() []error {
	var errs []error
	if err := checkEndpoint(e.BaseURL); err != nil {
		errs = append(errs, fmt.Errorf("embeddings.embedder.base_url: %w", err))
	} else if e.APIKey != "" && !httpsOrLoopback(e.BaseURL) && !insecureHTTPListed(e.BaseURL, e.InsecureHTTPHosts) {
		errs = append(errs, errors.New("embeddings.embedder.base_url: refusing to send an API key over plain http to a non-loopback host; use https, a loopback embedder, or list the host in embeddings.insecure_http_hosts"))
	}
	if strings.TrimSpace(e.Model) == "" {
		errs = append(errs, errors.New("embeddings.embedder.model is required"))
	}
	if e.Dimensions < 1 || e.Dimensions > MaxDimensions {
		errs = append(errs, fmt.Errorf("embeddings.embedder.dimensions %d must be between 1 and %d", e.Dimensions, MaxDimensions))
	}
	return errs
}

// checkEndpoint accepts an http or https URL with a host and no credentials,
// query or fragment: a URL that is logged or sent in an error must hold no
// secret.
func checkEndpoint(raw string) error {
	if raw == "" {
		return errors.New("is empty: there is no default")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("is not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("must start with http:// or https://")
	}
	if u.Host == "" {
		return errors.New("has no host")
	}
	if u.User != nil {
		return errors.New("must not carry credentials; use a credential reference")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return errors.New("must not carry a query or fragment")
	}
	return nil
}

// httpsOrLoopback reports whether a credential may be sent to the endpoint: it
// uses https, or its host is a loopback name or address.
func httpsOrLoopback(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Scheme, "https") {
		return true
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}

// insecureHTTPListed reports whether raw is a plain http URL whose host (and,
// for an entry with a port, port) is named in hosts, the operator's
// embeddings.insecure_http_hosts. Matching is case-insensitive and exact: a bare
// host covers every port, host:port covers that port only (an http URL without
// a port is port 80), and there are no wildcards.
func insecureHTTPListed(raw string, hosts []string) bool {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "http") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" {
		port = "80"
	}
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == host || h == host+":"+port {
			return true
		}
	}
	return false
}

func (p Policy) validate() []error {
	var errs []error
	if p.MaxBytes < 0 || p.MaxBytes > MaxMaxBytes {
		errs = append(errs, fmt.Errorf("embeddings.policy.max_bytes %d must be between 1 and %d", p.MaxBytes, MaxMaxBytes))
	}
	if p.ChunkChars != 0 && (p.ChunkChars < MinChunkChars || p.ChunkChars > MaxChunkChars) {
		errs = append(errs, fmt.Errorf("embeddings.policy.chunk_chars %d must be between %d and %d", p.ChunkChars, MinChunkChars, MaxChunkChars))
	}
	for _, m := range p.MIMEAllowlist {
		if !mimePattern.MatchString(m) {
			errs = append(errs, fmt.Errorf("embeddings.policy.mime_allowlist entry %q is not a type/subtype", m))
		}
	}
	return errs
}

var mimePattern = regexp.MustCompile(`^[a-z]+/[a-z0-9.+-]+$`)
