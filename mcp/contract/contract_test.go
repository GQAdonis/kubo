package contract

import (
	"encoding/json"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/ipfs/kubo/core/commands"
)

// planTools is every tool named by the plan's tool changes (mcp-server-core-stdio,
// mcp-pinning-with-confirmation, mcp-tools-*, mcp-embedding-index-lifecycle).
var planTools = []string{
	"ipfs_node_info", "ipfs_node_status",
	"ipfs_add", "ipfs_cat", "ipfs_get", "ipfs_ls",
	"pin_ls", "pin_add", "pin_verify", "pin_update", "pin_rm",
	"mfs_ls", "mfs_stat", "mfs_read", "mfs_write", "mfs_mkdir", "mfs_cp", "mfs_mv", "mfs_rm",
	"ipns_publish", "ipns_resolve", "key_list", "key_gen", "key_rename", "key_rm",
	"dag_get", "dag_stat", "dag_put", "car_import", "car_export",
	"routing_findprovs", "routing_findpeer", "routing_provide",
	"swarm_peers", "stats_bw", "repo_stat", "repo_gc",
	"ipfs_index_search", "ipfs_index_cid", "ipfs_index_status", "ipfs_index_reconcile",
}

// destructiveTools are the tools that can remove an operator-held object (a
// pin, an MFS entry, a keystore key or repo blocks). pin_update is one because
// its default unpin=true removes the old pin (follow-up 2026-10-09).
var destructiveTools = []string{"pin_update", "pin_rm", "mfs_rm", "key_rm", "repo_gc"}

// removingRPC are the RPC commands that remove an operator-held object. A tool
// that calls one must be class destructive, or it would bypass the destructive
// opt-in (ADR-0003).
var removingRPC = []string{"pin/rm", "pin/update", "files/rm", "key/rm", "repo/gc"}

func TestRemovingRPCIsDestructive(t *testing.T) {
	for _, tl := range mustLoad(t).Tools {
		for _, r := range tl.AllowedRPC() {
			if slices.Contains(removingRPC, r) && tl.Confirmation != ClassDestructive {
				t.Errorf("%s calls %s but is class %s, not destructive", tl.Name, r, tl.Confirmation)
			}
		}
	}
}

// TestNodeStatusReasons checks the ipfs_node_status reason enum and the
// golden default-selection status: with the default --tools every destructive
// tool, and nothing else, is reported as destructive_disabled.
func TestNodeStatusReasons(t *testing.T) {
	c := mustLoad(t)
	out := resolveRef(t, "schemas/tools/ipfs_node_status.output.json")
	data, err := readTestdata("node-status-default.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if err := out.Validate(doc); err != nil {
		t.Fatalf("testdata/node-status-default.json: %v", err)
	}
	var got []string
	for _, u := range doc.(map[string]any)["unavailable_tools"].([]any) {
		m := u.(map[string]any)
		if m["reason"] == "destructive_disabled" {
			got = append(got, m["name"].(string))
		}
	}
	def := c.DefaultToolSet()
	var want []string
	for _, tl := range c.Tools {
		if !slices.Contains(def, tl.Name) && tl.Confirmation == ClassDestructive {
			want = append(want, tl.Name)
		}
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("destructive_disabled tools %v, want %v", got, want)
	}
	bad := map[string]any{
		"daemon_reachable": true, "version_supported": true, "contract_version": "0.1.0",
		"transport": "stdio", "read_only": false, "enabled_groups": []any{},
		"unavailable_tools": []any{map[string]any{"name": "pin_rm", "reason": "destructive_off"}},
	}
	if err := out.Validate(bad); err == nil {
		t.Error("an unknown reason must not validate")
	}
}

func TestCatalogLoads(t *testing.T) {
	c := mustLoad(t)
	if err := resolveRef(t, "catalog.schema.json").Validate(readJSON(t, "catalog.json")); err != nil {
		t.Fatalf("catalog.json does not match catalog.schema.json: %v", err)
	}
	if err := c.CheckToolNames(planTools); err != nil {
		t.Fatalf("plan tools missing from the catalog: %v", err)
	}
	got := make([]string, 0, len(c.Tools))
	for _, tl := range c.Tools {
		got = append(got, tl.Name)
	}
	if extra := without(got, planTools); len(extra) > 0 {
		t.Fatalf("catalog tools not in any planned change: %v", extra)
	}
}

func TestMissingToolIsNamed(t *testing.T) {
	err := mustLoad(t).CheckToolNames([]string{"pin_ls", "ipfs_frobnicate"})
	if err == nil || !strings.Contains(err.Error(), "ipfs_frobnicate") {
		t.Fatalf("expected an error naming ipfs_frobnicate, got %v", err)
	}
}

func TestDestructiveReadOnlyIsRejected(t *testing.T) {
	data, err := fs.ReadFile(files, "catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, tl := range raw["tools"].([]any) {
		m := tl.(map[string]any)
		if m["name"] == "pin_ls" {
			m["confirmation"] = ClassDestructive
			m["confirmationSummary"] = "Remove everything"
			m["annotations"].(map[string]any)["destructiveHint"] = true
			// readOnlyHint stays true: destructive and read-only at once.
		}
	}
	mutated, _ := json.Marshal(raw)
	_, err = Parse(mutated)
	if err == nil || !strings.Contains(err.Error(), `"pin_ls"`) || !strings.Contains(err.Error(), "readOnlyHint false") {
		t.Fatalf("expected a destructive+read-only violation naming pin_ls, got %v", err)
	}
}

func TestUntrustedContentRequiresEgress(t *testing.T) {
	c := mustLoad(t)
	tl, _ := c.Tool("ipfs_cat")
	tl.ContentEgress = false
	bad := *c
	bad.Tools = []Tool{tl}
	bad.Groups = []Group{{Name: tl.Group, Title: "x", Description: "x"}}
	bad.ToolSelection.Default.Groups = []string{tl.Group}
	if err := bad.Check(); err == nil || !strings.Contains(err.Error(), "contentEgress") {
		t.Fatalf("expected contentEgress violation, got %v", err)
	}
}

// TestPinsetEnumerationIsContentEgress pins ADR-0004's Round 6 amendment: a
// tool whose result enumerates what the node stores is contentEgress, so the
// pinset cannot reach an unacknowledged model through one tool when another
// tool returning the same CIDs is refused.
func TestPinsetEnumerationIsContentEgress(t *testing.T) {
	c := mustLoad(t)
	for _, name := range []string{"pin_ls", "pin_verify", "mfs_ls", "ipfs_index_search"} {
		tl, ok := c.Tool(name)
		if !ok {
			t.Fatalf("tool %s missing from the catalog", name)
		}
		if !tl.ContentEgress {
			t.Errorf("%s enumerates stored content and must have contentEgress true", name)
		}
	}
}

// TestRPCListRules pins the rpc/rpcOptional rules: optional commands are
// disjoint from required ones, and no tool lists a command whose AllowedPaths
// prefix grants more privileged commands.
func TestRPCListRules(t *testing.T) {
	c := mustLoad(t)
	add, _ := c.Tool("ipfs_add")
	if !slices.Equal(add.AllowedRPC(), []string{"add", "provide/once"}) {
		t.Fatalf("ipfs_add AllowedRPC = %v", add.AllowedRPC())
	}
	if slices.Contains(add.RPC, "provide/once") {
		t.Fatal("provide/once must not be required: ipfs_add would be hidden on daemons before 0.42")
	}
	cases := []struct {
		name   string
		mutate func(*Tool)
		want   string
	}{
		{"overlap", func(tl *Tool) { tl.RPCOptional = []string{tl.RPC[0]} }, "also in rpc"},
		{"config prefix", func(tl *Tool) { tl.RPC = append(slices.Clone(tl.RPC), "config") }, "prefix of more privileged"},
		{"optional prefix", func(tl *Tool) { tl.RPCOptional = []string{"key"} }, "prefix of more privileged"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tl, _ := c.Tool("routing_provide")
			tc.mutate(&tl)
			bad := *c
			bad.Tools = []Tool{tl}
			bad.Groups = []Group{{Name: tl.Group, Title: "x", Description: "x"}}
			bad.ToolSelection.Default.Groups = []string{tl.Group}
			if err := bad.Check(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q violation, got %v", tc.want, err)
			}
		})
	}
}

// TestMfsPathsRejectContentNamespaces pins the R4 rule: MFS tools reject
// /ipfs/, /ipns/ and /ipld/ paths, except an /ipfs/ path as the mfs_cp source.
func TestMfsPathsRejectContentNamespaces(t *testing.T) {
	mfs := resolveRef(t, "schemas/common.json#/$defs/MfsPath")
	src := resolveRef(t, "schemas/common.json#/$defs/MfsCopySource")
	cases := []struct {
		path        string
		mfs, cpFrom bool
	}{
		{"/", true, true},
		{"/a/b", true, true},
		{"/i", true, true},
		{"/ip", true, true},
		{"/ipfs", true, true},
		{"/ipns", true, true},
		{"/ipld", true, true},
		{"/ipfsx/a", true, true},
		{"/ipnz/a", true, true},
		{"/docs/ipfs/a", true, true},
		{"/ipfs/", false, false},
		{"/ipfs/bafy", false, true},
		{"/ipfs/bafy/a.txt", false, true},
		{"/ipns/k51x", false, false},
		{"/ipld/bafy", false, false},
		{"relative", false, false},
	}
	for _, tc := range cases {
		if got := mfs.Validate(tc.path) == nil; got != tc.mfs {
			t.Errorf("MfsPath %q: valid=%v, want %v", tc.path, got, tc.mfs)
		}
		if got := src.Validate(tc.path) == nil; got != tc.cpFrom {
			t.Errorf("MfsCopySource %q: valid=%v, want %v", tc.path, got, tc.cpFrom)
		}
	}
}

// TestMfsCpSourceRoundTrips pins input/output agreement for mfs_cp: the
// output echoes the source, so every source the input accepts must be a valid
// output source, including /ipfs/ content paths.
func TestMfsCpSourceRoundTrips(t *testing.T) {
	in := resolveRef(t, "schemas/tools/mfs_cp.input.json")
	out := resolveRef(t, "schemas/tools/mfs_cp.output.json")
	for _, src := range []string{"/docs/a.txt", "/ipfs/bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku", "/ipfs/bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi/readme.md"} {
		if err := in.Validate(map[string]any{"source": src, "destination": "/copy"}); err != nil {
			t.Fatalf("input rejects source %q: %v", src, err)
		}
		res := map[string]any{"source": src, "destination": "/copy", "cid": "bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku"}
		if err := out.Validate(res); err != nil {
			t.Errorf("output rejects echoed source %q: %v", src, err)
		}
	}
	for _, schema := range []string{"schemas/tools/mfs_cp.input.json", "schemas/tools/mfs_cp.output.json"} {
		var doc map[string]any
		readInto(t, schema, &doc)
		ref, _ := doc["properties"].(map[string]any)["source"].(map[string]any)["$ref"].(string)
		if ref != "../common.json#/$defs/MfsCopySource" {
			t.Errorf("%s: source $ref = %q, want MfsCopySource", schema, ref)
		}
	}
}

func TestToolSelection(t *testing.T) {
	c := mustLoad(t)
	def := c.DefaultToolSet()
	for _, d := range destructiveTools {
		if slices.Contains(def, d) {
			t.Errorf("default tool set contains destructive tool %s", d)
		}
	}
	for _, tl := range c.Tools {
		isDestructive := tl.Confirmation == ClassDestructive
		if isDestructive != slices.Contains(destructiveTools, tl.Name) {
			t.Errorf("%s: unexpected confirmation class %s", tl.Name, tl.Confirmation)
		}
		if !isDestructive && !slices.Contains(def, tl.Name) {
			t.Errorf("default tool set misses %s", tl.Name)
		}
	}
	all := append(slices.Clone(c.ToolSelection.Default.Groups), c.ToolSelection.DestructiveSelector)
	if got := c.SelectTools(all, false); len(got) != len(c.Tools) {
		t.Errorf("selecting every group plus destructive yields %d of %d tools", len(got), len(c.Tools))
	}
	for _, n := range c.SelectTools(all, true) {
		if tl, _ := c.Tool(n); tl.Confirmation != ClassNone {
			t.Errorf("read-only selection contains %s (%s)", n, tl.Confirmation)
		}
	}
}

func TestToolSchemas(t *testing.T) {
	c := mustLoad(t)
	referenced := map[string]bool{}
	for _, tl := range c.Tools {
		in := checkToolSchema(t, tl.InputSchema)
		out := checkToolSchema(t, tl.OutputSchema)
		referenced[tl.InputSchema], referenced[tl.OutputSchema] = true, true

		inProps, _ := in["properties"].(map[string]any)
		outProps, _ := out["properties"].(map[string]any)
		_, hasToken := inProps["confirm_token"]
		if needs := tl.Confirmation != ClassNone; needs != hasToken {
			t.Errorf("%s: confirm_token present=%v, want %v", tl.Name, hasToken, needs)
		}
		if tl.Paginated {
			for _, k := range []string{"cursor", "limit"} {
				if _, ok := inProps[k]; !ok {
					t.Errorf("%s: paginated tool input lacks %s", tl.Name, k)
				}
			}
			if _, ok := outProps["page"]; !ok {
				t.Errorf("%s: paginated tool output lacks page", tl.Name)
			}
		}
		for _, ph := range SummaryPlaceholders(tl.ConfirmationSummary) {
			if _, ok := inProps[ph]; !ok {
				t.Errorf("%s: confirmationSummary placeholder {%s} is not an input property", tl.Name, ph)
			}
		}
	}
	for _, p := range jsonFiles(t, "schemas/tools") {
		if !referenced[p] {
			t.Errorf("%s is not referenced by any catalog entry", p)
		}
	}
}

// checkToolSchema validates one tool schema against the meta-schema, both as
// authored and bundled, and returns the bundled form.
func checkToolSchema(t *testing.T, p string) map[string]any {
	t.Helper()
	raw, err := fs.ReadFile(files, p)
	if err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	if err := checkAgainstMeta(t, p, raw); err != nil {
		t.Error(err)
	}
	bundled, err := BundleSchema(p)
	if err != nil {
		t.Fatalf("bundle %s: %v", p, err)
	}
	if err := checkAgainstMeta(t, p, bundled); err != nil {
		t.Errorf("bundled: %v", err)
	}
	var sch jsonschema.Schema
	if err := json.Unmarshal(bundled, &sch); err != nil {
		t.Fatal(err)
	}
	if _, err := sch.Resolve(nil); err != nil {
		t.Errorf("%s: bundled schema is not self-contained: %v", p, err)
	}
	var m map[string]any
	if err := json.Unmarshal(bundled, &m); err != nil {
		t.Fatal(err)
	}
	if m["type"] != "object" {
		t.Errorf("%s: root type must be object", p)
	}
	return m
}

func TestAllSchemasAgainstMetaSchema(t *testing.T) {
	var docs []string
	for _, dir := range []string{"schemas", "confirmation", "resources", "derivations", "agentic", "fixtures", "config"} {
		for _, p := range jsonFiles(t, dir) {
			if strings.HasSuffix(p, ".schema.json") || p == "schemas/common.json" {
				docs = append(docs, p)
			}
		}
	}
	docs = append(docs, "catalog.schema.json")
	for _, p := range docs {
		raw, _ := fs.ReadFile(files, p)
		if err := checkAgainstMeta(t, p, raw); err != nil {
			t.Error(err)
		}
	}
}

// TestSelfDescribedDocuments validates every JSON document that names its
// schema with a relative "$schema".
func TestSelfDescribedDocuments(t *testing.T) {
	var all []string
	for _, dir := range []string{"resources", "derivations", "agentic", "fixtures", "config/examples"} {
		all = append(all, jsonFiles(t, dir)...)
	}
	all = append(all, "catalog.json")
	n := 0
	for _, p := range all {
		doc, ok := readJSON(t, p).(map[string]any)
		if !ok {
			continue
		}
		s, _ := doc["$schema"].(string)
		if !strings.HasPrefix(s, ".") {
			continue
		}
		n++
		if err := resolveRef(t, path.Join(path.Dir(p), s)).Validate(doc); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if n < 15 {
		t.Fatalf("only %d self-described documents validated", n)
	}
}

func TestExamples(t *testing.T) {
	schemaFor := map[string]string{
		"schemas":      "schemas/common.json",
		"confirmation": "confirmation/confirmation.schema.json",
		"resources":    "resources/resource-uri.schema.json",
	}
	for dir, schemaFile := range schemaFor {
		exs := jsonFiles(t, dir+"/examples")
		if len(exs) == 0 {
			t.Errorf("%s has no examples", dir)
		}
		for _, p := range exs {
			def := path.Base(path.Dir(p))
			if err := resolveRef(t, schemaFile+"#/$defs/"+def).Validate(readJSON(t, p)); err != nil {
				t.Errorf("%s does not match %s#/$defs/%s: %v", p, schemaFile, def, err)
			}
		}
	}
	// One example per URI form and per confirmation message.
	for _, d := range []string{
		"resources/examples/IpfsUri", "resources/examples/IpnsUri", "resources/examples/UiUri",
		"confirmation/examples/ElicitationRequest", "confirmation/examples/ConfirmationRequired",
		"confirmation/examples/ConfirmTokenClaims", "confirmation/examples/TokenAcceptance",
		"confirmation/examples/InputRequiredResult", "confirmation/examples/InputRetry",
		"confirmation/examples/RequestStateClaims", "confirmation/examples/RequestStateAcceptance",
	} {
		if len(jsonFiles(t, d)) == 0 {
			t.Errorf("no example in %s", d)
		}
	}
}

func TestConfirmTokenNeverForAgents(t *testing.T) {
	rs := resolveRef(t, "confirmation/confirmation.schema.json#/$defs/ConfirmationRequired")
	doc := readJSON(t, "confirmation/examples/ConfirmationRequired/a2a-no-token.json").(map[string]any)
	doc["confirm_token"] = strings.Repeat("A", 40)
	if err := rs.Validate(doc); err == nil {
		t.Fatal("a confirm_token for an a2a principal must not validate")
	}
	acc := resolveRef(t, "confirmation/confirmation.schema.json#/$defs/TokenAcceptance")
	if err := acc.Validate(map[string]any{"confirmation_state": "pending", "accepted": true, "error_code": "none"}); err == nil {
		t.Fatal("a token accepted while the confirmation is pending must not validate")
	}
}

// TestRequestStateIsNotConsent pins the elicitation round trip: the request
// state only binds, the user's answer approves, and a confirm token is never a
// request state.
func TestRequestStateIsNotConsent(t *testing.T) {
	acc := resolveRef(t, "confirmation/confirmation.schema.json#/$defs/RequestStateAcceptance")
	mustFail := []map[string]any{
		// A confirm token presented as a request state, with a forged accept.
		{"state_check": "wrong_use", "confirmation_state": "pending", "answer": "accept_confirmed", "accepted": true, "error_code": "none"},
		// A non-eliciting session or an externally resolved confirmation.
		{"state_check": "not_eliciting", "confirmation_state": "pending", "answer": "accept_confirmed", "accepted": true, "error_code": "none"},
		// A valid state without an accepting answer.
		{"state_check": "valid", "confirmation_state": "pending", "answer": "missing", "accepted": true, "error_code": "none"},
		{"state_check": "valid", "confirmation_state": "pending", "answer": "accept_unconfirmed", "accepted": true, "error_code": "none"},
		// A state for a confirmation that is no longer pending (replay).
		{"state_check": "valid", "confirmation_state": "approved", "answer": "accept_confirmed", "accepted": true, "error_code": "none"},
		{"state_check": "valid", "confirmation_state": "executed", "answer": "accept_confirmed", "accepted": true, "error_code": "none"},
	}
	for _, doc := range mustFail {
		if err := acc.Validate(doc); err == nil {
			t.Errorf("RequestStateAcceptance must reject %v", doc)
		}
	}
	claims := resolveRef(t, "confirmation/confirmation.schema.json#/$defs/RequestStateClaims")
	tok := readJSON(t, "confirmation/examples/ConfirmTokenClaims/pin-rm.json").(map[string]any)
	tok["use"] = "confirm_token"
	if err := claims.Validate(tok); err == nil {
		t.Error("confirm token claims must not validate as request state claims")
	}
	retry := resolveRef(t, "confirmation/confirmation.schema.json#/$defs/InputRetry")
	doc := readJSON(t, "confirmation/examples/InputRetry/pin-rm-accept.json").(map[string]any)
	doc["arguments"].(map[string]any)["confirm_token"] = strings.Repeat("A", 40)
	if err := retry.Validate(doc); err == nil {
		t.Error("an InputRetry carrying confirm_token must not validate")
	}
}

// TestNativePromptHasNoChatApproval: a confirmation the system prompt decides
// is destructive and never lists the in-chat resolver as an approver.
func TestNativePromptHasNoChatApproval(t *testing.T) {
	rs := resolveRef(t, "confirmation/confirmation.schema.json#/$defs/ConfirmationRequired")
	doc := readJSON(t, "confirmation/examples/ConfirmationRequired/ui-native-prompt.json").(map[string]any)
	doc["resolvers"] = []any{"agui-interrupt", "desktop"}
	if err := rs.Validate(doc); err == nil {
		t.Error("requires_native_prompt with agui-interrupt as a resolver must not validate")
	}
	doc = readJSON(t, "confirmation/examples/ConfirmationRequired/ui-native-prompt.json").(map[string]any)
	doc["summary"].(map[string]any)["class"] = "write"
	if err := rs.Validate(doc); err == nil {
		t.Error("requires_native_prompt on a write confirmation must not validate")
	}
}

func TestDerivedWebMCPAnnotations(t *testing.T) {
	c := mustLoad(t)
	rules, err := LoadRules("derivations/webmcp.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Tools map[string]map[string]bool `json:"tools"`
	}
	readGolden(t, &golden)
	if len(golden.Tools) != len(c.Tools) {
		t.Errorf("golden table has %d tools, catalog %d", len(golden.Tools), len(c.Tools))
	}
	for _, tl := range c.Tools {
		got, err := c.Derive(rules, tl)
		if err != nil {
			t.Fatal(err)
		}
		want, ok := golden.Tools[tl.Name]
		if !ok {
			t.Errorf("%s missing from the golden table", tl.Name)
			continue
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s: derived %s=%v, golden %v", tl.Name, k, got[k], v)
			}
		}
		if len(got) != len(want) {
			t.Errorf("%s: derived %v, golden %v", tl.Name, slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)))
		}
		if tl.ReturnsUntrustedContent && got["untrustedContentHint"] != true {
			t.Errorf("%s: untrusted content without untrustedContentHint", tl.Name)
		}
	}
	for _, f := range []string{"derivations/scopes.json"} {
		r, err := LoadRules(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, tl := range c.Tools {
			if _, err := c.Derive(r, tl); err != nil {
				t.Error(err)
			}
		}
	}
}

func TestBindings(t *testing.T) {
	c := mustLoad(t)
	var comps struct {
		Components []struct{ Name, Kind string } `json:"components"`
		ChatRules  map[string]string             `json:"chatRules"`
	}
	readInto(t, "derivations/a2ui-components.json", &comps)
	views := map[string]bool{}
	for _, comp := range comps.Components {
		if comp.Kind == "view" {
			views[comp.Name] = true
		}
	}
	var conv struct {
		Schemes []struct {
			Scheme string   `json:"scheme"`
			Views  []string `json:"views"`
		} `json:"schemes"`
	}
	readInto(t, "resources/uri-conventions.json", &conv)
	appViews := map[string]bool{}
	for _, s := range conv.Schemes {
		for _, v := range s.Views {
			appViews[v] = true
			if err := resolveRef(t, "resources/resource-uri.schema.json#/$defs/UiUri").Validate("ui://ipfs-mcp/" + v); err != nil {
				t.Errorf("view %s has no valid ui:// URI: %v", v, err)
			}
		}
	}
	var skills struct {
		Skills []struct {
			ID     string   `json:"id"`
			Groups []string `json:"groups"`
		} `json:"skills"`
	}
	readInto(t, "agentic/a2a-skills.json", &skills)
	skillGroups := map[string][]string{}
	covered := map[string]int{}
	for _, s := range skills.Skills {
		skillGroups[s.ID] = s.Groups
		for _, g := range s.Groups {
			if _, ok := c.Group(g); !ok {
				t.Errorf("skill %s names unknown group %s", s.ID, g)
			}
			covered[g]++
		}
	}
	for _, g := range c.Groups {
		if covered[g.Name] != 1 {
			t.Errorf("group %s is covered by %d A2A skills, want 1", g.Name, covered[g.Name])
		}
	}
	for _, tl := range c.Tools {
		if comp := tl.UI.A2UIComponent; comp != nil && !views[*comp] {
			t.Errorf("%s: unknown A2UI view component %s", tl.Name, *comp)
		}
		if v := tl.UI.AppView; v != nil && !appViews[*v] {
			t.Errorf("%s: unknown MCP App view %s", tl.Name, *v)
		}
		if !slices.Contains(skillGroups[tl.A2A.Skill], tl.Group) {
			t.Errorf("%s: A2A skill %q does not cover group %s", tl.Name, tl.A2A.Skill, tl.Group)
		}
		for name, rule := range comps.ChatRules {
			if _, err := c.EvalRule(rule, tl); err != nil {
				t.Errorf("chat rule %s: %v", name, err)
			}
		}
	}
}

// TestRPCPathsExist checks every required command path against Kubo's own
// command tree, so a typo cannot hide a tool on every daemon.
func TestRPCPathsExist(t *testing.T) {
	for _, tl := range mustLoad(t).Tools {
		for _, p := range tl.AllowedRPC() {
			cmd := commands.Root
			for seg := range strings.SplitSeq(p, "/") {
				next, ok := cmd.Subcommands[seg]
				if !ok {
					t.Errorf("%s: /api/v0/%s is not a Kubo command", tl.Name, p)
					break
				}
				cmd = next
			}
		}
	}
}

func readGolden(t *testing.T, v any) {
	t.Helper()
	data, err := readTestdata("webmcp-expected.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}

func without(a, b []string) []string {
	var out []string
	for _, x := range a {
		if !slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}

func TestInvalidSchemaIsNamed(t *testing.T) {
	err := checkAgainstMeta(t, "schemas/tools/broken.input.json", []byte(`{"type": 5, "properties": []}`))
	if err == nil || !strings.Contains(err.Error(), "schemas/tools/broken.input.json") {
		t.Fatalf("expected an error naming the invalid schema file, got %v", err)
	}
}
