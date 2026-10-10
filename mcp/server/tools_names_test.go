package server

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeKeystore is a stateful stand-in for the daemon's key and name commands.
type fakeKeystore struct {
	mu sync.Mutex
	// keys maps a key name to its ID.
	keys map[string]string
	// records maps an IPNS name (a key ID) to the published value.
	records map[string]string
}

func installNames(d *fakeDaemon) *fakeKeystore {
	k := &fakeKeystore{
		keys:    map[string]string{"self": testPeerID},
		records: map[string]string{},
	}
	d.handle = map[string]func([]string, map[string]any) (any, error){
		"key/list": k.list, "key/gen": k.gen, "key/rename": k.rename, "key/rm": k.rm,
		"name/publish": k.publish, "name/resolve": k.resolve,
	}
	return k
}

func (k *fakeKeystore) has(name string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	_, ok := k.keys[name]
	return ok
}

func (k *fakeKeystore) list(_ []string, _ map[string]any) (any, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	entries := make([]map[string]string, 0, len(k.keys))
	for name, id := range k.keys {
		entries = append(entries, map[string]string{"Name": name, "Id": id})
	}
	return map[string]any{"Keys": entries}, nil
}

func (k *fakeKeystore) gen(args []string, _ map[string]any) (any, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, ok := k.keys[args[0]]; ok {
		return nil, &RPCError{Command: "key/gen", Message: fmt.Sprintf("key with name '%s' already exists", args[0])}
	}
	id := fmt.Sprintf("12D3KooWgenerated%02d%s", len(k.keys), strings.Repeat("A", 20))
	k.keys[args[0]] = id
	return map[string]string{"Name": args[0], "Id": id}, nil
}

func (k *fakeKeystore) rename(args []string, opts map[string]any) (any, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	id, ok := k.keys[args[0]]
	if !ok {
		return nil, &RPCError{Command: "key/rename", Message: "no key named " + args[0] + " was found"}
	}
	if _, taken := k.keys[args[1]]; taken {
		if force, _ := opts["force"].(bool); !force {
			return nil, &RPCError{Command: "key/rename", Message: "key by that name already exists, refusing to overwrite"}
		}
	}
	delete(k.keys, args[0])
	k.keys[args[1]] = id
	return map[string]any{"Was": args[0], "Now": args[1], "Id": id, "Overwrite": false}, nil
}

func (k *fakeKeystore) rm(args []string, _ map[string]any) (any, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	id, ok := k.keys[args[0]]
	if !ok {
		return nil, &RPCError{Command: "key/rm", Message: "no key named " + args[0] + " was found"}
	}
	delete(k.keys, args[0])
	return map[string]any{"Keys": []map[string]string{{"Name": args[0], "Id": id}}}, nil
}

func (k *fakeKeystore) publish(args []string, opts map[string]any) (any, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	name, _ := opts["key"].(string)
	id, ok := k.keys[name]
	if !ok {
		return nil, &RPCError{Command: "name/publish", Message: "no key by the given name was found"}
	}
	k.records[id] = args[0]
	return map[string]string{"Name": id, "Value": args[0]}, nil
}

func (k *fakeKeystore) resolve(args []string, _ map[string]any) (any, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	name := strings.TrimPrefix(args[0], "/ipns/")
	v, ok := k.records[name]
	if !ok {
		return nil, &RPCError{Command: "name/resolve", Message: "could not resolve name"}
	}
	return map[string]string{"Path": v}, nil
}

// namesServer starts a server with the names group over a fake keystore. The
// client approves every confirmation.
func namesServer(t *testing.T, opts Options) (*mcp.ClientSession, *fakeDaemon, *fakeKeystore) {
	t.Helper()
	d := newFakeDaemon(t)
	k := installNames(d)
	s := mustNew(t, d, opts)
	return connectElicit(t, s, func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	}), d, k
}

func TestNamesGroupHasNoKeyExportOrImport(t *testing.T) {
	cs, _, _ := namesServer(t, Options{Tools: []string{"names", "destructive"}})
	names := toolNames(t, cs)
	for _, n := range []string{"ipns_publish", "ipns_resolve", "key_list", "key_gen", "key_rename", "key_rm"} {
		assert.Contains(t, names, n)
	}
	for _, n := range names {
		assert.False(t, strings.HasPrefix(n, "key_export") || strings.HasPrefix(n, "key_import"), "tool %s", n)
	}
}

func TestKeyRmIsOffByDefault(t *testing.T) {
	cs, _, _ := namesServer(t, Options{})
	names := toolNames(t, cs)
	assert.Contains(t, names, "key_gen")
	assert.NotContains(t, names, "key_rm", "destructive tools are opt-in")
}

func TestIPNSPublishDefaultsAndRoundTrip(t *testing.T) {
	cs, d, k := namesServer(t, Options{})
	c := testCID(t, 1)

	raw := callOK(t, cs, "ipns_publish", map[string]any{"path": "/ipfs/" + c})
	validateOutput(t, "ipns_publish", raw)
	var pub ipnsPublishOutput
	require.NoError(t, json.Unmarshal(raw, &pub))
	assert.Equal(t, testPeerID, pub.Name)
	assert.Equal(t, "/ipfs/"+c, pub.Value)

	last := d.log[len(d.log)-1]
	assert.Equal(t, "name/publish", last.Command)
	assert.Equal(t, "self", last.Opts["key"], "the node's own key is the default")
	assert.Equal(t, "48h", last.Opts["lifetime"], "the contract default lifetime")
	assert.NotContains(t, last.Opts, "ttl")
	assert.Equal(t, 1, d.count("name/publish"))
	assert.Equal(t, "/ipfs/"+c, k.records[testPeerID])

	rawRes := callOK(t, cs, "ipns_resolve", map[string]any{"name": pub.Name})
	validateOutput(t, "ipns_resolve", rawRes)
	var res ipnsResolveOutput
	require.NoError(t, json.Unmarshal(rawRes, &res))
	assert.Equal(t, "/ipfs/"+c, res.Path)
	assert.Equal(t, pub.Name, res.Name)
	resolve := d.log[len(d.log)-1]
	assert.Equal(t, []string{"/ipns/" + pub.Name}, resolve.Args, "a bare name becomes an /ipns/ path")
	assert.Equal(t, true, resolve.Opts["recursive"])
	assert.Equal(t, false, resolve.Opts["nocache"])
}

func TestIPNSPublishHonorsKeyLifetimeAndTTL(t *testing.T) {
	cs, d, k := namesServer(t, Options{})
	callOK(t, cs, "key_gen", map[string]any{"name": "blog"})
	c := testCID(t, 2)

	callOK(t, cs, "ipns_publish", map[string]any{"path": c, "key": "blog", "lifetime": "24h", "ttl": "5m"})
	last := d.log[len(d.log)-1]
	assert.Equal(t, []string{"/ipfs/" + c}, last.Args, "a bare CID becomes an /ipfs/ path")
	assert.Equal(t, "blog", last.Opts["key"])
	assert.Equal(t, "24h", last.Opts["lifetime"])
	assert.Equal(t, "5m", last.Opts["ttl"])
	assert.Equal(t, "/ipfs/"+c, k.records[k.keys["blog"]])
	assert.NotContains(t, k.records, testPeerID, "the node's own name is untouched")
}

func TestIPNSPublishUnknownKeyIsAToolError(t *testing.T) {
	cs, _, _ := namesServer(t, Options{})
	res := toolCall(t, cs, "ipns_publish", map[string]any{"path": testCID(t, 3), "key": "nokey"})
	require.True(t, res.IsError)
	assert.Equal(t, CodeNotFound, decodeToolError(t, res).Code)
}

func TestIPNSResolveUnknownNameIsNotFound(t *testing.T) {
	cs, _, _ := namesServer(t, Options{})
	res := toolCall(t, cs, "ipns_resolve", map[string]any{"name": "/ipns/k51qzi5uqu5dunknown"})
	require.True(t, res.IsError)
	te := decodeToolError(t, res)
	assert.Equal(t, CodeNotFound, te.Code)
	assert.Equal(t, "ipns_resolve", te.Tool)
	validateAgainstCommon(t, "ToolError", res.StructuredContent)
}

func TestIPNSResolvePassesFlags(t *testing.T) {
	cs, d, k := namesServer(t, Options{})
	k.records["k51abc"] = "/ipfs/" + testCID(t, 4)
	callOK(t, cs, "ipns_resolve", map[string]any{"name": "/ipns/k51abc", "recursive": false, "nocache": true})
	last := d.log[len(d.log)-1]
	assert.Equal(t, false, last.Opts["recursive"])
	assert.Equal(t, true, last.Opts["nocache"])
}

func TestKeyListSortedAndMatchesSchema(t *testing.T) {
	cs, _, k := namesServer(t, Options{})
	k.keys["zeta"] = "12D3KooWzeta"
	k.keys["alpha"] = "12D3KooWalpha"
	raw := callOK(t, cs, "key_list", nil)
	validateOutput(t, "key_list", raw)
	var out keyListOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	var got []string
	for _, key := range out.Keys {
		got = append(got, key.Name)
	}
	assert.Equal(t, []string{"alpha", "self", "zeta"}, got)
	assert.NotContains(t, string(raw), "PrivKey")
}

func TestKeyGenOptions(t *testing.T) {
	cs, d, _ := namesServer(t, Options{})

	raw := callOK(t, cs, "key_gen", map[string]any{"name": "ed"})
	validateOutput(t, "key_gen", raw)
	last := d.log[len(d.log)-1]
	assert.Equal(t, "ed25519", last.Opts["type"], "ed25519 is the default")
	assert.NotContains(t, last.Opts, "size")

	callOK(t, cs, "key_gen", map[string]any{"name": "rsa-small", "type": "rsa"})
	assert.Equal(t, 2048, d.log[len(d.log)-1].Opts["size"], "rsa without a size gets 2048 bits")

	callOK(t, cs, "key_gen", map[string]any{"name": "rsa-big", "type": "rsa", "size": 4096})
	assert.Equal(t, 4096, d.log[len(d.log)-1].Opts["size"])

	callOK(t, cs, "key_gen", map[string]any{"name": "ed2", "type": "ed25519", "size": 4096})
	assert.NotContains(t, d.log[len(d.log)-1].Opts, "size", "size is ignored for ed25519")
}

func TestKeyGenExistingNameIsAlreadyExists(t *testing.T) {
	cs, _, _ := namesServer(t, Options{})
	res := toolCall(t, cs, "key_gen", map[string]any{"name": "self"})
	require.True(t, res.IsError)
	te := decodeToolError(t, res)
	assert.Equal(t, "already_exists", te.Code)
	validateAgainstCommon(t, "ToolError", res.StructuredContent)
}

func TestKeyRenameNeverOverwrites(t *testing.T) {
	cs, d, k := namesServer(t, Options{})
	callOK(t, cs, "key_gen", map[string]any{"name": "blog"})
	callOK(t, cs, "key_gen", map[string]any{"name": "site"})

	res := toolCall(t, cs, "key_rename", map[string]any{"old_name": "blog", "new_name": "site"})
	require.True(t, res.IsError, "an existing key is not overwritten")
	assert.Equal(t, "already_exists", decodeToolError(t, res).Code)
	assert.Equal(t, false, d.log[len(d.log)-1].Opts["force"])
	assert.True(t, k.has("blog"))

	raw := callOK(t, cs, "key_rename", map[string]any{"old_name": "blog", "new_name": "docs"})
	validateOutput(t, "key_rename", raw)
	var out keyRenameOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, keyRenameOutput{OldName: "blog", NewName: "docs", ID: k.keys["docs"]}, out)
	assert.False(t, k.has("blog"))
}

func TestKeyRmRequiresDestructiveConfirmation(t *testing.T) {
	d := newFakeDaemon(t)
	k := installNames(d)
	k.keys["old"] = "12D3KooWold"
	s := mustNew(t, d, Options{Tools: []string{"names", "destructive"}})

	// A client that cannot elicit and has no resolver: refused, key stays.
	plain := connect(t, s)
	res := toolCall(t, plain, "key_rm", map[string]any{"name": "old"})
	require.True(t, res.IsError)
	assert.Equal(t, CodeConfirmUnavailable, decodeToolError(t, res).Code)
	assert.True(t, k.has("old"))
	assert.Zero(t, d.count("key/rm"))
	var listed keyListOutput
	require.NoError(t, json.Unmarshal(callOK(t, plain, "key_list", nil), &listed))
	assert.True(t, slices.ContainsFunc(listed.Keys, func(i keyInfo) bool { return i.Name == "old" }))

	// A user who declines.
	declined := connectElicit(t, s, func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "decline"}
	})
	res = toolCall(t, declined, "key_rm", map[string]any{"name": "old"})
	assert.Equal(t, CodeConfirmDeclined, decodeToolError(t, res).Code)
	assert.True(t, k.has("old"))

	// A user who accepts: the prompt names the key.
	var prompt string
	accepted := connectElicit(t, s, func(req *mcp.ElicitRequest) *mcp.ElicitResult {
		prompt = req.Params.Message
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	})
	raw := callOK(t, accepted, "key_rm", map[string]any{"name": "old"})
	validateOutput(t, "key_rm", raw)
	assert.Contains(t, prompt, "old")
	assert.False(t, k.has("old"))
	assert.Equal(t, 1, d.count("key/rm"))
}

func TestPublishIsConfirmedAsAWrite(t *testing.T) {
	d := newFakeDaemon(t)
	k := installNames(d)
	s := mustNew(t, d, Options{})
	plain := connect(t, s)

	res := toolCall(t, plain, "ipns_publish", map[string]any{"path": testCID(t, 5)})
	require.True(t, res.IsError)
	assert.Equal(t, CodeConfirmUnavailable, decodeToolError(t, res).Code)
	assert.Zero(t, d.count("name/publish"))
	assert.Empty(t, k.records)

	callOK(t, plain, "key_list", nil)
}

func TestIPNSToolAnnotations(t *testing.T) {
	cs, _, _ := namesServer(t, Options{Tools: []string{"names", "destructive"}})
	res, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	byName := map[string]*mcp.Tool{}
	for _, tool := range res.Tools {
		byName[tool.Name] = tool
	}
	pub := byName["ipns_publish"].Annotations
	assert.False(t, pub.ReadOnlyHint)
	require.NotNil(t, pub.DestructiveHint)
	assert.True(t, *pub.DestructiveHint, "publish replaces the previous record")
	assert.True(t, byName["ipns_resolve"].Annotations.ReadOnlyHint)
	assert.True(t, *byName["key_rm"].Annotations.DestructiveHint)
	assert.True(t, byName["key_list"].Annotations.ReadOnlyHint)
}
