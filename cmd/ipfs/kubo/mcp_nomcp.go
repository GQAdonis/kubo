//go:build nomcp

package kubo

import (
	"errors"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/kubo/core/commands"
)

// mcpCmd is the stub for builds with the nomcp tag: the MCP server is not
// compiled in.
var mcpCmd = &cmds.Command{
	Status: cmds.Experimental,
	Helptext: cmds.HelpText{
		Tagline: "Serve a running Kubo node to AI agents over MCP (disabled).",
		ShortDescription: `
This version of ipfs is compiled without MCP support (build tag nomcp).
Use a build without that tag to run 'ipfs mcp'.
`,
	},
	Extra:    commands.CreateCmdExtras(commands.SetDoesNotUseRepo(true), commands.SetDoesNotUseConfigAsInput(true)),
	NoRemote: true,
	Run: func(*cmds.Request, cmds.ResponseEmitter, cmds.Environment) error {
		return errors.New("MCP support is not included in this build (compiled with the nomcp tag)")
	},
}
