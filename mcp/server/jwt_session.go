package server

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

// SessionPrincipal is a configured principal a session subject can resolve to.
type SessionPrincipal struct {
	// Identity is what the principal is granted, as for a static bearer.
	Identity Identity
	// Subjects are the `sub` values that resolve to it.
	Subjects []string
	// RequireAMR is the effective step-up rule: at least one of these must be
	// in the token's `amr`. Empty requires none.
	RequireAMR []string
	// MaxAuthAge is the effective bound on now - auth_time. Zero sets none; a
	// positive value also makes `auth_time` mandatory.
	MaxAuthAge time.Duration
}

// Kind defaults of the step-up rule (ADR-0013 section 5): an admin session
// needs a second factor and a sign-in within the hour. Other kinds need none.
var defaultAdminAMR = []string{"totp", "webauthn", "passkey", "lookup_secret"}

const defaultAdminMaxAuthAge = time.Hour

// stepUpOK reports whether the token meets the principal's step-up rule. A
// token without `auth_time` fails where a bound applies, and so does one whose
// `auth_time` lies in the future beyond the clock skew.
func (p SessionPrincipal) stepUpOK(vt VerifiedToken, now time.Time, skew time.Duration) bool {
	if len(p.RequireAMR) > 0 && !slices.ContainsFunc(vt.AMR, func(m string) bool { return slices.Contains(p.RequireAMR, m) }) {
		return false
	}
	if p.MaxAuthAge > 0 {
		if vt.AuthTime.IsZero() || vt.AuthTime.After(now.Add(skew)) || now.Sub(vt.AuthTime) > p.MaxAuthAge {
			return false
		}
	}
	return true
}

// Session is what the verifier knows about the person behind a session
// identity. It is set only on identities a JWT produced and never holds the
// token.
type Session struct {
	// Subject is the `sub` claim, logged in audit records.
	Subject string
	// Email is the `email` claim, for display only.
	Email string
	// Expiry is the `exp` claim.
	Expiry time.Time
	// Principals lists every principal the subject maps to (name and kind),
	// whatever the route.
	Principals []Principal
	// Admin is the admin step-up state for the session report: AdminOK,
	// AdminStepUpRequired or AdminNotEligible.
	Admin string
}

// Values of Session.Admin, the admin field of the session report.
const (
	AdminOK             = "ok"
	AdminStepUpRequired = "step_up_required"
	AdminNotEligible    = "not_eligible"
)

// Denial is a refusal of a request whose token is valid: the caller is
// authenticated but this request may not proceed. It carries the HTTP status
// the listener answers with.
type Denial struct {
	Status  int
	Code    string
	Message string
}

// Denial codes. They are HTTP-level and say nothing about the token.
const (
	DenialPermissionDenied = "permission_denied"
	// DenialStepUpRequired is the settings error code of ADR-0013 section 5.
	DenialStepUpRequired = "step_up_required"
)

func (d *Denial) Error() string { return d.Code + ": " + d.Message }

// oidcRouteOf names the auth.oidc.routes key of a request path, or "" for a
// path no key covers.
func oidcRouteOf(p string) string {
	under := func(route string) bool { return p == route || strings.HasPrefix(p, route+"/") }
	switch {
	case under(RouteMCP):
		return OIDCRouteMCP
	case under(RouteA2A):
		return OIDCRouteA2A
	case under(RouteAGUI):
		return OIDCRouteAGUI
	case under(RouteSettings):
		return OIDCRouteSettings
	case under(RouteIndexSearch), under(RouteIndexStatus):
		return OIDCRouteIndex
	case p == RouteAuthSession:
		return OIDCRouteSession
	}
	return ""
}

// RouteAuthSession is the session report route (contract routes/session.json).
const RouteAuthSession = "/auth/session"

// oidcRouteKinds are the principal kinds that may serve a route, in the order
// a subject mapped to several of them resolves. They follow the kinds each
// route is mounted with, minus the kinds that cannot be mapped by subject:
// /agui and /index/* are the console chat, the settings route is admin only,
// and /mcp and /a2a take a subject only where a human-mcp or ui principal lists
// it. a2a has no kind that takes a subject.
func oidcRouteKinds(route string) []string {
	switch route {
	case OIDCRouteMCP:
		return []string{KindHumanMCP, KindUI}
	case OIDCRouteAGUI:
		return []string{KindUI}
	case OIDCRouteIndex:
		return []string{KindUI, KindHumanMCP}
	case OIDCRouteSettings:
		return []string{KindAdmin}
	case OIDCRouteSession:
		return []string{KindUI, KindHumanMCP, KindAdmin}
	}
	return nil
}

// sessionKinds are the kinds a subject can be mapped to.
func isSessionKind(kind string) bool {
	return kind == KindUI || kind == KindHumanMCP || kind == KindAdmin
}

// indexSessionPrincipals indexes principals by subject and rejects a subject
// listed twice for one kind.
func indexSessionPrincipals(in []SessionPrincipal) (map[string][]SessionPrincipal, error) {
	out := make(map[string][]SessionPrincipal)
	for _, p := range in {
		if p.Identity.ID == "" || !isSessionKind(p.Identity.Kind) {
			return nil, fmt.Errorf("principal %q (%s) cannot be mapped by subject", p.Identity.ID, p.Identity.Kind)
		}
		for _, sub := range p.Subjects {
			for _, other := range out[sub] {
				if other.Identity.Kind == p.Identity.Kind {
					return nil, fmt.Errorf("principals[].subjects: a subject is listed by principals %q and %q of kind %s", other.Identity.ID, p.Identity.ID, p.Identity.Kind)
				}
			}
			out[sub] = append(out[sub], p)
		}
	}
	return out, nil
}

// resolve picks the principal a valid token acts as on route. Email is never
// consulted: only `sub`, matched exactly, selects a principal.
func (v *JWTVerifier) resolve(vt VerifiedToken, route string) (Identity, error) {
	mapped := v.subjects[vt.Subject]
	var chosen *SessionPrincipal
	for _, kind := range oidcRouteKinds(route) {
		if i := slices.IndexFunc(mapped, func(p SessionPrincipal) bool { return p.Identity.Kind == kind }); i >= 0 {
			chosen = &mapped[i]
			break
		}
	}
	if chosen == nil {
		v.log.Warn("session subject maps to no principal on this route", "sub", vt.Subject, "route", route)
		return Identity{}, &Denial{Status: http.StatusForbidden, Code: DenialPermissionDenied, Message: "no principal is configured for this session on this route"}
	}
	now := v.now()
	admin := AdminNotEligible
	for _, p := range mapped {
		if p.Identity.Kind == KindAdmin {
			admin = AdminOK
			if !p.stepUpOK(vt, now, v.cfg.ClockSkew) {
				admin = AdminStepUpRequired
			}
		}
	}
	// The session report only reports; every other route enforces the rule of
	// the principal it resolved to.
	if route != OIDCRouteSession && !chosen.stepUpOK(vt, now, v.cfg.ClockSkew) {
		v.log.Warn("session needs a stronger or newer sign-in", "sub", vt.Subject, "principal", chosen.Identity.ID, "route", route)
		return Identity{}, &Denial{Status: http.StatusForbidden, Code: DenialStepUpRequired, Message: "verify again with a second factor to continue"}
	}
	id := chosen.Identity
	id.Session = &Session{
		Subject:    vt.Subject,
		Email:      vt.Email,
		Expiry:     vt.Expiry,
		Principals: sessionPrincipals(mapped),
		Admin:      admin,
	}
	return id, nil
}

// sessionPrincipals lists the principals of a subject in a stable order.
func sessionPrincipals(mapped []SessionPrincipal) []Principal {
	out := make([]Principal, 0, len(mapped))
	for _, p := range mapped {
		out = append(out, p.Identity.Principal)
	}
	slices.SortFunc(out, func(a, b Principal) int { return strings.Compare(a.ID, b.ID) })
	return out
}
