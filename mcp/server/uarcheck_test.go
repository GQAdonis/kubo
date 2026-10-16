package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func codes(r UARReport, severity string) []string {
	var out []string
	for _, f := range r.Findings {
		if severity == "" || f.Severity == severity {
			out = append(out, f.Code)
		}
	}
	return out
}

func check(t *testing.T, in UARCheckInput) UARReport {
	t.Helper()
	return NewUARChecker(nil, nil, WithSettingsPath(testUARSettingsPath)).Check(t.Context(), in)
}

// withoutPin drops the contract's pinned minimum version and settings path
// (v1.1.0 ships pinned), to cover the unpinned behavior.
func withoutPin() UARCheckerOption {
	return func(c *UARChecker) { c.pre.MinVersion, c.pre.SettingsPath, c.pre.PinnedFrom = nil, nil, nil }
}

func TestCheckUARRefusesUARDefaults(t *testing.T) {
	u := newFakeUAR(t, "defaults")
	u.set(func(u *fakeUAR) { u.requireAuth = false }) // JWT off, as shipped by the embedded config

	rep := check(t, u.input("1.4.0"))
	assert.Equal(t, VerdictRefuse, rep.Verdict())
	got := codes(rep, SeverityRefuse)
	assert.Contains(t, got, FindingAuthNotRequired)
	assert.Contains(t, got, FindingRemoteModel)
	assert.Contains(t, got, FindingRemoteEmbedder)
	assert.Contains(t, codes(rep, SeverityReadOnly), FindingGovernanceDisabled)
	for _, f := range rep.Findings {
		if f.Code == FindingRemoteModel {
			assert.Equal(t, "api.openai.com", f.Host)
		}
	}
}

func TestCheckUARAcceptsShippedProfile(t *testing.T) {
	u := newFakeUAR(t, "profile")
	rep := check(t, u.input("1.4.0"))
	assert.Equal(t, VerdictOK, rep.Verdict(), rep.Summary())
	assert.Empty(t, rep.Findings)
	assert.Equal(t, "1.4.0", rep.Version)
	assert.Equal(t, "profile-1", rep.Revision)
}

func TestCheckUARRemoteProviderNeedsMatchingAck(t *testing.T) {
	u := newFakeUAR(t, "defaults")
	u.set(func(u *fakeUAR) { u.settings["governance"] = map[string]any{"enabled": true} })

	in := u.input("1.4.0")
	assert.Equal(t, VerdictRefuse, check(t, in).Verdict(), "no acknowledgement")

	in.EgressAck = []string{"local"}
	assert.Equal(t, VerdictRefuse, check(t, in).Verdict(), `"local" is valid only when the providers are local`)

	in.EgressAck = []string{"other.example.test"}
	assert.Equal(t, VerdictRefuse, check(t, in).Verdict(), "an unrelated host")

	in.EgressAck = []string{"api.openai.com"}
	rep := check(t, in)
	assert.Equal(t, VerdictOK, rep.Verdict(), rep.Summary())
}

func TestCheckUARVersionMinimumCountsPublishedReleasesOnly(t *testing.T) {
	for _, tc := range []struct {
		name, version, min string
		want               string
		code               string
	}{
		{"equal", "1.4.0", "1.4.0", VerdictOK, ""},
		{"newer", "1.10.0", "1.9.9", VerdictOK, ""},
		{"v prefix", "v2.0.0", "1.4.0", VerdictOK, ""},
		{"older", "1.3.9", "1.4.0", VerdictRefuse, FindingVersionTooOld},
		{"release candidate", "1.5.0-rc1", "1.4.0", VerdictRefuse, FindingVersionNotPublished},
		{"build metadata", "1.5.0+abc1234", "1.4.0", VerdictRefuse, FindingVersionNotPublished},
		{"dev build", "dev", "1.4.0", VerdictRefuse, FindingVersionNotPublished},
		{"empty", "", "1.4.0", VerdictRefuse, FindingVersionNotPublished},
		{"no minimum set", "1.4.0", "", VerdictRefuse, FindingNoMinVersion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := newFakeUAR(t, "profile")
			u.set(func(u *fakeUAR) { u.version = tc.version })
			opts := []UARCheckerOption{WithSettingsPath(testUARSettingsPath)}
			if tc.min == "" {
				opts = append([]UARCheckerOption{withoutPin()}, opts...)
			}
			rep := NewUARChecker(nil, nil, opts...).Check(t.Context(), u.input(tc.min))
			assert.Equal(t, tc.want, rep.Verdict(), rep.Summary())
			if tc.code != "" {
				assert.Contains(t, codes(rep, SeverityRefuse), tc.code)
			}
		})
	}
}

func TestCheckUARAuthProbe(t *testing.T) {
	u := newFakeUAR(t, "profile")
	u.set(func(u *fakeUAR) { u.requireAuth = false })
	rep := check(t, u.input("1.4.0"))
	assert.Equal(t, VerdictRefuse, rep.Verdict())
	assert.Contains(t, codes(rep, SeverityRefuse), FindingAuthNotRequired)

	// 403 is as good as 401.
	u = newFakeUAR(t, "profile")
	u.set(func(u *fakeUAR) { u.settingsStatus = http.StatusForbidden })
	rep = check(t, u.input("1.4.0"))
	assert.NotContains(t, codes(rep, ""), FindingAuthNotRequired)
}

func TestCheckUARUnreadableSettingsFailClosedUnlessAsserted(t *testing.T) {
	for name, mod := range map[string]func(*fakeUAR, *UARCheckInput){
		"no admin credential configured": func(_ *fakeUAR, in *UARCheckInput) { in.UAR.AdminCredential = nil },
		"admin variable unset": func(_ *fakeUAR, in *UARCheckInput) {
			in.UAR.AdminCredential = &SecretRef{Env: "TEST_UAR_ADMIN_TOKEN_UNSET"}
		},
		"admin token rejected": func(_ *fakeUAR, in *UARCheckInput) {
			t.Setenv("TEST_UAR_WRONG", "wrong-admin-token-0123456789")
			in.UAR.AdminCredential = &SecretRef{Env: "TEST_UAR_WRONG"}
		},
		"server error": func(u *fakeUAR, _ *UARCheckInput) {
			u.set(func(u *fakeUAR) { u.settingsStatus = http.StatusInternalServerError })
		},
		"not json": func(u *fakeUAR, _ *UARCheckInput) {
			u.set(func(u *fakeUAR) { u.settings = nil })
		},
		"model not exposed": func(u *fakeUAR, _ *UARCheckInput) {
			u.set(func(u *fakeUAR) { delete(u.settings, "llm") })
		},
		"embedder not exposed": func(u *fakeUAR, _ *UARCheckInput) {
			u.set(func(u *fakeUAR) { delete(u.settings, "memory_embedder") })
		},
	} {
		t.Run(name, func(t *testing.T) {
			u := newFakeUAR(t, "profile")
			in := u.input("1.4.0")
			mod(u, &in)

			rep := check(t, in)
			assert.Equal(t, VerdictRefuse, rep.Verdict(), "verify fails closed: %s", rep.Summary())
			assert.Contains(t, codes(rep, SeverityRefuse), FindingSettingsUnreadable)

			in.Check = CheckAssert
			rep = check(t, in)
			assert.Equal(t, VerdictOK, rep.Verdict(), "assert waives what cannot be read: %s", rep.Summary())
			assert.Contains(t, codes(rep, SeverityInfo), FindingSettingsAsserted)
		})
	}
}

func TestCheckUARAssertDoesNotWaiveWhatIsReadable(t *testing.T) {
	u := newFakeUAR(t, "defaults")
	in := u.input("1.4.0")
	in.Check = CheckAssert
	rep := check(t, in)
	assert.Equal(t, VerdictRefuse, rep.Verdict(), "a readable remote model is refused even with assert")
	assert.Contains(t, codes(rep, SeverityRefuse), FindingRemoteModel)

	// Assert never waives the version or the unauthenticated-access finding.
	u = newFakeUAR(t, "profile")
	u.set(func(u *fakeUAR) { u.version = "0.9.0"; u.requireAuth = false })
	in = u.input("1.4.0")
	in.Check = CheckAssert
	got := codes(check(t, in), SeverityRefuse)
	assert.Contains(t, got, FindingVersionTooOld)
	assert.Contains(t, got, FindingAuthNotRequired)
}

func TestCheckUARShellToolsAndForeignMCPServersWithdrawWrites(t *testing.T) {
	for name, tc := range map[string]struct {
		mod  func(u *fakeUAR)
		code string
	}{
		"terminal_exec enabled": {func(u *fakeUAR) { u.field("native_tools")["terminal_exec_enabled"] = true }, FindingShellToolEnabled},
		"another MCP server": {func(u *fakeUAR) {
			u.settings["mcp_servers"] = []any{
				map[string]any{"name": "ipfs", "transport": "http", "url": "http://127.0.0.1:5050/mcp"},
				map[string]any{"name": "fs", "transport": "stdio", "command": "mcp-fs"},
			}
		}, FindingNonKuboMCPServer},
		"ipfs mcp on another port": {func(u *fakeUAR) {
			u.settings["mcp_servers"] = []any{map[string]any{"name": "ipfs", "transport": "http", "url": "http://127.0.0.1:5999/mcp"}}
		}, FindingNonKuboMCPServer},
		"governance disabled":   {func(u *fakeUAR) { u.settings["governance"] = map[string]any{"enabled": false} }, FindingGovernanceDisabled},
		"inventory not exposed": {func(u *fakeUAR) { delete(u.settings, "native_tools") }, FindingInventoryUnreadable},
		"mcp list not exposed":  {func(u *fakeUAR) { delete(u.settings, "mcp_servers") }, FindingInventoryUnreadable},
	} {
		t.Run(name, func(t *testing.T) {
			u := newFakeUAR(t, "profile")
			u.set(tc.mod)
			rep := check(t, u.input("1.4.0"))
			assert.Equal(t, VerdictReadOnly, rep.Verdict(), rep.Summary())
			assert.Contains(t, codes(rep, SeverityReadOnly), tc.code)
			assert.NotEmpty(t, rep.Restrictions())
		})
	}
}

func TestCheckUARCardProblemsRefuse(t *testing.T) {
	u := newFakeUAR(t, "profile")
	in := u.input("1.4.0")
	in.UAR.CardURL = u.srv.URL + "/missing.json"
	assert.Equal(t, []string{FindingCardUnreadable}, codes(check(t, in), SeverityRefuse))

	u.srv.Close()
	assert.Equal(t, []string{FindingCardUnreadable}, codes(check(t, u.input("1.4.0")), SeverityRefuse), "an unreachable UAR is refused")

	in.UAR.CardURL = "not a url"
	assert.Equal(t, VerdictRefuse, check(t, in).Verdict())
}

// rewriteTransport answers every request in memory and records the
// Authorization header of each, so a test can use a non-loopback host name
// without opening a connection.
type rewriteTransport struct {
	t    *testing.T
	auth []string
	urls []string
	body map[string]string
}

func (r *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	assert.Empty(r.t, req.Header.Get("Authorization"), "the admin key is never sent as a bearer")
	r.auth = append(r.auth, req.Header.Get("X-UAR-Admin-Key"))
	r.urls = append(r.urls, req.URL.String())
	body, ok := r.body[req.URL.Path]
	status := http.StatusOK
	switch {
	case !ok:
		status, body = http.StatusUnauthorized, ""
	}
	return &http.Response{
		StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req,
	}, nil
}

func TestCheckUARNeverSendsTheAdminCredentialInCleartext(t *testing.T) {
	t.Setenv(fakeUARAdminEnv, fakeUARAdminToken)
	rt := &rewriteTransport{t: t, body: map[string]string{uarCardSuffix: `{"version":"1.4.0"}`}}
	checker := NewUARChecker(&http.Client{Transport: rt}, nil, WithSettingsPath(testUARSettingsPath))
	min := "1.4.0"
	rep := checker.Check(t.Context(), UARCheckInput{
		UAR: UARConfig{
			CardURL:         "http://uar.example.test/.well-known/agent-card.json",
			AdminCredential: &SecretRef{Env: fakeUARAdminEnv},
			MinVersion:      &min,
		},
	})
	assert.Equal(t, VerdictRefuse, rep.Verdict(), "unreadable settings fail closed")
	for _, a := range rt.auth {
		assert.Empty(t, a, "no request carried a credential")
	}
	raw, err := json.Marshal(rep)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), fakeUARAdminToken)

	// Over https the credential is sent.
	rt = &rewriteTransport{t: t, body: map[string]string{uarCardSuffix: `{"version":"1.4.0"}`}}
	checker = NewUARChecker(&http.Client{Transport: rt}, nil, WithSettingsPath(testUARSettingsPath))
	checker.Check(t.Context(), UARCheckInput{
		UAR: UARConfig{
			CardURL:         "https://uar.example.test/.well-known/agent-card.json",
			AdminCredential: &SecretRef{Env: fakeUARAdminEnv},
			MinVersion:      &min,
		},
	})
	assert.Contains(t, rt.auth, fakeUARAdminToken)
}

func TestCheckUARDoesNotFollowRedirects(t *testing.T) {
	target := newFakeUAR(t, "profile")
	redirect := newFakeUAR(t, "profile")
	redirect.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == uarCardSuffix {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"1.4.0"}`))
			return
		}
		http.Redirect(w, r, target.srv.URL+r.URL.Path, http.StatusFound)
	})
	rep := check(t, redirect.input("1.4.0"))
	assert.Equal(t, VerdictRefuse, rep.Verdict())
	assert.Zero(t, target.hits[testUARSettingsPath], "the redirect target was never contacted")
	assert.Empty(t, target.authSeen, "no credential reached the redirect target")
}

func TestUARBase(t *testing.T) {
	for in, want := range map[string]string{
		"http://127.0.0.1:8090/.well-known/agent-card.json":           "http://127.0.0.1:8090",
		"https://uar.example.test/prefix/.well-known/agent-card.json": "https://uar.example.test/prefix",
		"http://127.0.0.1:8090/card.json":                             "http://127.0.0.1:8090",
		"http://127.0.0.1:8090":                                       "http://127.0.0.1:8090",
	} {
		got, err := uarBase(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"", "ftp://x/y", "/relative", "http://"} {
		_, err := uarBase(bad)
		assert.Error(t, err, bad)
	}
}
