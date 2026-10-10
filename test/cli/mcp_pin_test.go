package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/ipfs/kubo/mcp/contract"
	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mcpClient runs `ipfs mcp args...` against the repo of node and connects a
// go-sdk client to it. A nil options value gives a client that cannot elicit.
func mcpClient(t *testing.T, node *harness.Node, co *mcp.ClientOptions, args ...string) *mcp.ClientSession {
	t.Helper()
	cmd := exec.Command(node.IPFSBin, append([]string{"mcp"}, args...)...)
	cmd.Env = mcpEnv(node.Dir)
	cmd.Dir = node.Dir
	stderr := &syncBuffer{}
	cmd.Stderr = stderr
	ctx, cancel := context.WithTimeout(t.Context(), mcpTimeout)
	t.Cleanup(cancel)
	client := mcp.NewClient(&mcp.Implementation{Name: "kubo-cli-test", Version: "0"}, co)
	cs, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	require.NoError(t, err, "stderr: %s", stderr.String())
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// elicitor is a client that answers every confirmation with the given action
// and records the prompts.
type elicitor struct {
	action  string
	prompts []string
}

func (e *elicitor) options() *mcp.ClientOptions {
	return &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			e.prompts = append(e.prompts, req.Params.Message)
			return &mcp.ElicitResult{Action: e.action, Content: map[string]any{"confirm": e.action == "accept"}}, nil
		},
	}
}

func callMCP(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	return res
}

func mcpToolErrorCode(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	require.True(t, res.IsError, "want a tool error, got %+v", res.StructuredContent)
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var out struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	return out.Error.Code
}

// requireMatchesOutputSchema checks structuredContent against the tool's
// contract output schema.
func requireMatchesOutputSchema(t *testing.T, tool string, res *mcp.CallToolResult) {
	t.Helper()
	require.False(t, res.IsError, "tool %s: %+v", tool, res.Content)
	cat, err := contract.Load()
	require.NoError(t, err)
	entry, ok := cat.Tool(tool)
	require.True(t, ok)
	rawSchema, err := contract.BundleSchema(entry.OutputSchema)
	require.NoError(t, err)
	var schema jsonschema.Schema
	require.NoError(t, json.Unmarshal(rawSchema, &schema))
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var generic any
	require.NoError(t, json.Unmarshal(raw, &generic))
	require.NoError(t, resolved.Validate(generic), "%s output: %s", tool, raw)
	require.Len(t, res.Content, 1)
	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.JSONEq(t, text.Text, string(raw), "the text fallback matches structuredContent")
}

func mcpNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func isPinned(node *harness.Node, cid string) bool {
	return node.RunIPFS("pin", "ls", "--type=recursive", cid).ExitCode() == 0
}

func TestMCPPin(t *testing.T) {
	t.Parallel()

	t.Run("pin_add, pin_ls and pin_verify against a daemon", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		cid := node.IPFSAddStr("pin me over mcp", "--pin=false", "--cid-version=1")
		require.False(t, isPinned(node, cid))

		e := &elicitor{action: "accept"}
		cs := mcpClient(t, node, e.options())

		add := callMCP(t, cs, "pin_add", map[string]any{"path": "/ipfs/" + cid, "name": "mcp test"})
		requireMatchesOutputSchema(t, "pin_add", add)
		require.Len(t, e.prompts, 1, "a write is confirmed before it runs")
		assert.Contains(t, e.prompts[0], cid)
		assert.True(t, isPinned(node, cid))

		ls := callMCP(t, cs, "pin_ls", map[string]any{"type": "recursive", "name": "mcp test"})
		requireMatchesOutputSchema(t, "pin_ls", ls)
		pins := ls.StructuredContent.(map[string]any)["pins"].([]any)
		require.Len(t, pins, 1)
		assert.Equal(t, cid, pins[0].(map[string]any)["cid"])
		assert.Equal(t, "mcp test", pins[0].(map[string]any)["name"])

		verify := callMCP(t, cs, "pin_verify", map[string]any{})
		requireMatchesOutputSchema(t, "pin_verify", verify)
		results := verify.StructuredContent.(map[string]any)["results"].([]any)
		require.NotEmpty(t, results)
		for _, r := range results {
			assert.Equal(t, true, r.(map[string]any)["ok"], "%v", r)
		}
		assert.Len(t, e.prompts, 1, "reads never prompt")
	})

	t.Run("pin_ls paginates over real pins", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		want := map[string]struct{}{}
		for i := range 5 {
			want[node.IPFSAddStr(fmt.Sprintf("page me %d", i), "--cid-version=1")] = struct{}{}
		}
		cs := mcpClient(t, node, nil)

		got := map[string]struct{}{}
		cursor := ""
		for pages := 0; ; pages++ {
			require.Less(t, pages, 10, "pagination must end")
			args := map[string]any{"type": "recursive", "limit": 2}
			if cursor != "" {
				args["cursor"] = cursor
			}
			res := callMCP(t, cs, "pin_ls", args)
			requireMatchesOutputSchema(t, "pin_ls", res)
			sc := res.StructuredContent.(map[string]any)
			for _, p := range sc["pins"].([]any) {
				got[p.(map[string]any)["cid"].(string)] = struct{}{}
			}
			page := sc["page"].(map[string]any)
			if page["has_more"] != true {
				break
			}
			cursor = page["next_cursor"].(string)
		}
		for c := range want {
			assert.Contains(t, got, c)
		}
	})

	t.Run("destructive pin tools are off by default and listed as unavailable", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()

		cs := mcpClient(t, node, nil)
		assert.NotContains(t, mcpNames(t, cs), "pin_rm")
		assert.NotContains(t, mcpNames(t, cs), "pin_update")
		sc := callMCP(t, cs, "ipfs_node_status", nil).StructuredContent.(map[string]any)
		reasons := map[string]any{}
		for _, u := range sc["unavailable_tools"].([]any) {
			m := u.(map[string]any)
			reasons[m["name"].(string)] = m["reason"]
		}
		assert.Equal(t, "destructive_disabled", reasons["pin_rm"])
		assert.Equal(t, "destructive_disabled", reasons["pin_update"])

		destructive := mcpClient(t, node, nil, "--tools", "destructive")
		assert.Contains(t, mcpNames(t, destructive), "pin_rm")
		assert.Contains(t, mcpNames(t, destructive), "pin_update")
	})

	t.Run("an unconfirmed pin_rm leaves the pin", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		cid := node.IPFSAddStr("keep me", "--cid-version=1")
		require.True(t, isPinned(node, cid))
		args := map[string]any{"path": "/ipfs/" + cid}

		// A client that cannot elicit and has no out-of-band prompt: refused.
		plain := mcpClient(t, node, nil, "--tools", "destructive")
		res := callMCP(t, plain, "pin_rm", args)
		assert.Equal(t, "confirmation_unavailable", mcpToolErrorCode(t, res))
		sc := res.StructuredContent.(map[string]any)["error"].(map[string]any)
		assert.NotContains(t, sc, "confirmation", "no token is issued where nothing could approve it")
		assert.True(t, isPinned(node, cid))

		// A client whose user declines.
		declines := &elicitor{action: "decline"}
		declining := mcpClient(t, node, declines.options(), "--tools", "destructive")
		res = callMCP(t, declining, "pin_rm", args)
		assert.Equal(t, "confirmation_declined", mcpToolErrorCode(t, res))
		require.Len(t, declines.prompts, 1)
		assert.Contains(t, declines.prompts[0], cid, "the prompt names the CID")
		assert.True(t, isPinned(node, cid))

		// A client whose user cancels.
		cancels := &elicitor{action: "cancel"}
		cancelling := mcpClient(t, node, cancels.options(), "--tools", "destructive")
		assert.Equal(t, "confirmation_declined", mcpToolErrorCode(t, callMCP(t, cancelling, "pin_rm", args)))
		assert.True(t, isPinned(node, cid))
	})

	t.Run("a confirmed pin_rm removes the pin", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		cid := node.IPFSAddStr("remove me", "--cid-version=1")
		require.True(t, isPinned(node, cid))

		// The confirmation is a human decision through elicitation, not a
		// token replayed by the caller.
		e := &elicitor{action: "accept"}
		cs := mcpClient(t, node, e.options(), "--tools", "destructive")
		res := callMCP(t, cs, "pin_rm", map[string]any{"path": "/ipfs/" + cid})
		requireMatchesOutputSchema(t, "pin_rm", res)
		assert.Equal(t, []any{cid}, res.StructuredContent.(map[string]any)["unpinned"])
		require.Len(t, e.prompts, 1)
		assert.Contains(t, e.prompts[0], cid)
		assert.False(t, isPinned(node, cid))
	})

	t.Run("a confirm token cannot confirm by itself", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		cid := node.IPFSAddStr("token is not consent", "--cid-version=1")
		cs := mcpClient(t, node, nil, "--tools", "destructive")

		res := callMCP(t, cs, "pin_rm", map[string]any{
			"path":          "/ipfs/" + cid,
			"confirm_token": "0123456789012345678901234567890123456789",
		})
		assert.Equal(t, "confirmation_invalid", mcpToolErrorCode(t, res))
		assert.True(t, isPinned(node, cid))
	})

	t.Run("a confirmed pin_update moves the pin", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		from := node.IPFSAddStr("version one", "--cid-version=1")
		to := node.IPFSAddStr("version two", "--cid-version=1", "--pin=false")
		require.True(t, isPinned(node, from))
		require.False(t, isPinned(node, to))

		e := &elicitor{action: "accept"}
		cs := mcpClient(t, node, e.options(), "--tools", "destructive")
		res := callMCP(t, cs, "pin_update", map[string]any{"from": "/ipfs/" + from, "to": "/ipfs/" + to})
		requireMatchesOutputSchema(t, "pin_update", res)
		assert.True(t, isPinned(node, to))
		assert.False(t, isPinned(node, from))
	})

	t.Run("read-only mode offers no pin writes", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		cs := mcpClient(t, node, nil, "--read-only", "--tools", "pin,destructive")
		assert.ElementsMatch(t, []string{"pin_ls", "pin_verify"}, mcpNames(t, cs))
	})
}
