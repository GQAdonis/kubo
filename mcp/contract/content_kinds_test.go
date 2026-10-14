package contract

import "testing"

// TestContentKinds pins the vector-search-contract 1.3 acceptance: the
// content-kinds derivation validates against its schema, every listed MIME
// type maps to exactly one kind, and text/plain and text/markdown map to the
// text kind with the single stage native.
func TestContentKinds(t *testing.T) {
	doc := readJSON(t, "derivations/content-kinds.json")
	if err := resolveRef(t, "derivations/content-kinds.schema.json").Validate(doc); err != nil {
		t.Fatalf("content-kinds.json does not match content-kinds.schema.json: %v", err)
	}
	var ck struct {
		Kinds map[string]struct {
			Stages []struct {
				Stage  string `json:"stage"`
				Source string `json:"source"`
			} `json:"stages"`
		} `json:"kinds"`
		MimeTypes map[string]string `json:"mimeTypes"`
	}
	readInto(t, "derivations/content-kinds.json", &ck)

	counts := make(map[string]int, len(ck.Kinds))
	for mt, kind := range ck.MimeTypes {
		if _, ok := ck.Kinds[kind]; !ok {
			t.Errorf("%s maps to unknown kind %q", mt, kind)
			continue
		}
		counts[kind]++
	}
	for kind := range ck.Kinds {
		if counts[kind] == 0 {
			t.Errorf("kind %q has no MIME types in the map", kind)
		}
	}

	for _, mt := range []string{"text/plain", "text/markdown"} {
		if ck.MimeTypes[mt] != "text" {
			t.Errorf("%s must map to kind text, got %q", mt, ck.MimeTypes[mt])
		}
	}
	text := ck.Kinds["text"]
	if len(text.Stages) != 1 || text.Stages[0].Stage != "native" || text.Stages[0].Source != "text" {
		t.Errorf("kind text must have the single stage native with source text, got %+v", text.Stages)
	}
}
