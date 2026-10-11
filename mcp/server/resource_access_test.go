package server

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resourceConfig has a principal with no egress_ack, one that is not granted
// the content group, and one that may read content.
const resourceConfig = `{
  "principals": [
    {"name": "no-ack",    "kind": "human-mcp", "credential": {"env": "P_READER"}},
    {"name": "node-only", "kind": "human-mcp", "credential": {"env": "P_WRITER"}, "groups": ["node"], "egress_ack": ["local"]},
    {"name": "reader",    "kind": "human-mcp", "credential": {"env": "P_ADMIN"},  "egress_ack": ["local"]}
  ]
}`

// resources/read on ipfs:// and ipns:// returns the bytes ipfs_cat returns, so
// it must pass the same checks: the content read grant and egress_ack (R11-H2).
func TestContentResourcesFollowTheIPFSCatPolicy(t *testing.T) {
	for _, scheme := range []string{"ipfs", "ipns"} {
		t.Run(scheme, func(t *testing.T) {
			e := newPrincipalEnv(t, resourceConfig, egressTokens(), Options{}, nil)
			uri := scheme + "://" + e.cid

			before := len(e.cf.log)
			noAck := e.client(t, pTokReader, nil)
			_, err := noAck.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri})
			require.Error(t, err, "a principal with no egress_ack must not read content through a resource")
			assert.Contains(t, err.Error(), CodeEgressNotAcknowledged)

			nodeOnly := e.client(t, pTokWriter, nil)
			_, err = nodeOnly.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri})
			require.Error(t, err, "a principal without the content group must not read content through a resource")
			assert.Contains(t, err.Error(), CodePermissionDenied)
			assert.Len(t, e.cf.log, before, "a refused read never reaches the daemon")
		})
	}
}

func TestContentResourceIsReadableWithTheGrantAndAck(t *testing.T) {
	e := newPrincipalEnv(t, resourceConfig, egressTokens(), Options{}, nil)
	e.cf.handle["files/stat"] = func([]string, map[string]any) (any, error) {
		return map[string]any{"Hash": e.cid, "Size": 17, "CumulativeSize": 17, "Blocks": 0, "Type": "file"}, nil
	}
	reader := e.client(t, pTokAdmin, nil)
	res, err := reader.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "ipfs://" + e.cid})
	require.NoError(t, err)
	require.Len(t, res.Contents, 1)
	assert.Equal(t, "SECRET-NODE-BYTES", res.Contents[0].Text)
}
