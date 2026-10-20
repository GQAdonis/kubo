//go:build !noagent

package cli

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Final-boundary bootstrap evidence: uses make build's binary, a harness
// daemon and a fake OpenAI-compatible server, all on non-default dynamic ports.
func TestMCPRunFactsConfigOffPreservesOrdinaryRuns(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "enabled"
		if !enabled {
			name = "disabled"
		}
		t.Run(name, func(t *testing.T) {
			node := agentLLMNode(t)
			model := newCLIModel(t, cliSayStep("Ordinary reply."))
			configPath := aguiConfig(t, model.baseURL(), true)
			raw, err := os.ReadFile(configPath)
			require.NoError(t, err)
			var config map[string]any
			require.NoError(t, json.Unmarshal(raw, &config))
			config["agent"].(map[string]any)["run_facts"] = enabled
			raw, err = json.Marshal(config)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(configPath, raw, 0o600))
			p := startMCPHTTPArgs(t, node, aguiEnvVars(), "--mcp-config", configPath)
			r := aguiPost(t, p, aguiChatToken, aguiInput("facts-config-thread", "r1", "hello"), nil)
			require.Equal(t, 200, r.Status, r.Body)
			count := 0
			for _, event := range r.Events {
				if event["type"] != "CUSTOM" || event["name"] != "ipfs.run.facts" {
					continue
				}
				count++
				value := event["value"].(map[string]any)
				assert.Equal(t, "facts-config-thread", value["thread_id"])
				assert.Equal(t, "r1", value["run_id"])
				assert.NotEmpty(t, value["activity_id"])
				facts := value["facts"].(map[string]any)
				assert.Equal(t, "kubo_contract", facts["tool_scope"])
				assert.Equal(t, "effective_policy_offer", facts["provenance"].(map[string]any)["offered_tools"])
			}
			if enabled {
				assert.Equal(t, 1, count)
			} else {
				assert.Zero(t, count)
			}
			assert.Equal(t, 1, model.hits())
			assert.Equal(t, "RUN_FINISHED", r.Events[len(r.Events)-1]["type"])
		})
	}
}

func TestMCPContextReferencesConfigOffPreservesOrdinaryRuns(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "enabled"
		if !enabled {
			name = "disabled"
		}
		t.Run(name, func(t *testing.T) {
			node := agentLLMNode(t)
			model := newCLIModel(t, cliSayStep("References received."))
			configPath := aguiConfig(t, model.baseURL(), true)
			raw, err := os.ReadFile(configPath)
			require.NoError(t, err)
			var config map[string]any
			require.NoError(t, json.Unmarshal(raw, &config))
			config["agent"].(map[string]any)["context_references"] = enabled
			raw, err = json.Marshal(config)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(configPath, raw, 0o600))
			p := startMCPHTTPArgs(t, node, aguiEnvVars(), "--mcp-config", configPath)
			refs := []any{map[string]any{"id": "page", "provenance": "page", "path": "/notes/a:b@c", "path_kind": "mfs"}}
			body := aguiInput("context-config-thread", "r1", "acknowledge references without reading")
			body["forwardedProps"] = map[string]any{"ipfs": map[string]any{"context": refs}}
			r := aguiPost(t, p, aguiChatToken, body, nil)
			if enabled {
				require.Equal(t, 200, r.Status, r.Body)
			} else {
				require.Equal(t, 422, r.Status, r.Body)
				assert.Empty(t, r.Events)
				assert.Contains(t, r.Body, "invalid_argument")
				assert.Zero(t, model.hits())
				r = aguiPost(t, p, aguiChatToken, aguiInput("context-config-thread", "r2", "ordinary turn"), nil)
				require.Equal(t, 200, r.Status, r.Body)
			}
			count := 0
			for _, event := range r.Events {
				if event["type"] != "CUSTOM" || event["name"] != "ipfs.run.facts" {
					continue
				}
				count++
				facts := event["value"].(map[string]any)["facts"].(map[string]any)
				assert.Equal(t, "accepted_turn", facts["provenance"].(map[string]any)["context_snapshot"])
				if enabled {
					expected, err := json.Marshal(refs)
					require.NoError(t, err)
					actual, err := json.Marshal(facts["context_snapshot"])
					require.NoError(t, err)
					assert.JSONEq(t, string(expected), string(actual))
				} else {
					assert.Empty(t, facts["context_snapshot"])
				}
			}
			assert.Equal(t, 1, count)
			assert.Equal(t, 1, model.hits())
		})
	}
}
