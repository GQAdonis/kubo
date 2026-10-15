package cli

import (
	"fmt"
	"net"
	"testing"

	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Principals in the config whose credentials all fail to resolve stop startup
// instead of leaving an anonymous listener (S5).
func TestMCPHTTPStartupFailsWhenNoPrincipalCredentialResolves(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	listen := l.Addr().String()
	require.NoError(t, l.Close())

	cfg := principalConfig(t, fmt.Sprintf(`{"http":{"enabled":true,"listen":%q},"principals":[
	  {"name":"reader","kind":"human-mcp","credential":{"env":"CLI_MCP_S5_ABSENT_READER"}},
	  {"name":"writer","kind":"human-mcp","credential":{"env":"CLI_MCP_S5_ABSENT_WRITER"}}]}`, listen))

	stdout, stderr, code := runMCP(t, node.IPFSBin, node.Dir, "--api", node.APIAddr().String(), "--mcp-config", cfg)
	assert.NotZero(t, code)
	assert.Empty(t, stdout)
	for _, want := range []string{"reader", "env CLI_MCP_S5_ABSENT_READER", "writer", "env CLI_MCP_S5_ABSENT_WRITER", "anonymous"} {
		assert.Contains(t, stderr, want)
	}
}
