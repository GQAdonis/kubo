package server

import (
	"encoding/json"
	"io/fs"
	"strings"
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

// confirmationDefsPrefix namespaces the confirmation schema's definitions when
// they are bundled next to common.json's, so the two sets cannot collide.
const confirmationDefsPrefix = "confirmation."

// validateAgainstCommon checks v against one definition of common.json. The
// definitions it references in the confirmation schema are bundled in too, so
// the confirmation payload of a ToolError is checked and not waved through.
func validateAgainstCommon(t *testing.T, def string, v any) {
	t.Helper()
	common := readSchemaDefs(t, "schemas/common.json")
	confirmation := readSchemaDefs(t, "confirmation/confirmation.schema.json")

	defs := make(map[string]any, len(common)+len(confirmation))
	for name, d := range common {
		defs[name] = rewriteRefs(d, func(ref string) string {
			if rest, ok := strings.CutPrefix(ref, "../confirmation/confirmation.schema.json#/$defs/"); ok {
				return "#/$defs/" + confirmationDefsPrefix + rest
			}
			return ref
		})
	}
	for name, d := range confirmation {
		defs[confirmationDefsPrefix+name] = rewriteRefs(d, func(ref string) string {
			if rest, ok := strings.CutPrefix(ref, "#/$defs/"); ok {
				return "#/$defs/" + confirmationDefsPrefix + rest
			}
			return ref
		})
	}
	wrapper := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$ref":    "#/$defs/" + def,
		"$defs":   defs,
	}
	rawWrapper, err := json.Marshal(wrapper)
	require.NoError(t, err)
	assertNoExternalRefs(t, rawWrapper)
	validateJSON(t, rawWrapper, v)
}

// readSchemaDefs returns the $defs of an embedded contract schema file.
func readSchemaDefs(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := fs.ReadFile(contract.FS(), path)
	require.NoError(t, err)
	var doc struct {
		Defs map[string]any `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.NotEmpty(t, doc.Defs, path)
	return doc.Defs
}

// rewriteRefs returns a copy of a decoded JSON schema with every $ref passed
// through fn.
func rewriteRefs(n any, fn func(string) string) any {
	switch x := n.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			if ref, ok := v.(string); ok && k == "$ref" {
				out[k] = fn(ref)
				continue
			}
			out[k] = rewriteRefs(v, fn)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = rewriteRefs(x[i], fn)
		}
		return out
	}
	return n
}

// assertNoExternalRefs fails when a bundled schema still points outside
// itself, which would mean a reference was resolved to nothing.
func assertNoExternalRefs(t *testing.T, raw []byte) {
	t.Helper()
	var doc any
	require.NoError(t, json.Unmarshal(raw, &doc))
	rewriteRefs(doc, func(ref string) string {
		require.True(t, strings.HasPrefix(ref, "#/"), "unbundled reference %q", ref)
		return ref
	})
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
