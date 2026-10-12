package server

import (
	"encoding/json"
	"strings"
)

// pathFields are the argument names that carry MFS paths or content paths.
var pathFields = []string{"path", "paths", "source", "destination", "root", "cid"}

// rejectDotSegments refuses "." and ".." path segments in every path-bearing
// argument before any RPC. Kubo cleans such segments, so /a/../ipns/x would
// otherwise turn an MFS path into a network content path. The contract schema
// rejects them too; this keeps the server safe if a schema is loosened.
func rejectDotSegments(args json.RawMessage) error {
	if len(args) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(args, &m); err != nil {
		return nil // the schema validator owns malformed arguments
	}
	for _, f := range pathFields {
		v, ok := m[f]
		if !ok {
			continue
		}
		var one string
		var many []string
		switch {
		case json.Unmarshal(v, &one) == nil:
			many = []string{one}
		case json.Unmarshal(v, &many) != nil:
			continue
		}
		for _, p := range many {
			if hasDotSegment(p) {
				return invalidArgument(f + " must not contain '.' or '..' path segments")
			}
		}
	}
	return nil
}

func hasDotSegment(p string) bool {
	for seg := range strings.SplitSeq(p, "/") {
		if seg == "." || seg == ".." {
			return true
		}
	}
	return false
}
