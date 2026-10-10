package contract

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	toolNameRe    = regexp.MustCompile(`^(ipfs|pin|ipns|key|mfs|dag|car|routing|swarm|stats|repo)_[a-z0-9]+(_[a-z0-9]+)*$`)
	placeholderRe = regexp.MustCompile(`\{([a-z0-9_]+)\}`)
)

// prefixGrantingCommands are RPC commands no tool may list. Kubo matches
// API.Authorizations AllowedPaths by string prefix, so an entry for one of
// these grants far more than the tool needs: /api/v0/config also allows
// config writes (two arguments), config/replace and config/profile/apply.
var prefixGrantingCommands = []string{"config", "key", "repo", "files", "pin", "name", "dag", "routing", "provide"}

// Check enforces the cross-field rules of the catalog that JSON Schema does
// not express. It returns every violation, each naming the tool.
func (c *Catalog) Check() error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	groups := make(map[string]int, len(c.Groups))
	for _, g := range c.Groups {
		if _, dup := groups[g.Name]; dup {
			add("group %q defined twice", g.Name)
		}
		groups[g.Name] = 0
	}
	if sel := c.ToolSelection.DestructiveSelector; sel == "" {
		add("toolSelection.destructiveSelector is empty")
	} else if _, clash := groups[sel]; clash {
		add("destructive selector %q collides with a group name", sel)
	}
	if slices.Contains(c.ToolSelection.Default.Classes, ClassDestructive) {
		add("toolSelection.default.classes must not contain destructive")
	}
	for _, g := range c.ToolSelection.Default.Groups {
		if _, ok := groups[g]; !ok {
			add("toolSelection.default.groups names unknown group %q", g)
		}
	}

	seen := make(map[string]struct{}, len(c.Tools))
	for _, t := range c.Tools {
		if _, dup := seen[t.Name]; dup {
			add("tool %q defined twice", t.Name)
		}
		seen[t.Name] = struct{}{}
		if n, ok := groups[t.Group]; ok {
			groups[t.Group] = n + 1
		} else {
			add("tool %q: unknown group %q", t.Name, t.Group)
		}
		for _, e := range checkTool(t, c.MinKuboVersion) {
			add("tool %q: %s", t.Name, e)
		}
	}
	for g, n := range groups {
		if n == 0 {
			add("group %q has no tools", g)
		}
	}
	return errors.Join(errs...)
}

func checkTool(t Tool, globalMin string) []string {
	var errs []string
	a := t.Annotations
	if !toolNameRe.MatchString(t.Name) {
		errs = append(errs, "name is not snake_case with a known prefix")
	}
	switch t.Confirmation {
	case ClassNone:
		if !a.ReadOnlyHint || a.DestructiveHint {
			errs = append(errs, "confirmation none requires readOnlyHint true and destructiveHint false")
		}
		if t.ConfirmationSummary != "" {
			errs = append(errs, "confirmationSummary is only for write and destructive tools")
		}
	case ClassWrite, ClassDestructive:
		if a.ReadOnlyHint {
			errs = append(errs, fmt.Sprintf("confirmation %s requires readOnlyHint false", t.Confirmation))
		}
		if t.Confirmation == ClassDestructive && !a.DestructiveHint {
			errs = append(errs, "confirmation destructive requires destructiveHint true")
		}
		if t.ConfirmationSummary == "" {
			errs = append(errs, "write and destructive tools need a confirmationSummary")
		}
	default:
		errs = append(errs, fmt.Sprintf("unknown confirmation class %q", t.Confirmation))
	}
	if t.ReturnsUntrustedContent && !t.ContentEgress {
		errs = append(errs, "returnsUntrustedContent requires contentEgress")
	}
	if cmp, err := compareVersions(t.MinKuboVersion, globalMin); err != nil {
		errs = append(errs, err.Error())
	} else if cmp < 0 {
		errs = append(errs, fmt.Sprintf("minKuboVersion %s is below the global minimum %s", t.MinKuboVersion, globalMin))
	}
	for _, p := range t.RPCOptional {
		if slices.Contains(t.RPC, p) {
			errs = append(errs, fmt.Sprintf("rpcOptional %s is also in rpc", p))
		}
	}
	for _, p := range slices.Concat(t.RPC, t.RPCOptional) {
		if slices.Contains(prefixGrantingCommands, p) {
			errs = append(errs, fmt.Sprintf("rpc %s is a prefix of more privileged commands and must not be listed", p))
		}
	}
	if want := "schemas/tools/" + t.Name + ".input.json"; t.InputSchema != want {
		errs = append(errs, "inputSchema must be "+want)
	}
	if want := "schemas/tools/" + t.Name + ".output.json"; t.OutputSchema != want {
		errs = append(errs, "outputSchema must be "+want)
	}
	return errs
}

// SummaryPlaceholders returns the input property names a confirmation
// summary template refers to.
func SummaryPlaceholders(tmpl string) []string {
	var out []string
	for _, m := range placeholderRe.FindAllStringSubmatch(tmpl, -1) {
		out = append(out, m[1])
	}
	return out
}

func compareVersions(a, b string) (int, error) {
	pa, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	pb, err := parseVersion(b)
	if err != nil {
		return 0, err
	}
	for i := range pa {
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1, nil
			}
			return 1, nil
		}
	}
	return 0, nil
}

func parseVersion(v string) ([3]int, error) {
	var out [3]int
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, fmt.Errorf("version %q is not MAJOR.MINOR.PATCH", v)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, fmt.Errorf("version %q: %w", v, err)
		}
		out[i] = n
	}
	return out, nil
}
