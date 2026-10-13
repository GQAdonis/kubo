package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"sync"
	"testing"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
)

// rawHandler answers one raw command. upload is the request's file, if any.
// It returns the response body.
type rawHandler func(args []string, opts map[string]any, upload []byte) ([]byte, error)

// uploadHandler answers one command that takes a named file part.
type uploadHandler func(name string, opts map[string]any, data []byte) ([]byte, error)

// contentFake is a fakeDaemon that also implements rawDaemon and
// uploadDaemon.
type contentFake struct {
	*fakeDaemon
	raw    map[string]rawHandler
	upload map[string]uploadHandler
}

func newContentFake(t *testing.T) *contentFake {
	t.Helper()
	d := newFakeDaemon(t)
	d.handle = map[string]func([]string, map[string]any) (any, error){}
	return &contentFake{fakeDaemon: d, raw: map[string]rawHandler{}, upload: map[string]uploadHandler{}}
}

func (f *contentFake) record(command string, args []string, opts map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, command)
	f.log = append(f.log, fakeCall{Command: command, Args: args, Opts: opts})
}

// CallRaw implements rawDaemon.
func (f *contentFake) CallRaw(_ context.Context, command string, args []string, opts map[string]any, upload []byte, fn func(io.Reader) error) error {
	f.record(command, args, opts)
	h, ok := f.raw[command]
	if !ok {
		return &RPCError{Command: command, Message: "fake: unexpected raw command"}
	}
	out, err := h(args, opts, upload)
	if err != nil {
		return err
	}
	if err := fn(bytes.NewReader(out)); err != nil && err != ErrStopStream {
		return err
	}
	return nil
}

// CallUpload implements uploadDaemon.
func (f *contentFake) CallUpload(_ context.Context, command string, args []string, opts map[string]any, name string, data []byte, fn func(io.Reader) error) error {
	f.record(command, args, opts)
	h, ok := f.upload[command]
	if !ok {
		return &RPCError{Command: command, Message: "fake: unexpected upload command"}
	}
	out, err := h(name, opts, data)
	if err != nil {
		return err
	}
	return fn(bytes.NewReader(out))
}

func (f *contentFake) lastCall(command string) fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.log) - 1; i >= 0; i-- {
		if f.log[i].Command == command {
			return f.log[i]
		}
	}
	return fakeCall{}
}

// fakeCID returns the CIDv1 raw sha2-256 of data.
func fakeCID(t testing.TB, data []byte) string {
	t.Helper()
	return cidOf(data)
}

func cidOf(data []byte) string {
	sum := sha256.Sum256(data)
	m, err := mh.Encode(sum[:], mh.SHA2_256)
	if err != nil {
		panic(err) // sha2-256 of 32 bytes cannot fail
	}
	return cid.NewCidV1(cid.Raw, m).String()
}

// addHandler stores uploads like the daemon's add: one line per file, and a
// directory line last when wrap-with-directory is set.
func addHandler(added *[][]byte) uploadHandler {
	return func(name string, opts map[string]any, data []byte) ([]byte, error) {
		*added = append(*added, data)
		c := cidOf(data)
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		_ = enc.Encode(map[string]any{"Name": cmpOr(name, c), "Hash": c, "Size": fmt.Sprint(len(data))})
		if wrap, _ := opts["wrap-with-directory"].(bool); wrap {
			_ = enc.Encode(map[string]any{"Name": "", "Hash": cidOf([]byte("dir:" + name)), "Size": "100"})
		}
		return buf.Bytes(), nil
	}
}

// mfsFake is a stateful in-memory stand-in for the daemon's files/* commands
// whose responses have the shape recorded from a real daemon (see
// recordedFiles*). Files are keyed by absolute path; directories by path.
type mfsFake struct {
	mu    sync.Mutex
	files map[string][]byte
	dirs  map[string]struct{}
}

func installMFS(d *contentFake) *mfsFake {
	m := &mfsFake{files: map[string][]byte{}, dirs: map[string]struct{}{"/": {}}}
	d.handle["files/ls"] = m.ls
	d.handle["files/stat"] = m.stat
	d.handle["files/mkdir"] = m.mkdir
	d.handle["files/cp"] = m.cp
	d.handle["files/mv"] = m.mv
	d.handle["files/rm"] = m.rm
	d.raw["files/write"] = func(args []string, opts map[string]any, upload []byte) ([]byte, error) {
		return nil, m.write(args[0], upload, opts)
	}
	d.raw["files/read"] = m.read
	return m
}

func rpcErr(cmd, msg string) error { return &RPCError{Command: cmd, Message: msg} }

func (m *mfsFake) exists(p string) bool {
	_, f := m.files[p]
	_, d := m.dirs[p]
	return f || d
}

func (m *mfsFake) hash(p string) string {
	if _, ok := m.dirs[p]; ok {
		return cidOf([]byte("dir:" + p + fmt.Sprint(len(m.children(p)))))
	}
	return cidOf(m.files[p])
}

func (m *mfsFake) children(p string) []string {
	var out []string
	for f := range m.files {
		if path.Dir(f) == p {
			out = append(out, f)
		}
	}
	for d := range m.dirs {
		if d != "/" && path.Dir(d) == p {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

func (m *mfsFake) ls(args []string, opts map[string]any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := args[0]
	if _, ok := m.files[p]; ok {
		return map[string]any{"Entries": []any{map[string]any{"Name": path.Base(p), "Type": 0, "Size": len(m.files[p]), "Hash": m.hash(p)}}}, nil
	}
	if _, ok := m.dirs[p]; !ok {
		return nil, rpcErr("files/ls", "file does not exist")
	}
	entries := []any{}
	for _, c := range m.children(p) {
		typ := 0
		if _, ok := m.dirs[c]; ok {
			typ = 1
		}
		entries = append(entries, map[string]any{"Name": path.Base(c), "Type": typ, "Size": len(m.files[c]), "Hash": m.hash(c)})
	}
	if len(entries) == 0 {
		return map[string]any{"Entries": nil}, nil
	}
	return map[string]any{"Entries": entries}, nil
}

func (m *mfsFake) stat(args []string, _ map[string]any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := args[0]
	if data, ok := m.files[p]; ok {
		return map[string]any{"Hash": m.hash(p), "Size": len(data), "CumulativeSize": len(data) + 11, "Blocks": 0, "Type": "file"}, nil
	}
	if _, ok := m.dirs[p]; ok {
		return map[string]any{"Hash": m.hash(p), "Size": 0, "CumulativeSize": 4, "Blocks": len(m.children(p)), "Type": "directory"}, nil
	}
	return nil, rpcErr("files/stat", "file does not exist")
}

func (m *mfsFake) mkdirLocked(p string, parents bool) error {
	if m.exists(p) {
		if parents {
			return nil
		}
		return rpcErr("files/mkdir", "directory already exists")
	}
	parent := path.Dir(p)
	if _, ok := m.dirs[parent]; !ok {
		if !parents {
			return rpcErr("files/mkdir", "no such file or directory")
		}
		if err := m.mkdirLocked(parent, true); err != nil {
			return err
		}
	}
	m.dirs[p] = struct{}{}
	return nil
}

func (m *mfsFake) mkdir(args []string, opts map[string]any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	parents, _ := opts["parents"].(bool)
	return nil, m.mkdirLocked(args[0], parents)
}

func (m *mfsFake) write(p string, data []byte, opts map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.dirs[p]; ok {
		return rpcErr("files/write", "cannot write to a directory")
	}
	if _, ok := m.dirs[path.Dir(p)]; !ok {
		if parents, _ := opts["parents"].(bool); !parents {
			return rpcErr("files/write", "no such file or directory")
		}
		if err := m.mkdirLocked(path.Dir(p), true); err != nil {
			return err
		}
	}
	m.files[p] = append([]byte(nil), data...)
	return nil
}

func (m *mfsFake) read(args []string, opts map[string]any, _ []byte) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[args[0]]
	if !ok {
		return nil, rpcErr("files/read", "file does not exist")
	}
	off, _ := opts["offset"].(int64)
	if off > int64(len(data)) {
		off = int64(len(data))
	}
	data = data[off:]
	if n, ok := opts["count"].(int64); ok && n < int64(len(data)) {
		data = data[:n]
	}
	return data, nil
}

func (m *mfsFake) cp(args []string, _ map[string]any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	src, dst := args[0], args[1]
	if m.exists(dst) {
		return nil, rpcErr("files/cp", "cp: cannot put node in path "+dst+": directory already has entry by that name")
	}
	data, ok := m.files[src]
	if !ok {
		return nil, rpcErr("files/cp", "cp: source does not exist")
	}
	m.files[dst] = append([]byte(nil), data...)
	return nil, nil
}

func (m *mfsFake) mv(args []string, _ map[string]any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	src, dst := args[0], args[1]
	if m.exists(dst) {
		return nil, rpcErr("files/mv", "directory already has entry by that name")
	}
	data, ok := m.files[src]
	if !ok {
		return nil, rpcErr("files/mv", "source does not exist")
	}
	delete(m.files, src)
	m.files[dst] = data
	return nil, nil
}

func (m *mfsFake) rm(args []string, opts map[string]any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := args[0]
	if _, ok := m.files[p]; ok {
		delete(m.files, p)
		return nil, nil
	}
	if _, ok := m.dirs[p]; ok {
		if rec, _ := opts["recursive"].(bool); !rec && len(m.children(p)) > 0 {
			return nil, rpcErr("files/rm", p+" is a directory, use -r to remove directories")
		}
		delete(m.dirs, p)
		return nil, nil
	}
	return nil, rpcErr("files/rm", "file does not exist")
}

func (m *mfsFake) has(p string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.exists(p)
}
