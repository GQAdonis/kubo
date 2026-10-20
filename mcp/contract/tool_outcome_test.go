package contract

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Semantic ID/tool/digest/time correlation and ordinary host delivery are
// exercised by the phase's real server/App scenarios, not this schema fixture.
func TestToolOutcomeSchemaCases(t *testing.T) {
	fixtures := readJSON(t, "confirmation/fixtures/tool-outcome-cases.json").(map[string]any)
	for _, raw := range fixtures["schemaCases"].([]any) {
		fixture := raw.(map[string]any)
		t.Run(fixture["name"].(string), func(t *testing.T) {
			schema := resolveRef(t, "confirmation/confirmation.schema.json#/$defs/"+fixture["definition"].(string))
			err := schema.Validate(fixture["input"])
			if fixture["expected"].(bool) {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
}
