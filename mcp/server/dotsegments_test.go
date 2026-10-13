package server

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRejectDotSegments(t *testing.T) {
	for _, tc := range []struct {
		name string
		args string
		bad  bool
	}{
		{"mfs dotdot to ipns", `{"path":"/a/../ipns/x.example"}`, true},
		{"mfs trailing dot", `{"path":"/a/."}`, true},
		{"copy source sub-path", `{"source":"/ipfs/bafy/../x","destination":"/d"}`, true},
		{"destination", `{"source":"/s","destination":"/d/../e"}`, true},
		{"content path list", `{"paths":["/ipfs/bafy/a","/ipfs/bafy/../b"]}`, true},
		{"bare cid sub-path", `{"path":"bafy/./x"}`, true},
		{"clean", `{"path":"/a/b.c/..d/e.","paths":["x"],"source":"/ipfs/bafy/a"}`, false},
		{"non-path fields ignored", `{"content":"../x","name":".."}`, false},
		{"empty", ``, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := rejectDotSegments(json.RawMessage(tc.args))
			if !tc.bad {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			var te *ToolError
			require.ErrorAs(t, err, &te)
			assert.Equal(t, CodeInvalidArgument, te.Code)
		})
	}
}

func TestDotSegmentPathsNeverReachTheDaemon(t *testing.T) {
	cs, d, _ := mfsServer(t, false)
	for tool, args := range map[string]map[string]any{
		"mfs_ls":   {"path": "/a/../ipns/x.example"},
		"mfs_stat": {"path": "/a/./b"},
		"mfs_cp":   {"source": "/ipfs/" + testCID(t, 61) + "/../x", "destination": "/y"},
		"mfs_mv":   {"source": "/a", "destination": "/b/../ipns/x.example"},
		"ipfs_cat": {"path": "/ipfs/" + testCID(t, 62) + "/a/../b"},
	} {
		res := toolCall(t, cs, tool, args)
		require.True(t, res.IsError, tool)
		assert.Equal(t, CodeInvalidArgument, decodeToolError(t, res).Code, tool)
	}
	for _, c := range []string{"files/ls", "files/stat", "files/cp", "files/mv", "cat"} {
		assert.Zero(t, d.count(c), c)
	}
}
