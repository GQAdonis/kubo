package cli

import (
	"strings"
	"testing"

	"github.com/ipfs/boxo/ipns"
	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mcpNamesNode starts an isolated daemon with the test profile.
func mcpNamesNode(t *testing.T) *harness.Node {
	t.Helper()
	node := harness.NewT(t).NewNode().Init("--profile=test").StartDaemon()
	t.Cleanup(func() { node.StopDaemon() })
	return node
}

func keyListed(node *harness.Node, name string) bool {
	return strings.Contains(node.IPFS("key", "list").Stdout.String(), name+"\n")
}

func TestMCPIPNS(t *testing.T) {
	t.Parallel()

	t.Run("publish and resolve round trip with the node key", func(t *testing.T) {
		t.Parallel()
		node := mcpNamesNode(t)
		cid := node.IPFSAddStr("published over mcp", "--cid-version=1")
		e := &elicitor{action: "accept"}
		cs := mcpClient(t, node, e.options())

		pub := callMCP(t, cs, "ipns_publish", map[string]any{"path": "/ipfs/" + cid, "lifetime": "1h", "ttl": "1m"})
		requireMatchesOutputSchema(t, "ipns_publish", pub)
		require.Len(t, e.prompts, 1, "a publish is confirmed before it runs")
		assert.Contains(t, e.prompts[0], cid)
		sc := pub.StructuredContent.(map[string]any)
		name := ipns.NameFromPeer(node.PeerID()).String()
		assert.Equal(t, strings.TrimPrefix(name, "/ipns/"), strings.TrimPrefix(sc["name"].(string), "/ipns/"), "self is the default key")
		assert.Equal(t, "/ipfs/"+cid, sc["value"])

		res := callMCP(t, cs, "ipns_resolve", map[string]any{"name": name})
		requireMatchesOutputSchema(t, "ipns_resolve", res)
		assert.Equal(t, "/ipfs/"+cid, res.StructuredContent.(map[string]any)["path"])
		assert.Len(t, e.prompts, 1, "resolve is a read and never prompts")

		// The same record is visible to the CLI.
		assert.Equal(t, "/ipfs/"+cid, strings.TrimSpace(node.IPFS("name", "resolve", name).Stdout.String()))
	})

	t.Run("publish under a chosen key leaves the node key alone", func(t *testing.T) {
		t.Parallel()
		node := mcpNamesNode(t)
		cid := node.IPFSAddStr("published under blog", "--cid-version=1")
		e := &elicitor{action: "accept"}
		cs := mcpClient(t, node, e.options())

		gen := callMCP(t, cs, "key_gen", map[string]any{"name": "blog"})
		requireMatchesOutputSchema(t, "key_gen", gen)
		assert.True(t, keyListed(node, "blog"))
		keyID := gen.StructuredContent.(map[string]any)["id"].(string)

		pub := callMCP(t, cs, "ipns_publish", map[string]any{"path": cid, "key": "blog"})
		requireMatchesOutputSchema(t, "ipns_publish", pub)
		assert.Equal(t, keyID, strings.TrimPrefix(pub.StructuredContent.(map[string]any)["name"].(string), "/ipns/"), "the name is the chosen key's")
		assert.NotEqual(t, node.PeerID().String(), keyID)

		res := callMCP(t, cs, "ipns_resolve", map[string]any{"name": "/ipns/" + keyID})
		requireMatchesOutputSchema(t, "ipns_resolve", res)
		assert.Equal(t, "/ipfs/"+cid, res.StructuredContent.(map[string]any)["path"])

		unknown := callMCP(t, cs, "ipns_publish", map[string]any{"path": cid, "key": "no-such-key"})
		assert.NotEmpty(t, mcpToolErrorCode(t, unknown))
	})

	t.Run("an unconfirmed publish publishes nothing", func(t *testing.T) {
		t.Parallel()
		node := mcpNamesNode(t)
		cid := node.IPFSAddStr("not published", "--cid-version=1")

		plain := mcpClient(t, node, nil)
		res := callMCP(t, plain, "ipns_publish", map[string]any{"path": "/ipfs/" + cid})
		assert.Equal(t, "confirmation_unavailable", mcpToolErrorCode(t, res))

		declines := &elicitor{action: "decline"}
		declining := mcpClient(t, node, declines.options())
		res = callMCP(t, declining, "ipns_publish", map[string]any{"path": "/ipfs/" + cid})
		assert.Equal(t, "confirmation_declined", mcpToolErrorCode(t, res))

		name := ipns.NameFromPeer(node.PeerID()).String()
		assert.NotEqual(t, 0, node.RunIPFS("name", "resolve", "--offline", name).ExitCode(), "no record exists")
	})

	t.Run("resolving an unknown name is a tool error", func(t *testing.T) {
		t.Parallel()
		node := mcpNamesNode(t)
		cs := mcpClient(t, node, nil)
		res := callMCP(t, cs, "ipns_resolve", map[string]any{"name": "/ipns/k51qzi5uqu5dh9ihj4p4cm1ot3k7vrb5cgxd3cvfwvj3x8e9dmoeo2w3m0fdl7", "nocache": true})
		assert.NotEmpty(t, mcpToolErrorCode(t, res))
	})
}

func TestMCPIPNSKeys(t *testing.T) {
	t.Parallel()

	t.Run("generate, list and rename keys", func(t *testing.T) {
		t.Parallel()
		node := mcpNamesNode(t)
		e := &elicitor{action: "accept"}
		cs := mcpClient(t, node, e.options())

		gen := callMCP(t, cs, "key_gen", map[string]any{"name": "alpha"})
		requireMatchesOutputSchema(t, "key_gen", gen)
		callMCP(t, cs, "key_gen", map[string]any{"name": "beta", "type": "rsa", "size": 2048})
		assert.True(t, keyListed(node, "alpha"))
		assert.True(t, keyListed(node, "beta"))

		dup := callMCP(t, cs, "key_gen", map[string]any{"name": "alpha"})
		assert.Equal(t, "already_exists", mcpToolErrorCode(t, dup))

		list := callMCP(t, cs, "key_list", map[string]any{})
		requireMatchesOutputSchema(t, "key_list", list)
		var names []string
		for _, k := range list.StructuredContent.(map[string]any)["keys"].([]any) {
			names = append(names, k.(map[string]any)["name"].(string))
		}
		assert.Equal(t, []string{"alpha", "beta", "self"}, names)

		ren := callMCP(t, cs, "key_rename", map[string]any{"old_name": "alpha", "new_name": "gamma"})
		requireMatchesOutputSchema(t, "key_rename", ren)
		assert.True(t, keyListed(node, "gamma"))
		assert.False(t, keyListed(node, "alpha"))

		over := callMCP(t, cs, "key_rename", map[string]any{"old_name": "gamma", "new_name": "beta"})
		assert.Equal(t, "already_exists", mcpToolErrorCode(t, over), "rename never overwrites")
		assert.True(t, keyListed(node, "gamma"))
		assert.True(t, keyListed(node, "beta"))
	})

	t.Run("no tool exports or imports keys", func(t *testing.T) {
		t.Parallel()
		node := mcpNamesNode(t)
		cs := mcpClient(t, node, nil, "--tools", "names,destructive")
		names := mcpNames(t, cs)
		assert.ElementsMatch(t, []string{"ipns_publish", "ipns_resolve", "key_list", "key_gen", "key_rename", "key_rm"}, names)
	})

	t.Run("key_rm is off by default", func(t *testing.T) {
		t.Parallel()
		node := mcpNamesNode(t)
		cs := mcpClient(t, node, nil)
		assert.NotContains(t, mcpNames(t, cs), "key_rm")
		assert.Contains(t, mcpNames(t, cs), "key_gen")
	})

	t.Run("an unconfirmed key_rm leaves the key", func(t *testing.T) {
		t.Parallel()
		node := mcpNamesNode(t)
		node.IPFS("key", "gen", "keepme")
		require.True(t, keyListed(node, "keepme"))
		args := map[string]any{"name": "keepme"}

		plain := mcpClient(t, node, nil, "--tools", "names,destructive")
		assert.Equal(t, "confirmation_unavailable", mcpToolErrorCode(t, callMCP(t, plain, "key_rm", args)))
		assert.True(t, keyListed(node, "keepme"))

		declines := &elicitor{action: "decline"}
		declining := mcpClient(t, node, declines.options(), "--tools", "names,destructive")
		assert.Equal(t, "confirmation_declined", mcpToolErrorCode(t, callMCP(t, declining, "key_rm", args)))
		require.Len(t, declines.prompts, 1)
		assert.Contains(t, declines.prompts[0], "keepme", "the prompt names the key")
		assert.True(t, keyListed(node, "keepme"))

		list := callMCP(t, plain, "key_list", map[string]any{})
		var found bool
		for _, k := range list.StructuredContent.(map[string]any)["keys"].([]any) {
			found = found || k.(map[string]any)["name"] == "keepme"
		}
		assert.True(t, found, "key_list still shows the key")
	})

	t.Run("a confirmed key_rm removes the key", func(t *testing.T) {
		t.Parallel()
		node := mcpNamesNode(t)
		node.IPFS("key", "gen", "dropme")
		e := &elicitor{action: "accept"}
		cs := mcpClient(t, node, e.options(), "--tools", "names,destructive")

		res := callMCP(t, cs, "key_rm", map[string]any{"name": "dropme"})
		requireMatchesOutputSchema(t, "key_rm", res)
		removed := res.StructuredContent.(map[string]any)["removed"].([]any)
		require.Len(t, removed, 1)
		assert.Equal(t, "dropme", removed[0].(map[string]any)["name"])
		require.Len(t, e.prompts, 1)
		assert.False(t, keyListed(node, "dropme"))

		missing := callMCP(t, cs, "key_rm", map[string]any{"name": "dropme"})
		assert.Equal(t, "not_found", mcpToolErrorCode(t, missing))
	})
}
