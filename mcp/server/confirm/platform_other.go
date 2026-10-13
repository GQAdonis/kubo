//go:build !darwin && !linux

package confirm

import "github.com/ipfs/kubo/mcp/server"

// platformFactor refuses: there is no native presence factor here, and Windows
// has no resolver this phase (the Desktop channel is write-only and inert on
// win32).
func platformFactor() factor {
	return refusingFactor{
		resolver: server.ResolverDesktop,
		why:      "this platform has no native confirmation prompt; run the server on macOS or Linux, or use an MCP client that supports elicitation",
	}
}
