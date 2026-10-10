package server

import (
	"encoding/json"
	"io/fs"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// callOK calls a tool, requires success and returns the structured content as
// JSON.
func callOK(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) json.RawMessage {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.False(t, res.IsError, "tool %s failed: %+v", name, res.Content)
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	return raw
}

func decodeToolError(t *testing.T, res *mcp.CallToolResult) ToolError {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var out toolErrorResult
	require.NoError(t, json.Unmarshal(raw, &out))
	require.NotNil(t, out.Error)
	return *out.Error
}

// validateOutput checks v against the tool's contract output schema.
func validateOutput(t *testing.T, tool string, v any) {
	t.Helper()
	cat, err := contract.Load()
	require.NoError(t, err)
	entry, ok := cat.Tool(tool)
	require.True(t, ok)
	raw, err := contract.BundleSchema(entry.OutputSchema)
	require.NoError(t, err)
	validateJSON(t, raw, v)
}

// validateAgainstCommon checks v against one definition of common.json.
func validateAgainstCommon(t *testing.T, def string, v any) {
	t.Helper()
	raw, err := fs.ReadFile(contract.FS(), "schemas/common.json")
	require.NoError(t, err)
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &doc))
	// Wrap the definition so common.json's local references still resolve.
	wrapper := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$ref":    "#/$defs/" + def,
		"$defs":   doc["$defs"],
	}
	rawWrapper, err := json.Marshal(wrapper)
	require.NoError(t, err)
	// ToolError references the confirmation schema, which is not needed for
	// these results; replace that reference by an open schema.
	rawWrapper = replaceConfirmationRef(rawWrapper)
	validateJSON(t, rawWrapper, v)
}

func replaceConfirmationRef(raw []byte) []byte {
	var doc any
	_ = json.Unmarshal(raw, &doc)
	var walk func(n any) any
	walk = func(n any) any {
		switch x := n.(type) {
		case map[string]any:
			if ref, ok := x["$ref"].(string); ok && len(ref) > 0 && ref[0] != '#' {
				return map[string]any{}
			}
			for k, v := range x {
				x[k] = walk(v)
			}
		case []any:
			for i := range x {
				x[i] = walk(x[i])
			}
		}
		return n
	}
	out, _ := json.Marshal(walk(doc))
	return out
}

func validateJSON(t *testing.T, schema []byte, v any) {
	t.Helper()
	var s jsonschema.Schema
	require.NoError(t, json.Unmarshal(schema, &s))
	resolved, err := s.Resolve(nil)
	require.NoError(t, err)
	// Validate the JSON form, as a client sees it.
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	var generic any
	require.NoError(t, json.Unmarshal(raw, &generic))
	require.NoError(t, resolved.Validate(generic), "value: %s", raw)
}
