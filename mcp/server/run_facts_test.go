//go:build !noagent

package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validateRunFactsEvent(t *testing.T, value any) {
	t.Helper()
	packs := map[string]string{"facts": "agentic/run-context.schema.json", "common": "schemas/common.json", "confirmation": "confirmation/confirmation.schema.json", "ui": "agentic/ui-negotiation.schema.json"}
	defs := map[string]any{}
	for prefix, file := range packs {
		for name, definition := range readSchemaDefs(t, file) {
			defs[prefix+"."+name] = rewriteRefs(definition, func(ref string) string {
				if local, ok := strings.CutPrefix(ref, "#/$defs/"); ok {
					return "#/$defs/" + prefix + "." + local
				}
				other, def, ok := strings.Cut(ref, "#/$defs/")
				require.True(t, ok, "unsupported reference %q", ref)
				resolved := path.Join(path.Dir(file), other)
				for pack, target := range packs {
					if target == resolved {
						return "#/$defs/" + pack + "." + def
					}
				}
				t.Fatalf("unbundled facts schema %q", ref)
				return ""
			})
		}
	}
	schema := asJSON(t, map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$defs": defs, "$ref": "#/$defs/facts.RunFactsEvent"})
	assertNoExternalRefs(t, schema)
	validateJSON(t, schema, value)
}

func factsFromEvents(t *testing.T, events []aguiEvent, thread, run string) map[string]any {
	t.Helper()
	var found map[string]any
	for i, event := range events {
		if event["type"] != "CUSTOM" || event["name"] != runFactsEventName {
			continue
		}
		require.Nil(t, found, "facts emitted more than once")
		value, ok := event["value"].(map[string]any)
		require.True(t, ok)
		validateRunFactsEvent(t, value)
		assert.Equal(t, thread, value["thread_id"])
		assert.Equal(t, run, value["run_id"])
		require.Equal(t, "RUN_STARTED", events[0]["type"])
		wantIndex := 1
		if len(events) > 1 && events[1]["type"] == "CUSTOM" && events[1]["name"] == "a2ui.catalog" {
			wantIndex = 2
		}
		assert.Equal(t, wantIndex, i, "facts precede tool/text and follow only optional catalog")
		assert.ElementsMatch(t, []string{"type", "threadId", "runId"}, keysOf(events[0]), "standard RUN_STARTED gains no custom fields")
		found = value
	}
	require.NotNil(t, found)
	return found
}

func promptDigestForTest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func factToolNames(t *testing.T, value any) []string {
	t.Helper()
	var names []string
	require.NoError(t, json.Unmarshal(asJSON(t, value), &names))
	return names
}

func TestRunFactsUseActualEffectiveModelOfferAndSelectedPrompt(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, sayStep("Hello."))
	r := e.post(aguiTokReadUI, aguiBody("facts-thread", "facts-run", []any{userMsg("m1", "hello")}), nil)
	require.Equal(t, 200, r.Status, r.Body)
	value := factsFromEvents(t, r.Events, "facts-thread", "facts-run")
	facts := value["facts"].(map[string]any)
	reqs := e.model.requests()
	require.Len(t, reqs, 1)
	actual := make([]string, 0, len(reqs[0].Tools))
	for _, tool := range reqs[0].Tools {
		actual = append(actual, tool.Function.Name)
	}
	slices.Sort(actual)
	assert.Equal(t, actual, factToolNames(t, facts["offered_tools"]))
	assert.NotContains(t, factToolNames(t, facts["offered_tools"]), "pin_add")
	assert.EqualValues(t, len(e.s.cat.Tools), facts["catalog_size"])
	assert.Less(t, len(actual), len(e.s.cat.Tools))
	assert.Equal(t, "kubo_contract", facts["tool_scope"])
	assert.Equal(t, promptDigestForTest(reqs[0].Messages[0].Content), facts["system_prompt_digest"])
	assert.Equal(t, "process_conversation", facts["memory"].(map[string]any)["scope"])
	assert.Empty(t, facts["context_snapshot"], "accepted ordinary turn has a known empty reference snapshot")
	assert.Equal(t, "accepted_turn", facts["provenance"].(map[string]any)["context_snapshot"])
	assert.NotContains(t, string(asJSON(t, facts)), reqs[0].Messages[0].Content)
	provenance := facts["provenance"].(map[string]any)
	assert.Equal(t, "effective_policy_offer", provenance["offered_tools"])
	assert.Equal(t, "loaded_contract", provenance["catalog_size"])
	assert.Equal(t, "effective_run_snapshot", provenance["system_prompt_digest"])
}

func TestRunFactsResumeRetainsSnapshotAndActivityAcrossSettingsChange(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc", 12), sayStep("Pinned."), sayStep("New turn."))
	first := e.post(aguiTokChat, aguiBody("snapshot-thread", "r1", []any{userMsg("m1", "pin")}), nil)
	in := finishedInterrupt(t, first.Events)
	initial := factsFromEvents(t, first.Events, "snapshot-thread", "r1")
	require.NoError(t, e.s.ApplyAgentSettings(&AgentSettings{BaseURL: e.model.baseURL(), Model: "changed-model", SystemPrompt: "private changed prompt", Timeout: 5 * time.Second}))
	resume := e.resumeReq(aguiTokChat, "snapshot-thread", in["id"].(string), ResumeResolved, map[string]any{"approved": true})
	// resumeReq uses a fresh run id; it must be the same one projected by facts.
	resumeRun := resume.Events[0]["runId"].(string)
	continued := factsFromEvents(t, resume.Events, "snapshot-thread", resumeRun)
	assert.Equal(t, initial["activity_id"], continued["activity_id"])
	assert.Equal(t, initial["facts"].(map[string]any)["system_prompt_digest"], continued["facts"].(map[string]any)["system_prompt_digest"])
	assert.Equal(t, "test-model", continued["facts"].(map[string]any)["backend"].(map[string]any)["model"])
	next := e.post(aguiTokChat, aguiBody("snapshot-thread", "r3", []any{userMsg("m2", "hello")}), nil)
	fresh := factsFromEvents(t, next.Events, "snapshot-thread", "r3")
	assert.NotEqual(t, initial["activity_id"], fresh["activity_id"])
	reqs := e.model.requests()
	require.Len(t, reqs, 3)
	assert.Equal(t, "test-model", reqs[1].Model)
	assert.Equal(t, "changed-model", reqs[2].Model)
	assert.Equal(t, promptDigestForTest(reqs[2].Messages[0].Content), fresh["facts"].(map[string]any)["system_prompt_digest"])
	assert.NotEqual(t, initial["facts"].(map[string]any)["system_prompt_digest"], fresh["facts"].(map[string]any)["system_prompt_digest"])
	assert.NotContains(t, string(asJSON(t, fresh)), "private changed prompt")
}

func TestRunFactsUnknownAndExplicitEmptyStayDistinctAndSafe(t *testing.T) {
	e := newAgentEnv(t, agentEnvOpts{}, sayStep("No tools."))
	require.NoError(t, e.s.EnableAGUI(AGUISettings{}))
	_, events := e.run(NewIdentity(Principal{ID: "empty", Kind: KindUI}, nil, nil, nil), "hello")
	require.NotNil(t, events[0].Facts)
	facts := events[0].Facts
	require.NotNil(t, facts.OfferedTools)
	assert.Empty(t, *facts.OfferedTools)
	assert.Contains(t, string(asJSON(t, facts)), `"offered_tools":[]`)
	assert.Equal(t, []string{}, facts.Egress.AcknowledgedHosts)
	backend := selectedBackendFacts(BackendKindOpenAI, "https://user:credential@example.test:443/private/path?secret=x", "model")
	assert.Equal(t, "example.test:443", backend.Host)
	assert.NotContains(t, string(asJSON(t, backend)), "credential")
	assert.NotContains(t, string(asJSON(t, backend)), "placement")
	assert.Empty(t, selectedBackendFacts(BackendKindOpenAI, "https://unsafe_host.example/v1", "model").Host)
	assert.Nil(t, acknowledgedRunEgress(Identity{EgressAck: []string{"https://credential@private.example/path"}}))
}

func TestRunFactsOffAndA2UIOffAreIndependent(t *testing.T) {
	off := false
	for _, factsOff := range []bool{true, false} {
		opts := aguiEnvOpts{a2ui: &off}
		if factsOff {
			opts.runFacts = &off
		}
		e := newAGUIEnv(t, opts, sayStep("Still works."))
		r := e.post(aguiTokChat, aguiBody("off-thread", "r1", []any{userMsg("m1", "hello")}), nil)
		require.Equal(t, 200, r.Status)
		for _, event := range r.Events {
			assert.NotEqual(t, "a2ui.catalog", event["name"])
			assert.NotEqual(t, "a2ui", event["name"])
		}
		if factsOff {
			for _, event := range r.Events {
				assert.NotEqual(t, runFactsEventName, event["name"])
			}
		} else {
			factsFromEvents(t, r.Events, "off-thread", "r1")
		}
		assert.Equal(t, 1, e.model.hits())
	}
}

func TestUARRunFactsAreTrustedLocalFactsWithoutBackendSpoofing(t *testing.T) {
	e := newUARProxyEnv(t, func(_ int, _ map[string]any) (int, []byte) {
		return 200, []byte("data: {\"type\":\"RUN_STARTED\"}\n\ndata: {\"type\":\"CUSTOM\",\"name\":\"ipfs.run.facts\",\"value\":{\"thread_id\":\"forged\",\"run_id\":\"forged\",\"facts\":{\"memory\":{\"scope\":\"none\"},\"offered_tools\":[\"forged-tool\"]}}}\n\ndata: {\"type\":\"RUN_FINISHED\",\"outcome\":{\"type\":\"success\"}}\n\n")
	}, uarProxyOpts{})
	r := e.say("uar-facts", "r1", "hello")
	require.Equal(t, 200, r.Status, r.Body)
	value := factsFromEvents(t, r.Events, "uar-facts", "r1")
	facts := value["facts"].(map[string]any)
	assert.Equal(t, BackendKindUAR, facts["backend"].(map[string]any)["kind"])
	for _, unknown := range []string{"offered_tools", "catalog_size", "tool_scope", "system_prompt_digest", "memory"} {
		assert.NotContains(t, facts, unknown)
	}
	assert.NotContains(t, string(asJSON(t, value)), "forged")
	assert.NotContains(t, facts["backend"].(map[string]any), "model")
	assert.NotContains(t, facts["backend"].(map[string]any), "placement")
	assert.Equal(t, "effective_run_snapshot", facts["provenance"].(map[string]any)["backend"])
	off := false
	disabled := newUARProxyEnv(t, replay(t, "run-text.sse"), uarProxyOpts{runFacts: &off})
	result := disabled.say("uar-off", "r1", "hello")
	for _, event := range result.Events {
		assert.NotEqual(t, runFactsEventName, event["name"])
	}
}

func TestRunFactsAlreadySelectedRunSurvivesConcurrentSettingsApply(t *testing.T) {
	e := newAgentEnv(t, agentEnvOpts{prompt: "original private prompt"}, sayStep("First."), sayStep("Second."))
	require.NoError(t, e.s.EnableAGUI(AGUISettings{}))
	var first *RunFacts
	_, err := e.a.Run(t.Context(), uiID(nil), RunInput{Message: "hello"}, func(event Event) {
		if event.Type == EventRunStarted {
			first = event.Facts
			require.NoError(t, e.s.ApplyAgentSettings(&AgentSettings{BaseURL: e.model.baseURL(), Model: "new-model", SystemPrompt: "replacement private prompt", Timeout: 5 * time.Second}))
		}
	})
	require.NoError(t, err)
	require.NotNil(t, first)
	_, events := e.run(uiID(nil), "another accepted message")
	reqs := e.model.requests()
	require.Len(t, reqs, 2)
	assert.Equal(t, "test-model", reqs[0].Model)
	assert.Equal(t, "new-model", reqs[1].Model)
	assert.Equal(t, promptDigestForTest(reqs[0].Messages[0].Content), first.SystemPromptDigest)
	assert.Equal(t, promptDigestForTest(reqs[1].Messages[0].Content), events[0].Facts.SystemPromptDigest)
	assert.NotEqual(t, first.SystemPromptDigest, events[0].Facts.SystemPromptDigest)
	assert.NotContains(t, string(asJSON(t, first)), "original private prompt")
	assert.NotContains(t, string(asJSON(t, events[0].Facts)), "replacement private prompt")
}

func TestRunFactsOffPreservesConfirmationAndInterruptActivity(t *testing.T) {
	off := false
	e := newAGUIEnv(t, aguiEnvOpts{runFacts: &off}, pinAddScript(t, "pending-call", 13))
	r := e.post(aguiTokChat, aguiBody("off-interrupt", "r1", []any{userMsg("m1", "pin")}), nil)
	require.Equal(t, 200, r.Status, r.Body)
	interrupt := finishedInterrupt(t, r.Events)
	activity, id := activityMetadata(t, interrupt)
	assert.NotEmpty(t, activity)
	for _, event := range r.Events {
		assert.NotEqual(t, runFactsEventName, event["name"])
	}
	pending := e.s.PendingConfirmations()
	require.Len(t, pending, 1)
	assert.Equal(t, id, pending[0].ID)
	assert.Equal(t, activity, pending[0].ActivityID)
	assert.Equal(t, StatePending, pending[0].State)
	assert.Zero(t, e.d.count("pin/add"))
	proxy := newUARProxyEnv(t, replay(t, "run-tool-approval-interrupt.sse"), uarProxyOpts{runFacts: &off})
	backend := proxy.say("uar-off-interrupt", "r1", "pin")
	backendActivity, _ := activityMetadata(t, finishedInterrupt(t, backend.Events))
	assert.NotEmpty(t, backendActivity)
	for _, event := range backend.Events {
		assert.NotEqual(t, runFactsEventName, event["name"])
	}
	assert.Empty(t, proxy.s.PendingConfirmations(), "backend approval never creates node consent")
}
