package contract

import (
	"testing"
)

const testCID = "bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi"

// TestPathPatternsRejectDotSegments pins R13 security L2: path cleaning turns
// "/a/../ipns/x" into "/ipns/x", so every path type rejects "." and ".."
// segments, and MFS paths also reject an empty first segment ("//ipfs/x").
func TestPathPatternsRejectDotSegments(t *testing.T) {
	cases := []struct {
		ref    string
		accept []string
		reject []string
	}{
		{
			"schemas/common.json#/$defs/MfsPath",
			[]string{"/", "/a", "/a/b/", "/a//b", "/ipfs", "/ipnsy/z", "/a/ipfs/x", "/.hidden", "/..x", "/a/..."},
			[]string{"/.", "/..", "/a/../ipns/x.example", "/a/./b", "/a/..", "//ipfs/x", "//a", "/a/../../ipfs/" + testCID},
		},
		{
			"schemas/common.json#/$defs/MfsCopySource",
			[]string{"/a/b", "/ipfs/" + testCID, "/ipfs/" + testCID + "/sub/file.txt", "/ipns"},
			[]string{"/ipfs/" + testCID + "/../../ipns/x", "/a/../ipns/x", "/ipfs/../ipns/x", "/ipfs/" + testCID + "/./a"},
		},
		{
			"schemas/common.json#/$defs/ContentPath",
			[]string{testCID, testCID + "/a/b.txt", "/ipfs/" + testCID + "/a", "/ipns/example.com", "/ipns/example.com/a/"},
			[]string{"/ipfs/" + testCID + "/../../ipns/evil.example", testCID + "/../x", "/ipns/..", "/ipns/.", "/ipfs//x", "/ipns/a/./b", "../x"},
		},
		{
			"schemas/common.json#/$defs/IpnsName",
			[]string{"/ipns/example.com", "example.com/a/b", "/ipns/k51x/sub/"},
			[]string{"..", "/ipns/..", "example.com/../x", "/ipns/a/./b", ".example.com"},
		},
		{
			"resources/resource-uri.schema.json#/$defs/IpfsUri",
			[]string{"ipfs://" + testCID, "ipfs://" + testCID + "/a/b.txt"},
			[]string{"ipfs://" + testCID + "/../x", "ipfs://" + testCID + "/a/./b"},
		},
		{
			"resources/resource-uri.schema.json#/$defs/IpnsUri",
			[]string{"ipns://example.com", "ipns://example.com/a"},
			[]string{"ipns://..", "ipns://.x", "ipns://a/../b"},
		},
	}
	for _, tc := range cases {
		rs := resolveRef(t, tc.ref)
		for _, v := range tc.accept {
			if err := rs.Validate(v); err != nil {
				t.Errorf("%s rejects %q: %v", tc.ref, v, err)
			}
		}
		for _, v := range tc.reject {
			if rs.Validate(v) == nil {
				t.Errorf("%s accepts %q", tc.ref, v)
			}
		}
	}
}

func TestAddFilenameRejectsDotNames(t *testing.T) {
	in := resolveRef(t, "schemas/tools/ipfs_add.input.json")
	base := func(name string) map[string]any {
		return map[string]any{"content": "x", "encoding": "utf8", "pin": false, "provide": false, "filename": name}
	}
	for _, name := range []string{"a.txt", ".env", "..x"} {
		if err := in.Validate(base(name)); err != nil {
			t.Errorf("ipfs_add rejects filename %q: %v", name, err)
		}
	}
	for _, name := range []string{".", "..", "a/b"} {
		if in.Validate(base(name)) == nil {
			t.Errorf("ipfs_add accepts filename %q", name)
		}
	}
}

// TestExposedIsAnAllowList pins R13 security L7: the WebMCP exposed rule
// names the classes it registers, and an unknown class is an error.
func TestExposedIsAnAllowList(t *testing.T) {
	c := mustLoad(t)
	rules, err := LoadRules("derivations/webmcp.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := rules.Rules["exposed"]; got != "in:confirmation:none,write" {
		t.Fatalf("exposed rule = %q, want the allow-list in:confirmation:none,write", got)
	}
	for _, tl := range c.Tools {
		v, err := c.EvalRule(rules.Rules["exposed"], tl)
		if err != nil {
			t.Fatal(err)
		}
		if want := tl.Confirmation != ClassDestructive; v != want {
			t.Errorf("%s (%s): exposed=%v, want %v", tl.Name, tl.Confirmation, v, want)
		}
	}
	probe := c.Tools[0]
	probe.Confirmation = "irreversible"
	if _, err := c.EvalRule("in:confirmation:none,write", probe); err == nil {
		t.Error("an unknown confirmation class must be an error, not a default")
	}
	if _, err := c.EvalRule("in:confirmation:", probe); err == nil {
		t.Error("an empty allow-list must be malformed")
	}
}
