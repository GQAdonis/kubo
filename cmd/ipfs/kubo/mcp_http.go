//go:build !nomcp

package kubo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	cmds "github.com/ipfs/go-ipfs-cmds"
	mcpserver "github.com/ipfs/kubo/mcp/server"
)

// Options of `ipfs mcp` that belong to the Streamable HTTP transport and to the
// config file. They are attached from init so the base command in mcp.go only
// calls loadMCPSettings.
const (
	mcpHTTPOptionName          = "http"
	mcpAuthTokenEnvOptionName  = "auth-token-env"
	mcpAuthTokenFileOptionName = "auth-token-file"
	mcpPrintPathsOptionName    = "print-rpc-allowed-paths"
	mcpNoAppViewsOptionName    = "no-app-views"
	mcpConfigOptionName        = "mcp-config"
	mcpConfigEnv               = "IPFS_MCP_CONFIG"
)

func init() {
	mcpCmd.Options = append(mcpCmd.Options,
		// No default: an absent key means the flag was not given, so an explicit
		// --http=false can override http.enabled in the config file.
		mcpHTTPFlag{cmds.BoolOption(mcpHTTPOptionName,
			"Serve Streamable HTTP instead of stdio. Optionally name the address: --http=127.0.0.1:5050 (default; also "+mcpserver.HTTPListenEnv+"). Loopback only unless a bearer token is configured.")},
		cmds.StringOption(mcpConfigOptionName, "Path of the ipfs mcp JSON config file (also "+mcpConfigEnv+"; the flag wins). Not the Kubo repo config."),
		cmds.StringOption(mcpAuthTokenEnvOptionName, "Name of an environment variable holding a bearer token that grants the operator full access over HTTP."),
		cmds.StringOption(mcpAuthTokenFileOptionName, "Absolute path of a file holding a bearer token that grants the operator full access over HTTP."),
		cmds.BoolOption(mcpNoAppViewsOptionName, "Turn the MCP App views off: no ui:// resource and no _meta.ui on tools (also tools.app_views false in --mcp-config).").WithDefault(false),
		cmds.BoolOption(mcpPrintPathsOptionName, "Print an API.Authorizations entry that allows only the RPC commands of the enabled tools, then exit.").WithDefault(false),
	)
}

// mcpConfigFile is the ipfs mcp config file: --mcp-config, else the
// IPFS_MCP_CONFIG environment variable. The root --config flag names the Kubo
// repo config file and is deliberately not read here.
func mcpConfigFile(req *cmds.Request) string {
	if v, _ := req.Options[mcpConfigOptionName].(string); v != "" {
		return v
	}
	return os.Getenv(mcpConfigEnv)
}

// mcpHTTPFlag is --http: a bool flag that also takes an address. A bare --http
// (or --http=true) selects the default address, --http=false turns HTTP off and
// --http=<addr> names an address. Parse returns a bool for the first two and the
// address string for the third, so the value lives in the request that carries
// it and no state is shared between invocations. (go-ipfs-cmds runs a string
// value of a bool option through Parse again, which returns it unchanged.)
type mcpHTTPFlag struct{ cmds.Option }

// Parse implements cmds.Option.
func (f mcpHTTPFlag) Parse(v string) (any, error) {
	if b, err := strconv.ParseBool(v); err == nil {
		return b, nil
	}
	if _, err := mcpserver.ParseListen(v); err != nil {
		return nil, err
	}
	return v, nil
}

// WithDefault implements cmds.Option.
func (f mcpHTTPFlag) WithDefault(v any) cmds.Option { return mcpHTTPFlag{f.Option.WithDefault(v)} }

// mcpSettings is the effective configuration of one `ipfs mcp` run: flags
// override the config file, which overrides the contract defaults.
type mcpSettings struct {
	tools    []string
	readOnly bool
	disabled []string
	// printDisabled is disabled plus the tools whose feature is configured off
	// (no embeddings section, or embeddings.bulk.enabled false); only the
	// printed RPC path list uses it.
	printDisabled []string
	noAppViews    bool
	printPaths    bool
	// http is nil when stdio is served.
	http *mcpHTTPSettings

	// cfg is the validated --mcp-config (defaults when none was named).
	cfg *mcpserver.Config
	// agent holds the --llm-* flags and resolved is the backend they and the
	// config file name; both are empty unless an operator configured one.
	agent    agentInputs
	resolved agentResolved
}

type mcpHTTPSettings struct {
	opts mcpserver.HTTPOptions
	// anonymous is true when no credential is configured (loopback only).
	anonymous bool
	// warnings are the principals whose credential reference did not resolve.
	// They are disabled, not fatal, while at least one principal or an operator
	// token resolves; when none does, startup fails (errNoPrincipalResolved).
	warnings []string
	// notes are informational startup lines.
	notes []string
}

func (s *mcpSettings) transport() string {
	if s.http != nil {
		return "http"
	}
	return "stdio"
}

// loadMCPConfig reads and validates the --mcp-config file once, or returns the
// defaults when none is named. Callers pass the result on instead of reading
// the file again, so every consumer sees the same version of it.
func loadMCPConfig(req *cmds.Request) (*mcpserver.Config, error) {
	if file := mcpConfigFile(req); file != "" {
		return mcpserver.LoadConfig(file)
	}
	return mcpserver.ParseConfig([]byte("{}"))
}

func loadMCPSettings(req *cmds.Request, cfg *mcpserver.Config) (*mcpSettings, error) {
	toolsOpt, _ := req.Options[mcpToolsOptionName].(string)
	readOnlyFlag, _ := req.Options[mcpReadOnlyOptionName].(bool)
	printPaths, _ := req.Options[mcpPrintPathsOptionName].(bool)
	noAppViewsFlag, _ := req.Options[mcpNoAppViewsOptionName].(bool)

	s := &mcpSettings{
		tools:      splitList(toolsOpt),
		readOnly:   readOnlyFlag || cfg.Tools.ReadOnly,
		disabled:   cfg.Tools.Disabled,
		noAppViews: noAppViewsFlag || !cfg.Tools.AppViewsEnabled(),
		printPaths: printPaths,
		cfg:        cfg,
		agent:      agentInputsFromRequest(req),
	}
	// A tool whose feature is configured off is not in tools.disabled: the
	// server reports it not_configured. Its RPC paths still stay out of the
	// printed list.
	s.printDisabled = slices.Clone(s.disabled)
	switch e := cfg.Embeddings; {
	case e == nil:
		s.printDisabled = append(s.printDisabled, mcpserver.IndexTools()...)
	case !e.Bulk.IsEnabled():
		s.printDisabled = append(s.printDisabled, mcpserver.BulkTools()...)
	}
	if err := validateAgent(s); err != nil {
		return nil, err
	}
	if err := validateEmbeddings(cfg); err != nil {
		return nil, err
	}
	if len(s.tools) == 0 {
		s.tools = slices.Clone(cfg.Tools.Groups)
	}
	// --http overrides http.enabled in either direction; the config file decides
	// only when the flag was not given.
	httpOn, httpAddr, httpGiven := httpFlagOf(req)
	if !httpGiven {
		httpOn = cfg.HTTP.Enabled
	}
	if httpOn {
		httpCfg, err := httpSettings(req, cfg, s, httpAddr)
		if err != nil {
			return nil, err
		}
		s.http = httpCfg
	}
	return s, nil
}

// httpFlagOf reads --http from the request: whether it was given, whether it
// turns HTTP on, and the address it names (empty for a bare flag).
func httpFlagOf(req *cmds.Request) (on bool, addr string, given bool) {
	switch v := req.Options[mcpHTTPOptionName].(type) {
	case bool:
		return v, "", true
	case string:
		return true, v, true
	}
	return false, "", false
}

// httpSettings builds the HTTP options. It may add the destructive selector to
// s.tools, because a principal's destructive grant needs the tools to exist.
func httpSettings(req *cmds.Request, cfg *mcpserver.Config, s *mcpSettings, flagAddr string) (*mcpHTTPSettings, error) {
	listen := flagAddr
	if listen == "" {
		listen = os.Getenv(mcpserver.HTTPListenEnv)
	}
	if listen == "" {
		listen = cfg.HTTP.Listen
	}
	timeout, err := cfg.SessionTimeoutDuration()
	if err != nil {
		return nil, err
	}
	// The operator token follows what the operator selected, not what a
	// principal's grant makes necessary.
	operatorDestructive := slices.Contains(s.tools, destructiveSelector)
	if cfg.GrantsDestructive() && !operatorDestructive {
		s.tools = append(s.tools, destructiveSelector)
	}
	creds, warnings := cfg.Credentials()
	operator, err := operatorCredential(req, operatorDestructive)
	if err != nil {
		return nil, err
	}
	// Principals that were configured but resolve to nothing must not turn into
	// an anonymous listener the operator never asked for. Some resolved, or an
	// operator token, keeps working with only those enabled.
	sessionOnly := cfg.Auth.OIDC.Enabled && len(cfg.SessionPrincipals()) > 0
	if len(creds) == 0 && len(operator) == 0 && !sessionOnly {
		if err := errNoPrincipalResolved(cfg.Principals); err != nil {
			return nil, err
		}
	}
	creds = append(creds, operator...)

	out := &mcpHTTPSettings{warnings: warnings, opts: mcpserver.HTTPOptions{
		Listen:         listen,
		AllowedOrigins: cfg.HTTP.AllowedOrigins,
		MaxBodyBytes:   cfg.HTTP.MaxBodyBytes,
		SessionTimeout: timeout,
	}}
	// Validate before contacting the daemon, so a bad address fails fast.
	l, err := mcpserver.ParseListen(listen)
	if err != nil {
		return nil, err
	}
	oidc, err := cfg.OIDCSettings()
	if err != nil {
		return nil, err
	}
	if len(creds) == 0 && oidc == nil {
		if !l.IsLoopback() {
			return nil, fmt.Errorf("refusing to listen on non-loopback %s without authentication: set --%s, --%s or a principal in --%s", l, mcpAuthTokenEnvOptionName, mcpAuthTokenFileOptionName, mcpConfigOptionName)
		}
		out.anonymous = true
		return out, nil
	}
	static, err := mcpserver.NewStaticVerifier(creds)
	if err != nil {
		return nil, err
	}
	if oidc == nil {
		out.opts.Verifier = static
		return out, nil
	}
	// auth.oidc is on: session tokens are tried after the static bearers. The
	// key set is read from the configured URL only now, and a failure to read
	// it never stops startup.
	jwt, err := mcpserver.NewJWTVerifier(mcpserver.JWTOptions{
		OIDC:            *oidc,
		Principals:      cfg.SessionPrincipals(),
		OriginAllowlist: cfg.Agent.OriginAllowlist,
	})
	if err != nil {
		return nil, err
	}
	out.opts.Verifier = mcpserver.NewChainVerifier(static, jwt)
	out.notes = append(out.notes, fmt.Sprintf("auth.oidc is on: session tokens from %s are accepted on routes %s (keys at %s)",
		oidc.Issuer, strings.Join(oidc.Routes, ","), jwt.JWKSURL()))
	return out, nil
}

// errNoPrincipalResolved is the startup error when the config names principals
// and no static credential resolved and no session can authenticate. It names
// each principal and the environment variable or file it expects, never a
// value; a session-only principal (subjects, no credential) is named with the
// auth.oidc.enabled key that would make it work. It returns nil when no
// principal is configured: the anonymous loopback default.
func errNoPrincipalResolved(principals []mcpserver.PrincipalConfig) error {
	if len(principals) == 0 {
		return nil
	}
	refs := make([]string, 0, len(principals))
	var sessionOnly []string
	for _, p := range principals {
		if !p.HasCredential() {
			if len(p.Subjects) > 0 {
				sessionOnly = append(sessionOnly, p.Name)
			}
			continue
		}
		refs = append(refs, fmt.Sprintf("%s (%s)", p.Name, p.Credential))
	}
	var parts []string
	if len(refs) > 0 {
		parts = append(parts, fmt.Sprintf("no principal has a resolvable credential: set the environment variable or create the file for at least one (%s)", strings.Join(refs, ", ")))
	}
	if len(sessionOnly) > 0 {
		parts = append(parts, fmt.Sprintf("the session-only principal(s) %s authenticate nothing while auth.oidc.enabled is false: set auth.oidc.enabled to true with an issuer, audiences and jwks_url, or give them a credential", strings.Join(sessionOnly, ", ")))
	}
	return fmt.Errorf("the config names %d principal(s) but the listener would serve anonymous callers: %s; or remove the principals to run anonymously on loopback", len(principals), strings.Join(parts, "; "))
}

// destructiveSelector is the contract's --tools selector for destructive tools.
const destructiveSelector = "destructive"

// operatorCredential reads --auth-token-env / --auth-token-file.
func operatorCredential(req *cmds.Request, destructive bool) ([]mcpserver.StaticCredential, error) {
	env, _ := req.Options[mcpAuthTokenEnvOptionName].(string)
	file, _ := req.Options[mcpAuthTokenFileOptionName].(string)
	if env == "" && file == "" {
		return nil, nil
	}
	if env != "" && file != "" {
		return nil, errors.New("use only one of --" + mcpAuthTokenEnvOptionName + " and --" + mcpAuthTokenFileOptionName)
	}
	token, err := mcpserver.SecretRef{Env: env, File: file}.Read()
	if err != nil {
		return nil, fmt.Errorf("bearer token: %w", err)
	}
	id, err := mcpserver.OperatorIdentity(destructive)
	if err != nil {
		return nil, err
	}
	return []mcpserver.StaticCredential{{Identity: id, Token: token}}, nil
}

// printRPCAllowedPaths writes the API.Authorizations entry for the enabled
// tools to stdout. It does not contact the daemon.
func printRPCAllowedPaths(s *mcpSettings) error {
	paths, err := mcpserver.RPCAllowedPaths(mcpserver.Options{Tools: s.tools, ReadOnly: s.readOnly, Disabled: s.printDisabled})
	if err != nil {
		return err
	}
	entry, err := mcpserver.RPCAuthorizationEntry(paths)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "%s\n", entry)
	return err
}

// serveHTTP serves srv until ctx ends. The bound address goes to stderr, where
// a launcher can read it when the port was left to the OS.
func (h *mcpHTTPSettings) serveHTTP(ctx context.Context, srv *mcpserver.Server) error {
	hs, err := mcpserver.NewHTTPServer(srv, h.opts)
	if err != nil {
		return err
	}
	addr, err := hs.Listen()
	if err != nil {
		return err
	}
	for _, line := range h.warnings {
		fmt.Fprintln(os.Stderr, "ipfs mcp: warning: "+line)
	}
	for _, line := range h.notes {
		fmt.Fprintln(os.Stderr, "ipfs mcp: "+line)
	}
	for _, line := range hs.StartupReport(ctx) {
		fmt.Fprintln(os.Stderr, "ipfs mcp: "+line)
	}
	if h.anonymous {
		fmt.Fprintln(os.Stderr, "ipfs mcp: no bearer token or principal is configured; any local process can use the read-only tools. Configure --auth-token-env, --auth-token-file or principals in --mcp-config for write access.")
	}
	fmt.Fprintf(os.Stderr, "ipfs mcp: serving MCP over HTTP on http://%s%s\n", addr, mcpserver.RouteMCP)
	return hs.Serve(ctx)
}
