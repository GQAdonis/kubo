package contract

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// These cases assert the public projection boundaries; semantic auth and stream
// behavior is exercised by the real server/UI scenarios at the phase boundary.
func TestDecisionReadSchemaCases(t *testing.T) {
	fixtures := readJSON(t, "decisions/fixtures/cases.json").(map[string]any)
	for _, raw := range fixtures["schemaCases"].([]any) {
		fixture := raw.(map[string]any)
		t.Run(fixture["name"].(string), func(t *testing.T) {
			schema := resolveRef(t, "decisions/decision-read.schema.json#/$defs/"+fixture["definition"].(string))
			err := schema.Validate(fixture["input"])
			if fixture["expected"].(bool) {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
}

func TestDecisionReadExamples(t *testing.T) {
	examples := map[string]string{
		"list":              "ListResponse",
		"item":              "ItemResponse",
		"changed":           "DecisionChanged",
		"snapshot-required": "SnapshotRequired",
		"cursor-refresh":    "ReadError",
	}
	for name, definition := range examples {
		t.Run(name, func(t *testing.T) {
			schema := resolveRef(t, "decisions/decision-read.schema.json#/$defs/"+definition)
			require.NoError(t, schema.Validate(readJSON(t, "decisions/examples/"+name+".json")))
		})
	}
}
