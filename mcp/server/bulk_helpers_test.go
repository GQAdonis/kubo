package server

import (
	"context"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// fakeNode is the content a bulk run walks: pins, UnixFS directories, MFS and
// the blocks the node holds. It answers the daemon commands the run uses and
// refuses to fetch what the node does not hold, as an offline read does.
type fakeNode struct {
	mu sync.Mutex
	// files maps a CID to its bytes.
	files map[string][]byte
	// absent are CIDs the network has but this node does not hold.
	absent map[string]bool
	// partial are files whose root block is here and whose data is not.
	partial map[string]bool
	// other maps a CID to a stat type that is not file or directory.
	other map[string]string
	// dirs maps a directory CID to its entries.
	dirs map[string][]nodeEntry
	// recursive and direct are the pinned roots.
	recursive, direct []string
	// mfs maps an MFS directory path to its entries.
	mfs map[string][]nodeMFS
}

type nodeEntry struct{ Name, CID string }

type nodeMFS struct {
	Name string
	CID  string
	Dir  bool
}

func newFakeNode() *fakeNode {
	return &fakeNode{
		files: map[string][]byte{}, absent: map[string]bool{}, partial: map[string]bool{}, other: map[string]string{},
		dirs: map[string][]nodeEntry{}, mfs: map[string][]nodeMFS{},
	}
}

// file stores bytes and returns their CID.
func (n *fakeNode) file(data string) string {
	c := cidOf([]byte(data))
	n.files[c] = []byte(data)
	return c
}

// dir stores a directory of the entries and returns its CID.
func (n *fakeNode) dir(name string, entries ...nodeEntry) string {
	c := cidOf([]byte("dir:" + name))
	n.dirs[c] = entries
	return c
}

func (n *fakeNode) pinRecursive(cids ...string) { n.recursive = append(n.recursive, cids...) }
func (n *fakeNode) pinDirect(cids ...string)    { n.direct = append(n.direct, cids...) }

const offlineMessage = "block was not found locally (offline): ipld: could not find "

// mfsCID is the CID the fake gives the MFS directory at path.
func mfsCID(path string) string { return cidOf([]byte("mfsdir:" + path)) }

// buildMFS turns the MFS tree in n.mfs into directories of the node, so the walk
// reads it as /ipfs/<root>/<relative path> like the real one. The root always
// exists.
func (n *fakeNode) buildMFS() {
	if _, ok := n.mfs["/"]; !ok {
		n.mfs["/"] = nil
	}
	for dir, entries := range n.mfs {
		ents := make([]nodeEntry, 0, len(entries))
		for _, e := range entries {
			c := e.CID
			if e.Dir && c == "" {
				c = mfsCID(path.Join(dir, e.Name))
				if _, ok := n.mfs[path.Join(dir, e.Name)]; !ok {
					n.dirs[c] = []nodeEntry{}
				}
			}
			ents = append(ents, nodeEntry{Name: e.Name, CID: c})
		}
		n.dirs[mfsCID(dir)] = ents
	}
}

// resolve follows an /ipfs/<cid>/a/b path through the node's directories and
// answers what an offline resolve does: an error when a block on the way is not
// held.
func (n *fakeNode) resolve(p string) (string, error) {
	parts := strings.Split(strings.TrimPrefix(p, "/ipfs/"), "/")
	c := parts[0]
	for _, seg := range parts[1:] {
		if seg == "" {
			continue
		}
		if n.absent[c] {
			return "", &RPCError{Command: "resolve", Message: offlineMessage + c}
		}
		next := ""
		for _, en := range n.dirs[c] {
			if en.Name == seg {
				next = en.CID
				break
			}
		}
		if next == "" {
			return "", &RPCError{Command: "resolve", Message: "no link named " + seg + " under " + c}
		}
		c = next
	}
	return c, nil
}

// install replaces the environment's commands with the node's.
func (n *fakeNode) install(e *indexEnv) {
	n.buildMFS()
	h := e.d.handle
	h["pin/ls"] = func(_ []string, opts map[string]any) (any, error) {
		typ, _ := opts["type"].(string)
		keys := map[string]any{}
		src := n.recursive
		if typ == "direct" {
			src = n.direct
		}
		for _, c := range src {
			keys[c] = map[string]any{"Type": typ}
		}
		return map[string]any{"Keys": keys}, nil
	}
	h["files/stat"] = func(args []string, opts map[string]any) (any, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		if args[0] == "/" {
			// The MFS root is a node the daemon holds in memory.
			c := mfsCID("/")
			return map[string]any{"Hash": c, "Size": 0, "CumulativeSize": 0, "Blocks": len(n.dirs[c]), "Type": "directory"}, nil
		}
		offline, _ := opts["offline"].(bool)
		c, err := n.resolve(args[0])
		if err != nil {
			return nil, err
		}
		if !offline && n.absent[c] {
			return nil, &RPCError{Command: "files/stat", Message: "the fake would have fetched " + c}
		}
		switch {
		case n.absent[c]:
			return nil, &RPCError{Command: "files/stat", Message: offlineMessage + c}
		case n.other[c] != "":
			return map[string]any{"Hash": c, "Size": 0, "CumulativeSize": 0, "Blocks": 0, "Type": n.other[c]}, nil
		case n.dirs[c] != nil:
			return map[string]any{"Hash": c, "Size": 0, "CumulativeSize": 0, "Blocks": len(n.dirs[c]), "Type": "directory"}, nil
		}
		data, ok := n.files[c]
		if !ok {
			return nil, &RPCError{Command: "files/stat", Message: "path not found"}
		}
		return map[string]any{"Hash": c, "Size": len(data), "CumulativeSize": len(data), "Blocks": 1, "Type": "file"}, nil
	}
	h["ls"] = func(args []string, opts map[string]any) (any, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		c, err := n.resolve(args[0])
		if err != nil {
			return nil, err
		}
		if offline, _ := opts["offline"].(bool); !offline && n.absent[c] {
			return nil, &RPCError{Command: "ls", Message: "the fake would have fetched " + c}
		}
		if n.absent[c] {
			return nil, &RPCError{Command: "ls", Message: offlineMessage + c}
		}
		links := []map[string]any{}
		for _, en := range n.dirs[c] {
			typ := 2
			if n.dirs[en.CID] != nil {
				typ = 1
			}
			links = append(links, map[string]any{"Name": en.Name, "Hash": en.CID, "Size": len(n.files[en.CID]), "Type": typ})
		}
		return map[string]any{"Objects": []any{map[string]any{"Hash": "/ipfs/" + c, "Links": links}}}, nil
	}
	// files/ls loads MFS through the online DAG service, which an offline flag
	// does not reach: the walk must never call it.
	h["files/ls"] = func([]string, map[string]any) (any, error) {
		return nil, &RPCError{Command: "files/ls", Message: "the bulk walk must not call files/ls"}
	}
	e.d.raw["cat"] = func(args []string, opts map[string]any, _ []byte) ([]byte, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		c, err := n.resolve(args[0])
		if err != nil {
			return nil, err
		}
		offline, _ := opts["offline"].(bool)
		if !offline && (n.absent[c] || n.partial[c]) {
			return nil, &RPCError{Command: "cat", Message: "the fake would have fetched " + c}
		}
		if n.absent[c] || n.partial[c] {
			return nil, &RPCError{Command: "cat", Message: offlineMessage + c}
		}
		data, ok := n.files[c]
		if !ok {
			return nil, &RPCError{Command: "cat", Message: "path not found"}
		}
		if l, ok := opts["length"].(int64); ok && l < int64(len(data)) {
			data = data[:l]
		}
		return data, nil
	}
}

// bulkEnv is a media environment with a fake node behind it.
type bulkEnv struct {
	*mediaEnv
	node *fakeNode
	// waits are the durations the run asked to wait, in order.
	waits []time.Duration
	// onWait, when set, runs inside every wait, before it returns.
	onWait func(d time.Duration)
	mu     sync.Mutex
}

type bulkOpts struct {
	media mediaOpts
	node  *fakeNode
}

func newBulkEnv(t *testing.T, o bulkOpts) *bulkEnv {
	t.Helper()
	m := newMediaEnv(t, o.media)
	t.Cleanup(m.s.StopBulk)
	if o.node == nil {
		o.node = newFakeNode()
	}
	o.node.install(m.indexEnv)
	b := &bulkEnv{mediaEnv: m, node: o.node}
	m.s.bulkClock = func() time.Time { return time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC) }
	m.s.bulkSleep = func(ctx context.Context, d time.Duration) error {
		b.mu.Lock()
		b.waits = append(b.waits, d)
		hook := b.onWait
		b.mu.Unlock()
		if hook != nil {
			hook(d)
		}
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		return nil
	}
	return b
}

func (b *bulkEnv) waited() []time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.waits)
}

// start calls ipfs_index_bulk_start as id and returns the result.
func (b *bulkEnv) start(id Identity, args map[string]any) *mcp.CallToolResult {
	b.t.Helper()
	return toolCall(b.t, b.connect(&id), "ipfs_index_bulk_start", args)
}

// run starts a run, waits for its goroutine to end and returns the stored run.
func (b *bulkEnv) run(id Identity, args map[string]any) (map[string]any, embeddings.Run) {
	b.t.Helper()
	res := b.start(id, args)
	require.False(b.t, res.IsError, "%+v", res.Content)
	out := structured(b.t, res)
	validateOutput(b.t, "ipfs_index_bulk_start", out)
	b.wait()
	return out, *b.store.stored(b.t)
}

// wait blocks until the live run, if any, has ended.
func (b *bulkEnv) wait() {
	b.t.Helper()
	if r := b.s.bulk.active(); r != nil {
		select {
		case <-r.done:
		case <-time.After(20 * time.Second):
			b.t.Fatal("the bulk run did not end")
		}
	}
}

// status calls ipfs_index_bulk_status as id and returns the decoded run, nil
// when none exists.
func (b *bulkEnv) status(id Identity) map[string]any {
	b.t.Helper()
	res := toolCall(b.t, b.connect(&id), "ipfs_index_bulk_status", nil)
	require.False(b.t, res.IsError, "%+v", res.Content)
	out := structured(b.t, res)
	validateOutput(b.t, "ipfs_index_bulk_status", out)
	return out
}

// commands lists the daemon commands called, in order.
func (b *bulkEnv) commands() []string {
	b.d.mu.Lock()
	defer b.d.mu.Unlock()
	cmds := make([]string, 0, len(b.d.log))
	for _, c := range b.d.log {
		cmds = append(cmds, c.Command)
	}
	return cmds
}

// indexedCIDs lists the CIDs that have index entries.
func (b *bulkEnv) indexedCIDs() []string {
	seen := map[string]bool{}
	for _, r := range b.rows() {
		seen[r.CID] = true
	}
	cids := make([]string, 0, len(seen))
	for c := range seen {
		cids = append(cids, c)
	}
	sort.Strings(cids)
	return cids
}

func sortedCIDs(cids ...string) []string {
	out := slices.Clone(cids)
	sort.Strings(out)
	return out
}

// filler returns text whose first word picks the embedder axis, padded to size.
func filler(first string, size int) string {
	s := first + " "
	if len(s) >= size {
		return s
	}
	return s + strings.Repeat("x", size-len(s))
}
