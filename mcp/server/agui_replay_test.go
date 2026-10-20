//go:build !noagent

package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Replay of the contract's AG-UI fixtures (mcp/contract/fixtures/agui) against
// the real handler. A scripted model stands in for the LLM; the daemon is a
// fake. A fixture that resumes an earlier one is replayed after it, with the
// interrupt ID the server actually issued.

type aguiFixture struct {
	File        string
	Description string
	Input       map[string]any
	Events      []aguiEvent
}

func loadAGUIFixture(t *testing.T, name string) aguiFixture {
	t.Helper()
	raw, err := fs.ReadFile(contract.FS(), "fixtures/agui/"+name)
	require.NoError(t, err)
	var f aguiFixture
	require.NoError(t, json.Unmarshal(raw, &f))
	f.File = name
	return f
}

func fixtureNames(t *testing.T) []string {
	t.Helper()
	ents, err := fs.ReadDir(contract.FS(), "fixtures/agui")
	require.NoError(t, err)
	var names []string
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	return names
}

// scenario replays fixtures in order. A fixture whose input resumes an
// interrupt is sent with the interrupt ID of the previous run. during, if set,
// runs after RUN_STARTED of the fixture's run arrived and before the rest of
// the stream is read: the person acting while a run waits.
type aguiScenario struct {
	files    []string
	token    string
	limits   LoopLimits
	runFacts *bool
	during   map[string]func(t *testing.T, e *aguiEnv)
	// userMessages: nothing; the fixtures carry them.
}

var replayRunFactsOff = false

var aguiScenarios = []aguiScenario{
	{files: []string{"read-pin-ls.json"}},
	{files: []string{"read-pin-ls-negotiated.json"}},
	{files: []string{"write-interrupt-activity-facts-off.json"}, runFacts: &replayRunFactsOff},
	{files: []string{"egress-refused.json"}, token: aguiTokNoAck},
	{files: []string{"write-interrupt.json"}},
	{files: []string{"write-interrupt.json", "write-resume-approved.json"}},
	{files: []string{"write-interrupt.json", "write-resume-denied.json"}},
	{files: []string{"destructive-native-prompt.json"}},
	{files: []string{"two-calls-write-interrupt.json"}},
	{files: []string{"two-calls-write-interrupt.json", "two-calls-resume-then-native-interrupt.json"}},
	{
		files: []string{"destructive-native-prompt.json", "destructive-native-prompt-wait-approved.json"},
		during: map[string]func(*testing.T, *aguiEnv){"destructive-native-prompt-wait-approved.json": func(t *testing.T, e *aguiEnv) {
			_, resolve := e.presence.last(t)
			require.NoError(t, resolve(true))
		}},
	},
	{
		files: []string{"destructive-native-prompt.json", "destructive-native-prompt-wait-denied.json"},
		during: map[string]func(*testing.T, *aguiEnv){"destructive-native-prompt-wait-denied.json": func(t *testing.T, e *aguiEnv) {
			_, resolve := e.presence.last(t)
			require.NoError(t, resolve(false))
		}},
	},
	{
		files: []string{"destructive-native-prompt.json", "destructive-native-prompt-wait-expired.json"},
		during: map[string]func(*testing.T, *aguiEnv){"destructive-native-prompt-wait-expired.json": func(t *testing.T, e *aguiEnv) {
			e.clock.Advance(confirmTTL + 1)
		}},
	},
	{files: []string{"run-error-step-limit.json"}, limits: LoopLimits{MaxSteps: 1}},
	{files: []string{"run-error-backend-failure.json"}},
	{
		files: []string{"two-calls-write-interrupt.json", "two-calls-resume-then-native-interrupt.json", "two-calls-native-wait-approved.json"},
		during: map[string]func(*testing.T, *aguiEnv){"two-calls-native-wait-approved.json": func(t *testing.T, e *aguiEnv) {
			_, resolve := e.presence.last(t)
			require.NoError(t, resolve(true))
		}},
	},
}

// modelStepsFor derives the scripted model from the fixtures: a reply with the
// calls of each fixture that announces tool calls, and a reply with the text of
// each fixture that says something.
func modelStepsFor(fixtures []aguiFixture) []func(modelRequest) []string {
	var steps []func(modelRequest) []string
	for _, f := range fixtures {
		var calls []scriptedCall
		for _, ev := range f.Events {
			if ev["type"] == "TOOL_CALL_START" {
				calls = append(calls, scriptedCall{ID: ev["toolCallId"].(string), Name: ev["toolCallName"].(string)})
			}
		}
		for _, ev := range f.Events {
			if ev["type"] == "TOOL_CALL_ARGS" {
				for i := range calls {
					if calls[i].ID == ev["toolCallId"] {
						calls[i].Args = ev["delta"].(string)
					}
				}
			}
		}
		if len(calls) > 0 {
			steps = append(steps, callStep(40, calls...))
		}
		var text strings.Builder
		for _, ev := range f.Events {
			if ev["type"] == "TEXT_MESSAGE_CONTENT" {
				text.WriteString(ev["delta"].(string))
			}
		}
		if text.Len() > 0 {
			steps = append(steps, sayStep(text.String()))
		}
	}
	return steps
}

func TestAGUIFixturesReplay(t *testing.T) {
	covered := map[string]bool{}
	for _, sc := range aguiScenarios {
		for _, f := range sc.files {
			covered[f] = true
		}
	}
	for _, name := range fixtureNames(t) {
		if name == "read-pin-ls-context-facts.json" {
			continue
		} // TestContextFactsExampleUsesActualSelectedSnapshot below.
		assert.True(t, covered[name], "fixture %s has no replay scenario", name)
	}

	for _, sc := range aguiScenarios {
		t.Run(strings.TrimSuffix(sc.files[len(sc.files)-1], ".json"), func(t *testing.T) {
			var fixtures []aguiFixture
			for _, f := range sc.files {
				fixtures = append(fixtures, loadAGUIFixture(t, f))
			}
			e := newAGUIEnv(t, aguiEnvOpts{limits: sc.limits, runFacts: sc.runFacts}, modelStepsFor(fixtures)...)
			token := sc.token
			if token == "" {
				token = aguiTokChat
			}
			var lastInterrupt string
			var sent []map[string]any
			var received []aguiEvent
			defer func() { validateWithAGUICore(t, sent, received) }()
			for i, f := range fixtures {
				in := f.Input
				if resume, ok := in["resume"].([]any); ok {
					require.NotEmpty(t, lastInterrupt, "%s resumes an interrupt that the previous run did not issue", f.File)
					in = cloneMap(in)
					entry := cloneMap(resume[0].(map[string]any))
					entry["interruptId"] = lastInterrupt
					in["resume"] = []any{entry}
				}
				sent = append(sent, in)
				resp := e.open(t.Context(), token, in, nil)
				require.Equal(t, http.StatusOK, resp.StatusCode, f.File)
				stream := newSSE(resp.Body)
				first, ok := stream.next()
				require.True(t, ok)
				got := []aguiEvent{first}
				if act := sc.during[f.File]; act != nil {
					act(t, e)
				}
				got = append(got, stream.all()...)
				received = append(received, got...)

				diffs := replayDiff(t, f.Events, got)
				assert.Empty(t, diffs, "replaying %s (step %d)\nactual types: %v", f.File, i+1, typesOf(got))
				// Every A2UI surface emitted during the replay names only
				// catalog components (mcp-agui-a2ui-registry 1.2).
				assertSurfaceMessages(t, customSurfaceMessages(t, got))
				lastInterrupt = ""
				if fin := got[len(got)-1]; fin["type"] == "RUN_FINISHED" {
					if out := fin["outcome"].(map[string]any); out["type"] == "interrupt" {
						lastInterrupt = out["interrupts"].([]any)[0].(map[string]any)["id"].(string)
					}
				}
			}
		})
	}
}

// This contract example deliberately supplies synthetic model/host/tool facts.
// Replay its actual input and all ordinary/negotiated events, then compare facts
// to the selected source snapshot instead of asserting those sample values.
func TestContextFactsExampleUsesActualSelectedSnapshot(t *testing.T) {
	f := loadAGUIFixture(t, "read-pin-ls-context-facts.json")
	e := newAGUIEnv(t, aguiEnvOpts{}, modelStepsFor([]aguiFixture{f})...)
	r := e.post(aguiTokChat, f.Input, nil)
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	var ordinary []aguiEvent
	for _, event := range f.Events {
		if event["type"] == "CUSTOM" && event["name"] == runFactsEventName {
			validateRunFactsEvent(t, event["value"])
			continue
		}
		ordinary = append(ordinary, event)
	}
	assert.Empty(t, replayDiff(t, ordinary, r.Events), "all non-fact event assertions still apply")
	value := factsFromEvents(t, r.Events, f.Input["threadId"].(string), f.Input["runId"].(string))
	facts := value["facts"].(map[string]any)
	reqs := e.model.requests()
	require.NotEmpty(t, reqs)
	actual := make([]string, 0, len(reqs[0].Tools))
	for _, tool := range reqs[0].Tools {
		actual = append(actual, tool.Function.Name)
	}
	assert.ElementsMatch(t, actual, factToolNames(t, facts["offered_tools"]))
	assert.EqualValues(t, len(e.s.cat.Tools), facts["catalog_size"])
	assert.Equal(t, promptDigestForTest(reqs[0].Messages[0].Content), facts["system_prompt_digest"])
	backend := facts["backend"].(map[string]any)
	assert.Equal(t, "test-model", backend["model"])
	assert.Equal(t, e.model.host(), backend["host"])
	assert.NotContains(t, backend, "placement")
	refs := f.Input["forwardedProps"].(map[string]any)["ipfs"].(map[string]any)["context"]
	assert.JSONEq(t, string(asJSON(t, refs)), string(asJSON(t, facts["context_snapshot"])))
	assert.Equal(t, "accepted_turn", facts["provenance"].(map[string]any)["context_snapshot"])
	assert.Equal(t, 1, e.d.count("pin/ls"))
	assert.Zero(t, e.d.count("files/read"))
	assert.Zero(t, e.d.count("files/stat"))
	validateWithAGUICore(t, []map[string]any{f.Input}, r.Events)
}

func typesOf(evs []aguiEvent) []string {
	out := make([]string, 0, len(evs))
	for _, e := range evs {
		out = append(out, fmt.Sprint(e["type"]))
	}
	return out
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ---- comparison ----

type idMapper struct {
	prefix string
	m      map[string]string
}

func (m *idMapper) get(s string) string {
	if v, ok := m.m[s]; ok {
		return v
	}
	v := fmt.Sprintf("%s%d", m.prefix, len(m.m)+1)
	m.m[s] = v
	return v
}

// normalizer maps the random IDs of one side to stable names in order of first
// appearance, so two streams can be compared.
type normalizer struct {
	msg, intr, conf, activity idMapper
}

func newNormalizer() *normalizer {
	return &normalizer{
		msg:      idMapper{"msg#", map[string]string{}},
		intr:     idMapper{"int#", map[string]string{}},
		conf:     idMapper{"cf#", map[string]string{}},
		activity: idMapper{"act#", map[string]string{}},
	}
}

// Only the newly optional, independently schema-checked run-facts event can
// supplement a legacy fixture. Catalog/surface/unknown CUSTOM events compare.
func withoutUnspecifiedRunFacts(t *testing.T, expected, in []aguiEvent) []aguiEvent {
	t.Helper()
	for _, event := range expected {
		if event["type"] == "CUSTOM" && event["name"] == runFactsEventName {
			return in
		}
	}
	out := make([]aguiEvent, 0, len(in))
	for _, e := range in {
		if e["type"] == "CUSTOM" && e["name"] == runFactsEventName {
			validateRunFactsEvent(t, e["value"])
			continue
		}
		out = append(out, e)
	}
	return out
}

// mergeText joins consecutive TEXT_MESSAGE_CONTENT deltas of one message.
func mergeText(in []aguiEvent) []aguiEvent {
	var out []aguiEvent
	for _, e := range in {
		if n := len(out); n > 0 && e["type"] == "TEXT_MESSAGE_CONTENT" && out[n-1]["type"] == "TEXT_MESSAGE_CONTENT" && out[n-1]["messageId"] == e["messageId"] {
			merged := cloneMap(out[n-1])
			merged["delta"] = merged["delta"].(string) + e["delta"].(string)
			out[n-1] = merged
			continue
		}
		out = append(out, e)
	}
	return out
}

var untrustedFrame = regexp.MustCompile(`(?s)^<untrusted-content[^>]*>\n(.*)\n</untrusted-content>$`)

// resultMatches reports whether a tool result has the shape the fixture shows:
// the same error code and tool for an error, or every field of the fixture's
// result present in the actual one with the same kind of value.
func resultMatches(exp, act string) error {
	if m := untrustedFrame.FindStringSubmatch(act); m != nil {
		act = m[1]
	}
	var e, a any
	if err := json.Unmarshal([]byte(exp), &e); err != nil {
		return fmt.Errorf("fixture result is not JSON: %w", err)
	}
	if err := json.Unmarshal([]byte(act), &a); err != nil {
		return fmt.Errorf("actual result is not JSON: %q", act)
	}
	if em, ok := e.(map[string]any)["error"].(map[string]any); ok {
		am, ok := a.(map[string]any)["error"].(map[string]any)
		if !ok {
			return fmt.Errorf("expected an error result, got %s", act)
		}
		if em["code"] != am["code"] || em["tool"] != am["tool"] {
			return fmt.Errorf("error %v/%v, want %v/%v", am["code"], am["tool"], em["code"], em["tool"])
		}
		return nil
	}
	return shapeSubset("$", e, a)
}

func shapeSubset(path string, exp, act any) error {
	switch x := exp.(type) {
	case map[string]any:
		am, ok := act.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: want an object, got %T", path, act)
		}
		for k, v := range x {
			av, ok := am[k]
			if !ok {
				return fmt.Errorf("%s.%s is missing", path, k)
			}
			if err := shapeSubset(path+"."+k, v, av); err != nil {
				return err
			}
		}
	case []any:
		aa, ok := act.([]any)
		if !ok {
			return fmt.Errorf("%s: want an array, got %T", path, act)
		}
		if len(x) > 0 {
			if len(aa) == 0 {
				return fmt.Errorf("%s: want a non-empty array", path)
			}
			return shapeSubset(path+"[0]", x[0], aa[0])
		}
	default:
		if fmt.Sprintf("%T", exp) != fmt.Sprintf("%T", act) {
			return fmt.Errorf("%s: want %T, got %T", path, exp, act)
		}
	}
	return nil
}

// replayDiff compares the actual events with the fixture's. It returns the
// differences, empty when they match.
func replayDiff(t *testing.T, exp, act []aguiEvent) []string {
	t.Helper()
	act = withoutUnspecifiedRunFacts(t, exp, act)
	exp, act = mergeText(exp), mergeText(act)
	var diffs []string
	if len(exp) != len(act) {
		diffs = append(diffs, fmt.Sprintf("event count: fixture %d (%v), actual %d (%v)", len(exp), typesOf(exp), len(act), typesOf(act)))
	}
	en, an := newNormalizer(), newNormalizer()
	for i := 0; i < len(exp) && i < len(act); i++ {
		ee, ae := deepCopy(t, exp[i]), deepCopy(t, act[i])
		if err := normalizePair(t, en, an, ee, ae); err != nil {
			diffs = append(diffs, fmt.Sprintf("event %d (%v): %v", i, exp[i]["type"], err))
			continue
		}
		eb, _ := json.Marshal(ee)
		ab, _ := json.Marshal(ae)
		if string(eb) != string(ab) {
			diffs = append(diffs, fmt.Sprintf("event %d:\n  fixture %s\n  actual  %s", i, eb, ab))
		}
	}
	return diffs
}

func deepCopy(t *testing.T, e aguiEvent) aguiEvent {
	t.Helper()
	b, err := json.Marshal(e)
	require.NoError(t, err)
	var out aguiEvent
	require.NoError(t, json.Unmarshal(b, &out))
	return out
}

// normalizePair rewrites random IDs, the time of an expiry and free text so
// the two events compare equal when they match. It reports tool results whose
// shape differs.
func normalizePair(t *testing.T, en, an *normalizer, exp, act aguiEvent) error {
	for _, side := range []struct {
		n *normalizer
		e aguiEvent
	}{{en, exp}, {an, act}} {
		for _, k := range []string{"messageId", "parentMessageId"} {
			if v, ok := side.e[k].(string); ok {
				side.e[k] = side.n.msg.get(v)
			}
		}
	}
	switch exp["type"] {
	case "CUSTOM":
		if exp["name"] == aguiA2UIEventName {
			ev, _ := exp["value"].(map[string]any)
			av, _ := act["value"].(map[string]any)
			ed, _ := ev["updateDataModel"].(map[string]any)
			ad, _ := av["updateDataModel"].(map[string]any)
			if ed != nil && ad != nil {
				ecall, _ := ed["value"].(map[string]any)
				acall, _ := ad["value"].(map[string]any)
				for _, call := range []map[string]any{ecall, acall} {
					if args, ok := call["arguments"].(string); ok {
						call["arguments"] = canonArgs(args)
					}
				}
				er, ok := ecall["result"].(string)
				if !ok {
					return fmt.Errorf("A2UI fixture result must use the existing JSON-string call wire")
				}
				ar, ok := acall["result"].(string)
				if !ok {
					return fmt.Errorf("A2UI result must be a JSON string")
				}
				if err := resultMatches(er, ar); err != nil {
					return err
				}
				ecall["result"], acall["result"] = "<result>", "<result>"
			}
		}
	case "TOOL_CALL_ARGS":
		for _, e := range []aguiEvent{exp, act} {
			e["delta"] = canonArgs(e["delta"].(string))
		}
	case "TOOL_CALL_RESULT":
		if err := resultMatches(exp["content"].(string), fmt.Sprint(act["content"])); err != nil {
			return err
		}
		exp["content"], act["content"] = "<result>", "<result>"
	case "MESSAGES_SNAPSHOT":
		em, am := exp["messages"].([]any), act["messages"].([]any)
		if len(em) != len(am) {
			return fmt.Errorf("snapshot holds %d messages, fixture %d", len(am), len(em))
		}
		for i := range em {
			x, y := em[i].(map[string]any), am[i].(map[string]any)
			x["id"], y["id"] = en.msg.get(x["id"].(string)), an.msg.get(y["id"].(string))
			if x["role"] == "tool" {
				if err := resultMatches(x["content"].(string), fmt.Sprint(y["content"])); err != nil {
					return err
				}
				x["content"], y["content"] = "<result>", "<result>"
			}
			for _, m := range []map[string]any{x, y} {
				for _, c := range asList(m["toolCalls"]) {
					fn := c.(map[string]any)["function"].(map[string]any)
					fn["arguments"] = canonArgs(fn["arguments"].(string))
				}
			}
		}
	case "RUN_FINISHED":
		if err := normalizeInterrupt(t, en, an, exp, act); err != nil {
			return err
		}
	}
	return nil
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func canonArgs(s string) string {
	var v any
	if json.Unmarshal([]byte(s), &v) != nil {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func normalizeInterrupt(t *testing.T, en, an *normalizer, exp, act aguiEvent) error {
	eo, _ := exp["outcome"].(map[string]any)
	ao, _ := act["outcome"].(map[string]any)
	if eo == nil || ao == nil || eo["type"] != "interrupt" {
		return nil
	}
	ei, ai := eo["interrupts"].([]any), ao["interrupts"].([]any)
	if len(ei) != 1 || len(ai) != 1 {
		return fmt.Errorf("interrupts: %d in fixture, %d in run; both must be exactly 1", len(ei), len(ai))
	}
	x, y := ei[0].(map[string]any), ai[0].(map[string]any)
	for _, p := range []struct {
		n *normalizer
		m map[string]any
	}{{en, x}, {an, y}} {
		p.m["id"] = p.n.intr.get(p.m["id"].(string))
		meta := p.m["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)
		meta["confirmation_id"] = p.n.conf.get(meta["confirmation_id"].(string))
		if msg, _ := p.m["message"].(string); len(msg) < 5 {
			return fmt.Errorf("interrupt message %q is too short", msg)
		}
		// The words of a native-prompt interrupt are fixed (the chat shows them);
		// a write's summary names the path the model sent and is not compared.
		if native, _ := meta["requires_native_prompt"].(bool); !native {
			p.m["message"] = "<message>"
		}
		if _, ok := p.m["expiresAt"].(string); !ok {
			return fmt.Errorf("interrupt has no expiresAt")
		}
		p.m["expiresAt"] = "<time>"
	}
	// Legacy fixtures intentionally omit these newly optional facts.
	// Validate every emitted value before excluding only an absent legacy
	// counterpart; all incumbent fields and unknown fields still compare.
	em := x["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)
	am := y["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)
	for field, def := range map[string]string{"effect_digest": "EffectDigest", "destination": "Destination", "activity_id": "ActivityId"} {
		if value, emitted := am[field]; emitted {
			validateConfirmationDef(t, def, value)
			if _, present := em[field]; !present {
				delete(am, field)
			} else if field == "activity_id" {
				validateConfirmationDef(t, def, em[field])
				em[field] = en.activity.get(em[field].(string))
				am[field] = an.activity.get(value.(string))
			}
		}
	}
	return nil
}
