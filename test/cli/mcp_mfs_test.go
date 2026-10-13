package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mcpMFSNode(t *testing.T) *harness.Node {
	t.Helper()
	node := harness.NewT(t).NewNode().Init("--profile=test").StartDaemon()
	t.Cleanup(func() { node.StopDaemon() })
	return node
}

func mfsExists(node *harness.Node, p string) bool {
	return node.RunIPFS("files", "stat", p).ExitCode() == 0
}

func mfsHash(node *harness.Node, p string) string {
	return strings.TrimSpace(node.RunIPFS("files", "stat", "--hash", p).Stdout.String())
}

func TestMCPMFS(t *testing.T) {
	t.Parallel()

	t.Run("write is confirmed, then read ls and stat decode against a daemon", func(t *testing.T) {
		t.Parallel()
		node := mcpMFSNode(t)
		e := &elicitor{action: "accept"}
		cs := mcpClient(t, node, e.options())

		mk := callMCP(t, cs, "mfs_mkdir", map[string]any{"path": "/notes"})
		requireMatchesOutputSchema(t, "mfs_mkdir", mk)
		assert.Equal(t, mfsHash(node, "/notes"), mcpSC(mk)["cid"])
		assert.True(t, mfsExists(node, "/notes"))

		w := callMCP(t, cs, "mfs_write", map[string]any{"path": "/notes/a.txt", "content": "hello mfs"})
		requireMatchesOutputSchema(t, "mfs_write", w)
		require.Len(t, e.prompts, 2, "mkdir and write are each confirmed")
		assert.Contains(t, e.prompts[1], "/notes/a.txt")
		assert.EqualValues(t, 9, mcpSC(w)["bytes_written"])
		assert.Equal(t, mfsHash(node, "/notes/a.txt"), mcpSC(w)["cid"])
		assert.Equal(t, "hello mfs", node.RunIPFS("files", "read", "/notes/a.txt").Stdout.String())

		r := callMCP(t, cs, "mfs_read", map[string]any{"path": "/notes/a.txt"})
		requireMatchesOutputSchema(t, "mfs_read", r)
		assert.Equal(t, "hello mfs", mcpSC(r)["content"])
		assert.EqualValues(t, 9, mcpSC(r)["size"])
		assert.Equal(t, true, r.Meta["ai.ipfs/untrusted-content"])
		assert.Len(t, e.prompts, 2, "reads never prompt")

		part := callMCP(t, cs, "mfs_read", map[string]any{"path": "/notes/a.txt", "offset": 6, "length": 3})
		requireMatchesOutputSchema(t, "mfs_read", part)
		assert.Equal(t, "mfs", mcpSC(part)["content"])

		ls := callMCP(t, cs, "mfs_ls", map[string]any{"path": "/notes"})
		requireMatchesOutputSchema(t, "mfs_ls", ls)
		entries := mcpSC(ls)["entries"].([]any)
		require.Len(t, entries, 1)
		entry := entries[0].(map[string]any)
		assert.Equal(t, "a.txt", entry["name"])
		assert.Equal(t, "file", entry["type"])
		assert.EqualValues(t, 9, entry["size"])
		assert.Equal(t, mfsHash(node, "/notes/a.txt"), entry["cid"])

		root := callMCP(t, cs, "mfs_ls", map[string]any{})
		requireMatchesOutputSchema(t, "mfs_ls", root)
		assert.Equal(t, "directory", mcpSC(root)["entries"].([]any)[0].(map[string]any)["type"])

		st := callMCP(t, cs, "mfs_stat", map[string]any{"path": "/notes"})
		requireMatchesOutputSchema(t, "mfs_stat", st)
		assert.Equal(t, "directory", mcpSC(st)["type"])
		assert.Equal(t, mfsHash(node, "/notes"), mcpSC(st)["cid"])

		empty := callMCP(t, cs, "mfs_mkdir", map[string]any{"path": "/a/b/c", "parents": true})
		requireMatchesOutputSchema(t, "mfs_mkdir", empty)
		emptyLs := callMCP(t, cs, "mfs_ls", map[string]any{"path": "/a/b/c"})
		requireMatchesOutputSchema(t, "mfs_ls", emptyLs)
		assert.Empty(t, mcpSC(emptyLs)["entries"], "an empty directory decodes to an empty list")
	})

	t.Run("binary and large content", func(t *testing.T) {
		t.Parallel()
		node := mcpMFSNode(t)
		cs := mcpClient(t, node, (&elicitor{action: "accept"}).options())

		callMCP(t, cs, "mfs_write", map[string]any{"path": "/bin", "content": "//4A", "encoding": "base64"})
		assert.Equal(t, []byte{0xff, 0xfe, 0x00}, []byte(node.RunIPFS("files", "read", "/bin").Stdout.String()))
		r := callMCP(t, cs, "mfs_read", map[string]any{"path": "/bin"})
		requireMatchesOutputSchema(t, "mfs_read", r)
		assert.Equal(t, "base64", mcpSC(r)["encoding"])
		assert.Equal(t, "//4A", mcpSC(r)["content"])

		big := bytes.Repeat([]byte("z"), mcpContentCap+100)
		node.PipeStrToIPFS(string(big), "files", "write", "--create", "/big")
		capped := callMCP(t, cs, "mfs_read", map[string]any{"path": "/big"})
		requireMatchesOutputSchema(t, "mfs_read", capped)
		assert.Equal(t, true, mcpSC(capped)["truncated"])
		assert.EqualValues(t, mcpContentCap, mcpSC(capped)["length"])
		assert.EqualValues(t, mcpContentCap+100, mcpSC(capped)["size"])
	})

	t.Run("cp mv and content paths", func(t *testing.T) {
		t.Parallel()
		node := mcpMFSNode(t)
		cs := mcpClient(t, node, (&elicitor{action: "accept"}).options())

		cid := node.IPFSAddStr("imported content", "--pin=false")
		cp := callMCP(t, cs, "mfs_cp", map[string]any{"source": "/ipfs/" + cid, "destination": "/imported.txt"})
		requireMatchesOutputSchema(t, "mfs_cp", cp)
		assert.Equal(t, cid, mcpSC(cp)["cid"])
		assert.Equal(t, "imported content", node.RunIPFS("files", "read", "/imported.txt").Stdout.String())

		again := callMCP(t, cs, "mfs_cp", map[string]any{"source": "/ipfs/" + cid, "destination": "/imported.txt"})
		require.True(t, again.IsError, "an existing destination is not overwritten")

		mv := callMCP(t, cs, "mfs_mv", map[string]any{"source": "/imported.txt", "destination": "/renamed.txt"})
		requireMatchesOutputSchema(t, "mfs_mv", mv)
		assert.False(t, mfsExists(node, "/imported.txt"))
		assert.True(t, mfsExists(node, "/renamed.txt"))

		for _, tool := range []string{"mfs_ls", "mfs_stat", "mfs_read"} {
			res := callMCP(t, cs, tool, map[string]any{"path": "/ipfs/" + cid})
			assert.Equal(t, "invalid_argument", mcpToolErrorCode(t, res), tool+" is MFS only")
		}
	})

	t.Run("mfs_rm is off by default and confirmed when enabled", func(t *testing.T) {
		t.Parallel()
		node := mcpMFSNode(t)
		node.PipeStrToIPFS("keep me", "files", "write", "--create", "/keep.txt")
		node.PipeStrToIPFS("drop me", "files", "write", "--create", "/drop.txt")

		plain := mcpClient(t, node, nil)
		assert.NotContains(t, mcpNames(t, plain), "mfs_rm")
		assert.Contains(t, mcpNames(t, plain), "mfs_write")

		declines := &elicitor{action: "decline"}
		declining := mcpClient(t, node, declines.options(), "--tools", "destructive")
		require.Contains(t, mcpNames(t, declining), "mfs_rm")
		res := callMCP(t, declining, "mfs_rm", map[string]any{"path": "/keep.txt"})
		assert.Equal(t, "confirmation_declined", mcpToolErrorCode(t, res))
		require.Len(t, declines.prompts, 1)
		assert.Contains(t, declines.prompts[0], "/keep.txt")
		assert.True(t, mfsExists(node, "/keep.txt"), "an unconfirmed removal leaves the path")

		silent := mcpClient(t, node, nil, "--tools", "destructive")
		res = callMCP(t, silent, "mfs_rm", map[string]any{"path": "/keep.txt"})
		require.True(t, res.IsError)
		assert.True(t, mfsExists(node, "/keep.txt"))

		approves := &elicitor{action: "accept"}
		approving := mcpClient(t, node, approves.options(), "--tools", "destructive")
		rm := callMCP(t, approving, "mfs_rm", map[string]any{"path": "/drop.txt"})
		requireMatchesOutputSchema(t, "mfs_rm", rm)
		assert.Equal(t, "/drop.txt", mcpSC(rm)["removed"])
		assert.False(t, mfsExists(node, "/drop.txt"))
		assert.True(t, mfsExists(node, "/keep.txt"))
	})
}
