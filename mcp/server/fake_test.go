package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

const (
	testPeerID = "12D3KooWRBy97UB99e3J6hiPesre1MZeuNQvfan4gBziswrRJsNK"
	testAPI    = "/ip4/127.0.0.1/tcp/5999"
)

// fakeDaemon is a Daemon that answers from canned values.
type fakeDaemon struct {
	mu       sync.Mutex
	version  string
	commands []string
	// callErr maps a command to the error Call returns for it.
	callErr map[string]error
	calls   []string
	// handle answers a command with a value that is encoded to JSON, or an
	// error; Call fails for commands with neither.
	handle map[string]func(args []string, opts map[string]any) (any, error)
	// stream answers a streaming command with the values of the stream.
	stream map[string]func(args []string, opts map[string]any) ([]any, error)
	// log records every call with its arguments and options.
	log []fakeCall
}

type fakeCall struct {
	Command string
	Args    []string
	Opts    map[string]any
}

// count returns how many times command was called.
func (f *fakeDaemon) count(command string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.log {
		if c.Command == command {
			n++
		}
	}
	return n
}

func newFakeDaemon(t *testing.T) *fakeDaemon {
	t.Helper()
	cat, err := contract.Load()
	require.NoError(t, err)
	seen := map[string]struct{}{}
	var cmds []string
	for _, tool := range cat.Tools {
		for _, c := range tool.RPC {
			if _, ok := seen[c]; !ok {
				seen[c] = struct{}{}
				cmds = append(cmds, c)
			}
		}
	}
	return &fakeDaemon{version: "0.44.0-dev", commands: cmds}
}

func (f *fakeDaemon) Endpoint() string { return testAPI }

func (f *fakeDaemon) without(cmds ...string) *fakeDaemon {
	var kept []string
outer:
	for _, c := range f.commands {
		for _, x := range cmds {
			if c == x {
				continue outer
			}
		}
		kept = append(kept, c)
	}
	f.commands = kept
	return f
}

func (f *fakeDaemon) Stream(_ context.Context, command string, args []string, opts map[string]any, fn func(*json.Decoder) error) error {
	f.mu.Lock()
	f.calls = append(f.calls, command)
	f.log = append(f.log, fakeCall{Command: command, Args: args, Opts: opts})
	h, ok := f.stream[command]
	f.mu.Unlock()
	if !ok {
		return &RPCError{Command: command, Message: "fake: unexpected stream command"}
	}
	values, err := h(args, opts)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, v := range values {
		if err := enc.Encode(v); err != nil {
			return err
		}
	}
	if err := fn(json.NewDecoder(&buf)); err != nil && !errors.Is(err, ErrStopStream) {
		return err
	}
	return nil
}

func (f *fakeDaemon) Call(_ context.Context, command string, args []string, opts map[string]any, out any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, command)
	f.log = append(f.log, fakeCall{Command: command, Args: args, Opts: opts})
	if err, ok := f.callErr[command]; ok {
		return err
	}
	var resp any
	switch command {
	case "version":
		resp = map[string]string{"Version": f.version, "Commit": "abc1234", "Repo": "18", "System": "arm64/darwin", "Golang": "go1.26.5"}
	case "id":
		resp = map[string]any{
			"ID":           testPeerID,
			"PublicKey":    "CAESI",
			"Addresses":    []string{"/ip4/127.0.0.1/tcp/4001/p2p/" + testPeerID},
			"AgentVersion": "kubo/0.44.0-dev/abc1234",
		}
	case "commands":
		resp = commandTree(f.commands)
	default:
		h, ok := f.handle[command]
		if !ok {
			return &RPCError{Command: command, Message: "fake: unexpected command"}
		}
		v, err := h(args, opts)
		if err != nil {
			return err
		}
		resp = v
	}
	if out == nil {
		return nil
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// commandTree builds the /api/v0/commands response for slash paths.
func commandTree(paths []string) commandNode {
	root := commandNode{Name: "ipfs"}
	for _, p := range paths {
		node := &root
		start := 0
		for i := 0; i <= len(p); i++ {
			if i < len(p) && p[i] != '/' {
				continue
			}
			name := p[start:i]
			start = i + 1
			idx := -1
			for j, sub := range node.Subcommands {
				if sub.Name == name {
					idx = j
				}
			}
			if idx < 0 {
				node.Subcommands = append(node.Subcommands, commandNode{Name: name})
				idx = len(node.Subcommands) - 1
			}
			node = &node.Subcommands[idx]
		}
	}
	return root
}

// connect starts the server over an in-memory transport and returns a client
// session.
func connect(t *testing.T, s *Server) *mcp.ClientSession {
	t.Helper()
	ctx := t.Context()
	ct, st := mcp.NewInMemoryTransports()
	ss, err := s.MCP().Connect(ctx, st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func toolNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// stubTool is a tool implementation that returns an empty object; tests use
// it to stand in for groups that later changes implement.
func stubTool(name string) toolImpl {
	return toolImpl{Name: name, Call: func(context.Context, json.RawMessage) (any, error) {
		return map[string]any{}, nil
	}}
}

func mustNew(t *testing.T, d Daemon, opts Options) *Server {
	t.Helper()
	s, err := New(t.Context(), d, opts)
	require.NoError(t, err, fmt.Sprintf("options %+v", opts))
	return s
}

// harness connects several client sessions to one server, each with its own
// principal and client options.
type harness struct {
	s          *Server
	mu         sync.Mutex
	principals map[mcp.Session]Principal
}

func newHarness(s *Server) *harness {
	h := &harness{s: s, principals: map[mcp.Session]Principal{}}
	s.MCP().AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			h.mu.Lock()
			p, ok := h.principals[req.GetSession()]
			h.mu.Unlock()
			if ok {
				ctx = WithPrincipal(ctx, p)
			}
			return next(ctx, method, req)
		}
	})
	return h
}

// connect opens a session. A nil principal leaves the server's default.
func (h *harness) connect(t testing.TB, co *mcp.ClientOptions, p *Principal) *mcp.ClientSession {
	t.Helper()
	ctx := t.Context()
	ct, st := mcp.NewInMemoryTransports()
	ss, err := h.s.MCP().Connect(ctx, st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	if p != nil {
		h.mu.Lock()
		h.principals[ss] = *p
		h.mu.Unlock()
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, co)
	cs, err := client.Connect(ctx, ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// elicitOptions returns client options whose elicitation handler calls decide.
func elicitOptions(decide func(*mcp.ElicitRequest) *mcp.ElicitResult) *mcp.ClientOptions {
	return &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return decide(req), nil
		},
	}
}

// connectElicit opens a session whose client can elicit.
func connectElicit(t testing.TB, s *Server, decide func(*mcp.ElicitRequest) *mcp.ElicitResult) *mcp.ClientSession {
	t.Helper()
	return newHarness(s).connect(t, elicitOptions(decide), nil)
}
