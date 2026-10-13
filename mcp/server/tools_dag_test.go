package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rawFakeDaemon adds the raw request and response capability to a fakeDaemon.
type rawFakeDaemon struct {
	*fakeDaemon
	// raw answers a raw command with the bytes of its response.
	raw map[string]func(args []string, opts map[string]any, upload []byte) ([]byte, error)
	// uploads records the request body of each raw command.
	uploads map[string][]byte
}

func (r *rawFakeDaemon) CallRaw(_ context.Context, command string, args []string, opts map[string]any, upload []byte, fn func(io.Reader) error) error {
	r.mu.Lock()
	r.calls = append(r.calls, command)
	r.log = append(r.log, fakeCall{Command: command, Args: args, Opts: opts})
	if upload != nil {
		r.uploads[command] = upload
	}
	h, ok := r.raw[command]
	r.mu.Unlock()
	if !ok {
		return &RPCError{Command: command, Message: "fake: unexpected raw command"}
	}
	data, err := h(args, opts, upload)
	if err != nil {
		return err
	}
	return fn(bytes.NewReader(data))
}

func newRawFake(t *testing.T) *rawFakeDaemon {
	t.Helper()
	d := newFakeDaemon(t)
	d.handle = map[string]func([]string, map[string]any) (any, error){}
	return &rawFakeDaemon{
		fakeDaemon: d,
		raw:        map[string]func([]string, map[string]any, []byte) ([]byte, error){},
		uploads:    map[string][]byte{},
	}
}

// dagServer starts a server with the dag group over d; the client approves
// every confirmation.
func dagServer(t *testing.T, d Daemon, opts Options) *mcp.ClientSession {
	t.Helper()
	s := mustNew(t, d, opts)
	return connectElicit(t, s, func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	})
}

func linkCID(c string) map[string]any { return map[string]any{"/": c} }

func resourceLinks(res *mcp.CallToolResult) []*mcp.ResourceLink {
	var links []*mcp.ResourceLink
	for _, c := range res.Content {
		if l, ok := c.(*mcp.ResourceLink); ok {
			links = append(links, l)
		}
	}
	return links
}

func TestDagGetInlinesSmallNodes(t *testing.T) {
	c := testCID(t, 20)
	d := newRawFake(t)
	d.handle["dag/resolve"] = func(args []string, _ map[string]any) (any, error) {
		return map[string]any{"Cid": linkCID(c), "RemPath": ""}, nil
	}
	d.raw["dag/get"] = func([]string, map[string]any, []byte) ([]byte, error) {
		return []byte(`{"hello":"world"}` + "\n"), nil
	}
	cs := dagServer(t, d, Options{})

	res := toolCall(t, cs, "dag_get", map[string]any{"path": "/ipfs/" + c + "/hello"})
	require.False(t, res.IsError, "%+v", res.Content)
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	validateOutput(t, "dag_get", json.RawMessage(raw))
	var out dagGetOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, c, out.CID)
	assert.Equal(t, "raw", out.Codec)
	assert.False(t, out.Truncated)
	assert.JSONEq(t, `{"hello":"world"}`, string(out.Node))
	assert.Empty(t, resourceLinks(res), "an inlined node has no link")
	assert.Equal(t, "dag-json", d.log[len(d.log)-1].Opts["output-codec"])
}

func TestDagGetLinksLargeNodes(t *testing.T) {
	c := testCID(t, 21)
	d := newRawFake(t)
	d.handle["dag/resolve"] = func([]string, map[string]any) (any, error) {
		return map[string]any{"Cid": linkCID(c)}, nil
	}
	big := `{"data":"` + strings.Repeat("x", 4096) + `"}`
	d.raw["dag/get"] = func([]string, map[string]any, []byte) ([]byte, error) { return []byte(big), nil }
	cs := dagServer(t, d, Options{})

	res := toolCall(t, cs, "dag_get", map[string]any{"path": "/ipfs/" + c, "max_inline_bytes": 1024})
	require.False(t, res.IsError, "%+v", res.Content)
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	validateOutput(t, "dag_get", json.RawMessage(raw))
	var out dagGetOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.True(t, out.Truncated)
	assert.Empty(t, out.Node, "the large node is not inlined")
	require.NotNil(t, out.Resource)
	assert.Equal(t, "ipfs://"+c, out.Resource.URI)
	links := resourceLinks(res)
	require.Len(t, links, 1, "the link is also sent as a resource_link block")
	assert.Equal(t, "ipfs://"+c, links[0].URI)
	assert.NotContains(t, textOf(t, res), "xxxx", "the text fallback has no node data either")

	// A cap of zero always links.
	res = toolCall(t, cs, "dag_get", map[string]any{"path": "/ipfs/" + c, "max_inline_bytes": 0})
	require.False(t, res.IsError)
	assert.Len(t, resourceLinks(res), 1)
}

func textOf(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, res.Content)
	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	return text.Text
}

func TestDagGetUnknownPathIsAToolError(t *testing.T) {
	d := newRawFake(t)
	d.handle["dag/resolve"] = func([]string, map[string]any) (any, error) {
		return nil, &RPCError{Command: "dag/resolve", Message: "block was not found locally"}
	}
	cs := dagServer(t, d, Options{})
	res := toolCall(t, cs, "dag_get", map[string]any{"path": testCID(t, 22)})
	require.True(t, res.IsError)
	assert.Equal(t, CodeNotFound, decodeToolError(t, res).Code)
}

func TestDagGetWithoutResolveAuthorization(t *testing.T) {
	c := testCID(t, 32)
	d := newRawFake(t)
	d.handle["dag/resolve"] = func([]string, map[string]any) (any, error) {
		return nil, &RPCError{Command: "dag/resolve", Message: "not allowed", Code: cmds.ErrForbidden}
	}
	d.raw["dag/get"] = func([]string, map[string]any, []byte) ([]byte, error) { return []byte(`{"a":1}`), nil }
	cs := dagServer(t, d, Options{})

	// A bare CID needs no resolution: its own CID is the answer.
	raw := callOK(t, cs, "dag_get", map[string]any{"path": c})
	var out dagGetOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, c, out.CID)

	// A sub-path does, so the refusal is reported.
	res := toolCall(t, cs, "dag_get", map[string]any{"path": "/ipfs/" + c + "/a"})
	require.True(t, res.IsError)
	assert.Equal(t, CodePermissionDenied, decodeToolError(t, res).Code)
}

func TestDagStat(t *testing.T) {
	a, b := testCID(t, 23), testCID(t, 24)
	d := newRawFake(t)
	d.handle["dag/stat"] = func(args []string, opts map[string]any) (any, error) {
		return map[string]any{
			"UniqueBlocks": 5, "TotalSize": 1000, "SharedSize": 100, "Ratio": 1.1,
			"DagStats": []any{
				map[string]any{"Cid": a, "Size": 600, "NumBlocks": 3},
				map[string]any{"Cid": linkCID(b), "Size": 500, "NumBlocks": 3},
			},
		}, nil
	}
	cs := dagServer(t, d, Options{})
	raw := callOK(t, cs, "dag_stat", map[string]any{"paths": []string{a, "/ipfs/" + b}})
	validateOutput(t, "dag_stat", raw)
	var out dagStatOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, []dagStatEntry{{CID: a, Size: 600, NumBlocks: 3}, {CID: b, Size: 500, NumBlocks: 3}}, out.DAGs)
	assert.Equal(t, uint64(1000), out.TotalSize)
	assert.Equal(t, 5, out.UniqueBlocks)
	last := d.log[len(d.log)-1]
	assert.Equal(t, []string{"/ipfs/" + a, "/ipfs/" + b}, last.Args)
	assert.Equal(t, false, last.Opts["progress"])
}

func TestDagPut(t *testing.T) {
	c := testCID(t, 25)
	d := newRawFake(t)
	d.raw["dag/put"] = func([]string, map[string]any, []byte) ([]byte, error) {
		return []byte(`{"Cid":{"/":"` + c + `"}}` + "\n"), nil
	}
	cs := dagServer(t, d, Options{})

	data := `{"name":"mcp","n":[1,2,3]}`
	raw := callOK(t, cs, "dag_put", map[string]any{"data": data, "pin": true})
	validateOutput(t, "dag_put", raw)
	var out dagPutOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, dagPutOutput{CID: c, Pinned: true}, out)
	assert.Equal(t, data, string(d.uploads["dag/put"]))
	last := d.log[len(d.log)-1]
	assert.Equal(t, "dag-cbor", last.Opts["store-codec"], "dag-cbor is the default")
	assert.Equal(t, "dag-json", last.Opts["input-codec"])
	assert.Equal(t, true, last.Opts["pin"])

	raw = callOK(t, cs, "dag_put", map[string]any{"data": data, "pin": false, "store_codec": "dag-json"})
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.False(t, out.Pinned)
	assert.Equal(t, "dag-json", d.log[len(d.log)-1].Opts["store-codec"])
}

// failAfterReader emits its data, then fails the way the RPC client does when
// the daemon reports a stream error in the trailer.
type failAfterReader struct {
	data []byte
	err  error
}

func (r *failAfterReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

type streamFailDaemon struct {
	*rawFakeDaemon
	body io.Reader
}

func (d *streamFailDaemon) CallRaw(_ context.Context, command string, _ []string, _ map[string]any, _ []byte, fn func(io.Reader) error) error {
	d.mu.Lock()
	d.calls = append(d.calls, command)
	d.mu.Unlock()
	return fn(d.body)
}

func TestDagPutSurfacesErrorAfterCID(t *testing.T) {
	c := testCID(t, 26)
	d := &streamFailDaemon{
		rawFakeDaemon: newRawFake(t),
		body: &failAfterReader{
			data: []byte(`{"Cid":{"/":"` + c + `"}}` + "\n"),
			err:  errors.New("pin failed: commit aborted"),
		},
	}
	cs := dagServer(t, d, Options{})

	res := toolCall(t, cs, "dag_put", map[string]any{"data": `{"a":1}`, "pin": true})
	require.True(t, res.IsError, "a failure after the CID must not report success")
	assert.Contains(t, textOf(t, res), "commit aborted")
}

func TestDagPutRequiresExplicitPinAndValidJSON(t *testing.T) {
	d := newRawFake(t)
	cs := dagServer(t, d, Options{})

	res := toolCall(t, cs, "dag_put", map[string]any{"data": `{"a":1}`})
	require.True(t, res.IsError)
	assert.Equal(t, CodeInvalidArgument, decodeToolError(t, res).Code, "pin is required and explicit")

	res = toolCall(t, cs, "dag_put", map[string]any{"data": `{not json`, "pin": false})
	require.True(t, res.IsError)
	assert.Equal(t, CodeInvalidArgument, decodeToolError(t, res).Code)
	assert.Zero(t, d.count("dag/put"), "nothing is stored")
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func TestCarImportRequiresExplicitPinRoots(t *testing.T) {
	d := newRawFake(t)
	cs := dagServer(t, d, Options{})
	res := toolCall(t, cs, "car_import", map[string]any{"car": b64([]byte("not a real car"))})
	require.True(t, res.IsError)
	assert.Equal(t, CodeInvalidArgument, decodeToolError(t, res).Code)
	assert.Zero(t, d.count("dag/import"), "nothing is imported")
}

func TestCarImportOnlyAcceptsProvidedData(t *testing.T) {
	d := newRawFake(t)
	cs := dagServer(t, d, Options{})
	for _, path := range []string{"/etc/passwd", "../fixtures/example.car", "file:///tmp/x.car", "C:\\x.car"} {
		res := toolCall(t, cs, "car_import", map[string]any{"car": path, "pin_roots": true})
		require.True(t, res.IsError, path)
		assert.Equal(t, CodeInvalidArgument, decodeToolError(t, res).Code, path)
	}
	assert.Zero(t, d.count("dag/import"), "a path is never read; only base64 data is imported")
}

func TestCarImportPinsRootsAndReportsStats(t *testing.T) {
	root, other := testCID(t, 26), testCID(t, 27)
	d := newRawFake(t)
	d.raw["dag/import"] = func(_ []string, opts map[string]any, _ []byte) ([]byte, error) {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		_ = enc.Encode(map[string]any{"Root": map[string]any{"Cid": linkCID(root), "PinErrorMsg": ""}})
		_ = enc.Encode(map[string]any{"Root": map[string]any{"Cid": linkCID(other), "PinErrorMsg": "pin failed: block not found"}})
		_ = enc.Encode(map[string]any{"Stats": map[string]any{"BlockCount": 7, "BlockBytesCount": 700}})
		return buf.Bytes(), nil
	}
	cs := dagServer(t, d, Options{})

	car := []byte("pretend car bytes")
	raw := callOK(t, cs, "car_import", map[string]any{"car": b64(car), "pin_roots": true})
	validateOutput(t, "car_import", raw)
	var out carImportOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, []carImportRoot{
		{CID: root, Pinned: true},
		{CID: other, Pinned: false, PinError: "pin failed: block not found"},
	}, out.Roots)
	assert.Equal(t, 7, out.BlockCount)
	assert.Equal(t, car, d.uploads["dag/import"])

	opts := d.log[len(d.log)-1].Opts
	assert.Equal(t, true, opts["pin-roots"])
	assert.Equal(t, false, opts["fast-provide-root"], "an import does not announce its content")
	assert.Equal(t, false, opts["fast-provide-dag"])
}

func TestCarImportWithoutPinningLeavesRootsUnpinned(t *testing.T) {
	d := newRawFake(t)
	d.raw["dag/import"] = func([]string, map[string]any, []byte) ([]byte, error) {
		return []byte(`{"Stats":{"BlockCount":3,"BlockBytesCount":30}}` + "\n"), nil
	}
	cs := dagServer(t, d, Options{})
	raw := callOK(t, cs, "car_import", map[string]any{"car": b64([]byte("car")), "pin_roots": false})
	validateOutput(t, "car_import", raw)
	var out carImportOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Empty(t, out.Roots)
	assert.Equal(t, 3, out.BlockCount)
	assert.Equal(t, false, d.log[len(d.log)-1].Opts["pin-roots"])
}

func TestCarImportSizeLimit(t *testing.T) {
	d := newRawFake(t)
	cs := dagServer(t, d, Options{})
	res := toolCall(t, cs, "car_import", map[string]any{"car": b64(make([]byte, maxCarImportBytes+1)), "pin_roots": false})
	require.True(t, res.IsError)
	assert.Equal(t, codeSizeLimitExceeded, decodeToolError(t, res).Code)
	assert.Zero(t, d.count("dag/import"))

	// Exactly at the cap is accepted.
	d.raw["dag/import"] = func([]string, map[string]any, []byte) ([]byte, error) { return nil, nil }
	res = toolCall(t, cs, "car_import", map[string]any{"car": b64(make([]byte, maxCarImportBytes)), "pin_roots": false})
	assert.False(t, res.IsError, "%+v", res.Content)
}

func TestCarImportIsConfirmedAsAWrite(t *testing.T) {
	d := newRawFake(t)
	s := mustNew(t, d, Options{})
	plain := connect(t, s)
	res := toolCall(t, plain, "car_import", map[string]any{"car": b64([]byte("car")), "pin_roots": true})
	require.True(t, res.IsError)
	assert.Equal(t, CodeConfirmUnavailable, decodeToolError(t, res).Code)
	assert.Zero(t, d.count("dag/import"))
}

func TestCarExportInlineWithinCap(t *testing.T) {
	root := testCID(t, 28)
	car := []byte("small car")
	d := newRawFake(t)
	d.raw["dag/export"] = func(args []string, opts map[string]any, _ []byte) ([]byte, error) { return car, nil }
	cs := dagServer(t, d, Options{})

	res := toolCall(t, cs, "car_export", map[string]any{"root": root})
	require.False(t, res.IsError, "%+v", res.Content)
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	validateOutput(t, "car_export", json.RawMessage(raw))
	var out carExportOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.True(t, out.Inline)
	assert.Equal(t, b64(car), out.CarBase64)
	require.NotNil(t, out.Size)
	assert.Equal(t, int64(len(car)), *out.Size)
	assert.Empty(t, resourceLinks(res))
	assert.Equal(t, []string{root}, d.log[len(d.log)-1].Args)
}

func TestCarExportLinksWhenOverCap(t *testing.T) {
	root := testCID(t, 29)
	d := newRawFake(t)
	d.raw["dag/export"] = func([]string, map[string]any, []byte) ([]byte, error) {
		return bytes.Repeat([]byte("c"), 5000), nil
	}
	cs := dagServer(t, d, Options{})

	res := toolCall(t, cs, "car_export", map[string]any{"root": root, "max_inline_bytes": 4096})
	require.False(t, res.IsError, "%+v", res.Content)
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	validateOutput(t, "car_export", json.RawMessage(raw))
	var out carExportOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.False(t, out.Inline)
	assert.Empty(t, out.CarBase64, "a large CAR is never inlined")
	require.NotNil(t, out.Resource)
	assert.Equal(t, "ipfs://"+root, out.Resource.URI)
	links := resourceLinks(res)
	require.Len(t, links, 1)
	assert.Equal(t, "ipfs://"+root, links[0].URI)
	assert.NotContains(t, textOf(t, res), "cccc")

	// A cap of exactly the CAR size inlines it.
	res = toolCall(t, cs, "car_export", map[string]any{"root": root, "max_inline_bytes": 5000})
	require.False(t, res.IsError)
	assert.Empty(t, resourceLinks(res))
}

func TestCarExportResultStaysSmallForHugeCars(t *testing.T) {
	root := testCID(t, 30)
	d := newRawFake(t)
	d.raw["dag/export"] = func([]string, map[string]any, []byte) ([]byte, error) {
		return bytes.Repeat([]byte("c"), 1<<20), nil
	}
	cs := dagServer(t, d, Options{})
	res := toolCall(t, cs, "car_export", map[string]any{"root": root, "max_inline_bytes": 16})
	require.False(t, res.IsError)
	assert.Less(t, len(textOf(t, res)), 4096, "the result stays small however large the CAR is")
}

func TestDagToolsNeedARawDaemon(t *testing.T) {
	d := newFakeDaemon(t)
	d.handle = map[string]func([]string, map[string]any) (any, error){}
	cs := dagServer(t, d, Options{})
	res := toolCall(t, cs, "car_export", map[string]any{"root": testCID(t, 31)})
	require.True(t, res.IsError)
	assert.Equal(t, CodeToolUnavailable, decodeToolError(t, res).Code)
}

func TestCidString(t *testing.T) {
	assert.Equal(t, "bafy", cidString(json.RawMessage(`"bafy"`)))
	assert.Equal(t, "bafy", cidString(json.RawMessage(`{"/":"bafy"}`)))
	assert.Empty(t, cidString(json.RawMessage(`42`)))
	assert.Empty(t, cidString(nil))
}

func TestResourceURI(t *testing.T) {
	assert.Equal(t, "ipfs://bafy/a/b", resourceURI("/ipfs/bafy/a/b", "x"))
	assert.Equal(t, "ipns://k51/a", resourceURI("/ipns/k51/a", "x"))
	assert.Equal(t, "ipfs://cid", resourceURI("/ipld/cid", "cid"))
	assert.Equal(t, "ipfs://bafy", resourceURI("bafy", "x"))
}
