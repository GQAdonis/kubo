package contract

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	commonFile      = "schemas/common.json"
	commonRefPrefix = "../common.json#/$defs/"
	localRefPrefix  = "#/$defs/"
)

// BundleSchema returns the tool schema at path (for example
// "schemas/tools/pin_ls.input.json") as a self-contained JSON Schema: every
// reference into schemas/common.json is rewritten to a local "#/$defs/..."
// reference and the referenced definitions are copied into the schema's
// $defs. MCP clients receive this form as inputSchema and outputSchema.
func BundleSchema(path string) ([]byte, error) {
	raw, err := files.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	common, err := commonDefs()
	if err != nil {
		return nil, err
	}

	need := map[string]struct{}{}
	if err := rewriteRefs(root, need); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	defs := map[string]any{}
	if existing, ok := root["$defs"].(map[string]any); ok {
		defs = existing
	}
	queue := make([]string, 0, len(need))
	for n := range need {
		queue = append(queue, n)
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if _, done := defs[name]; done {
			continue
		}
		def, ok := common[name]
		if !ok {
			return nil, fmt.Errorf("%s: common.json has no $defs/%s", path, name)
		}
		more := map[string]struct{}{}
		if err := rewriteRefs(def, more); err != nil {
			return nil, fmt.Errorf("common.json $defs/%s: %w", name, err)
		}
		defs[name] = def
		for m := range more {
			queue = append(queue, m)
		}
	}
	if len(defs) > 0 {
		root["$defs"] = defs
	}
	return json.Marshal(root)
}

func commonDefs() (map[string]any, error) {
	raw, err := files.ReadFile(commonFile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", commonFile, err)
	}
	var doc struct {
		Defs map[string]any `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", commonFile, err)
	}
	return doc.Defs, nil
}

// rewriteRefs rewrites common.json references in v to local ones and records
// every local definition name it points to. References to other files are
// rejected: tool schemas may only depend on common.json.
func rewriteRefs(v any, need map[string]struct{}) error {
	switch x := v.(type) {
	case map[string]any:
		if ref, ok := x["$ref"].(string); ok {
			switch {
			case strings.HasPrefix(ref, commonRefPrefix):
				name := strings.TrimPrefix(ref, commonRefPrefix)
				x["$ref"] = localRefPrefix + name
				need[name] = struct{}{}
			case strings.HasPrefix(ref, localRefPrefix):
				need[strings.TrimPrefix(ref, localRefPrefix)] = struct{}{}
			default:
				return fmt.Errorf("unsupported $ref %q", ref)
			}
		}
		for k, child := range x {
			if k == "$ref" {
				continue
			}
			if err := rewriteRefs(child, need); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range x {
			if err := rewriteRefs(child, need); err != nil {
				return err
			}
		}
	}
	return nil
}
