package contract

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Runtime framing, egress, snapshot selection and removal expectations live in
// the semantic fixture cases and must be exercised against both real adapters.
func TestRunContextSchemaCases(t *testing.T) {
	fixtures := readJSON(t, "agentic/fixtures/run-context-cases.json").(map[string]any)
	for _, raw := range fixtures["schemaCases"].([]any) {
		fixture := raw.(map[string]any)
		t.Run(fixture["name"].(string), func(t *testing.T) {
			schema := resolveRef(t, "agentic/run-context.schema.json#/$defs/"+fixture["definition"].(string))
			err := schema.Validate(fixture["input"])
			if fixture["expected"].(bool) {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
}

func TestRunContextExamples(t *testing.T) {
	require.NoError(t, resolveRef(t, "schemas/common.json#/$defs/ToolError").Validate(readJSON(t, "agentic/examples/context-invalid.json")))
	examples := map[string]string{
		"run-facts-builtin":        "RunFactsEvent",
		"run-facts-uar-unreported": "RunFactsEvent",
		"context-references":       "ContextReferences",
	}
	for name, definition := range examples {
		t.Run(name, func(t *testing.T) {
			schema := resolveRef(t, "agentic/run-context.schema.json#/$defs/"+definition)
			require.NoError(t, schema.Validate(readJSON(t, "agentic/examples/"+name+".json")))
		})
	}
}

func TestReviewedEffectWireSchemaCases(t *testing.T) {
	fixtures := readJSON(t, "confirmation/fixtures/reviewed-wire-cases.json").(map[string]any)
	for _, raw := range fixtures["schemaCases"].([]any) {
		fixture := raw.(map[string]any)
		t.Run(fixture["name"].(string), func(t *testing.T) {
			schema := resolveRef(t, fixture["schema"].(string)+"#/$defs/"+fixture["definition"].(string))
			err := schema.Validate(fixture["input"])
			if fixture["expected"].(bool) {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
}
