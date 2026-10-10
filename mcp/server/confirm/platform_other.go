//go:build !darwin && !linux

package confirm

import "github.com/ipfs/kubo/mcp/server"

// platformFactor refuses: there is no native presence factor here. On Windows
// the only way to confirm is the paired IPFS Desktop window.
func platformFactor() factor {
	return refusingFactor{
		resolver: server.ResolverDesktop,
		why:      "this platform has no native confirmation prompt; pair IPFS Desktop and use --confirm=desktop",
	}
}
