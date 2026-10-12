//go:build !nomcp

package kubo

import (
	"fmt"
	"maps"
	"os"
	"slices"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/kubo/core/commands"
	mcpserver "github.com/ipfs/kubo/mcp/server"
	"github.com/ipfs/kubo/mcp/server/confirm"
)

// Out-of-band confirmation for `ipfs mcp`: the --confirm option, `ipfs mcp
// confirm` and `ipfs mcp pair`. It is attached to mcpCmd from init so the
// command definition in mcp.go is untouched. On by default with native only:
// macOS asks Touch ID or the password (needs /usr/bin/swift from the Xcode
// command line tools), Linux asks polkit (needs the operator-installed
// action), Windows has no resolver this phase. A mechanism the host cannot
// offer is skipped and a client that cannot elicit gets
// confirmation_unavailable. --confirm=none or IPFS_MCP_CONFIRM=none turns the
// resolvers off. desktop (write-class confirmations through the paired IPFS
// Desktop window) and cli are opt-in: name them in --confirm, IPFS_MCP_CONFIRM,
// confirmation.modes (or confirmation.cli for cli). confirmation.resolvers
// narrows the native and desktop resolvers after the modes are resolved.

const mcpConfirmOptionName = "confirm"

func init() {
	mcpCmd.Options = append(mcpCmd.Options, cmds.StringOption(mcpConfirmOptionName,
		"Out-of-band confirmation for clients that cannot elicit: native, desktop, cli, none (comma separated). Default: native. desktop and cli are opt-in."))

	extra := commands.CreateCmdExtras(commands.SetDoesNotUseRepo(true), commands.SetDoesNotUseConfigAsInput(true))
	subs := map[string]*cmds.Command{
		"confirm": {
			Status: cmds.Experimental,
			Helptext: cmds.HelpText{
				Tagline: "Approve or deny a pending MCP confirmation from a terminal.",
				ShortDescription: `
Headless opt-in for servers with no desktop session. cli confirmation must be
on for the server (--confirm=cli, IPFS_MCP_CONFIRM, confirmation.modes or
confirmation.cli) and for this command, which needs the same enable path and
the same --mcp-config as the server, and a terminal on stdin; it reads the
answer from that terminal only, never from a pipe. Prefer
'ipfs mcp --confirm=native' (Touch ID or password on macOS, polkit on Linux)
where there is a screen: this command has no presence factor beyond the
terminal itself.
`,
			},
			Arguments: []cmds.Argument{
				cmds.StringArg("id", false, false, "Confirmation ID; optional when only one is waiting."),
			},
			Extra:    extra,
			NoRemote: true,
			Run: func(req *cmds.Request, _ cmds.ResponseEmitter, _ cmds.Environment) error {
				cfg, err := loadMCPConfig(req)
				if err != nil {
					return err
				}
				modes, _, err := confirmModes(req, cfg)
				if err != nil {
					return err
				}
				id := ""
				if len(req.Arguments) > 0 {
					id = req.Arguments[0]
				}
				return confirm.RunCLI(req.Context, confirm.CLIOptions{
					Modes: modes, ID: id, In: os.Stdin, Out: os.Stdout,
				})
			},
		},
		"pair": {
			Status: cmds.Experimental,
			Helptext: cmds.HelpText{
				Tagline: "Create the pairing key for the IPFS Desktop confirmation window.",
				ShortDescription: `
Prints a new key once and stores only its hash. Enter the key in IPFS
Desktop; start the server with --confirm=desktop. It needs a terminal on
stdin and refuses while an ipfs mcp server is running; stop the server first.
Running it again replaces the pairing, and the old key stops working. The key
is a secret: anyone who has it can approve for Desktop.
`,
			},
			Extra:    extra,
			NoRemote: true,
			Run: func(*cmds.Request, cmds.ResponseEmitter, cmds.Environment) error {
				key, err := confirm.RunPair(confirm.PairOptions{In: os.Stdin})
				if err != nil {
					return err
				}
				fmt.Println(key)
				return nil
			},
		},
	}
	if mcpCmd.Subcommands == nil {
		mcpCmd.Subcommands = make(map[string]*cmds.Command, len(subs))
	}
	maps.Copy(mcpCmd.Subcommands, subs)
}

// confirmModes resolves the gate modes with precedence --confirm, then
// IPFS_MCP_CONFIRM, then confirmation.modes in the --mcp-config file, which
// confirmation.cli true extends with cli. explicit is false when none of them
// is set, so the default (native) applies. An empty list, "none" or
// `"modes": []` turns the resolvers off.
func confirmModes(req *cmds.Request, cfg *mcpserver.Config) (modes []string, explicit bool, err error) {
	// A flag that was given, even empty, decides: --confirm= means no resolvers.
	// Only an absent flag falls back to the environment.
	v, given := req.Options[mcpConfirmOptionName].(string)
	if !given {
		if env, set := os.LookupEnv(confirm.EnvModes); set && env != "" {
			v, given = env, true
		}
	}
	if given {
		modes, err = confirm.ParseModes(v)
		return modes, true, err
	}
	modes, explicit = confirm.FileModes(cfg.Confirmation.Modes, cfg.Confirmation.CLI)
	return modes, explicit, nil
}

// confirmConfig is the full gate configuration: the modes from confirmModes
// (confirmation.cli only counts when --confirm and IPFS_MCP_CONFIRM are not
// given) narrowed by confirmation.resolvers, which applies whatever decided
// the modes and can only remove resolvers.
func confirmConfig(req *cmds.Request, cfg *mcpserver.Config) (confirm.Config, error) {
	modes, explicit, err := confirmModes(req, cfg)
	if err != nil {
		return confirm.Config{}, err
	}
	if !explicit {
		modes = slices.Clone(confirm.DefaultModes)
	}
	c := confirm.Config{Modes: modes}
	if r := cfg.Confirmation.Resolvers; r != nil {
		c.Resolvers = append([]string{}, *r...)
	}
	return c, nil
}
