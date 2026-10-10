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
// command definition in mcp.go is untouched. On by default with native and
// desktop: macOS asks Touch ID or the password (needs /usr/bin/swift from the
// Xcode command line tools), Linux asks polkit (needs the operator-installed
// action), Windows needs a paired IPFS Desktop. A mechanism the host cannot
// offer is skipped and a client that cannot elicit gets
// confirmation_unavailable. --confirm=none or IPFS_MCP_CONFIRM=none turns the
// resolvers off; cli is only enabled by naming it.

const mcpConfirmOptionName = "confirm"

func init() {
	mcpCmd.Options = append(mcpCmd.Options, cmds.StringOption(mcpConfirmOptionName,
		"Out-of-band confirmation for clients that cannot elicit: native, desktop, cli, none (comma separated). Default: native,desktop."))

	serve := mcpCmd.Run
	mcpCmd.Run = func(req *cmds.Request, re cmds.ResponseEmitter, env cmds.Environment) error {
		modes, explicit, err := confirmModes(req)
		if err != nil {
			return fmt.Errorf("ipfs mcp: %w", err)
		}
		if explicit {
			confirm.Configure(confirm.Config{Modes: modes}) // explicit, including "none" and an empty file list
		}
		defer confirm.Shutdown()
		return serve(req, re, env)
	}

	extra := commands.CreateCmdExtras(commands.SetDoesNotUseRepo(true), commands.SetDoesNotUseConfigAsInput(true))
	subs := map[string]*cmds.Command{
		"confirm": {
			Status: cmds.Experimental,
			Helptext: cmds.HelpText{
				Tagline: "Approve or deny a pending MCP confirmation from a terminal.",
				ShortDescription: `
Headless opt-in for servers with no desktop session. It needs --confirm=cli
here and on the server, and a terminal on stdin; it reads the answer from
that terminal only, never from a pipe. Prefer 'ipfs mcp --confirm=native'
(Touch ID or password on macOS, polkit on Linux) where there is a screen:
this command has no presence factor beyond the terminal itself.
`,
			},
			Arguments: []cmds.Argument{
				cmds.StringArg("id", false, false, "Confirmation ID; optional when only one is waiting."),
			},
			Extra:    extra,
			NoRemote: true,
			Run: func(req *cmds.Request, _ cmds.ResponseEmitter, _ cmds.Environment) error {
				modes, _, err := confirmModes(req)
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
Desktop; start the server with --confirm=desktop. Running it again replaces
the pairing. The key is a secret: anyone who has it can approve for Desktop.
`,
			},
			Extra:    extra,
			NoRemote: true,
			Run: func(*cmds.Request, cmds.ResponseEmitter, cmds.Environment) error {
				key, err := confirm.Pair(confirm.Config{}.PairingPath())
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
// IPFS_MCP_CONFIRM, then confirmation.modes in the --mcp-config file. explicit
// is false when none of them is set, so the default (native, desktop) applies.
// An empty list, "none" or `"modes": []` turns the resolvers off.
func confirmModes(req *cmds.Request) (modes []string, explicit bool, err error) {
	v, _ := req.Options[mcpConfirmOptionName].(string)
	if v == "" {
		v = os.Getenv(confirm.EnvModes)
	}
	if v != "" {
		modes, err = confirm.ParseModes(v)
		return modes, true, err
	}
	if file := mcpConfigFile(req); file != "" {
		cfg, err := mcpserver.LoadConfig(file)
		if err != nil {
			return nil, false, err
		}
		if cfg.Confirmation.Modes != nil {
			return slices.Clone(*cfg.Confirmation.Modes), true, nil
		}
	}
	return nil, false, nil
}
