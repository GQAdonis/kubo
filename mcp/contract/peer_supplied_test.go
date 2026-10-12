package contract

import (
	"slices"
	"strings"
	"testing"
)

// ownNodeAddressTools return the node's own addresses, which are not
// peer-supplied (operator decision 2026-10-12, security L5).
var ownNodeAddressTools = []string{"ipfs_node_info"}

// TestPeerSuppliedFieldsAreMarked pins the field-level untrusted marker: each
// peerSuppliedFields path exists in the output schema, and the object that
// holds it has an untrusted property of type PeerSuppliedUntrusted.
func TestPeerSuppliedFieldsAreMarked(t *testing.T) {
	c := mustLoad(t)
	marked := 0
	for _, tl := range c.Tools {
		if len(tl.PeerSuppliedFields) == 0 {
			continue
		}
		var out map[string]any
		readInto(t, tl.OutputSchema, &out)
		for _, f := range tl.PeerSuppliedFields {
			holder, ok := schemaHolder(out, f)
			if !ok {
				t.Errorf("%s: peerSuppliedFields %s does not resolve in %s", tl.Name, f, tl.OutputSchema)
				continue
			}
			props, _ := holder["properties"].(map[string]any)
			u, _ := props["untrusted"].(map[string]any)
			if u["$ref"] != "../common.json#/$defs/PeerSuppliedUntrusted" {
				t.Errorf("%s: the object holding %s has no untrusted PeerSuppliedUntrusted marker", tl.Name, f)
			}
			if req, _ := holder["required"].([]any); slices.Contains(req, any("untrusted")) {
				t.Errorf("%s: untrusted must stay optional (absent is read as true) for backward compatibility", tl.Name)
			}
			marked++
		}
	}
	if marked != 3 {
		t.Errorf("marked %d peer-supplied fields, want 3 (swarm_peers, routing_findpeer, routing_findprovs)", marked)
	}
}

// TestEveryPeerAddressIsDeclared fails when a tool output carries multiaddrs
// but neither lists them as peer-supplied nor is a known own-node tool.
func TestEveryPeerAddressIsDeclared(t *testing.T) {
	c := mustLoad(t)
	for _, tl := range c.Tools {
		data, err := files.ReadFile(tl.OutputSchema)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "#/$defs/Multiaddr") {
			continue
		}
		if len(tl.PeerSuppliedFields) == 0 && !slices.Contains(ownNodeAddressTools, tl.Name) {
			t.Errorf("%s returns multiaddrs but declares no peerSuppliedFields", tl.Name)
		}
		if len(tl.PeerSuppliedFields) > 0 && tl.ContentEgress {
			t.Errorf("%s: the peer-supplied marker is independent of contentEgress (OQ-EGRESS-ENUM)", tl.Name)
		}
	}
}

func TestAnyRule(t *testing.T) {
	c := mustLoad(t)
	for _, tl := range c.Tools {
		got, err := c.EvalRule("any:returnsUntrustedContent,peerSupplied", tl)
		if err != nil {
			t.Fatal(err)
		}
		if want := tl.ReturnsUntrustedContent || len(tl.PeerSuppliedFields) > 0; got != want {
			t.Errorf("%s: any rule = %v, want %v", tl.Name, got, want)
		}
	}
	if _, err := c.EvalRule("any:", c.Tools[0]); err == nil {
		t.Error("an empty any rule must be malformed")
	}
	if _, err := c.EvalRule("any:confirmation", c.Tools[0]); err == nil {
		t.Error("any over a non-boolean field must be an error")
	}
}

// schemaHolder walks a slash path such as /peers/*/addr through properties
// and items and returns the object schema that holds the last segment.
func schemaHolder(root map[string]any, path string) (map[string]any, bool) {
	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	cur := root
	for i, seg := range segs {
		if seg == "*" {
			items, ok := cur["items"].(map[string]any)
			if !ok {
				return nil, false
			}
			cur = items
			continue
		}
		props, ok := cur["properties"].(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := props[seg].(map[string]any)
		if !ok {
			return nil, false
		}
		if i == len(segs)-1 {
			return cur, true
		}
		cur = next
	}
	return nil, false
}
