package server

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

const indexTestDims = 8

// knownAxes gives the words the tests search for their own embedding axis.
var knownAxes = map[string]int{"cats": 0, "dogs": 1, "fish": 2, "they": 3}

// memStore is an in-memory embeddings.Store.
type memStore struct {
	// ctx is the owning test's context, for helpers that have no t.
	ctx  context.Context
	mu   sync.Mutex
	rows []embeddings.Row
	// failCount makes CountCID fail, as an unreachable store would.
	failCount atomic.Bool
}

func (m *memStore) Bootstrap(context.Context) error { return nil }

func (m *memStore) Replace(_ context.Context, cid string, rows []embeddings.Row) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows = slices.DeleteFunc(m.rows, func(r embeddings.Row) bool { return r.CID == cid })
	m.rows = append(m.rows, rows...)
	return nil
}

func cosineDistance(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	return 1 - dot/(math.Sqrt(na)*math.Sqrt(nb))
}

func (m *memStore) nearest(v []float32, k int, keep func(embeddings.Row) bool) []embeddings.Hit {
	m.mu.Lock()
	defer m.mu.Unlock()
	var hits []embeddings.Hit
	for _, r := range m.rows {
		if keep(r) {
			hits = append(hits, embeddings.Hit{CID: r.CID, Path: r.Path, MIME: r.MIME, ChunkIndex: r.ChunkIndex, Model: r.Model, Distance: cosineDistance(r.Vector, v)})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Distance < hits[j].Distance })
	return hits[:min(k, len(hits))]
}

func (m *memStore) Search(_ context.Context, v []float32, k int) ([]embeddings.Hit, error) {
	return m.nearest(v, k, func(embeddings.Row) bool { return true }), nil
}

func (m *memStore) SearchCID(_ context.Context, v []float32, cid string, k int) ([]embeddings.Hit, error) {
	return m.nearest(v, k, func(r embeddings.Row) bool { return r.CID == cid }), nil
}

func (m *memStore) DeleteCID(_ context.Context, cid string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	before := len(m.rows)
	m.rows = slices.DeleteFunc(m.rows, func(r embeddings.Row) bool { return r.CID == cid })
	return before - len(m.rows), nil
}

func (m *memStore) Count(context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.rows), nil
}

func (m *memStore) CountCID(_ context.Context, cid string) (int, error) {
	if m.failCount.Load() {
		return 0, fmt.Errorf("store is down")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.rows {
		if r.CID == cid {
			n++
		}
	}
	return n, nil
}

func (m *memStore) ListCIDs(_ context.Context, after string, limit int) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	set := map[string]struct{}{}
	for _, r := range m.rows {
		if r.CID > after {
			set[r.CID] = struct{}{}
		}
	}
	cids := make([]string, 0, len(set))
	for c := range set {
		cids = append(cids, c)
	}
	sort.Strings(cids)
	return cids[:min(limit, len(cids))], nil
}

func (m *memStore) CountCIDs(ctx context.Context) (int, error) {
	cids, err := m.ListCIDs(ctx, "", math.MaxInt32)
	return len(cids), err
}

// cids lists the CIDs that have rows.
func (m *memStore) cids() []string {
	cids, _ := m.ListCIDs(m.ctx, "", math.MaxInt32)
	return cids
}

// embedServer is an OpenAI-compatible /v1/embeddings server whose vector for a
// text is one-hot on an axis chosen by the text's first word.
type embedServer struct {
	*httptest.Server
	requests atomic.Int32
	down     atomic.Bool
	mu       sync.Mutex
	texts    []string
}

func newEmbedServer(t *testing.T) *embedServer {
	t.Helper()
	e := &embedServer{}
	e.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.requests.Add(1)
		if e.down.Load() {
			http.Error(w, `{"error":{"message":"model not loaded"}}`, http.StatusInternalServerError)
			return
		}
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		data := make([]item, 0, len(req.Input))
		for i, text := range req.Input {
			e.mu.Lock()
			e.texts = append(e.texts, text)
			e.mu.Unlock()
			first, _, _ := strings.Cut(text, " ")
			v := make([]float32, indexTestDims)
			if axis, ok := knownAxes[strings.ToLower(strings.TrimRight(first, ".,"))]; ok {
				v[axis] = 1
			} else {
				h := fnv.New32a()
				_, _ = h.Write([]byte(strings.ToLower(first)))
				v[4+h.Sum32()%4] = 1
			}
			data = append(data, item{i, v})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(e.Close)
	return e
}

func (e *embedServer) host() string { return strings.TrimPrefix(e.URL, "http://") }

// indexEnv is a server with the index tools over a content fake that also
// answers the pin commands, plus the parts a test inspects.
type indexEnv struct {
	t      *testing.T
	d      *contentFake
	s      *Server
	store  *memStore
	embed  *embedServer
	pins   *pinStore
	files  map[string][]byte // content paths the node can cat
	added  [][]byte
	gcKeys []string // keys the next repo/gc reports
	// covered makes pin/ls report a pin of another kind for these CIDs.
	covered map[string]bool
	// pinLsErr makes pin/ls fail for these CIDs.
	pinLsErr map[string]bool

	mu  sync.Mutex
	ids map[mcp.Session]Identity
	// catOffline records, per path, whether the last cat asked for offline.
	catOffline map[string]bool
}

type indexEnvOptions struct {
	policy  embeddings.Policy
	options Options
	// noEmbeddings leaves the embeddings feature unconfigured.
	noEmbeddings bool
}

func newIndexEnv(t *testing.T, o indexEnvOptions) *indexEnv {
	t.Helper()
	e := &indexEnv{t: t, store: &memStore{ctx: t.Context()}, embed: newEmbedServer(t), files: map[string][]byte{}, covered: map[string]bool{}, pinLsErr: map[string]bool{}, ids: map[mcp.Session]Identity{}}
	e.d = newContentFake(t)
	e.pins = installPins(e.d.fakeDaemon, nil)
	basePinLs := e.d.handle["pin/ls"]
	e.d.handle["pin/ls"] = func(args []string, opts map[string]any) (any, error) {
		c := rootOf(args[0])
		if e.pinLsErr[c] {
			return nil, &RPCError{Command: "pin/ls", Message: "pinner unavailable"}
		}
		if e.covered[c] {
			return map[string]any{"Keys": map[string]any{c: map[string]any{"Type": "indirect through bafyroot"}}}, nil
		}
		return basePinLs(args, opts)
	}
	e.d.upload["add"] = addHandler(&e.added)
	e.d.raw["cat"] = func(args []string, opts map[string]any, _ []byte) ([]byte, error) {
		offline, _ := opts["offline"].(bool)
		e.mu.Lock()
		if e.catOffline == nil {
			e.catOffline = map[string]bool{}
		}
		e.catOffline[args[0]] = offline
		e.mu.Unlock()
		data, ok := e.files[args[0]]
		if !ok {
			return nil, &RPCError{Command: "cat", Message: "path not found"}
		}
		if n, ok := opts["length"].(int64); ok && n < int64(len(data)) {
			data = data[:n]
		}
		return data, nil
	}
	e.d.handle["files/stat"] = func(args []string, _ map[string]any) (any, error) {
		data, ok := e.files[args[0]]
		if !ok {
			if strings.HasSuffix(args[0], "/dir") {
				return map[string]any{"Hash": "x", "Size": 0, "CumulativeSize": 0, "Blocks": 1, "Type": "directory"}, nil
			}
			return nil, &RPCError{Command: "files/stat", Message: "path not found"}
		}
		return map[string]any{"Hash": rootOf(args[0]), "Size": len(data), "CumulativeSize": len(data), "Blocks": 1, "Type": "file"}, nil
	}
	e.d.stream["repo/gc"] = func([]string, map[string]any) ([]any, error) {
		lines := make([]any, 0, len(e.gcKeys))
		for _, k := range e.gcKeys {
			lines = append(lines, map[string]any{"Key": map[string]any{"/": k}})
		}
		return lines, nil
	}
	opts := o.options
	if opts.Tools == nil {
		opts.Tools = []string{"content", "pin", "diag", "index", "destructive"}
	}
	e.s = mustNew(t, e.d, opts)
	e.s.MCP().AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			e.mu.Lock()
			id, ok := e.ids[req.GetSession()]
			e.mu.Unlock()
			if ok {
				ctx = WithIdentity(WithPrincipal(ctx, id.Principal), id)
			}
			return next(ctx, method, req)
		}
	})
	if !o.noEmbeddings {
		set := embeddings.Settings{
			Mode:     embeddings.ModeEmbedded,
			Embedder: embeddings.EmbedderSettings{BaseURL: e.embed.URL + "/v1", Model: "fake-embed", Dimensions: indexTestDims},
			Policy:   o.policy,
		}
		emb, err := embeddings.NewEmbedder(set.Embedder, nil)
		require.NoError(t, err)
		e.s.attachEmbeddings(embeddings.NewService(set, e.store, emb))
	}
	return e
}

// identity is a configured principal whose egress_ack is ack, granted every
// group and write on every group.
func indexIdentity(ack ...string) Identity {
	id := NewIdentity(Principal{ID: "indexer", Kind: KindHumanMCP}, []string{AllGroups}, []string{AllGroups}, []string{AllGroups})
	id.Configured = true
	id.EgressAck = ack
	return id
}

// connect opens a session as id (the operator when nil) whose client approves
// every confirmation.
func (e *indexEnv) connect(id *Identity) *mcp.ClientSession {
	e.t.Helper()
	return e.connectDeciding(id, func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	})
}

func (e *indexEnv) connectDeciding(id *Identity, decide func(*mcp.ElicitRequest) *mcp.ElicitResult) *mcp.ClientSession {
	e.t.Helper()
	ctx := e.t.Context()
	ct, st := mcp.NewInMemoryTransports()
	ss, err := e.s.MCP().Connect(ctx, st, nil)
	require.NoError(e.t, err)
	e.t.Cleanup(func() { _ = ss.Close() })
	if id != nil {
		e.mu.Lock()
		e.ids[ss] = *id
		e.mu.Unlock()
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, elicitOptions(decide)).Connect(ctx, ct, nil)
	require.NoError(e.t, err)
	e.t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// addText adds text through ipfs_add as cs and returns the decoded result.
func (e *indexEnv) addText(cs *mcp.ClientSession, text, filename string) *mcp.CallToolResult {
	e.t.Helper()
	args := map[string]any{"content": text, "pin": true, "provide": false}
	if filename != "" {
		args["filename"] = filename
	}
	return toolCall(e.t, cs, "ipfs_add", args)
}

// seed stores chunk rows for a CID without reading or embedding anything.
func (e *indexEnv) seed(cid string, chunks int) {
	e.t.Helper()
	rows := make([]embeddings.Row, 0, chunks)
	for i := range chunks {
		v := make([]float32, indexTestDims)
		v[i%indexTestDims] = 1
		rows = append(rows, embeddings.Row{CID: cid, Path: "/ipfs/" + cid, MIME: "text/plain", ChunkIndex: i, Model: "fake-embed", Vector: v})
	}
	require.NoError(e.t, e.store.Replace(e.t.Context(), cid, rows))
}

func structured(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

func warningsOf(res *mcp.CallToolResult) []string {
	raw, _ := json.Marshal(res.StructuredContent)
	var m struct {
		Warnings []string `json:"warnings"`
	}
	_ = json.Unmarshal(raw, &m)
	return m.Warnings
}
