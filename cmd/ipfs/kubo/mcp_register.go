package kubo

// Register `ipfs mcp` as a CLI-only local command. The command map is shared
// with Root.Subcommands (see ipfs.go), so registration order between init
// functions does not matter. The command is never part of /api/v0.
func init() {
	localCommands["mcp"] = mcpCmd
}
