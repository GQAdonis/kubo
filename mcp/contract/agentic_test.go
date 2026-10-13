package contract

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

const configSchema = "config/ipfs-mcp-config.schema.json"

var reservedPorts = []string{"4000", "4001", "5001", "8080", "1906", "1907", "1908", "50051", "8181", "8081", "11434", "23001", "28000"}

func readTestdata(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join("testdata", name))
}

// --- AG-UI and A2A fixtures (task 2.1) ---

type aguiFixture struct {
	Input struct {
		Messages []struct {
			ToolCalls []struct {
				ID       string `json:"id"`
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"toolCalls"`
		} `json:"messages"`
		Resume []json.RawMessage `json:"resume"`
	} `json:"input"`
	Events []struct {
		Type         string `json:"type"`
		ToolCallID   string `json:"toolCallId"`
		ToolCallName string `json:"toolCallName"`
		Outcome      *struct {
			Type       string `json:"type"`
			Interrupts []struct {
				ToolCallID string `json:"toolCallId"`
				Metadata   struct {
					IPFS struct {
						Tool                 string `json:"tool"`
						Class                string `json:"class"`
						RequiresNativePrompt bool   `json:"requires_native_prompt"`
					} `json:"ipfs-mcp"`
				} `json:"metadata"`
			} `json:"interrupts"`
		} `json:"outcome"`
	} `json:"events"`
}

func TestAGUIFixtures(t *testing.T) {
	c := mustLoad(t)
	paths := jsonFiles(t, "fixtures/agui")
	if len(paths) < 4 {
		t.Fatalf("want at least 4 AG-UI fixtures, have %d", len(paths))
	}
	sawInterrupt := false
	for _, p := range paths {
		var f aguiFixture
		readInto(t, p, &f)
		calls := map[string]string{}
		for _, m := range f.Input.Messages {
			for _, tc := range m.ToolCalls {
				calls[tc.ID] = tc.Function.Name
			}
		}
		ev := f.Events
		if ev[0].Type != "RUN_STARTED" {
			t.Errorf("%s: first event %s, want RUN_STARTED", p, ev[0].Type)
		}
		if last := ev[len(ev)-1].Type; last != "RUN_FINISHED" && last != "RUN_ERROR" {
			t.Errorf("%s: last event %s", p, last)
		}
		for i, e := range ev {
			switch e.Type {
			case "TOOL_CALL_START":
				if _, ok := c.Tool(e.ToolCallName); !ok {
					t.Errorf("%s: tool %q is not in the catalog", p, e.ToolCallName)
				}
				calls[e.ToolCallID] = e.ToolCallName
			case "TOOL_CALL_RESULT":
				if _, ok := calls[e.ToolCallID]; !ok {
					t.Errorf("%s: result for unknown tool call %s", p, e.ToolCallID)
				}
			case "RUN_FINISHED":
				if e.Outcome == nil || e.Outcome.Type != "interrupt" {
					continue
				}
				sawInterrupt = true
				if i == 0 || ev[i-1].Type != "MESSAGES_SNAPSHOT" {
					t.Errorf("%s: interrupt outcome must follow a MESSAGES_SNAPSHOT", p)
				}
				for _, in := range e.Outcome.Interrupts {
					name := calls[in.ToolCallID]
					tl, ok := c.Tool(name)
					switch {
					case !ok:
						t.Errorf("%s: interrupt for unknown tool call %s", p, in.ToolCallID)
					case tl.Confirmation == ClassNone:
						t.Errorf("%s: interrupt for read tool %s", p, name)
					case in.Metadata.IPFS.Tool != name || in.Metadata.IPFS.Class != tl.Confirmation:
						t.Errorf("%s: interrupt metadata %+v does not match %s/%s", p, in.Metadata.IPFS, name, tl.Confirmation)
					case (tl.Confirmation == ClassDestructive) != in.Metadata.IPFS.RequiresNativePrompt:
						t.Errorf("%s: requires_native_prompt must be true exactly for destructive tools", p)
					}
				}
			}
		}
	}
	if !sawInterrupt {
		t.Error("no AG-UI fixture covers an interrupt")
	}
}

// TestAGUIFixtureRules checks the run rules stated in agui.schema.json:
// every parentMessageId is introduced before the turn's first result or the
// end of the run; a resumed call is reported on its original toolCallId and
// never re-emitted; every TOOL_CALL_RESULT is the tool's output or a
// ToolError; deny, native-prompt wait outcomes and expiry are all covered.
func TestAGUIFixtureRules(t *testing.T) {
	c := mustLoad(t)
	toolErr := resolveRef(t, "schemas/common.json#/$defs/ToolError")
	type msg struct {
		ID        string `json:"id"`
		ToolCalls []struct {
			ID       string `json:"id"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"toolCalls"`
	}
	type fixture struct {
		Input struct {
			Messages []msg `json:"messages"`
			Resume   []struct {
				Status  string         `json:"status"`
				Payload map[string]any `json:"payload"`
			} `json:"resume"`
		} `json:"input"`
		Events []struct {
			Type            string `json:"type"`
			MessageID       string `json:"messageId"`
			ParentMessageID string `json:"parentMessageId"`
			ToolCallID      string `json:"toolCallId"`
			ToolCallName    string `json:"toolCallName"`
			Content         string `json:"content"`
			Code            string `json:"code"`
			Messages        []msg  `json:"messages"`
		} `json:"events"`
	}
	seen := map[string]bool{}
	for _, p := range jsonFiles(t, "fixtures/agui") {
		var f fixture
		readInto(t, p, &f)
		known := map[string]bool{}
		calls := map[string]string{}
		resumed := map[string]bool{}
		for _, m := range f.Input.Messages {
			known[m.ID] = true
			for _, tc := range m.ToolCalls {
				calls[tc.ID] = tc.Function.Name
				resumed[tc.ID] = len(f.Input.Resume) > 0
			}
		}
		pending := map[string]bool{} // parentMessageIds not yet introduced
		for i, e := range f.Events {
			switch e.Type {
			case "TEXT_MESSAGE_START":
				known[e.MessageID] = true
				delete(pending, e.MessageID)
			case "MESSAGES_SNAPSHOT":
				for _, m := range e.Messages {
					known[m.ID] = true
					delete(pending, m.ID)
				}
			case "TOOL_CALL_START":
				calls[e.ToolCallID] = e.ToolCallName
				if resumed[e.ToolCallID] {
					t.Errorf("%s: resumed call %s is re-emitted with TOOL_CALL_START", p, e.ToolCallID)
				}
				if e.ParentMessageID != "" && !known[e.ParentMessageID] {
					pending[e.ParentMessageID] = true
				}
			}
			if e.Type == "RUN_ERROR" {
				seen["run_error:"+e.Code] = true
				if i != len(f.Events)-1 {
					t.Errorf("%s: RUN_ERROR must be the last event", p)
				}
			}
			if e.Type == "TOOL_CALL_RESULT" || e.Type == "RUN_FINISHED" || e.Type == "RUN_ERROR" {
				for id := range pending {
					t.Errorf("%s: parentMessageId %s is not introduced before %s", p, id, e.Type)
				}
				clear(pending)
			}
			if e.Type != "TOOL_CALL_RESULT" {
				continue
			}
			var body any
			if err := json.Unmarshal([]byte(e.Content), &body); err != nil {
				t.Errorf("%s: TOOL_CALL_RESULT content is not JSON: %v", p, err)
				continue
			}
			if m, ok := body.(map[string]any); ok && m["error"] != nil {
				if err := toolErr.Validate(body); err != nil {
					t.Errorf("%s: error result: %v", p, err)
				}
				code, _ := m["error"].(map[string]any)["code"].(string)
				seen[code] = true
				continue
			}
			tl, ok := c.Tool(calls[e.ToolCallID])
			if !ok {
				t.Errorf("%s: result for unknown call %s", p, e.ToolCallID)
				continue
			}
			if err := resolveRef(t, tl.OutputSchema).Validate(body); err != nil {
				t.Errorf("%s: %s result does not match %s: %v", p, tl.Name, tl.OutputSchema, err)
			}
		}
		for _, r := range f.Input.Resume {
			switch {
			case r.Payload["wait"] == true:
				seen["resume:wait"] = true
			case r.Payload["approved"] == false:
				seen["resume:deny"] = true
			case r.Payload["approved"] == true:
				seen["resume:approve"] = true
			}
		}
	}
	for _, want := range []string{"resume:approve", "resume:deny", "resume:wait", "confirmation_declined", "confirmation_expired", "run_error:max_steps", "run_error:backend_error"} {
		if !seen[want] {
			t.Errorf("no AG-UI fixture covers %s", want)
		}
	}
}

type a2aFixture struct {
	Card *struct {
		Skills []struct {
			ID string `json:"id"`
		} `json:"skills"`
	} `json:"card"`
	Exchanges []struct {
		Request struct {
			Params struct {
				Message struct {
					Parts []map[string]any `json:"parts"`
				} `json:"message"`
			} `json:"params"`
		} `json:"request"`
		Response *struct {
			Result map[string]any `json:"result"`
		} `json:"response"`
	} `json:"exchanges"`
}

func TestA2AFixtures(t *testing.T) {
	c := mustLoad(t)
	var skills struct {
		Skills []struct {
			ID string `json:"id"`
		} `json:"skills"`
	}
	readInto(t, "agentic/a2a-skills.json", &skills)
	known := map[string]bool{}
	for _, s := range skills.Skills {
		known[s.ID] = true
	}
	toolPart := resolveRef(t, "fixtures/a2a.schema.json#/$defs/ToolDataPart")
	for _, p := range jsonFiles(t, "fixtures/a2a") {
		var f a2aFixture
		readInto(t, p, &f)
		if f.Card != nil {
			for _, s := range f.Card.Skills {
				if !known[s.ID] {
					t.Errorf("%s: card skill %s is not in agentic/a2a-skills.json", p, s.ID)
				}
			}
		}
		for _, ex := range f.Exchanges {
			for _, part := range ex.Request.Params.Message.Parts {
				data, ok := part["data"].(map[string]any)
				if !ok {
					continue
				}
				if err := toolPart.Validate(part); err != nil {
					t.Errorf("%s: tool data part: %v", p, err)
				}
				name, _ := data["tool"].(string)
				tl, ok := c.Tool(name)
				if !ok {
					t.Errorf("%s: tool %q is not in the catalog", p, name)
					continue
				}
				if ex.Response == nil {
					continue
				}
				task, _ := ex.Response.Result["task"].(map[string]any)
				status, _ := task["status"].(map[string]any)
				if status["state"] == "TASK_STATE_INPUT_REQUIRED" && tl.Confirmation == ClassNone {
					t.Errorf("%s: read tool %s must not wait for confirmation", p, name)
				}
			}
		}
	}
}

// TestA2AStreamOrder pins A2A 1.0.0 section 3.1.2 in the fixture schema: a
// stream begins with the Task, and no Task follows it. The Round 6 fixture
// order (a status update first, the Task last) must not validate.
func TestA2AStreamOrder(t *testing.T) {
	rs := resolveRef(t, "fixtures/a2a.schema.json")
	doc := readJSON(t, "fixtures/a2a/stream-read.json").(map[string]any)
	if err := rs.Validate(doc); err != nil {
		t.Fatalf("stream-read.json: %v", err)
	}
	ex := doc["exchanges"].([]any)[0].(map[string]any)
	stream := ex["stream"].([]any)
	first, last := stream[0], stream[len(stream)-1]
	cases := map[string][]any{
		"status update first, task last":    append([]any{stream[1]}, stream[0]),
		"task after the first event":        {first, stream[1], first},
		"no task at the start":              stream[1:],
		"message stream with a second item": {map[string]any{"jsonrpc": "2.0", "id": 6, "result": map[string]any{"message": map[string]any{"messageId": "m", "role": "ROLE_AGENT", "parts": []any{map[string]any{"text": "x"}}}}}, last},
	}
	for name, bad := range cases {
		ex["stream"] = bad
		if err := rs.Validate(doc); err == nil {
			t.Errorf("%s: validated, want an error", name)
		}
	}
	ex["stream"] = stream
}

// --- ipfs mcp config schema (task 2.2) ---

func configRoot(t *testing.T) map[string]any {
	t.Helper()
	return readJSON(t, configSchema).(map[string]any)
}

func TestConfigExamplesValidate(t *testing.T) {
	rs := resolveRef(t, configSchema)
	for _, p := range jsonFiles(t, "config/examples") {
		if err := rs.Validate(readJSON(t, p)); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}

func TestConfigInvalidExamplesFail(t *testing.T) {
	rs := resolveRef(t, configSchema)
	var exp struct {
		Cases map[string]string `json:"cases"`
	}
	readInto(t, "config/invalid/expectations.json", &exp)
	for _, p := range jsonFiles(t, "config/invalid") {
		if path.Base(p) != "expectations.json" {
			if _, ok := exp.Cases[path.Base(p)]; !ok {
				t.Errorf("%s has no expectation", p)
			}
		}
	}
	for file, field := range exp.Cases {
		err := rs.Validate(readJSON(t, "config/invalid/"+file))
		if err == nil {
			t.Errorf("%s: validated, want failure", file)
			continue
		}
		if !strings.Contains(err.Error(), field) {
			t.Errorf("%s: error does not name %s: %v", file, field, err)
		}
	}
}

// TestConfigKeyAnnotations is the generator contract for
// docs/mcp/config-reference.md: every key has default, offSwitch and
// ownerChange; non-null defaults validate; null defaults are allowed only
// for nullable or required keys.
func TestConfigKeyAnnotations(t *testing.T) {
	root := configRoot(t)
	errs := checkConfigAnnotations(t, root)
	for _, e := range errs {
		t.Error(e)
	}
	if n := countConfigKeys(root); n < 40 {
		t.Errorf("walked only %d config keys", n)
	}
}

func TestConfigAnnotationWalkRejectsMissing(t *testing.T) {
	for _, drop := range []string{"default", "offSwitch", "ownerChange"} {
		root := configRoot(t)
		http := root["properties"].(map[string]any)["http"].(map[string]any)
		listen := http["properties"].(map[string]any)["listen"].(map[string]any)
		delete(listen, drop)
		errs := checkConfigAnnotations(t, root)
		if !slices.ContainsFunc(errs, func(e error) bool {
			return strings.Contains(e.Error(), "/http/listen") && strings.Contains(e.Error(), drop)
		}) {
			t.Errorf("removing %s from /http/listen was not reported: %v", drop, errs)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	c := mustLoad(t)
	props := configRoot(t)["properties"].(map[string]any)
	http := props["http"].(map[string]any)["properties"].(map[string]any)
	if got := http["listen"].(map[string]any)["default"]; got != "127.0.0.1:5050" {
		t.Errorf("default listen address %v, want 127.0.0.1:5050", got)
	}
	tools := props["tools"].(map[string]any)["properties"].(map[string]any)
	var groups []string
	for _, g := range tools["groups"].(map[string]any)["default"].([]any) {
		groups = append(groups, g.(string))
	}
	if !slices.Equal(groups, c.ToolSelection.Default.Groups) {
		t.Errorf("tools.groups default %v differs from catalog toolSelection.default.groups %v", groups, c.ToolSelection.Default.Groups)
	}
	if slices.Contains(groups, c.ToolSelection.DestructiveSelector) {
		t.Error("tools.groups default enables destructive tools")
	}
	pr := props["principals"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	for _, k := range []string{"write_groups", "destructive_groups", "egress_ack"} {
		if d, _ := pr[k].(map[string]any)["default"].([]any); len(d) != 0 {
			t.Errorf("principals[].%s default must be empty, got %v", k, d)
		}
	}
	if props["llm"].(map[string]any)["default"] != nil || props["embeddings"].(map[string]any)["default"] != nil {
		t.Error("llm and embeddings must default to null (off, no endpoint)")
	}
	// No reserved port is accepted as a listen address, in either form.
	rs := resolveRef(t, configSchema+"#/$defs/ListenAddress")
	for _, port := range reservedPorts {
		for _, addr := range []string{"127.0.0.1:" + port, "/ip4/127.0.0.1/tcp/" + port} {
			if rs.Validate(addr) == nil {
				t.Errorf("reserved listen address %s validated", addr)
			}
		}
	}
	if err := rs.Validate("127.0.0.1:5050"); err != nil {
		t.Errorf("default listen address rejected: %v", err)
	}
}

func checkConfigAnnotations(t *testing.T, root map[string]any) []error {
	t.Helper()
	defs := root["$defs"]
	owner := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	var errs []error
	var walk func(node map[string]any, at string)
	walk = func(node map[string]any, at string) {
		props, _ := node["properties"].(map[string]any)
		required := toStrings(node["required"])
		for k, v := range props {
			if k == "$schema" {
				continue
			}
			key := v.(map[string]any)
			loc := at + "/" + k
			for _, a := range []string{"default", "offSwitch", "ownerChange"} {
				if _, ok := key[a]; !ok {
					errs = append(errs, fmt.Errorf("%s: missing %s", loc, a))
				}
			}
			if off, ok := key["offSwitch"].(string); ok && len(off) < 10 {
				errs = append(errs, fmt.Errorf("%s: offSwitch too short", loc))
			}
			if oc, ok := key["ownerChange"].(string); ok {
				if !owner.MatchString(oc) || !changeExists(oc) {
					errs = append(errs, fmt.Errorf("%s: ownerChange %q is not an OpenSpec change", loc, oc))
				}
			}
			if d, ok := key["default"]; ok {
				if err := checkDefault(key, defs, d, slices.Contains(required, k)); err != nil {
					errs = append(errs, fmt.Errorf("%s: default: %w", loc, err))
				}
			}
			walk(key, loc)
			if items, ok := key["items"].(map[string]any); ok {
				walk(items, loc+"/*")
			}
		}
	}
	walk(root, "")
	return errs
}

func checkDefault(key map[string]any, defs, d any, required bool) error {
	sch := map[string]any{"$defs": defs}
	for k, v := range key {
		sch[k] = v
	}
	data, err := json.Marshal(sch)
	if err != nil {
		return err
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	rs, err := s.Resolve(nil)
	if err != nil {
		return err
	}
	verr := rs.Validate(d)
	if d == nil && verr != nil && required {
		return nil // required key: "no default" is documented as null
	}
	return verr
}

func countConfigKeys(node map[string]any) int {
	n := 0
	props, _ := node["properties"].(map[string]any)
	for _, v := range props {
		key := v.(map[string]any)
		n++
		n += countConfigKeys(key)
		if items, ok := key["items"].(map[string]any); ok {
			n += countConfigKeys(items)
		}
	}
	return n
}

func changeExists(id string) bool {
	base := filepath.Join("..", "..", "openspec", "changes")
	if _, err := os.Stat(filepath.Join(base, id)); err == nil {
		return true
	}
	matches, _ := fs.Glob(os.DirFS(filepath.Join(base, "archive")), "*-"+id)
	return len(matches) > 0
}

func toStrings(v any) []string {
	var out []string
	for _, x := range toSlice(v) {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toSlice(v any) []any {
	s, _ := v.([]any)
	return s
}
