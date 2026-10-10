package server

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/kubo/mcp/contract"
)

const (
	maxSummaryRunes = 500
	maxTargetRunes  = 4096
	summaryMinRunes = 5
)

var placeholderRe = regexp.MustCompile(`\{([a-z_]+)\}`)

// buildSummary renders the catalog's confirmationSummary template with the
// call's arguments, falling back to the input schema defaults for arguments
// the caller left out, and lists what the call touches.
func buildSummary(t contract.Tool, rawSchema json.RawMessage, args map[string]any) Summary {
	defaults := schemaDefaults(rawSchema)
	value := func(name string) (any, bool) {
		if v, ok := args[name]; ok {
			return v, true
		}
		v, ok := defaults[name]
		return v, ok
	}

	text := placeholderRe.ReplaceAllStringFunc(t.ConfirmationSummary, func(m string) string {
		name := m[1 : len(m)-1]
		v, ok := value(name)
		if !ok {
			return "default"
		}
		return formatSummaryValue(v)
	})
	text = clipRunes(strings.TrimSpace(text), maxSummaryRunes)
	if utf8.RuneCountInString(text) < summaryMinRunes {
		text = clipRunes(t.Name+" "+text, maxSummaryRunes)
	}

	var targets []Target
	for _, name := range summaryTargetKeys(t.ConfirmationSummary) {
		s, ok := args[name].(string)
		if !ok || s == "" {
			continue
		}
		kind, value := targetKind(t.Group, s)
		targets = append(targets, Target{Kind: kind, Value: clipRunes(value, maxTargetRunes)})
	}
	if targets == nil {
		targets = []Target{}
	}
	return Summary{Tool: t.Name, Class: t.Confirmation, Text: text, Targets: targets}
}

// summaryTargetKeys lists, in template order, the placeholders that name a
// thing the call affects: paths, names and CIDs rather than flags.
func summaryTargetKeys(tmpl string) []string {
	var keys []string
	for _, name := range contract.SummaryPlaceholders(tmpl) {
		if slices.Contains(keys, name) {
			continue
		}
		keys = append(keys, name)
	}
	return keys
}

// targetKind classifies a target value by the tool group it came from. A
// content path that is just /ipfs/<cid> is reported as the bare CID.
func targetKind(group, value string) (kind, normalized string) {
	switch group {
	case "mfs":
		return "mfs_path", value
	case "names":
		if strings.HasPrefix(value, "/ipns/") {
			return "ipns_name", value
		}
		return "key", value
	}
	if rest, ok := strings.CutPrefix(value, "/ipfs/"); ok {
		if _, err := cid.Decode(rest); err == nil {
			return "cid", rest
		}
	}
	if _, err := cid.Decode(value); err == nil {
		return "cid", value
	}
	return "content_path", value
}

func formatSummaryValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case nil:
		return "default"
	default:
		return fmt.Sprint(x)
	}
}

func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// schemaDefaults reads the default of each top-level input property.
func schemaDefaults(raw json.RawMessage) map[string]any {
	var doc struct {
		Properties map[string]struct {
			Default any `json:"default"`
		} `json:"properties"`
	}
	out := make(map[string]any)
	if err := json.Unmarshal(raw, &doc); err != nil {
		return out
	}
	for k, p := range doc.Properties {
		if p.Default != nil {
			out[k] = p.Default
		}
	}
	return out
}
