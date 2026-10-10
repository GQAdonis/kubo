package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
)

// testCID returns a deterministic CIDv1 for i.
func testCID(t testing.TB, i int) string {
	t.Helper()
	sum, err := mh.Sum([]byte(fmt.Sprintf("pin-test-%d", i)), mh.SHA2_256, -1)
	require.NoError(t, err)
	return cid.NewCidV1(cid.Raw, sum).String()
}

type fakePin struct {
	Type string
	Name string
}

// pinStore is a stateful stand-in for the daemon's pin commands.
type pinStore struct {
	mu   sync.Mutex
	pins map[string]fakePin
	// verifyLines is the pin/verify stream, in order.
	verifyLines []pinVerifyLine
}

// installPins makes d answer the pin commands from a fresh store.
func installPins(d *fakeDaemon, initial map[string]fakePin) *pinStore {
	p := &pinStore{pins: map[string]fakePin{}}
	for k, v := range initial {
		p.pins[k] = v
	}
	d.handle = map[string]func([]string, map[string]any) (any, error){
		"pin/add": p.add, "pin/rm": p.rm, "pin/update": p.update, "pin/ls": p.ls,
	}
	d.stream = map[string]func([]string, map[string]any) ([]any, error){"pin/verify": p.verify}
	return p
}

func (p *pinStore) has(c string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.pins[c]
	return ok
}

func rootOf(path string) string {
	return strings.Split(strings.TrimPrefix(path, "/ipfs/"), "/")[0]
}

func (p *pinStore) add(args []string, opts map[string]any) (any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := rootOf(args[0])
	typ := "recursive"
	if r, _ := opts["recursive"].(bool); !r {
		typ = "direct"
	}
	name, _ := opts["name"].(string)
	p.pins[c] = fakePin{Type: typ, Name: name}
	return map[string]any{"Pins": []string{c}}, nil
}

func (p *pinStore) rm(args []string, _ map[string]any) (any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := rootOf(args[0])
	if _, ok := p.pins[c]; !ok {
		return nil, &RPCError{Command: "pin/rm", Message: "not pinned or pinned indirectly"}
	}
	delete(p.pins, c)
	return map[string]any{"Pins": []string{c}}, nil
}

func (p *pinStore) update(args []string, opts map[string]any) (any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	from, to := rootOf(args[0]), rootOf(args[1])
	old, ok := p.pins[from]
	if !ok {
		return nil, &RPCError{Command: "pin/update", Message: "'from' cid was not recursively pinned already"}
	}
	p.pins[to] = old
	if unpin, _ := opts["unpin"].(bool); unpin {
		delete(p.pins, from)
	}
	return map[string]any{"Pins": []string{from, to}}, nil
}

func (p *pinStore) ls(args []string, opts map[string]any) (any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	typ, _ := opts["type"].(string)
	filter, _ := opts["name"].(string)
	keys := map[string]any{}
	for c, pin := range p.pins {
		if len(args) > 0 && c != rootOf(args[0]) {
			continue
		}
		if typ != "" && typ != "all" && typ != pin.Type {
			continue
		}
		if filter != "" && !strings.Contains(pin.Name, filter) {
			continue
		}
		keys[c] = map[string]any{"Type": pin.Type, "Name": pin.Name}
	}
	if len(args) > 0 && len(keys) == 0 {
		return nil, &RPCError{Command: "pin/ls", Message: "path '" + args[0] + "' is not pinned"}
	}
	return map[string]any{"Keys": keys}, nil
}

func (p *pinStore) verify(_ []string, opts map[string]any) ([]any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	verbose, _ := opts["verbose"].(bool)
	var out []any
	for _, l := range p.verifyLines {
		if !verbose && l.Ok {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

// toolCall calls a tool and returns the raw result.
func toolCall(t testing.TB, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	return res
}

func asJSON(t testing.TB, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}
