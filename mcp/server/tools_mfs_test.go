package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mfsServer starts a server over a stateful MFS fake. The client approves
// every confirmation. Destructive tools are enabled when destructive is set.
func mfsServer(t *testing.T, destructive bool) (*mcp.ClientSession, *contentFake, *mfsFake) {
	t.Helper()
	d := newContentFake(t)
	m := installMFS(d)
	opts := Options{}
	if destructive {
		opts.Tools = []string{"mfs", "destructive"}
	}
	return contentServer(t, d, opts), d, m
}

func TestMFSWriteThenReadReturnsTheWrittenText(t *testing.T) {
	cs, d, m := mfsServer(t, false)

	w := decodeMap(t, "mfs_write", callOK(t, cs, "mfs_write", map[string]any{"path": "/a.txt", "content": "hello mfs"}))
	assert.Equal(t, "/a.txt", w["path"])
	assert.EqualValues(t, 9, w["bytes_written"])
	assert.Equal(t, cidOf([]byte("hello mfs")), w["cid"])
	assert.Equal(t, []byte("hello mfs"), m.files["/a.txt"])
	opts := d.lastCall("files/write").Opts
	assert.Equal(t, true, opts["create"])
	assert.Equal(t, true, opts["truncate"], "the file content is replaced")

	res := toolCall(t, cs, "mfs_read", map[string]any{"path": "/a.txt"})
	require.False(t, res.IsError)
	raw, _ := json.Marshal(res.StructuredContent)
	r := decodeMap(t, "mfs_read", raw)
	assert.Equal(t, "hello mfs", r["content"])
	assert.EqualValues(t, 9, r["size"])
	assert.EqualValues(t, 9, r["length"])
	assert.Equal(t, false, r["truncated"])
	assert.Equal(t, true, res.Meta[untrustedMetaKey], "file bytes are untrusted")

	// Overwriting replaces the content.
	callOK(t, cs, "mfs_write", map[string]any{"path": "/a.txt", "content": "v2"})
	r = decodeMap(t, "mfs_read", callOK(t, cs, "mfs_read", map[string]any{"path": "/a.txt"}))
	assert.Equal(t, "v2", r["content"])
}

func TestMFSWriteIsConfirmedAndDeclinedWritesNothing(t *testing.T) {
	d := newContentFake(t)
	m := installMFS(d)
	s := mustNew(t, d, Options{})
	cs := connectElicit(t, s, func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "decline"}
	})
	res := toolCall(t, cs, "mfs_write", map[string]any{"path": "/a.txt", "content": "x"})
	require.True(t, res.IsError)
	assert.Equal(t, CodeConfirmDeclined, decodeToolError(t, res).Code)
	assert.False(t, m.has("/a.txt"))
	assert.Zero(t, d.count("files/write"))
}

func TestMFSWriteBase64AndLimits(t *testing.T) {
	cs, _, m := mfsServer(t, false)
	payload := []byte{0xff, 0x00, 0x7f}
	callOK(t, cs, "mfs_write", map[string]any{"path": "/b.bin", "content": base64.StdEncoding.EncodeToString(payload), "encoding": "base64"})
	assert.Equal(t, payload, m.files["/b.bin"])

	big := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, maxContentBytes+1))
	res := toolCall(t, cs, "mfs_write", map[string]any{"path": "/big", "content": big, "encoding": "base64"})
	require.True(t, res.IsError)
	assert.Equal(t, CodeInvalidArgument, decodeToolError(t, res).Code)
	assert.False(t, m.has("/big"))
}

func TestMFSWriteParents(t *testing.T) {
	cs, _, m := mfsServer(t, false)
	res := toolCall(t, cs, "mfs_write", map[string]any{"path": "/x/y/z.txt", "content": "1"})
	require.True(t, res.IsError, "missing parents are an error by default")
	assert.Equal(t, CodeRPCError, decodeToolError(t, res).Code)

	callOK(t, cs, "mfs_write", map[string]any{"path": "/x/y/z.txt", "content": "1", "parents": true})
	assert.True(t, m.has("/x/y/z.txt"))
}

func TestMFSReadRangeCapAndKinds(t *testing.T) {
	cs, d, m := mfsServer(t, false)
	m.files["/n.txt"] = []byte("0123456789")
	m.dirs["/d"] = struct{}{}
	m.files["/bin"] = []byte{0xff, 0xfe}
	m.files["/huge"] = bytes.Repeat([]byte("a"), maxContentBytes+10)

	r := decodeMap(t, "mfs_read", callOK(t, cs, "mfs_read", map[string]any{"path": "/n.txt", "offset": 3, "length": 4}))
	assert.Equal(t, "3456", r["content"])
	assert.EqualValues(t, 3, r["offset"])
	assert.EqualValues(t, 10, r["size"])
	assert.Equal(t, false, r["truncated"])
	assert.EqualValues(t, 4, d.lastCall("files/read").Opts["count"])

	r = decodeMap(t, "mfs_read", callOK(t, cs, "mfs_read", map[string]any{"path": "/huge"}))
	assert.Equal(t, true, r["truncated"])
	assert.EqualValues(t, maxContentBytes, r["length"])
	assert.EqualValues(t, maxContentBytes+10, r["size"])

	r = decodeMap(t, "mfs_read", callOK(t, cs, "mfs_read", map[string]any{"path": "/bin"}))
	assert.Equal(t, "base64", r["encoding"])

	r = decodeMap(t, "mfs_read", callOK(t, cs, "mfs_read", map[string]any{"path": "/n.txt", "offset": 99}))
	assert.EqualValues(t, 0, r["length"])
	assert.Equal(t, "", r["content"])

	res := toolCall(t, cs, "mfs_read", map[string]any{"path": "/d"})
	require.True(t, res.IsError)
	assert.Equal(t, CodeInvalidArgument, decodeToolError(t, res).Code)
	res = toolCall(t, cs, "mfs_read", map[string]any{"path": "/missing"})
	require.True(t, res.IsError)
}

func TestMFSLsStatMkdirOutputsMatchSchemas(t *testing.T) {
	cs, _, m := mfsServer(t, false)
	m.files["/notes.txt"] = []byte("abc")

	mk := decodeMap(t, "mfs_mkdir", callOK(t, cs, "mfs_mkdir", map[string]any{"path": "/a/b/c", "parents": true}))
	assert.Equal(t, "/a/b/c", mk["path"])
	assert.NotEmpty(t, mk["cid"])

	ls := decodeMap(t, "mfs_ls", callOK(t, cs, "mfs_ls", map[string]any{}))
	assert.Equal(t, "/", ls["path"])
	var names []string
	for _, e := range ls["entries"].([]any) {
		names = append(names, e.(map[string]any)["name"].(string))
	}
	assert.Equal(t, []string{"a", "notes.txt"}, names)

	st := decodeMap(t, "mfs_stat", callOK(t, cs, "mfs_stat", map[string]any{"path": "/notes.txt"}))
	assert.Equal(t, "file", st["type"])
	assert.EqualValues(t, 3, st["size"])
	st = decodeMap(t, "mfs_stat", callOK(t, cs, "mfs_stat", map[string]any{"path": "/a"}))
	assert.Equal(t, "directory", st["type"])

	res := toolCall(t, cs, "mfs_mkdir", map[string]any{"path": "/a"})
	require.True(t, res.IsError, "an existing directory is an error without parents")
}

func TestMFSLsPaginates(t *testing.T) {
	cs, _, m := mfsServer(t, false)
	for _, n := range []string{"e", "a", "d", "b", "c"} {
		m.files["/"+n] = []byte(n)
	}
	var got []string
	cursor := ""
	for range 5 {
		args := map[string]any{"limit": 2}
		if cursor != "" {
			args["cursor"] = cursor
		}
		res := toolCall(t, cs, "mfs_ls", args)
		require.False(t, res.IsError)
		assert.Equal(t, true, res.Meta[untrustedMetaKey], "entry names are untrusted")
		raw, _ := json.Marshal(res.StructuredContent)
		out := decodeMap(t, "mfs_ls", raw)
		for _, e := range out["entries"].([]any) {
			got = append(got, e.(map[string]any)["name"].(string))
		}
		page := out["page"].(map[string]any)
		if page["has_more"] != true {
			break
		}
		cursor = page["next_cursor"].(string)
	}
	assert.Equal(t, []string{"a", "b", "c", "d", "e"}, got)
}

func TestMFSCopyAndMove(t *testing.T) {
	cs, d, m := mfsServer(t, false)
	m.files["/src.txt"] = []byte("data")

	cp := decodeMap(t, "mfs_cp", callOK(t, cs, "mfs_cp", map[string]any{"source": "/src.txt", "destination": "/copy.txt"}))
	assert.Equal(t, cidOf([]byte("data")), cp["cid"])
	assert.True(t, m.has("/src.txt") && m.has("/copy.txt"))
	assert.Nil(t, d.lastCall("files/cp").Opts["force"], "an existing destination is never overwritten")

	res := toolCall(t, cs, "mfs_cp", map[string]any{"source": "/src.txt", "destination": "/copy.txt"})
	require.True(t, res.IsError, "copy onto an existing path fails")

	mv := decodeMap(t, "mfs_mv", callOK(t, cs, "mfs_mv", map[string]any{"source": "/copy.txt", "destination": "/moved.txt"}))
	assert.Equal(t, "/moved.txt", mv["destination"])
	assert.False(t, m.has("/copy.txt"))
	assert.True(t, m.has("/moved.txt"))
}

func TestMFSRejectsContentPathsExceptAsCopySource(t *testing.T) {
	cs, d, m := mfsServer(t, false)
	c := testCID(t, 60)
	for tool, args := range map[string]map[string]any{
		"mfs_ls":    {"path": "/ipfs/" + c},
		"mfs_stat":  {"path": "/ipns/example.org"},
		"mfs_read":  {"path": "/ipfs/" + c},
		"mfs_write": {"path": "/ipfs/" + c, "content": "x"},
		"mfs_mkdir": {"path": "/ipld/x"},
		"mfs_mv":    {"source": "/ipfs/" + c, "destination": "/x"},
		"mfs_cp":    {"source": "/ipns/example.org", "destination": "/x"},
	} {
		res := toolCall(t, cs, tool, args)
		require.True(t, res.IsError, tool)
		assert.Equal(t, CodeInvalidArgument, decodeToolError(t, res).Code, tool)
	}
	assert.Zero(t, d.count("files/ls")+d.count("files/stat")+d.count("files/read")+d.count("files/write"))

	// Copying an /ipfs/ path into MFS is the supported way to import content.
	m.files["/ipfs/"+c] = []byte("imported")
	callOK(t, cs, "mfs_cp", map[string]any{"source": "/ipfs/" + c, "destination": "/imported"})
	assert.Equal(t, []string{"/ipfs/" + c, "/imported"}, d.lastCall("files/cp").Args)
}

func TestMFSRmIsOffByDefault(t *testing.T) {
	cs, _, _ := mfsServer(t, false)
	assert.NotContains(t, toolNames(t, cs), "mfs_rm")
	assert.Contains(t, toolNames(t, cs), "mfs_write")

	cs, _, _ = mfsServer(t, true)
	assert.Contains(t, toolNames(t, cs), "mfs_rm")
}

func TestMFSRmRemovesAfterConfirmation(t *testing.T) {
	d := newContentFake(t)
	m := installMFS(d)
	m.files["/gone.txt"] = []byte("x")
	var prompts []string
	s := mustNew(t, d, Options{Tools: []string{"mfs", "destructive"}})
	cs := connectElicit(t, s, func(r *mcp.ElicitRequest) *mcp.ElicitResult {
		prompts = append(prompts, r.Params.Message)
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	})

	out := decodeMap(t, "mfs_rm", callOK(t, cs, "mfs_rm", map[string]any{"path": "/gone.txt"}))
	assert.Equal(t, "/gone.txt", out["removed"])
	assert.False(t, m.has("/gone.txt"))
	require.Len(t, prompts, 1)
	assert.Contains(t, prompts[0], "/gone.txt")
}

func TestMFSRmUnconfirmedLeavesThePath(t *testing.T) {
	for name, tc := range map[string]struct {
		decide func(*mcp.ElicitRequest) *mcp.ElicitResult
		code   string
	}{
		"declined": {func(*mcp.ElicitRequest) *mcp.ElicitResult { return &mcp.ElicitResult{Action: "decline"} }, CodeConfirmDeclined},
		"cancelled": {func(*mcp.ElicitRequest) *mcp.ElicitResult { return &mcp.ElicitResult{Action: "cancel"} },
			CodeConfirmDeclined},
	} {
		t.Run(name, func(t *testing.T) {
			d := newContentFake(t)
			m := installMFS(d)
			m.files["/keep.txt"] = []byte("x")
			s := mustNew(t, d, Options{Tools: []string{"mfs", "destructive"}})
			cs := connectElicit(t, s, tc.decide)
			res := toolCall(t, cs, "mfs_rm", map[string]any{"path": "/keep.txt"})
			require.True(t, res.IsError)
			assert.Equal(t, tc.code, decodeToolError(t, res).Code)
			assert.True(t, m.has("/keep.txt"))
			assert.Zero(t, d.count("files/rm"))
		})
	}

	t.Run("client that cannot ask", func(t *testing.T) {
		d := newContentFake(t)
		m := installMFS(d)
		m.files["/keep.txt"] = []byte("x")
		s := mustNew(t, d, Options{Tools: []string{"mfs", "destructive"}})
		cs := connect(t, s)
		res := toolCall(t, cs, "mfs_rm", map[string]any{"path": "/keep.txt"})
		require.True(t, res.IsError)
		assert.Equal(t, CodeConfirmUnavailable, decodeToolError(t, res).Code)
		assert.True(t, m.has("/keep.txt"))
	})
}

func TestMFSRmDirectoryNeedsRecursive(t *testing.T) {
	cs, d, m := mfsServer(t, true)
	m.dirs["/dir"] = struct{}{}
	m.files["/dir/f"] = []byte("x")
	res := toolCall(t, cs, "mfs_rm", map[string]any{"path": "/dir"})
	require.True(t, res.IsError)
	assert.True(t, m.has("/dir/f"))
	assert.Equal(t, false, d.lastCall("files/rm").Opts["recursive"])

	callOK(t, cs, "mfs_rm", map[string]any{"path": "/dir", "recursive": true})
	assert.False(t, m.has("/dir"))
}

// The shapes below were recorded from a Kubo daemon (see TestMCPMFS in
// test/cli, which asserts the same decoding against a live daemon).
const (
	recordedFilesLs = `{"Entries":[{"Name":"a.txt","Type":0,"Size":9,"Hash":"QmTKk2jLo6HNrPSDaEAvRMtyDhNMB6ufW54VQc7JzpuJkk"},` +
		`{"Name":"sub","Type":1,"Size":0,"Hash":"QmUNLLsPACCz1vLxQVkXqqLX5R1X345qqfHbsf67hvA3Nn"}]}`
	recordedFilesLsEmpty = `{"Entries":null}`
	recordedFilesStat    = `{"Hash":"QmTKk2jLo6HNrPSDaEAvRMtyDhNMB6ufW54VQc7JzpuJkk","Size":9,"CumulativeSize":67,"Blocks":1,"Type":"file"}`
)

func TestMFSDecodesRecordedFilesResponses(t *testing.T) {
	d := newContentFake(t)
	var lsBody string
	d.handle["files/ls"] = func([]string, map[string]any) (any, error) {
		var v any
		require.NoError(t, json.Unmarshal([]byte(lsBody), &v))
		return v, nil
	}
	d.handle["files/stat"] = func([]string, map[string]any) (any, error) {
		var v any
		require.NoError(t, json.Unmarshal([]byte(recordedFilesStat), &v))
		return v, nil
	}
	cs := contentServer(t, d, Options{})

	lsBody = recordedFilesLs
	ls := decodeMap(t, "mfs_ls", callOK(t, cs, "mfs_ls", map[string]any{"path": "/"}))
	entries := ls["entries"].([]any)
	require.Len(t, entries, 2)
	assert.Equal(t, map[string]any{
		"name": "a.txt", "type": "file", "size": float64(9),
		"cid": "QmTKk2jLo6HNrPSDaEAvRMtyDhNMB6ufW54VQc7JzpuJkk",
	}, entries[0])
	assert.Equal(t, "directory", entries[1].(map[string]any)["type"])
	assert.Equal(t, map[string]any{"long": true}, d.lastCall("files/ls").Opts)

	lsBody = recordedFilesLsEmpty
	ls = decodeMap(t, "mfs_ls", callOK(t, cs, "mfs_ls", map[string]any{"path": "/empty"}))
	assert.Empty(t, ls["entries"])

	st := decodeMap(t, "mfs_stat", callOK(t, cs, "mfs_stat", map[string]any{"path": "/a.txt"}))
	assert.Equal(t, "file", st["type"])
	assert.EqualValues(t, 9, st["size"])
	assert.EqualValues(t, 67, st["cumulative_size"])
	assert.EqualValues(t, 1, st["blocks"])
}

func TestMFSToolsCarryContractAnnotations(t *testing.T) {
	cs, _, _ := mfsServer(t, true)
	res, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	byName := map[string]*mcp.Tool{}
	for _, tool := range res.Tools {
		byName[tool.Name] = tool
	}
	for _, name := range []string{"mfs_ls", "mfs_stat", "mfs_read", "mfs_write", "mfs_mkdir", "mfs_cp", "mfs_mv", "mfs_rm"} {
		require.Contains(t, byName, name)
		require.NotNil(t, byName[name].OutputSchema, name)
	}
	assert.True(t, byName["mfs_ls"].Annotations.ReadOnlyHint)
	assert.True(t, *byName["mfs_rm"].Annotations.DestructiveHint)
	assert.False(t, *byName["mfs_ls"].Annotations.OpenWorldHint)
}
