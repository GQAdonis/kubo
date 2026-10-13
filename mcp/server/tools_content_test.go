package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contentServer starts a server over the fake whose client approves every
// confirmation.
func contentServer(t *testing.T, d *contentFake, opts Options) *mcp.ClientSession {
	t.Helper()
	s := mustNew(t, d, opts)
	return connectElicit(t, s, func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	})
}

// decodeMap decodes a structured result and checks it against the tool's
// output schema.
func decodeMap(t *testing.T, tool string, raw json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	validateOutput(t, tool, m)
	return m
}

func catFake(t *testing.T, files map[string][]byte) *contentFake {
	d := newContentFake(t)
	d.raw["cat"] = func(args []string, opts map[string]any, _ []byte) ([]byte, error) {
		data, ok := files[args[0]]
		if !ok {
			return nil, &RPCError{Command: "cat", Message: "path not found"}
		}
		if off, ok := opts["offset"].(int64); ok {
			data = data[min(off, int64(len(data))):]
		}
		if n, ok := opts["length"].(int64); ok && n < int64(len(data)) {
			data = data[:n]
		}
		return data, nil
	}
	return d
}

func TestIpfsAddSendsBytesAndKeepsDefaultRecipe(t *testing.T) {
	d := newContentFake(t)
	var added [][]byte
	d.upload["add"] = addHandler(&added)
	cs := contentServer(t, d, Options{})

	res := toolCall(t, cs, "ipfs_add", map[string]any{"content": "hello ipfs", "pin": true, "provide": false})
	require.False(t, res.IsError, "%+v", res.Content)
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	out := decodeMap(t, "ipfs_add", raw)

	assert.Equal(t, fakeCID(t, []byte("hello ipfs")), out["cid"])
	assert.EqualValues(t, 10, out["size"])
	assert.Equal(t, true, out["pinned"])
	assert.Equal(t, false, out["provided"])
	assert.Equal(t, [][]byte{[]byte("hello ipfs")}, added)

	// Only the explicit pin option is sent: the CID recipe stays the daemon's.
	opts := d.lastCall("add").Opts
	// fast-provide is off because the add default would announce the root.
	assert.Equal(t, map[string]any{"pin": true, "fast-provide-root": false, "fast-provide-dag": false}, opts)
	assert.Zero(t, d.count("provide/once"), "no announcement unless asked")

	// The result carries a text fallback and a resource_link to the content.
	require.Len(t, res.Content, 2)
	assert.JSONEq(t, string(raw), res.Content[0].(*mcp.TextContent).Text)
	link, ok := res.Content[1].(*mcp.ResourceLink)
	require.True(t, ok, "second block is a resource_link")
	assert.Equal(t, "ipfs://"+fakeCID(t, []byte("hello ipfs")), link.URI)
}

func TestIpfsAddRecipeOverridesAreOptIn(t *testing.T) {
	d := newContentFake(t)
	var added [][]byte
	d.upload["add"] = addHandler(&added)
	cs := contentServer(t, d, Options{})

	callOK(t, cs, "ipfs_add", map[string]any{
		"content": "x", "pin": false, "provide": false,
		"cid_version": 1, "raw_leaves": true, "chunker": "size-262144", "hash": "sha2-512",
	})
	assert.Equal(t, map[string]any{
		"pin": false, "fast-provide-root": false, "fast-provide-dag": false, "cid-version": 1, "raw-leaves": true, "chunker": "size-262144", "hash": "sha2-512",
	}, d.lastCall("add").Opts)
}

func TestIpfsAddPathLikeTextIsContentNotAFile(t *testing.T) {
	d := newContentFake(t)
	var added [][]byte
	d.upload["add"] = addHandler(&added)
	cs := contentServer(t, d, Options{})

	callOK(t, cs, "ipfs_add", map[string]any{"content": "/etc/passwd", "pin": false, "provide": false})
	assert.Equal(t, [][]byte{[]byte("/etc/passwd")}, added, "the literal string is the content")
	assert.Equal(t, []string{"add"}, d.calls[len(d.calls)-1:], "no other command ran")
}

func TestIpfsAddBase64FilenameAndProvide(t *testing.T) {
	d := newContentFake(t)
	var added [][]byte
	d.upload["add"] = addHandler(&added)
	d.handle["provide/once"] = func([]string, map[string]any) (any, error) { return map[string]any{"Queued": "x"}, nil }
	cs := contentServer(t, d, Options{})

	payload := []byte{0, 1, 2, 0xff, 0xfe}
	raw := callOK(t, cs, "ipfs_add", map[string]any{
		"content": base64.StdEncoding.EncodeToString(payload), "encoding": "base64",
		"filename": "blob.bin", "pin": true, "provide": true,
	})
	out := decodeMap(t, "ipfs_add", raw)
	assert.Equal(t, [][]byte{payload}, added)
	assert.Equal(t, true, d.lastCall("add").Opts["wrap-with-directory"])
	assert.Equal(t, "blob.bin", out["name"])
	assert.Equal(t, true, out["provided"])
	assert.Equal(t, 1, d.count("provide/once"))
	assert.Equal(t, []string{out["cid"].(string)}, d.lastCall("provide/once").Args)
	assert.True(t, strings.HasSuffix(out["resource"].(map[string]any)["uri"].(string), "/blob.bin"))
}

func TestIpfsAddProvideFailureIsAWarning(t *testing.T) {
	d := newContentFake(t)
	var added [][]byte
	d.upload["add"] = addHandler(&added)
	d.handle["provide/once"] = func([]string, map[string]any) (any, error) {
		return nil, &RPCError{Command: "provide/once", Message: "cannot provide: Provide.Enabled is false"}
	}
	cs := contentServer(t, d, Options{})

	out := decodeMap(t, "ipfs_add", callOK(t, cs, "ipfs_add", map[string]any{"content": "y", "pin": true, "provide": true}))
	assert.Equal(t, false, out["provided"])
	require.Len(t, out["warnings"], 1)
	assert.Contains(t, out["warnings"].([]any)[0], "Provide.Enabled")
}

func TestIpfsAddRejectsOversizedAndBadContent(t *testing.T) {
	d := newContentFake(t)
	var added [][]byte
	d.upload["add"] = addHandler(&added)
	cs := contentServer(t, d, Options{})

	big := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'a'}, maxContentBytes+1))
	res := toolCall(t, cs, "ipfs_add", map[string]any{"content": big, "encoding": "base64", "pin": true, "provide": false})
	require.True(t, res.IsError)
	assert.Equal(t, CodeInvalidArgument, decodeToolError(t, res).Code)

	res = toolCall(t, cs, "ipfs_add", map[string]any{"content": "***", "encoding": "base64", "pin": true, "provide": false})
	require.True(t, res.IsError)
	assert.Equal(t, CodeInvalidArgument, decodeToolError(t, res).Code)
	assert.Empty(t, added, "nothing reached the daemon")
}

func TestIpfsAddIsConfirmedBeforeItRuns(t *testing.T) {
	d := newContentFake(t)
	var added [][]byte
	d.upload["add"] = addHandler(&added)
	s := mustNew(t, d, Options{})
	cs := connectElicit(t, s, func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "decline"}
	})
	res := toolCall(t, cs, "ipfs_add", map[string]any{"content": "z", "pin": true, "provide": false})
	require.True(t, res.IsError)
	assert.Equal(t, CodeConfirmDeclined, decodeToolError(t, res).Code)
	assert.Empty(t, added)
}

func TestIpfsCatRangeCapAndUntrustedMarking(t *testing.T) {
	c := testCID(t, 1)
	d := catFake(t, map[string][]byte{"/ipfs/" + c: []byte("0123456789")})
	cs := contentServer(t, d, Options{})

	res := toolCall(t, cs, "ipfs_cat", map[string]any{"path": c, "offset": 2, "length": 4})
	require.False(t, res.IsError)
	raw, _ := json.Marshal(res.StructuredContent)
	out := decodeMap(t, "ipfs_cat", raw)
	assert.Equal(t, "2345", out["content"])
	assert.EqualValues(t, 4, out["length"])
	assert.EqualValues(t, 2, out["offset"])
	assert.Equal(t, false, out["truncated"])
	assert.Equal(t, "utf8", out["encoding"])
	assert.Equal(t, true, res.Meta[untrustedMetaKey], "result is marked untrusted")
	last := d.lastCall("cat")
	assert.Equal(t, []string{"/ipfs/" + c}, last.Args)
	assert.EqualValues(t, 4, last.Opts["length"])
	assert.EqualValues(t, 2, last.Opts["offset"])
}

func TestIpfsCatTruncatesAtTheCapWithoutLength(t *testing.T) {
	c := testCID(t, 2)
	d := catFake(t, map[string][]byte{"/ipfs/" + c: bytes.Repeat([]byte("a"), maxContentBytes+500)})
	cs := contentServer(t, d, Options{})

	out := decodeMap(t, "ipfs_cat", callOK(t, cs, "ipfs_cat", map[string]any{"path": "/ipfs/" + c}))
	assert.Equal(t, true, out["truncated"])
	assert.EqualValues(t, maxContentBytes, out["length"])
	assert.Len(t, out["content"], maxContentBytes)
	assert.EqualValues(t, maxContentBytes+1, d.lastCall("cat").Opts["length"], "asks for one byte more than the cap")
}

func TestIpfsCatExactlyCapIsNotTruncated(t *testing.T) {
	c := testCID(t, 3)
	d := catFake(t, map[string][]byte{"/ipfs/" + c: bytes.Repeat([]byte("a"), maxContentBytes)})
	cs := contentServer(t, d, Options{})
	out := decodeMap(t, "ipfs_cat", callOK(t, cs, "ipfs_cat", map[string]any{"path": c}))
	assert.Equal(t, false, out["truncated"])
}

func TestIpfsCatBinaryFallsBackToBase64(t *testing.T) {
	c := testCID(t, 4)
	bin := []byte{0xff, 0xfe, 0x00, 'a'}
	d := catFake(t, map[string][]byte{"/ipfs/" + c: bin})
	cs := contentServer(t, d, Options{})
	out := decodeMap(t, "ipfs_cat", callOK(t, cs, "ipfs_cat", map[string]any{"path": c}))
	assert.Equal(t, "base64", out["encoding"])
	assert.Equal(t, base64.StdEncoding.EncodeToString(bin), out["content"])

	out = decodeMap(t, "ipfs_cat", callOK(t, cs, "ipfs_cat", map[string]any{"path": c, "encoding": "base64"}))
	assert.Equal(t, "base64", out["encoding"])
}

func TestIpfsCatTruncationNeverSplitsARune(t *testing.T) {
	c := testCID(t, 5)
	// The multi-byte rune straddles the cap.
	data := append(bytes.Repeat([]byte("a"), maxContentBytes-1), []byte("é-tail")...)
	d := catFake(t, map[string][]byte{"/ipfs/" + c: data})
	cs := contentServer(t, d, Options{})
	out := decodeMap(t, "ipfs_cat", callOK(t, cs, "ipfs_cat", map[string]any{"path": c}))
	assert.Equal(t, "utf8", out["encoding"])
	assert.Equal(t, true, out["truncated"])
	assert.EqualValues(t, maxContentBytes-1, out["length"], "the partial rune is left out")
}

func TestIpfsCatMissingPathIsAToolError(t *testing.T) {
	d := catFake(t, nil)
	cs := contentServer(t, d, Options{})
	res := toolCall(t, cs, "ipfs_cat", map[string]any{"path": testCID(t, 6)})
	require.True(t, res.IsError)
	assert.Equal(t, CodeNotFound, decodeToolError(t, res).Code)
}

func getFake(t *testing.T, p string, st map[string]any, data []byte) *contentFake {
	d := catFake(t, map[string][]byte{p: data})
	d.handle["files/stat"] = func(args []string, _ map[string]any) (any, error) {
		require.Equal(t, []string{p}, args)
		return st, nil
	}
	return d
}

func TestIpfsGetInlinesSmallTextFiles(t *testing.T) {
	c := testCID(t, 10)
	p := "/ipfs/" + c + "/readme.md"
	d := getFake(t, p, map[string]any{"Hash": c, "Size": 12, "CumulativeSize": 40, "Blocks": 0, "Type": "file"}, []byte("# hello doc\n"))
	cs := contentServer(t, d, Options{})

	res := toolCall(t, cs, "ipfs_get", map[string]any{"path": c + "/readme.md"})
	require.False(t, res.IsError)
	raw, _ := json.Marshal(res.StructuredContent)
	out := decodeMap(t, "ipfs_get", raw)
	assert.Equal(t, "# hello doc\n", out["content"])
	assert.Equal(t, "file", out["type"])
	assert.Equal(t, "ipfs://"+c+"/readme.md", out["resource"].(map[string]any)["uri"])
	assert.Len(t, res.Content, 1, "inlined content needs no resource_link")
	assert.Equal(t, true, res.Meta[untrustedMetaKey])
}

func TestIpfsGetLargeFileIsAResourceLink(t *testing.T) {
	c := testCID(t, 11)
	p := "/ipfs/" + c
	d := getFake(t, p, map[string]any{"Hash": c, "Size": 100000, "CumulativeSize": 100100, "Blocks": 1, "Type": "file"}, nil)
	cs := contentServer(t, d, Options{})

	res := toolCall(t, cs, "ipfs_get", map[string]any{"path": "/ipfs/" + c})
	require.False(t, res.IsError)
	raw, _ := json.Marshal(res.StructuredContent)
	out := decodeMap(t, "ipfs_get", raw)
	assert.NotContains(t, out, "content")
	assert.Equal(t, true, out["truncated"])
	assert.Zero(t, d.count("cat"), "a file over the inline limit is not read")
	require.Len(t, res.Content, 2)
	link, ok := res.Content[1].(*mcp.ResourceLink)
	require.True(t, ok)
	assert.Equal(t, "ipfs://"+c, link.URI)
	require.NotNil(t, link.Size)
	assert.EqualValues(t, 100000, *link.Size)
}

func TestIpfsGetDirectoryAndZeroInlineLimit(t *testing.T) {
	c := testCID(t, 12)
	d := getFake(t, "/ipfs/"+c, map[string]any{"Hash": c, "Size": 0, "CumulativeSize": 300, "Blocks": 2, "Type": "directory"}, nil)
	cs := contentServer(t, d, Options{})
	res := toolCall(t, cs, "ipfs_get", map[string]any{"path": c})
	raw, _ := json.Marshal(res.StructuredContent)
	out := decodeMap(t, "ipfs_get", raw)
	assert.Equal(t, "directory", out["type"])
	require.Len(t, res.Content, 2)
	assert.IsType(t, &mcp.ResourceLink{}, res.Content[1])

	f := testCID(t, 13)
	d = getFake(t, "/ipfs/"+f, map[string]any{"Hash": f, "Size": 3, "CumulativeSize": 3, "Blocks": 0, "Type": "file"}, []byte("abc"))
	cs = contentServer(t, d, Options{})
	out = decodeMap(t, "ipfs_get", callOK(t, cs, "ipfs_get", map[string]any{"path": f, "max_inline_bytes": 0}))
	assert.NotContains(t, out, "content", "max_inline_bytes=0 inlines nothing")
}

func TestIpfsGetIPNSPathResolvesThroughStat(t *testing.T) {
	c := testCID(t, 14)
	d := getFake(t, "/ipns/example.org/a.txt", map[string]any{"Hash": c, "Size": 1, "CumulativeSize": 1, "Blocks": 0, "Type": "file"}, []byte("a"))
	cs := contentServer(t, d, Options{})
	out := decodeMap(t, "ipfs_get", callOK(t, cs, "ipfs_get", map[string]any{"path": "/ipns/example.org/a.txt"}))
	assert.Equal(t, "ipns://example.org/a.txt", out["resource"].(map[string]any)["uri"])
}

func TestIpfsLsPaginatesAndMapsTypes(t *testing.T) {
	root := testCID(t, 20)
	d := newContentFake(t)
	var links []any
	for i, typ := range []int{2, 1, 0, 5, 4, 3} {
		links = append(links, map[string]any{"Name": string(rune('a' + i)), "Hash": testCID(t, 30+i), "Size": 10 * i, "Type": typ, "Target": ""})
	}
	d.handle["ls"] = func(args []string, _ map[string]any) (any, error) {
		require.Equal(t, []string{"/ipfs/" + root}, args)
		return map[string]any{"Objects": []any{map[string]any{"Hash": root, "Links": links}}}, nil
	}
	cs := contentServer(t, d, Options{})

	var names, types []string
	cursor := ""
	for pages := 0; ; pages++ {
		require.Less(t, pages, 5)
		args := map[string]any{"path": root, "limit": 4}
		if cursor != "" {
			args["cursor"] = cursor
		}
		res := toolCall(t, cs, "ipfs_ls", args)
		require.False(t, res.IsError)
		assert.Equal(t, true, res.Meta[untrustedMetaKey], "entry names are untrusted")
		raw, _ := json.Marshal(res.StructuredContent)
		out := decodeMap(t, "ipfs_ls", raw)
		for _, e := range out["entries"].([]any) {
			names = append(names, e.(map[string]any)["name"].(string))
			types = append(types, e.(map[string]any)["type"].(string))
		}
		page := out["page"].(map[string]any)
		if page["has_more"] != true {
			break
		}
		cursor = page["next_cursor"].(string)
	}
	assert.Equal(t, []string{"a", "b", "c", "d", "e", "f"}, names)
	assert.Equal(t, []string{"file", "directory", "file", "directory", "symlink", "unknown"}, types)
}

func TestIpfsLsEmptyDirectory(t *testing.T) {
	d := newContentFake(t)
	d.handle["ls"] = func([]string, map[string]any) (any, error) {
		return map[string]any{"Objects": []any{map[string]any{"Hash": "x", "Links": nil}}}, nil
	}
	cs := contentServer(t, d, Options{})
	out := decodeMap(t, "ipfs_ls", callOK(t, cs, "ipfs_ls", map[string]any{"path": testCID(t, 21)}))
	assert.Empty(t, out["entries"])
	assert.Equal(t, false, out["page"].(map[string]any)["has_more"])
}

func TestIpfsLsRejectsBadLimit(t *testing.T) {
	cs := contentServer(t, newContentFake(t), Options{})
	res := toolCall(t, cs, "ipfs_ls", map[string]any{"path": testCID(t, 22), "cursor": "!!"})
	require.True(t, res.IsError)
	assert.Equal(t, CodeInvalidArgument, decodeToolError(t, res).Code)
}

// --- resources ------------------------------------------------------------

func readResource(t *testing.T, cs *mcp.ClientSession, uri string) (*mcp.ReadResourceResult, error) {
	t.Helper()
	return cs.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri})
}

func TestResourcesResolveIPFSAndIPNSURIs(t *testing.T) {
	c := testCID(t, 40)
	d := catFake(t, map[string][]byte{
		"/ipfs/" + c + "/a.txt":    []byte("file via ipfs"),
		"/ipns/example.org/b.txt":  []byte("file via ipns"),
		"/ipfs/" + c + "/blob.bin": {0xff, 0xfe, 0x01},
	})
	d.handle["files/stat"] = func(args []string, _ map[string]any) (any, error) {
		return map[string]any{"Hash": c, "Size": 13, "CumulativeSize": 13, "Blocks": 0, "Type": "file"}, nil
	}
	cs := contentServer(t, d, Options{})

	res, err := readResource(t, cs, "ipfs://"+c+"/a.txt")
	require.NoError(t, err)
	require.Len(t, res.Contents, 1)
	assert.Equal(t, "file via ipfs", res.Contents[0].Text)
	assert.Equal(t, "ipfs://"+c+"/a.txt", res.Contents[0].URI)
	assert.Equal(t, []string{"/ipfs/" + c + "/a.txt"}, d.lastCall("cat").Args)
	assert.Equal(t, true, res.Meta[untrustedMetaKey])

	res, err = readResource(t, cs, "ipns://example.org/b.txt")
	require.NoError(t, err)
	assert.Equal(t, "file via ipns", res.Contents[0].Text)
	assert.Equal(t, []string{"/ipns/example.org/b.txt"}, d.lastCall("cat").Args)

	res, err = readResource(t, cs, "ipfs://"+c+"/blob.bin")
	require.NoError(t, err)
	assert.Empty(t, res.Contents[0].Text)
	assert.Equal(t, []byte{0xff, 0xfe, 0x01}, res.Contents[0].Blob)
}

func TestResourceDirectoryIsAJSONListing(t *testing.T) {
	c := testCID(t, 41)
	d := newContentFake(t)
	d.handle["files/stat"] = func([]string, map[string]any) (any, error) {
		return map[string]any{"Hash": c, "Size": 0, "CumulativeSize": 5, "Blocks": 1, "Type": "directory"}, nil
	}
	d.handle["ls"] = func([]string, map[string]any) (any, error) {
		return map[string]any{"Objects": []any{map[string]any{"Hash": c, "Links": []any{
			map[string]any{"Name": "z.txt", "Hash": testCID(t, 42), "Size": 3, "Type": 2},
		}}}}, nil
	}
	cs := contentServer(t, d, Options{})
	res, err := readResource(t, cs, "ipfs://"+c)
	require.NoError(t, err)
	assert.Equal(t, "application/json", res.Contents[0].MIMEType)
	var doc struct {
		Path    string
		Entries []lsEntry
	}
	require.NoError(t, json.Unmarshal([]byte(res.Contents[0].Text), &doc))
	assert.Equal(t, "/ipfs/"+c, doc.Path)
	require.Len(t, doc.Entries, 1)
	assert.Equal(t, "z.txt", doc.Entries[0].Name)
}

func TestResourceTooLargeAndBadURIs(t *testing.T) {
	c := testCID(t, 43)
	d := newContentFake(t)
	d.handle["files/stat"] = func([]string, map[string]any) (any, error) {
		return map[string]any{"Hash": c, "Size": resourceMaxBytes + 1, "CumulativeSize": 1, "Blocks": 0, "Type": "file"}, nil
	}
	cs := contentServer(t, d, Options{})
	_, err := readResource(t, cs, "ipfs://"+c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ipfs_cat")
	assert.Zero(t, d.count("cat"))

	for _, uri := range []string{"ipfs://" + c + "/../x", "ipfs://" + c + "?x=1", "ipfs://" + c + "#frag"} {
		_, err := readResource(t, cs, uri)
		assert.Error(t, err, uri)
	}
	_, err = readResource(t, cs, "https://example.org/x")
	assert.Error(t, err)
}

func TestResourceTemplatesFollowTheContentGroup(t *testing.T) {
	cs := contentServer(t, newContentFake(t), Options{})
	res, err := cs.ListResourceTemplates(t.Context(), nil)
	require.NoError(t, err)
	var templates []string
	for _, rt := range res.ResourceTemplates {
		templates = append(templates, rt.URITemplate)
	}
	assert.ElementsMatch(t, []string{"ipfs://{+path}", "ipns://{+path}"}, templates)

	cs = contentServer(t, newContentFake(t), Options{Tools: []string{"node"}})
	_, err = readResource(t, cs, "ipfs://"+testCID(t, 44))
	assert.Error(t, err, "no content group, no content resources")
}

func TestContentToolsRegisteredWithContractAnnotations(t *testing.T) {
	cs := contentServer(t, newContentFake(t), Options{})
	res, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	byName := map[string]*mcp.Tool{}
	for _, tool := range res.Tools {
		byName[tool.Name] = tool
	}
	for _, name := range []string{"ipfs_add", "ipfs_cat", "ipfs_get", "ipfs_ls"} {
		require.Contains(t, byName, name)
		require.NotNil(t, byName[name].OutputSchema)
	}
	assert.True(t, byName["ipfs_cat"].Annotations.ReadOnlyHint)
	assert.False(t, byName["ipfs_add"].Annotations.ReadOnlyHint)
}
