package server

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/libp2p/go-libp2p/core/peer"
)

// parseTrustedPeers decodes trusted_peers into a set. Trust is by libp2p peer
// ID only; any other string is refused so an agent version or an address
// cannot be listed by mistake.
func parseTrustedPeers(ids []string) (map[peer.ID]struct{}, error) {
	set := make(map[peer.ID]struct{}, len(ids))
	for _, s := range ids {
		id, err := peer.Decode(s)
		if err != nil {
			return nil, fmt.Errorf("trusted_peers: %q is not a libp2p peer ID: %w", s, err)
		}
		set[id] = struct{}{}
	}
	return set, nil
}

// markPeerSupplied sets the boolean untrusted on every object that holds one of
// the catalog's peerSuppliedFields (JSON pointer paths where "*" matches each
// array element): false only when the object's id is a trusted peer. It
// returns the result as generic JSON, and reports whether any entry is
// untrusted.
func markPeerSupplied(out any, fields []string, trusted map[peer.ID]struct{}) (any, bool, error) {
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, false, fmt.Errorf("encode result: %w", err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, false, fmt.Errorf("decode result: %w", err)
	}
	anyUntrusted := false
	for _, f := range fields {
		segs := strings.Split(strings.TrimPrefix(f, "/"), "/")
		// The last segment is the field; the objects holding it get the marker.
		walkHolders(doc, segs[:len(segs)-1], func(o map[string]any) {
			id, _ := o["id"].(string)
			pid, derr := peer.Decode(id)
			_, ok := trusted[pid]
			untrusted := derr != nil || !ok
			o["untrusted"] = untrusted
			anyUntrusted = anyUntrusted || untrusted
		})
	}
	return doc, anyUntrusted, nil
}

func walkHolders(v any, segs []string, fn func(map[string]any)) {
	if len(segs) == 0 {
		if o, ok := v.(map[string]any); ok {
			fn(o)
		}
		return
	}
	switch x := v.(type) {
	case map[string]any:
		if segs[0] != "*" {
			walkHolders(x[segs[0]], segs[1:], fn)
		}
	case []any:
		if segs[0] == "*" {
			for _, e := range x {
				walkHolders(e, segs[1:], fn)
			}
		}
	}
}
