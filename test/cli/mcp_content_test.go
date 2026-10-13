package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mcpContentCap is the byte cap of ipfs_add and ipfs_cat.
const mcpContentCap = 1 << 20

func mcpContentNode(t *testing.T) *harness.Node {
	t.Helper()
	node := harness.NewT(t).NewNode().Init("--profile=test").StartDaemon()
	t.Cleanup(func() { node.StopDaemon() })
	return node
}

// onlyHash returns the CID `ipfs add` computes for the bytes, without storing
// them, with the given extra flags.
func mcpOnlyHash(node *harness.Node, data []byte, args ...string) string {
	return node.IPFSAdd(bytes.NewReader(data), append([]string{"--only-hash", "--pin=false"}, args...)...)
}

func mcpSC(res *mcp.CallToolResult) map[string]any {
	return res.StructuredContent.(map[string]any)
}

// mcpWriteFiles creates the files (slash-separated relative paths) under dir.
func mcpWriteFiles(dir string, files map[string]string) error {
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func TestMCPContent(t *testing.T) {
	t.Parallel()

	t.Run("ipfs_add yields the CID of ipfs add with default options", func(t *testing.T) {
		t.Parallel()
		node := mcpContentNode(t)
		e := &elicitor{action: "accept"}
		cs := mcpClient(t, node, e.options())

		text := "the default recipe stays the default"
		want := mcpOnlyHash(node, []byte(text))
		res := callMCP(t, cs, "ipfs_add", map[string]any{"content": text, "pin": false, "provide": false})
		requireMatchesOutputSchemaWithLinks(t, "ipfs_add", res)
		assert.Equal(t, want, mcpSC(res)["cid"])
		assert.EqualValues(t, len(text), mcpSC(res)["size"])
		require.Len(t, e.prompts, 1, "add is a confirmed write")
		assert.Contains(t, e.prompts[0], "pin=false")
		assert.False(t, isPinned(node, want), "pin=false does not pin")
		assert.Equal(t, text, node.RunIPFS("cat", want).Stdout.String())

		links := mcpResourceLinks(res)
		require.Len(t, links, 1)
		assert.Equal(t, "ipfs://"+want, links[0].URI)

		// A multi-chunk file hits the chunker default as well.
		big := bytes.Repeat([]byte("0123456789abcdef"), 40000) // 640000 bytes
		wantBig := mcpOnlyHash(node, big)
		res = callMCP(t, cs, "ipfs_add", map[string]any{
			"content": base64.StdEncoding.EncodeToString(big), "encoding": "base64", "pin": true, "provide": false,
		})
		requireMatchesOutputSchemaWithLinks(t, "ipfs_add", res)
		assert.Equal(t, wantBig, mcpSC(res)["cid"])
		assert.True(t, isPinned(node, wantBig), "pin=true pins recursively")
	})

	t.Run("ipfs_add recipe overrides match the same ipfs add flags", func(t *testing.T) {
		t.Parallel()
		node := mcpContentNode(t)
		cs := mcpClient(t, node, (&elicitor{action: "accept"}).options())
		data := bytes.Repeat([]byte("recipe override "), 30000)

		want := mcpOnlyHash(node, data, "--cid-version=1", "--raw-leaves", "--chunker=size-65536", "--hash=sha2-512")
		res := callMCP(t, cs, "ipfs_add", map[string]any{
			"content": string(data), "pin": false, "provide": false,
			"cid_version": 1, "raw_leaves": true, "chunker": "size-65536", "hash": "sha2-512",
		})
		requireMatchesOutputSchemaWithLinks(t, "ipfs_add", res)
		assert.Equal(t, want, mcpSC(res)["cid"])
	})

	t.Run("ipfs_add adds a path-like string literally and wraps a named file", func(t *testing.T) {
		t.Parallel()
		node := mcpContentNode(t)
		cs := mcpClient(t, node, (&elicitor{action: "accept"}).options())

		res := callMCP(t, cs, "ipfs_add", map[string]any{"content": "/etc/passwd", "pin": false, "provide": false})
		requireMatchesOutputSchemaWithLinks(t, "ipfs_add", res)
		cid := mcpSC(res)["cid"].(string)
		assert.Equal(t, "/etc/passwd", node.RunIPFS("cat", cid).Stdout.String(), "the literal string, not the host file")

		res = callMCP(t, cs, "ipfs_add", map[string]any{"content": "named", "filename": "note.txt", "pin": false, "provide": false})
		requireMatchesOutputSchemaWithLinks(t, "ipfs_add", res)
		root := mcpSC(res)["cid"].(string)
		assert.Equal(t, "note.txt", mcpSC(res)["name"])
		assert.Equal(t, "named", node.RunIPFS("cat", root+"/note.txt").Stdout.String())
		assert.Equal(t, "ipfs://"+root+"/note.txt", mcpResourceLinks(res)[0].URI)
	})

	t.Run("ipfs_add refuses oversize content and a declined add stores nothing", func(t *testing.T) {
		t.Parallel()
		node := mcpContentNode(t)

		accept := mcpClient(t, node, (&elicitor{action: "accept"}).options())
		over := callMCP(t, accept, "ipfs_add", map[string]any{
			"content": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), mcpContentCap+1)), "encoding": "base64",
			"pin": false, "provide": false,
		})
		assert.Equal(t, "invalid_argument", mcpToolErrorCode(t, over))

		data := []byte("never stored")
		cid := mcpOnlyHash(node, data)
		decline := &elicitor{action: "decline"}
		cs := mcpClient(t, node, decline.options())
		res := callMCP(t, cs, "ipfs_add", map[string]any{"content": string(data), "pin": true, "provide": false})
		assert.Equal(t, "confirmation_declined", mcpToolErrorCode(t, res))
		assert.False(t, hasLocalBlock(node, cid))
		assert.False(t, isPinned(node, cid))

		// A client that cannot ask cannot add either.
		silent := mcpClient(t, node, nil)
		res = callMCP(t, silent, "ipfs_add", map[string]any{"content": string(data), "pin": true, "provide": false})
		require.True(t, res.IsError)
		assert.False(t, hasLocalBlock(node, cid))
	})

	t.Run("ipfs_add provide reports whether the announcement happened", func(t *testing.T) {
		t.Parallel()
		node := mcpContentNode(t)
		cs := mcpClient(t, node, (&elicitor{action: "accept"}).options())
		res := callMCP(t, cs, "ipfs_add", map[string]any{"content": "announce me", "pin": true, "provide": true})
		requireMatchesOutputSchemaWithLinks(t, "ipfs_add", res)
		out := mcpSC(res)
		if out["provided"] == true {
			assert.Nil(t, out["warnings"])
		} else {
			require.NotEmpty(t, out["warnings"], "a missed announcement is reported, not silent")
		}
		assert.True(t, isPinned(node, out["cid"].(string)), "the add succeeded either way")
	})

	t.Run("ipfs_cat ranges, caps and marks content untrusted", func(t *testing.T) {
		t.Parallel()
		node := mcpContentNode(t)
		cs := mcpClient(t, node, nil)

		cid := node.IPFSAddStr("0123456789", "--pin=false")
		res := callMCP(t, cs, "ipfs_cat", map[string]any{"path": cid, "offset": 2, "length": 5})
		requireMatchesOutputSchema(t, "ipfs_cat", res)
		assert.Equal(t, "23456", mcpSC(res)["content"])
		assert.Equal(t, false, mcpSC(res)["truncated"])
		assert.Equal(t, true, res.Meta["ai.ipfs/untrusted-content"])

		whole := callMCP(t, cs, "ipfs_cat", map[string]any{"path": "/ipfs/" + cid})
		requireMatchesOutputSchema(t, "ipfs_cat", whole)
		assert.Equal(t, "0123456789", mcpSC(whole)["content"])

		large := node.IPFSAddDeterministicBytes(int64(mcpContentCap)+4096, "cat-cap", "--pin=false")
		capped := callMCP(t, cs, "ipfs_cat", map[string]any{"path": large, "encoding": "base64"})
		requireMatchesOutputSchema(t, "ipfs_cat", capped)
		assert.Equal(t, true, mcpSC(capped)["truncated"])
		assert.EqualValues(t, mcpContentCap, mcpSC(capped)["length"])
		raw, err := base64.StdEncoding.DecodeString(mcpSC(capped)["content"].(string))
		require.NoError(t, err)
		assert.Len(t, raw, mcpContentCap)

		// A later range reads the rest.
		rest := callMCP(t, cs, "ipfs_cat", map[string]any{"path": large, "offset": mcpContentCap, "encoding": "base64"})
		requireMatchesOutputSchema(t, "ipfs_cat", rest)
		assert.Equal(t, false, mcpSC(rest)["truncated"])
		assert.EqualValues(t, 4096, mcpSC(rest)["length"])

		bin := node.IPFSAdd(bytes.NewReader([]byte{0xff, 0xfe, 0x00}), "--pin=false")
		b := callMCP(t, cs, "ipfs_cat", map[string]any{"path": bin})
		requireMatchesOutputSchema(t, "ipfs_cat", b)
		assert.Equal(t, "base64", mcpSC(b)["encoding"], "invalid UTF-8 comes back as base64")
	})

	t.Run("ipfs_get and ipfs_ls over a directory", func(t *testing.T) {
		t.Parallel()
		node := mcpContentNode(t)
		cs := mcpClient(t, node, nil)

		dir := t.TempDir()
		require.NoError(t, mcpWriteFiles(dir, map[string]string{"a.txt": "alpha", "b.txt": "beta", "sub/c.txt": "gamma"}))
		root := strings.TrimSpace(node.RunIPFS("add", "-Q", "-r", "--pin=false", dir).Stdout.String())

		get := callMCP(t, cs, "ipfs_get", map[string]any{"path": root + "/a.txt"})
		requireMatchesOutputSchema(t, "ipfs_get", get)
		assert.Equal(t, "alpha", mcpSC(get)["content"])
		assert.Equal(t, "file", mcpSC(get)["type"])
		assert.Equal(t, "ipfs://"+root+"/a.txt", mcpSC(get)["resource"].(map[string]any)["uri"])
		assert.Equal(t, true, get.Meta["ai.ipfs/untrusted-content"])

		dget := callMCP(t, cs, "ipfs_get", map[string]any{"path": "/ipfs/" + root})
		requireMatchesOutputSchemaWithLinks(t, "ipfs_get", dget)
		assert.Equal(t, "directory", mcpSC(dget)["type"])
		assert.NotContains(t, mcpSC(dget), "content")
		links := mcpResourceLinks(dget)
		require.Len(t, links, 1)
		assert.Equal(t, "ipfs://"+root, links[0].URI)

		big := node.IPFSAddDeterministicBytes(200000, "get-big", "--pin=false")
		bget := callMCP(t, cs, "ipfs_get", map[string]any{"path": big})
		requireMatchesOutputSchemaWithLinks(t, "ipfs_get", bget)
		assert.NotContains(t, mcpSC(bget), "content", "over max_inline_bytes the content is a resource link")
		assert.Len(t, mcpResourceLinks(bget), 1)

		ls := callMCP(t, cs, "ipfs_ls", map[string]any{"path": root})
		requireMatchesOutputSchema(t, "ipfs_ls", ls)
		var names, types []string
		for _, e := range mcpSC(ls)["entries"].([]any) {
			names = append(names, e.(map[string]any)["name"].(string))
			types = append(types, e.(map[string]any)["type"].(string))
		}
		assert.Equal(t, []string{"a.txt", "b.txt", "sub"}, names)
		assert.Equal(t, []string{"file", "file", "directory"}, types)

		p1 := callMCP(t, cs, "ipfs_ls", map[string]any{"path": root, "limit": 2})
		requireMatchesOutputSchema(t, "ipfs_ls", p1)
		page := mcpSC(p1)["page"].(map[string]any)
		require.Equal(t, true, page["has_more"])
		p2 := callMCP(t, cs, "ipfs_ls", map[string]any{"path": root, "limit": 2, "cursor": page["next_cursor"]})
		requireMatchesOutputSchema(t, "ipfs_ls", p2)
		last := mcpSC(p2)["entries"].([]any)
		require.Len(t, last, 1)
		assert.Equal(t, "sub", last[0].(map[string]any)["name"])
	})

	t.Run("ipfs:// resources resolve files and directories", func(t *testing.T) {
		t.Parallel()
		node := mcpContentNode(t)
		cs := mcpClient(t, node, nil)

		dir := t.TempDir()
		require.NoError(t, mcpWriteFiles(dir, map[string]string{"hello.txt": "hello resource"}))
		root := strings.TrimSpace(node.RunIPFS("add", "-Q", "-r", "--pin=false", dir).Stdout.String())

		templates, err := cs.ListResourceTemplates(t.Context(), nil)
		require.NoError(t, err)
		var uris []string
		for _, rt := range templates.ResourceTemplates {
			uris = append(uris, rt.URITemplate)
		}
		assert.ElementsMatch(t, []string{"ipfs://{+path}", "ipns://{+path}"}, uris)

		file, err := cs.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "ipfs://" + root + "/hello.txt"})
		require.NoError(t, err)
		require.Len(t, file.Contents, 1)
		assert.Equal(t, "hello resource", file.Contents[0].Text)

		listing, err := cs.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "ipfs://" + root})
		require.NoError(t, err)
		var doc struct {
			Entries []struct{ Name string }
		}
		require.NoError(t, json.Unmarshal([]byte(listing.Contents[0].Text), &doc))
		require.Len(t, doc.Entries, 1)
		assert.Equal(t, "hello.txt", doc.Entries[0].Name)

		_, err = cs.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "ipfs://" + root + "/../x"})
		assert.Error(t, err)

		// The same bytes through an IPNS name published for the directory.
		node.RunIPFS("name", "publish", "--allow-offline", "--ttl=1s", "/ipfs/"+root)
		id := strings.TrimSpace(node.RunIPFS("config", "Identity.PeerID").Stdout.String())
		viaName, err := cs.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "ipns://" + id + "/hello.txt"})
		require.NoError(t, err)
		assert.Equal(t, "hello resource", viaName.Contents[0].Text)
	})
}
