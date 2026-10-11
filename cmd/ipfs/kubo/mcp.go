//go:build !nomcp

package kubo

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	cmds "github.com/ipfs/go-ipfs-cmds"
	ipfs "github.com/ipfs/kubo"
	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core/commands"
	mcpserver "github.com/ipfs/kubo/mcp/server"
)

const (
	mcpToolsOptionName    = "tools"
	mcpReadOnlyOptionName = "read-only"

	// mcpAPIAuthEnvOptionName and mcpAPIAuthFileOptionName keep the RPC secret
	// off the command line, where other local users can read it.
	mcpAPIAuthEnvOptionName  = "api-auth-env"
	mcpAPIAuthFileOptionName = "api-auth-file"
)

// mcpCmd is `ipfs mcp`. It is a local command: it runs inside the CLI process,
// is never served over /api/v0, and never opens or locks the repo. It talks to
// a running daemon through client/rpc only.
var mcpCmd = &cmds.Command{
	Status: cmds.Experimental,
	Helptext: cmds.HelpText{
		Tagline: "Serve a running Kubo node to AI agents over MCP.",
		ShortDescription: `
Runs a Model Context Protocol (MCP) server on stdin and stdout. An MCP
client such as Claude Code, Codex or OpenCode starts this command and talks
JSON-RPC to it. Only MCP messages are written to stdout; logs go to stderr.

The server is a client of a running Kubo daemon. It calls the daemon's
RPC API (/api/v0) and never opens the repo or takes repo.lock, so 'ipfs
daemon' can run on the same repo before, during and after. It has no
offline mode: with no reachable daemon it exits with an error that names
the API address it tried.
`,
		LongDescription: `
Finding the daemon, in order:

  --api <multiaddr or URL>   for example /ip4/127.0.0.1/tcp/5001
  $IPFS_PATH/api             the api file written by 'ipfs daemon'
  ~/.ipfs/api                when IPFS_PATH is not set

When the daemon's RPC API requires an API.Authorizations secret, give it with
--api-auth-env NAME (an environment variable) or --api-auth-file /abs/path.
--api-auth <secret> also works but puts the secret in the process list, where
other local users can read it, and prints a warning. Prefer a dedicated secret
whose AllowedPaths list only the commands of the enabled tools.

The daemon must be Kubo 0.29.0 or newer. The server refuses to start below
that. When a daemon lacks the RPC command a tool needs, that tool is left
out of tools/list and ipfs_node_status reports it as unavailable.

Tool groups: node, content, pin, mfs, names, dag, routing, diag, index.
By default every group is enabled, except the destructive tools. Use
--tools to choose groups (comma separated). Name 'destructive' to also
enable the destructive tools, the ones that can remove or overwrite data;
every such call still needs confirmation. --read-only offers only tools
that change nothing.

Confirmation. Every write tool and every destructive tool is confirmed before
it runs, on every transport. The server asks the client's user through MCP
elicitation. A client that cannot elicit gets a refusal
(confirmation_unavailable) and nothing changes, unless an out-of-band
confirmation prompt is available. A confirm token the server returns is a
binding handle and never consent.

Streamable HTTP. --http (off by default) serves MCP on HTTP instead of
stdin and stdout, at http://127.0.0.1:5050/mcp. Name another address with
--http=<host:port> or IPFS_MCP_HTTP_LISTEN. Ports of other local services
(4000, 4001, 5001, 8080 and others) are refused. A non-loopback address needs
a bearer token and is refused without one. Requests must carry a loopback Host
header and no foreign Origin (DNS rebinding and cross-origin protection).

A bearer token is read from an environment variable or a file, never from the
command line: --auth-token-env NAME or --auth-token-file /abs/path grants the
operator every group. --mcp-config (or IPFS_MCP_CONFIG; the flag wins) names a
JSON file that declares named principals, each with its own credential
reference, tool groups, and write_groups and destructive_groups grants. A
principal is read-only until a group is granted by name; destructive tools
stay off until granted. The file is validated against the contract schema
(mcp/contract/config). Without any credential, a loopback listener serves
the read-only tools to local processes. Put TLS in a reverse proxy.

--print-rpc-allowed-paths prints an API.Authorizations entry whose
AllowedPaths list only the RPC commands of the enabled tools (honors --tools
and --read-only), then exits. Kubo matches AllowedPaths by string prefix, so a
shorter path grants more than it appears to.

Principals. Each principal in the config file may carry an egress_ack: the
model and embedder hosts (or "local") its content may be forwarded to. Without
one, every tool that returns node content (contentEgress) is refused to it
with egress_not_acknowledged. A principal of kind uar names its UAR instance
in a uar block; ipfs mcp checks that instance at startup and at every session
initialize (see "ipfs mcp check-uar --help") and disables the principal, or
withdraws its write groups, when the check fails. No published UAR release
documents a settings endpoint yet, so "check": "verify" refuses a uar
principal until one is pinned; "check": "assert" records that you verified the
instance yourself. With --confirm=cli, a principal with write groups also
needs shell_capable: false; the --auth-token-* operator identity is not
exempt, so give writes under --confirm=cli to a principal of kind human-mcp
with its own credential, write_groups and shell_capable: false. A client that
identifies itself as UAR on stdio is refused content tools. Nothing is
contacted unless the config names it.

MCP App views. Tools that have a view carry _meta.ui, and the views are served
as ui:// resources from bundles embedded in the binary. --no-app-views (or
"app_views": false under "tools" in the config file) serves neither; every
tool is still offered and returns the same result.

Embeddings. The "embeddings" section of --mcp-config is absent by default, and
then no SurrealDB process is started, no store is opened and no embedder is
contacted. With "mode": "embedded", ipfs mcp starts and stops a SurrealDB
process from the binary at the absolute path in embedded.surreal_binary (never
looked up on PATH, never downloaded), bound to 127.0.0.1 on an ephemeral port,
with credentials generated for each start and kept in a 0600 file in
embedded.data_dir. With "mode": "remote" it connects to remote.endpoint, whose
credential is read from an environment variable or file. Text is sent to the
OpenAI-compatible embedder.base_url only when the host is in the egress_ack of
the identity that triggered it; the operator identity has no egress_ack and so
never triggers it. Credentials are references, never literals.

Search index. With an embeddings section, the ipfs_index_search,
ipfs_index_cid, ipfs_index_status and ipfs_index_reconcile tools are
offered; without one they are not. "policy": {"auto": true} also indexes
text that an agent adds with ipfs_add (off by default);
policy.mime_allowlist, max_bytes and chunk_chars limit what is indexed.
Indexing only reads content you added; it never adds, pins or announces
anything. Vectors of a CID are deleted after pin_rm or repo_gc only when no
pin of any type covers it, and ipfs_index_reconcile removes those of content
you unpinned elsewhere.

Remove the command at build time with: make build GOTAGS=nomcp

Example client configuration (Claude Code):

  claude mcp add ipfs -- ipfs mcp

Specification: https://modelcontextprotocol.io/specification
`,
	},
	Options: []cmds.Option{
		cmds.StringOption(mcpToolsOptionName, "Comma-separated tool groups to enable; add 'destructive' to enable destructive tools."),
		cmds.BoolOption(mcpReadOnlyOptionName, "Offer only tools that do not change anything.").WithDefault(false),
		cmds.StringOption(mcpAPIAuthEnvOptionName, "Name of an environment variable holding the API.Authorizations secret (for example bearer:<secret>) for the daemon's RPC API."),
		cmds.StringOption(mcpAPIAuthFileOptionName, "Absolute path of a file holding the API.Authorizations secret for the daemon's RPC API."),
	},
	Extra:    commands.CreateCmdExtras(commands.SetDoesNotUseRepo(true), commands.SetDoesNotUseConfigAsInput(true)),
	NoRemote: true,
	Run: func(req *cmds.Request, _ cmds.ResponseEmitter, _ cmds.Environment) error {
		apiAddr, _ := req.Options[commands.ApiOption].(string)
		settings, err := loadMCPSettings(req)
		if err != nil {
			return fmt.Errorf("ipfs mcp: %w", err)
		}
		if settings.printPaths {
			if err := printRPCAllowedPaths(settings); err != nil {
				return fmt.Errorf("ipfs mcp: %w", err)
			}
			return nil
		}

		// Stdout is the JSON-RPC channel; diagnostics go to stderr only.
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

		apiAuth, onArgv, err := resolveAPIAuth(req)
		if err != nil {
			return fmt.Errorf("ipfs mcp: %w", err)
		}
		if onArgv {
			fmt.Fprintln(os.Stderr, "ipfs mcp: warning: --api-auth puts the RPC secret on the command line, where other local users can read it; use --"+mcpAPIAuthEnvOptionName+" or --"+mcpAPIAuthFileOptionName+" instead")
		}

		daemon, err := mcpserver.Discover(mcpserver.DiscoverOptions{
			API:           apiAddr,
			Authorization: config.ConvertAuthSecret(apiAuth),
		})
		if err != nil {
			return fmt.Errorf("ipfs mcp: %w", err)
		}
		srv, err := mcpserver.New(req.Context, daemon, mcpserver.Options{
			Tools:      settings.tools,
			ReadOnly:   settings.readOnly,
			Disabled:   settings.disabled,
			NoAppViews: settings.noAppViews,
			Transport:  settings.transport(),
			Version:    ipfs.CurrentVersionNumber,
			Logger:     logger,
		})
		if err != nil {
			return fmt.Errorf("ipfs mcp: %w", err)
		}
		// Embeddings come first: they add the index tools, and the agent loop
		// reads the tool set when it is enabled.
		stopEmbeddings, err := enableMCPEmbeddings(req.Context, srv, settings, logger)
		if err != nil {
			return fmt.Errorf("ipfs mcp: %w", err)
		}
		defer stopEmbeddings()
		if err := enableMCPAgent(srv, settings); err != nil {
			return fmt.Errorf("ipfs mcp: %w", err)
		}
		if settings.http != nil {
			if err := settings.http.serveHTTP(req.Context, srv); err != nil {
				return fmt.Errorf("ipfs mcp: %w", err)
			}
			return nil
		}
		if err := srv.RunStdio(req.Context); err != nil {
			return fmt.Errorf("ipfs mcp: %w", err)
		}
		return nil
	},
}

// splitList splits a comma-separated option value, dropping empty items.
func splitList(s string) []string {
	var out []string
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// resolveAPIAuth returns the RPC API secret from --api-auth-env,
// --api-auth-file or --api-auth (exactly one). onArgv is true for --api-auth,
// whose value other local users can read in the process list.
func resolveAPIAuth(req *cmds.Request) (secret string, onArgv bool, err error) {
	env, _ := req.Options[mcpAPIAuthEnvOptionName].(string)
	file, _ := req.Options[mcpAPIAuthFileOptionName].(string)
	argv, _ := req.Options[commands.ApiAuthOption].(string)
	set := 0
	for _, v := range []string{env, file, argv} {
		if v != "" {
			set++
		}
	}
	switch {
	case set > 1:
		return "", false, fmt.Errorf("use only one of --%s, --%s and --%s", mcpAPIAuthEnvOptionName, mcpAPIAuthFileOptionName, commands.ApiAuthOption)
	case set == 0:
		return "", false, nil
	case argv != "":
		return argv, true, nil
	}
	secret, err = mcpserver.SecretRef{Env: env, File: file}.Read()
	if err != nil {
		return "", false, fmt.Errorf("RPC API secret: %w", err)
	}
	return secret, false, nil
}
