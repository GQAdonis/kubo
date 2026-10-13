package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ipfs/kubo/test/cli/harness"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	mcpDagFixture     = "./fixtures/TestDagStat.car"
	mcpDagFixtureRoot = "bafyreifrm6uf5o4dsaacuszf35zhibyojlqclabzrms7iak67pf62jygaq"
	mcpBigFixture     = "./fixtures/TestGatewayHAMTDirectory.car"
)

// mcpResourceLinks returns the resource_link blocks of a result.
func mcpResourceLinks(res *mcp.CallToolResult) []*mcp.ResourceLink {
	var links []*mcp.ResourceLink
	for _, c := range res.Content {
		if l, ok := c.(*mcp.ResourceLink); ok {
			links = append(links, l)
		}
	}
	return links
}

// requireMatchesOutputSchemaWithLinks is requireMatchesOutputSchema for
// results that carry resource_link blocks after the JSON text fallback.
func requireMatchesOutputSchemaWithLinks(t *testing.T, tool string, res *mcp.CallToolResult) {
	t.Helper()
	// Validate against the schema using the same path as the one-block case
	// by checking a copy that has only the text block.
	require.NotEmpty(t, res.Content)
	trimmed := *res
	trimmed.Content = res.Content[:1]
	requireMatchesOutputSchema(t, tool, &trimmed)
}

func mcpCarRoots(t *testing.T, car []byte) []string {
	t.Helper()
	r, err := carv2.NewReader(bytes.NewReader(car))
	require.NoError(t, err)
	defer r.Close()
	roots, err := r.Roots()
	require.NoError(t, err)
	out := make([]string, 0, len(roots))
	for _, c := range roots {
		out = append(out, c.String())
	}
	return out
}

// mcpHasPin reports whether cid is pinned in any way.
func mcpHasPin(node *harness.Node, cid string) bool {
	return node.RunIPFS("pin", "ls", cid).ExitCode() == 0
}

func mcpDagNode(t *testing.T) *harness.Node {
	t.Helper()
	node := harness.NewT(t).NewNode().Init("--profile=test").StartDaemon()
	t.Cleanup(func() { node.StopDaemon() })
	return node
}

func TestMCPDAG(t *testing.T) {
	t.Parallel()

	t.Run("dag_put and dag_get round trip", func(t *testing.T) {
		t.Parallel()
		node := mcpDagNode(t)
		e := &elicitor{action: "accept"}
		cs := mcpClient(t, node, e.options())

		data := `{"hello":"world","list":[1,2,3]}`
		put := callMCP(t, cs, "dag_put", map[string]any{"data": data, "pin": true})
		requireMatchesOutputSchema(t, "dag_put", put)
		require.Len(t, e.prompts, 1, "a write is confirmed before it runs")
		sc := put.StructuredContent.(map[string]any)
		cid := sc["cid"].(string)
		assert.Equal(t, true, sc["pinned"])
		assert.True(t, mcpHasPin(node, cid), "pin=true pins the node")

		get := callMCP(t, cs, "dag_get", map[string]any{"path": "/ipfs/" + cid})
		requireMatchesOutputSchema(t, "dag_get", get)
		gsc := get.StructuredContent.(map[string]any)
		assert.Equal(t, cid, gsc["cid"])
		assert.Equal(t, "dag-cbor", gsc["codec"])
		assert.Equal(t, false, gsc["truncated"])
		assert.Equal(t, map[string]any{"hello": "world", "list": []any{float64(1), float64(2), float64(3)}}, gsc["node"])
		assert.Len(t, e.prompts, 1, "reads never prompt")

		// A sub-path resolves inside the node.
		sub := callMCP(t, cs, "dag_get", map[string]any{"path": "/ipfs/" + cid + "/hello"})
		requireMatchesOutputSchema(t, "dag_get", sub)
		assert.Equal(t, "world", sub.StructuredContent.(map[string]any)["node"])
	})

	t.Run("dag_put states pin explicitly and does not pin when told not to", func(t *testing.T) {
		t.Parallel()
		node := mcpDagNode(t)
		e := &elicitor{action: "accept"}
		cs := mcpClient(t, node, e.options())

		missing := callMCP(t, cs, "dag_put", map[string]any{"data": `{"a":1}`})
		assert.Equal(t, "invalid_argument", mcpToolErrorCode(t, missing))
		assert.Empty(t, e.prompts, "invalid input never reaches the user")

		put := callMCP(t, cs, "dag_put", map[string]any{"data": `{"unpinned":true}`, "pin": false, "store_codec": "dag-json"})
		requireMatchesOutputSchema(t, "dag_put", put)
		cid := put.StructuredContent.(map[string]any)["cid"].(string)
		assert.False(t, mcpHasPin(node, cid))
		get := callMCP(t, cs, "dag_get", map[string]any{"path": cid})
		assert.Equal(t, "dag-json", get.StructuredContent.(map[string]any)["codec"])
	})

	t.Run("dag_get links nodes larger than max_inline_bytes", func(t *testing.T) {
		t.Parallel()
		node := mcpDagNode(t)
		e := &elicitor{action: "accept"}
		cs := mcpClient(t, node, e.options())

		put := callMCP(t, cs, "dag_put", map[string]any{"data": `{"blob":"` + strings.Repeat("x", 4096) + `"}`, "pin": false})
		cid := put.StructuredContent.(map[string]any)["cid"].(string)

		get := callMCP(t, cs, "dag_get", map[string]any{"path": cid, "max_inline_bytes": 512})
		requireMatchesOutputSchemaWithLinks(t, "dag_get", get)
		sc := get.StructuredContent.(map[string]any)
		assert.Equal(t, true, sc["truncated"])
		assert.NotContains(t, sc, "node", "the large node is not inlined")
		links := mcpResourceLinks(get)
		require.Len(t, links, 1)
		assert.Equal(t, "ipfs://"+cid, links[0].URI)
	})

	t.Run("dag_stat matches ipfs dag stat", func(t *testing.T) {
		t.Parallel()
		node := mcpDagNode(t)
		f, err := os.Open(mcpDagFixture)
		require.NoError(t, err)
		defer f.Close()
		require.NoError(t, node.IPFSDagImport(f, mcpDagFixtureRoot))
		cs := mcpClient(t, node, nil)

		res := callMCP(t, cs, "dag_stat", map[string]any{"paths": []string{mcpDagFixtureRoot}})
		requireMatchesOutputSchema(t, "dag_stat", res)
		var cli struct {
			UniqueBlocks int
			TotalSize    int
			DagStats     []struct{ Cid string }
		}
		out := node.RunIPFS("dag", "stat", "--progress=false", "--enc=json", mcpDagFixtureRoot).Stdout.Bytes()
		require.NoError(t, json.Unmarshal(out, &cli))
		sc := res.StructuredContent.(map[string]any)
		assert.EqualValues(t, cli.UniqueBlocks, sc["unique_blocks"])
		assert.EqualValues(t, cli.TotalSize, sc["total_size"])
		dags := sc["dags"].([]any)
		require.Len(t, dags, 1)
		assert.Equal(t, mcpDagFixtureRoot, dags[0].(map[string]any)["cid"])
	})
}

func TestMCPDAGCAR(t *testing.T) {
	t.Parallel()

	t.Run("CAR round trip from a fixture", func(t *testing.T) {
		t.Parallel()
		car, err := os.ReadFile(mcpDagFixture)
		require.NoError(t, err)
		root := mcpCarRoots(t, car)[0]

		source := mcpDagNode(t)
		e := &elicitor{action: "accept"}
		cs := mcpClient(t, source, e.options())
		imp := callMCP(t, cs, "car_import", map[string]any{"car": base64.StdEncoding.EncodeToString(car), "pin_roots": true})
		requireMatchesOutputSchema(t, "car_import", imp)
		require.Len(t, e.prompts, 1, "an import is confirmed as a write")
		roots := imp.StructuredContent.(map[string]any)["roots"].([]any)
		require.Len(t, roots, 1)
		assert.Equal(t, root, roots[0].(map[string]any)["cid"])
		assert.Equal(t, true, roots[0].(map[string]any)["pinned"])
		assert.True(t, isPinned(source, root))
		assert.NotZero(t, imp.StructuredContent.(map[string]any)["block_count"])

		exp := callMCP(t, cs, "car_export", map[string]any{"root": root})
		requireMatchesOutputSchema(t, "car_export", exp)
		sc := exp.StructuredContent.(map[string]any)
		assert.Equal(t, true, sc["inline"])
		exported, err := base64.StdEncoding.DecodeString(sc["car_base64"].(string))
		require.NoError(t, err)
		assert.Equal(t, []string{root}, mcpCarRoots(t, exported))

		// The exported CAR imports into a second node unpinned.
		target := mcpDagNode(t)
		cs2 := mcpClient(t, target, (&elicitor{action: "accept"}).options())
		imp2 := callMCP(t, cs2, "car_import", map[string]any{"car": sc["car_base64"], "pin_roots": false})
		requireMatchesOutputSchema(t, "car_import", imp2)
		assert.False(t, mcpHasPin(target, root), "pin_roots=false pins nothing")
		assert.Equal(t, 0, target.RunIPFS("block", "stat", "--offline", root).ExitCode(), "the blocks are there")
	})

	t.Run("car_import states pin_roots explicitly and never reads local paths", func(t *testing.T) {
		t.Parallel()
		node := mcpDagNode(t)
		e := &elicitor{action: "accept"}
		cs := mcpClient(t, node, e.options())
		car, err := os.ReadFile(mcpDagFixture)
		require.NoError(t, err)

		missing := callMCP(t, cs, "car_import", map[string]any{"car": base64.StdEncoding.EncodeToString(car)})
		assert.Equal(t, "invalid_argument", mcpToolErrorCode(t, missing))
		path, err := os.Getwd()
		require.NoError(t, err)
		byPath := callMCP(t, cs, "car_import", map[string]any{"car": path + "/" + mcpDagFixture, "pin_roots": true})
		assert.Equal(t, "invalid_argument", mcpToolErrorCode(t, byPath))
		assert.NotEqual(t, 0, node.RunIPFS("block", "stat", "--offline", mcpDagFixtureRoot).ExitCode(), "nothing was imported")
	})

	t.Run("an unconfirmed car_import imports nothing", func(t *testing.T) {
		t.Parallel()
		node := mcpDagNode(t)
		car, err := os.ReadFile(mcpDagFixture)
		require.NoError(t, err)
		args := map[string]any{"car": base64.StdEncoding.EncodeToString(car), "pin_roots": true}

		plain := mcpClient(t, node, nil)
		assert.Equal(t, "confirmation_unavailable", mcpToolErrorCode(t, callMCP(t, plain, "car_import", args)))
		declining := mcpClient(t, node, (&elicitor{action: "decline"}).options())
		assert.Equal(t, "confirmation_declined", mcpToolErrorCode(t, callMCP(t, declining, "car_import", args)))
		assert.NotEqual(t, 0, node.RunIPFS("block", "stat", "--offline", mcpDagFixtureRoot).ExitCode(), "nothing was imported")
	})

	t.Run("a CAR over the cap is exported as a resource link", func(t *testing.T) {
		t.Parallel()
		car, err := os.ReadFile(mcpBigFixture)
		require.NoError(t, err)
		require.Greater(t, len(car), 1<<20, "the fixture is larger than the default cap")
		root := mcpCarRoots(t, car)[0]

		node := mcpDagNode(t)
		cs := mcpClient(t, node, (&elicitor{action: "accept"}).options())
		imp := callMCP(t, cs, "car_import", map[string]any{"car": base64.StdEncoding.EncodeToString(car), "pin_roots": false})
		requireMatchesOutputSchema(t, "car_import", imp)

		exp := callMCP(t, cs, "car_export", map[string]any{"root": root})
		requireMatchesOutputSchemaWithLinks(t, "car_export", exp)
		sc := exp.StructuredContent.(map[string]any)
		assert.Equal(t, false, sc["inline"])
		assert.NotContains(t, sc, "car_base64", "a large CAR is not inlined")
		links := mcpResourceLinks(exp)
		require.Len(t, links, 1)
		assert.Equal(t, "ipfs://"+root, links[0].URI)

		// Raising the cap inlines the same CAR.
		inline := callMCP(t, cs, "car_export", map[string]any{"root": root, "max_inline_bytes": 8 * 1024 * 1024})
		requireMatchesOutputSchema(t, "car_export", inline)
		assert.Equal(t, true, inline.StructuredContent.(map[string]any)["inline"])
	})
}
