package server

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"
)

// Restriction reasons for grants withdrawn by policy.
const (
	// ReasonShellCapableUnasserted: cli confirmation is on and the principal has
	// no shell_capable: false assertion.
	ReasonShellCapableUnasserted = "shell_capable_unasserted"
	// ReasonUARCheckFailed: the UAR preflight refused the principal.
	ReasonUARCheckFailed = "uar_check_failed"
)

// uarCheckTimeout bounds one whole preflight (card, probe and settings).
const uarCheckTimeout = 25 * time.Second

// uarGuard runs and caches the per-principal UAR preflight.
type uarGuard struct {
	checker *UARChecker
	now     func() time.Time
	log     *slog.Logger

	mu         sync.Mutex
	kuboListen string
	entries    map[string]*uarEntry
}

type uarEntry struct {
	mu     sync.Mutex
	have   bool
	at     time.Time
	report UARReport
}

func newUARGuard(log *slog.Logger, now func() time.Time, opts ...UARCheckerOption) *uarGuard {
	return &uarGuard{checker: NewUARChecker(nil, now, opts...), now: now, log: log, entries: map[string]*uarEntry{}}
}

func (g *uarGuard) setKuboListen(addr string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.kuboListen = addr
}

// report returns the principal's verdict. fresh forces a new check, which a
// session initialize always does; otherwise a verdict younger than the
// contract's recheck interval (agentic/uar-preflight.json) is reused. A
// settings change is therefore seen on the next request after the interval,
// and on the next session at once.
func (g *uarGuard) report(ctx context.Context, id Identity, fresh bool) UARReport {
	g.mu.Lock()
	e, ok := g.entries[id.ID]
	if !ok {
		e = &uarEntry{}
		g.entries[id.ID] = e
	}
	listen := g.kuboListen
	g.mu.Unlock()

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.have && !fresh && g.now().Sub(e.at) < g.checker.recheck() {
		return e.report
	}
	// The check does not depend on the request: a client that gives up must not
	// leave a refusal in the cache for everyone else.
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), uarCheckTimeout)
	defer cancel()
	rep := g.checker.Check(cctx, UARCheckInput{UAR: *id.UAR, Check: id.Check, EgressAck: id.EgressAck, KuboListen: listen})
	if !e.have || e.report.Verdict() != rep.Verdict() || e.report.Revision != rep.Revision {
		g.log.Warn("uar preflight", "principal", id.ID, "verdict", rep.Verdict(), "version", rep.Version,
			"revision", rep.Revision, "findings", rep.Codes())
	}
	e.have, e.at, e.report = true, g.now(), rep
	return rep
}

// cliConfirmActive reports whether the headless CLI resolver is enabled.
func (s *Server) cliConfirmActive() bool {
	return slices.ContainsFunc(s.gate.list(), func(r Resolver) bool { return r.Name() == ResolverCLI })
}

func hasWrite(id Identity) bool {
	return len(id.WriteGroups) > 0 || len(id.DestructiveGroups) > 0
}

// withoutWrites returns the identity with its write and destructive grants
// withdrawn.
func withoutWrites(id Identity, why []Restriction) Identity {
	return Identity{
		Principal:    id.Principal,
		Scopes:       []string{ScopeRead},
		Groups:       slices.Clone(id.Groups),
		Configured:   id.Configured,
		EgressAck:    slices.Clone(id.EgressAck),
		BackendHosts: slices.Clone(id.BackendHosts),
		ShellCapable: id.ShellCapable,
		Check:        id.Check,
		UAR:          id.UAR,
		Restrictions: why,
	}
}

// shellRestriction is the restriction for a write principal without the
// shell_capable: false assertion while --confirm=cli is on, or nil.
func (s *Server) shellRestriction(id Identity) *Restriction {
	if !hasWrite(id) || !s.cliConfirmActive() {
		return nil
	}
	if id.ShellCapable != nil && !*id.ShellCapable {
		return nil
	}
	if id.ID == OperatorPrincipalID {
		// The --auth-token-* token has no config entry to carry the assertion,
		// and its holder is usually a coding agent with a shell. No exemption.
		return &Restriction{
			Reason: ReasonShellCapableUnasserted,
			Message: "cli confirmation (--confirm=cli or confirmation.cli) is on and the --auth-token-* operator identity has no shell_capable: false assertion; the CLI prompt has " +
				"no presence factor and the token is typically given to a coding agent that has a shell, so write groups are refused. " +
				"To grant writes under cli confirmation, configure a principal in --mcp-config with kind human-mcp, its own credential, " +
				"the write_groups it needs and shell_capable: false",
		}
	}
	return &Restriction{
		Reason: ReasonShellCapableUnasserted,
		Message: "cli confirmation (--confirm=cli or confirmation.cli) is on and principal " + id.ID + " has no shell_capable: false assertion; the CLI prompt has no " +
			"presence factor, so a principal that may have a shell cannot be given write groups. Set shell_capable: false " +
			"only if the principal has no shell-capable tool",
	}
}

// applyPolicy returns the effective identity for a request: the configured
// grants minus whatever policy withdraws. A non-nil ToolError means the
// principal is disabled and the request is refused. initialize forces a fresh
// UAR preflight.
func (s *Server) applyPolicy(ctx context.Context, id Identity, initialize bool) (Identity, *ToolError) {
	var why []Restriction
	if r := s.shellRestriction(id); r != nil {
		why = append(why, *r)
	}
	if id.Kind == KindUAR && id.UAR != nil {
		rep := s.uar.report(ctx, id, initialize)
		switch rep.Verdict() {
		case VerdictRefuse:
			return id, &ToolError{
				Code:    CodePermissionDenied,
				Message: "principal " + id.ID + " (uar) is disabled by the check-uar preflight: " + rep.Summary(),
				Details: map[string]any{"reason": ReasonUARCheckFailed, "findings": rep.Findings},
			}
		case VerdictReadOnly:
			why = append(why, rep.Restrictions()...)
		}
	}
	if len(why) == 0 || !hasWrite(id) {
		return id, nil
	}
	return withoutWrites(id, why), nil
}

// StartupReport returns the lines ipfs mcp prints at startup about the
// configured principals: every grant policy withdraws and the outcome of the
// UAR preflight. It runs the preflight, so it contacts the operator-configured
// UAR address of each kind uar principal and no other.
func (s *Server) StartupReport(ctx context.Context, ids []Identity) []string {
	var lines []string
	for _, id := range ids {
		if r := s.shellRestriction(id); r != nil {
			lines = append(lines, fmt.Sprintf("principal %s: write groups refused: %s", id.ID, r.Message))
		}
		if id.Kind != KindUAR || id.UAR == nil {
			continue
		}
		rep := s.uar.report(ctx, id, true)
		switch v := rep.Verdict(); v {
		case VerdictOK:
			lines = append(lines, fmt.Sprintf("principal %s: check-uar ok (UAR %s)", id.ID, rep.Version))
		default:
			lines = append(lines, fmt.Sprintf("principal %s: check-uar %s: %s (run ipfs mcp check-uar for details)", id.ID, v, rep.Codes()))
		}
		if id.Check == CheckAssert {
			lines = append(lines, fmt.Sprintf("principal %s: check: assert is set; the operator asserts that the UAR instance "+
				"uses only local or acknowledged model and embedder hosts, has governance on, no shell-capable tool and no MCP server besides ipfs mcp, "+
				"and that it refuses unauthenticated requests; Kubo has not verified this", id.ID))
		}
	}
	return lines
}
