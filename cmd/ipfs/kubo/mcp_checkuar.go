//go:build !nomcp

package kubo

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/kubo/core/commands"
	mcpserver "github.com/ipfs/kubo/mcp/server"
)

// `ipfs mcp check-uar` runs the preflight that ipfs mcp also runs, at startup
// and at every session initialize, for each principal of kind uar. It contacts
// only the address the operator names: the <url> argument or the card_url of
// the principal named by --principal. There is no default address.

const (
	checkUARPrincipalOption  = "principal"
	checkUARAdminEnvOption   = "admin-credential-env"
	checkUARMinVersionOption = "min-version"
	checkUARAckOption        = "egress-ack"
	checkUARCheckOption      = "check"
)

func init() {
	sub := &cmds.Command{
		Status: cmds.Experimental,
		Helptext: cmds.HelpText{
			Tagline: "Check a UAR instance against the rules for a uar principal.",
			ShortDescription: `
Reads the agent card of a Universal Agent Runtime (UAR) instance and, with an
admin credential, its settings, and reports whether it may be a principal of
kind uar. The command exits with an error unless the verdict is ok.

It checks that the version is a published release at or above the minimum,
that an unauthenticated request is refused, that the effective chat model and
embedders are local or listed in --egress-ack, and that governance is on,
terminal_exec is off and no MCP server besides ipfs mcp is configured. A
remote provider refuses the principal; the rest only withdraws write groups
(verdict read_only). Settings that cannot be read fail closed unless
--check=assert records that you verified them yourself.
`,
			LongDescription: `
The command contacts only the address you give it. There is no default
address and nothing is sent anywhere else. Name the instance with the agent
card URL (http://host:port/.well-known/agent-card.json) or its base URL, or
use --principal NAME with --mcp-config to read the address, admin credential,
minimum version, egress_ack and check mode of a principal from the config
file.

The admin credential is read from the environment variable named by
--admin-credential-env (or the principal's uar.admin_credential). It is sent
only over https or to a loopback host and is never printed.

The settings path is pinned in the Kubo MCP contract together with the minimum
UAR version, and is empty until a published UAR release documents a settings
endpoint. While it is empty nothing can be read or probed: --check=verify
refuses (settings_unreadable, auth_probe_inconclusive) and --check=assert
reports both as info. The version check always runs, against --min-version or
the pinned minimum.

A UAR bound to 0.0.0.0 answers identically on loopback, so this check does not
claim to detect the bind. Use the shipped profile (server.host: 127.0.0.1).
`,
		},
		Arguments: []cmds.Argument{
			cmds.StringArg("url", false, false, "Agent card URL or base URL of the UAR instance; optional with --"+checkUARPrincipalOption+"."),
		},
		Options: []cmds.Option{
			cmds.StringOption(checkUARPrincipalOption, "Name of a kind uar principal in the --mcp-config file to check."),
			cmds.StringOption(checkUARAdminEnvOption, "Name of the environment variable holding the UAR admin credential for the settings API."),
			cmds.StringOption(checkUARMinVersionOption, "Minimum published UAR release (x.y.z)."),
			cmds.StringOption(checkUARAckOption, "Comma-separated hosts the instance may forward content to, or 'local'."),
			cmds.StringOption(checkUARCheckOption, "'verify' (default) or 'assert' to waive settings Kubo cannot read."),
		},
		Extra:    commands.CreateCmdExtras(commands.SetDoesNotUseRepo(true), commands.SetDoesNotUseConfigAsInput(true)),
		NoRemote: true,
		Run: func(req *cmds.Request, _ cmds.ResponseEmitter, _ cmds.Environment) error {
			return runCheckUAR(req, os.Stdout)
		},
	}
	if mcpCmd.Subcommands == nil {
		mcpCmd.Subcommands = map[string]*cmds.Command{}
	}
	mcpCmd.Subcommands["check-uar"] = sub
}

// cardURL turns the argument into an agent card URL: a URL that already names
// a .json document is used as is, anything else is a base URL.
func cardURL(arg string) string {
	if strings.HasSuffix(strings.TrimRight(arg, "/"), ".json") {
		return arg
	}
	return strings.TrimRight(arg, "/") + "/.well-known/agent-card.json"
}

// checkUARInput resolves the command line and the config file into one check.
func checkUARInput(req *cmds.Request) (mcpserver.UARCheckInput, string, error) {
	principal, _ := req.Options[checkUARPrincipalOption].(string)
	adminEnv, _ := req.Options[checkUARAdminEnvOption].(string)
	minVersion, _ := req.Options[checkUARMinVersionOption].(string)
	ack, _ := req.Options[checkUARAckOption].(string)
	check, _ := req.Options[checkUARCheckOption].(string)
	var urlArg string
	if len(req.Arguments) > 0 {
		urlArg = req.Arguments[0]
	}

	var in mcpserver.UARCheckInput
	listen := os.Getenv(mcpserver.HTTPListenEnv)
	file := mcpConfigFile(req)
	var cfg *mcpserver.Config
	if file != "" {
		var err error
		if cfg, err = mcpserver.LoadConfig(file); err != nil {
			return in, "", err
		}
		if listen == "" {
			listen = cfg.HTTP.Listen
		}
	}
	in.KuboListen = listen

	switch {
	case principal != "" && urlArg != "":
		return in, "", errors.New("name either a <url> or --" + checkUARPrincipalOption + ", not both")
	case principal != "":
		if cfg == nil {
			return in, "", errors.New("--" + checkUARPrincipalOption + " needs --mcp-config or IPFS_MCP_CONFIG")
		}
		p, ok := findPrincipal(cfg, principal)
		if !ok || p.Kind != mcpserver.KindUAR || p.UAR == nil {
			return in, "", fmt.Errorf("%q is not a principal of kind uar in %s", principal, file)
		}
		in.UAR, in.Check, in.EgressAck = *p.UAR, p.Check, p.EgressAck
	case urlArg != "":
		in.UAR.CardURL = cardURL(urlArg)
	default:
		return in, "", errors.New("no UAR address: pass the <url> or --" + checkUARPrincipalOption + "; there is no default address")
	}

	// Flags refine what the principal or the URL gave.
	if adminEnv != "" {
		in.UAR.AdminCredential = &mcpserver.SecretRef{Env: adminEnv}
	}
	if minVersion != "" {
		in.UAR.MinVersion = &minVersion
	}
	if ack != "" {
		in.EgressAck = splitList(ack)
	}
	if check != "" {
		if check != mcpserver.CheckVerify && check != mcpserver.CheckAssert {
			return in, "", fmt.Errorf("--%s must be %q or %q", checkUARCheckOption, mcpserver.CheckVerify, mcpserver.CheckAssert)
		}
		in.Check = check
	}
	return in, in.UAR.CardURL, nil
}

func findPrincipal(cfg *mcpserver.Config, name string) (mcpserver.PrincipalConfig, bool) {
	for _, p := range cfg.Principals {
		if p.Name == name {
			return p, true
		}
	}
	return mcpserver.PrincipalConfig{}, false
}

// runCheckUAR runs the check and writes the report to w. It returns an error
// unless the verdict is ok.
func runCheckUAR(req *cmds.Request, w io.Writer) error {
	in, card, err := checkUARInput(req)
	if err != nil {
		return fmt.Errorf("ipfs mcp check-uar: %w", err)
	}
	rep := mcpserver.NewUARChecker(nil, nil).Check(req.Context, in)
	fmt.Fprintf(w, "UAR %s at %s\n", orNone(rep.Version), card)
	if rep.Revision != "" {
		fmt.Fprintf(w, "settings revision: %s\n", rep.Revision)
	}
	for _, f := range rep.Findings {
		fmt.Fprintf(w, "[%s] %s: %s\n", f.Severity, f.Code, f.Message)
	}
	if in.Check == mcpserver.CheckAssert {
		fmt.Fprintln(w, "check: assert is set; you vouch for what the UAR settings API does not expose")
	}
	fmt.Fprintf(w, "verdict: %s\n", rep.Verdict())
	if rep.Verdict() != mcpserver.VerdictOK {
		return fmt.Errorf("ipfs mcp check-uar: verdict %s", rep.Verdict())
	}
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "(version unknown)"
	}
	return s
}
