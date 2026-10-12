package contract

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// confirmationClasses are the known catalog confirmation classes.
var confirmationClasses = []string{ClassNone, ClassWrite, ClassDestructive}

// RuleFile is a derivation rule file in derivations/.
type RuleFile struct {
	Source      string            `json:"source"`
	Description string            `json:"description"`
	Rules       map[string]string `json:"rules"`
}

// LoadRules reads a rule file such as "derivations/webmcp.json".
func LoadRules(path string) (*RuleFile, error) {
	data, err := files.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var r RuleFile
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &r, nil
}

// Derive evaluates every rule of r for tool t and returns the values by
// rule name. Rule forms: always, copy:<field>, not-equal:<field>:<value>,
// in:<field>:<v>,<v>... and map:<field>:<k>=<v>,... (see derivations/derivation-rules.schema.json).
func (c *Catalog) Derive(r *RuleFile, t Tool) (map[string]any, error) {
	out := make(map[string]any, len(r.Rules))
	for name, rule := range r.Rules {
		v, err := c.EvalRule(rule, t)
		if err != nil {
			return nil, fmt.Errorf("rule %s for %s: %w", name, t.Name, err)
		}
		out[name] = v
	}
	return out, nil
}

// EvalRule evaluates one rule for tool t.
func (c *Catalog) EvalRule(rule string, t Tool) (any, error) {
	if rule == "always" {
		return true, nil
	}
	kind, rest, ok := strings.Cut(rule, ":")
	if !ok {
		return nil, fmt.Errorf("malformed rule %q", rule)
	}
	switch kind {
	case "copy":
		return c.field(rest, t)
	case "not-equal":
		f, want, ok := strings.Cut(rest, ":")
		if !ok {
			return nil, fmt.Errorf("malformed rule %q", rule)
		}
		v, err := c.field(f, t)
		if err != nil {
			return nil, err
		}
		return fmt.Sprint(v) != want, nil
	case "in":
		// Allow-list: true only when the field's value is one of the listed
		// values. A confirmation class outside the known set is an error, so a
		// new class is never exposed by default (R13 security L7).
		f, list, ok := strings.Cut(rest, ":")
		if !ok || list == "" {
			return nil, fmt.Errorf("malformed rule %q", rule)
		}
		v, err := c.field(f, t)
		if err != nil {
			return nil, err
		}
		s := fmt.Sprint(v)
		if f == "confirmation" && !slices.Contains(confirmationClasses, s) {
			return nil, fmt.Errorf("rule %q: unknown confirmation class %q", rule, s)
		}
		return slices.Contains(strings.Split(list, ","), s), nil
	case "map":
		f, pairs, ok := strings.Cut(rest, ":")
		if !ok {
			return nil, fmt.Errorf("malformed rule %q", rule)
		}
		v, err := c.field(f, t)
		if err != nil {
			return nil, err
		}
		for pair := range strings.SplitSeq(pairs, ",") {
			k, val, _ := strings.Cut(pair, "=")
			if k == fmt.Sprint(v) {
				return val, nil
			}
		}
		return nil, fmt.Errorf("rule %q has no value for %v", rule, v)
	}
	return nil, fmt.Errorf("unknown rule kind in %q", rule)
}

func (c *Catalog) field(name string, t Tool) (any, error) {
	switch name {
	case "annotations.readOnlyHint":
		return t.Annotations.ReadOnlyHint, nil
	case "confirmation":
		return t.Confirmation, nil
	case "returnsUntrustedContent":
		return t.ReturnsUntrustedContent, nil
	case "contentEgress":
		return t.ContentEgress, nil
	case "group.diagnostic":
		g, ok := c.Group(t.Group)
		if !ok {
			return nil, fmt.Errorf("unknown group %q", t.Group)
		}
		return g.Diagnostic, nil
	}
	return nil, fmt.Errorf("unknown field %q", name)
}
