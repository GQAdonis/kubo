package corehttp

import (
	"strings"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
)

func TestWebUIPaths(t *testing.T) {
	// The upstream v4.13.0 build that WebUIPath named before the fork build.
	const upstream = "/ipfs/bafybeiciqeyipumpmhxzlxnbqdbbv6u5uij4hy4wax64dmj7kvrhusiq6y"

	require.Equal(t, WebUIPath, WebUIPaths[0], "current build comes first")
	require.Contains(t, WebUIPaths, upstream, "earlier builds stay reachable")

	seen := make(map[string]struct{}, len(WebUIPaths))
	for _, p := range WebUIPaths {
		require.True(t, strings.HasPrefix(p, "/ipfs/"), p)
		_, err := cid.Parse(strings.TrimPrefix(p, "/ipfs/"))
		require.NoError(t, err, p)
		_, dup := seen[p]
		require.False(t, dup, "duplicate entry %s", p)
		seen[p] = struct{}{}
	}
}
