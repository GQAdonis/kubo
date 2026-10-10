package server

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/ipfs/kubo/mcp/contract"
)

// rpcPathPrefix is where Kubo serves RPC commands; API.Authorizations
// AllowedPaths entries are matched as prefixes of the request path.
const rpcPathPrefix = "/api/v0/"

// startupCommands are the RPC commands every `ipfs mcp` process calls before
// it serves anything (probe), whatever tools are enabled. "version" is allowed
// by Kubo for every authorized secret, and is listed anyway so the entry
// documents what the process uses.
var startupCommands = []string{"version", "commands"}

// RPCAllowedPaths returns the API.Authorizations AllowedPaths entries that let
// `ipfs mcp` run exactly the tools Options selects: the startup commands plus
// the contract's required RPC commands of each selected tool, as full paths.
// Only Options.Tools, Options.ReadOnly and Options.Disabled are read.
//
// Kubo matches AllowedPaths with strings.HasPrefix, not by path segment, so an
// entry also grants every longer path that starts with it. The helper prints
// full command paths to keep that to a minimum; a shorter prefix grants more
// than it appears to.
func RPCAllowedPaths(opts Options) ([]string, error) {
	cat, err := contract.Load()
	if err != nil {
		return nil, fmt.Errorf("load tool contract: %w", err)
	}
	probe := &Server{cat: cat}
	selectors, err := probe.resolveSelectors(opts.Tools)
	if err != nil {
		return nil, err
	}
	paths := slices.Clone(startupCommands)
	for _, name := range cat.SelectTools(selectors, opts.ReadOnly) {
		if slices.Contains(opts.Disabled, name) {
			continue
		}
		t, _ := cat.Tool(name)
		paths = append(paths, t.AllowedRPC()...)
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	for i, p := range paths {
		paths[i] = rpcPathPrefix + p
	}
	return paths, nil
}

// RPCAuthorizationEntry renders an API.Authorizations entry for the paths. The
// secret is a placeholder: generate a long random value, put it in the entry
// and pass the same value to `ipfs mcp --api-auth`.
func RPCAuthorizationEntry(paths []string) ([]byte, error) {
	entry := struct {
		AuthSecret   string   `json:"AuthSecret"`
		AllowedPaths []string `json:"AllowedPaths"`
	}{
		AuthSecret:   "bearer:REPLACE-WITH-A-LONG-RANDOM-SECRET",
		AllowedPaths: paths,
	}
	return json.MarshalIndent(entry, "", "  ")
}
