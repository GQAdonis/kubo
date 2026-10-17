package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// adminToken signs a token for the dual-role subject with the given step-up
// claims. A nil authAge omits auth_time.
func (e *sessionEnv) adminToken(amr []string, authAge *time.Duration) string {
	return e.token(sessionSub, func(c map[string]any) {
		c["amr"] = amr
		delete(c, "auth_time")
		if authAge != nil {
			c["auth_time"] = time.Now().Add(-*authAge).Unix()
		}
	})
}

func ageOf(d time.Duration) *time.Duration { return &d }

func TestAdminStepUp(t *testing.T) {
	e := newSessionEnv(t, sessionEnvOptions{principals: stepUpPrincipals(nil, 0)})
	for _, tc := range []struct {
		name  string
		amr   []string
		age   *time.Duration
		allow bool
	}{
		{"password only", []string{"password"}, ageOf(time.Minute), false},
		{"second factor but stale sign-in", []string{"password", "totp"}, ageOf(2 * time.Hour), false},
		{"second factor but no auth_time", []string{"password", "totp"}, nil, false},
		{"second factor, auth_time in the future", []string{"password", "totp"}, ageOf(-time.Hour), false},
		{"second factor, recent sign-in", []string{"password", "totp"}, ageOf(10 * time.Minute), true},
		{"webauthn", []string{"webauthn"}, ageOf(time.Minute), true},
		{"passkey", []string{"passkey"}, ageOf(time.Minute), true},
		{"lookup secret", []string{"lookup_secret"}, ageOf(time.Minute), true},
		{"no amr at all", nil, ageOf(time.Minute), false},
		{"unlisted method", []string{"code"}, ageOf(time.Minute), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := e.call(t, http.MethodGet, RouteSettings, e.adminToken(tc.amr, tc.age), "")
			if tc.allow {
				assert.Equal(t, http.StatusOK, code)
				return
			}
			require.Equal(t, http.StatusForbidden, code)
			var doc settingsErrorBody
			require.NoError(t, json.Unmarshal([]byte(body), &doc), body)
			assert.Equal(t, "step_up_required", doc.Error.Code)
			assert.NotEmpty(t, doc.Error.Message)
		})
	}
}

func TestStepUpAppliesToJWTOnlyAndOnlyWhereConfigured(t *testing.T) {
	e := newSessionEnv(t, sessionEnvOptions{principals: stepUpPrincipals(nil, 0)})
	weak := e.adminToken([]string{"password"}, ageOf(48*time.Hour))
	// The ui principal has no step-up rule: a weak, old session still chats.
	code, body := e.call(t, http.MethodGet, RouteAGUI, weak, "")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok console-ui", body)
}

func TestStepUpConfiguredValues(t *testing.T) {
	t.Run("custom methods and age", func(t *testing.T) {
		e := newSessionEnv(t, sessionEnvOptions{principals: stepUpPrincipals([]string{"webauthn"}, 5*time.Minute)})
		code, _ := e.call(t, http.MethodGet, RouteSettings, e.adminToken([]string{"totp"}, ageOf(time.Minute)), "")
		assert.Equal(t, http.StatusForbidden, code, "totp is not in the configured list")
		code, _ = e.call(t, http.MethodGet, RouteSettings, e.adminToken([]string{"webauthn"}, ageOf(10*time.Minute)), "")
		assert.Equal(t, http.StatusForbidden, code, "older than the configured bound")
		code, _ = e.call(t, http.MethodGet, RouteSettings, e.adminToken([]string{"webauthn"}, ageOf(time.Minute)), "")
		assert.Equal(t, http.StatusOK, code)
	})
	t.Run("explicit empty require_amr accepts a password session", func(t *testing.T) {
		e := newSessionEnv(t, sessionEnvOptions{principals: stepUpPrincipals([]string{}, time.Hour)})
		code, _ := e.call(t, http.MethodGet, RouteSettings, e.adminToken([]string{"password"}, ageOf(time.Minute)), "")
		assert.Equal(t, http.StatusOK, code)
		code, _ = e.call(t, http.MethodGet, RouteSettings, e.adminToken([]string{"password"}, nil), "")
		assert.Equal(t, http.StatusForbidden, code, "the age bound still needs auth_time")
	})
}

func TestStaticBearerIsNeverSubjectToStepUp(t *testing.T) {
	// A static credential has no amr or auth_time; the JWT verifier is not
	// involved and nothing about step-up can reach it.
	v := testVerifier(t)
	id, _, err := v.Verify(t.Context(), tokReader, reqTo(RouteMCP))
	require.NoError(t, err)
	assert.Nil(t, id.Session)
}

func TestSessionReportsAdminState(t *testing.T) {
	e := newSessionEnv(t, sessionEnvOptions{principals: stepUpPrincipals(nil, 0)})
	for _, tc := range []struct {
		name string
		sub  string
		amr  []string
		want string
	}{
		{"admin with second factor", sessionSub, []string{"password", "totp"}, AdminOK},
		{"admin, password only", sessionSub, []string{"password"}, AdminStepUpRequired},
		{"no admin mapping", otherSub, []string{"password", "totp"}, AdminNotEligible},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tok := e.token(tc.sub, func(c map[string]any) { c["amr"] = tc.amr })
			id, _, err := e.jwt.Verify(t.Context(), tok, reqTo(RouteAuthSession))
			require.NoError(t, err, "the session report never fails on step-up")
			require.NotNil(t, id.Session)
			assert.Equal(t, tc.want, id.Session.Admin)
		})
	}
}

func TestConfigStepUpDefaults(t *testing.T) {
	c, err := ParseConfig([]byte(`{"principals":[
		{"name":"u","kind":"ui","subjects":["s1"]},
		{"name":"a","kind":"admin","subjects":["s1"]},
		{"name":"a2","kind":"admin","subjects":["s2"],"require_amr":[],"max_auth_age":"4h"},
		{"name":"a3","kind":"admin","subjects":["s3"],"require_amr":["webauthn"]}
	]}`))
	require.NoError(t, err)
	byName := map[string]SessionPrincipal{}
	for _, p := range c.SessionPrincipals() {
		byName[p.Identity.ID] = p
	}
	assert.Empty(t, byName["u"].RequireAMR, "ui has no default step-up")
	assert.Zero(t, byName["u"].MaxAuthAge)
	assert.ElementsMatch(t, []string{"totp", "webauthn", "passkey", "lookup_secret"}, byName["a"].RequireAMR)
	assert.Equal(t, time.Hour, byName["a"].MaxAuthAge)
	assert.Empty(t, byName["a2"].RequireAMR, "an explicit [] accepts any amr")
	assert.Equal(t, 4*time.Hour, byName["a2"].MaxAuthAge)
	assert.Equal(t, []string{"webauthn"}, byName["a3"].RequireAMR)
	assert.Equal(t, time.Hour, byName["a3"].MaxAuthAge, "an absent max_auth_age keeps the admin default")
}

// stepUpPrincipals is the default principal set with the admin entry's step-up
// rule set to amr (nil: the kind default) and age (0: the kind default).
func stepUpPrincipals(amr []string, age time.Duration) []SessionPrincipal {
	ps := defaultSessionPrincipals()
	for i := range ps {
		if ps[i].Identity.Kind != KindAdmin {
			continue
		}
		ps[i].RequireAMR, ps[i].MaxAuthAge = defaultAdminAMR, defaultAdminMaxAuthAge
		if amr != nil {
			ps[i].RequireAMR = amr
		}
		if age != 0 {
			ps[i].MaxAuthAge = age
		}
	}
	return ps
}
