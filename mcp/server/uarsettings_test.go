package server

import (
	"io/fs"
	"net/http"
	"strings"
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const uarFixtureDir = "fixtures/uar/"

// contractFixture reads a recorded settings fixture from the contract.
func contractFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := fs.ReadFile(contract.FS(), uarFixtureDir+name)
	require.NoError(t, err)
	return raw
}

// TestUARContractFixtureVerdicts asserts the verdicts that
// mcp/contract/fixtures/uar/README.md promises, for a principal with an empty
// egress_ack, listen address 127.0.0.1:5050 and check: verify.
func TestUARContractFixtureVerdicts(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		verdict string
		codes   []string // findings that must be present
	}{
		{"settings-shipped-profile.json", VerdictOK, nil},
		{"settings-uar-defaults.json", VerdictRefuse,
			[]string{FindingRemoteModel, FindingRemoteEmbedder, FindingGovernanceDisabled}},
		{"settings-shell-enabled.json", VerdictReadOnly, []string{FindingShellToolEnabled, FindingNonKuboMCPServer}},
		{"settings-no-inventory.json", VerdictReadOnly, []string{FindingInventoryUnreadable}},
		{"observed-records-a7cb9729.json", VerdictRefuse, []string{FindingSettingsUnreadable}},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			u := newFakeUARBytes(t, contractFixture(t, tc.fixture))
			rep := check(t, u.input("1.4.0"))
			assert.Equal(t, tc.verdict, rep.Verdict(), rep.Summary())
			got := codes(rep, "")
			for _, c := range tc.codes {
				assert.Contains(t, got, c)
			}
			if tc.verdict == VerdictOK {
				assert.Empty(t, rep.Findings)
			}
		})
	}
}

func TestUARContractFixturesValidateExceptTheObservedRecords(t *testing.T) {
	entries, err := fs.ReadDir(contract.FS(), "fixtures/uar")
	require.NoError(t, err)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		_, why := decodeUARSettings(contractFixture(t, e.Name()))
		if strings.HasPrefix(e.Name(), "observed-records") {
			assert.NotEmpty(t, why, "the per-key record array is not the contract shape")
			continue
		}
		assert.Empty(t, why, e.Name())
	}
}

func TestCheckUARTopLevelArrayIsUnreadable(t *testing.T) {
	u := newFakeUARBytes(t, []byte(`[{"key":"llm.model","data":"x","revision":3}]`))
	rep := check(t, u.input("1.4.0"))
	assert.Equal(t, VerdictRefuse, rep.Verdict())
	assert.Contains(t, codes(rep, SeverityRefuse), FindingSettingsUnreadable)

	in := u.input("1.4.0")
	in.Check = CheckAssert
	assert.Equal(t, VerdictOK, check(t, in).Verdict(), "assert records the operator's own verification")
}

func TestCheckUARSettingsMustMatchTheSchema(t *testing.T) {
	for name, mod := range map[string]func(*fakeUAR){
		"revision missing":       func(u *fakeUAR) { delete(u.settings, "revision") },
		"revision negative":      func(u *fakeUAR) { u.settings["revision"] = -1 },
		"revision is an object":  func(u *fakeUAR) { u.settings["revision"] = map[string]any{} },
		"provider host is blank": func(u *fakeUAR) { u.field("llm")["provider_host"] = "not a host!" },
		"embedder has no host":   func(u *fakeUAR) { u.settings["memory_embedder"] = map[string]any{"model": "x"} },
		"transport is unknown": func(u *fakeUAR) {
			u.settings["mcp_servers"] = []any{map[string]any{"name": "ipfs", "transport": "carrier-pigeon"}}
		},
		"terminal_exec missing": func(u *fakeUAR) { u.settings["native_tools"] = map[string]any{} },
	} {
		t.Run(name, func(t *testing.T) {
			u := newFakeUAR(t, "profile")
			u.set(mod)
			rep := check(t, u.input("1.4.0"))
			assert.Equal(t, VerdictRefuse, rep.Verdict(), rep.Summary())
			assert.Contains(t, codes(rep, SeverityRefuse), FindingSettingsUnreadable)
		})
	}
}

func TestCheckUARRevisionIsAStringOrAnInteger(t *testing.T) {
	for name, tc := range map[string]struct {
		revision any
		want     string
	}{
		"string":  {"rev-abc", "rev-abc"},
		"integer": {7, "7"},
		"zero":    {0, "0"},
	} {
		t.Run(name, func(t *testing.T) {
			u := newFakeUAR(t, "profile")
			u.set(func(u *fakeUAR) { u.settings["revision"] = tc.revision })
			rep := check(t, u.input("1.4.0"))
			assert.Equal(t, VerdictOK, rep.Verdict(), rep.Summary())
			assert.Equal(t, tc.want, rep.Revision)
		})
	}
}

func TestCheckUARDisabledProviderHasNoEgress(t *testing.T) {
	u := newFakeUAR(t, "profile")
	u.set(func(u *fakeUAR) { u.field("memory_embedder")["provider_host"] = "disabled" })
	rep := check(t, u.input("1.4.0"))
	assert.Equal(t, VerdictOK, rep.Verdict(), "disabled is not an egress: %s", rep.Summary())

	u.set(func(u *fakeUAR) { u.field("memory_embedder")["provider_host"] = "local" })
	assert.Equal(t, VerdictOK, check(t, u.input("1.4.0")).Verdict(), "local is local")

	u.set(func(u *fakeUAR) { u.field("memory_embedder")["provider_host"] = "embed.example.test" })
	assert.Contains(t, codes(check(t, u.input("1.4.0")), SeverityRefuse), FindingRemoteEmbedder)
}

func TestCheckUARShellToolsListWithdrawsWrites(t *testing.T) {
	u := newFakeUAR(t, "profile")
	u.set(func(u *fakeUAR) { u.field("native_tools")["shell_tools"] = []any{"bash", "python_exec"} })
	rep := check(t, u.input("1.4.0"))
	assert.Equal(t, VerdictReadOnly, rep.Verdict(), rep.Summary())
	assert.Contains(t, codes(rep, SeverityReadOnly), FindingShellToolEnabled)
	assert.Contains(t, rep.Summary(), "bash")

	u.set(func(u *fakeUAR) { u.field("native_tools")["shell_tools"] = []any{} })
	assert.Equal(t, VerdictOK, check(t, u.input("1.4.0")).Verdict(), "an empty list is no shell tool")
}

// With no settings path pinned (the shipped contract), nothing under the UAR
// settings API is contacted and the verdict depends on the check mode.
func TestCheckUARWithoutAPinnedSettingsPath(t *testing.T) {
	pre, err := contract.LoadUARPreflight()
	require.NoError(t, err)
	if pre.SettingsPath != nil {
		t.Skip("the contract pins a settings path; this test covers the null pin")
	}
	nopath := func(u *fakeUAR, in UARCheckInput) UARReport {
		return NewUARChecker(nil, nil).Check(t.Context(), in)
	}

	t.Run("verify refuses and never touches the settings API", func(t *testing.T) {
		u := newFakeUAR(t, "profile")
		rep := nopath(u, u.input("1.4.0"))
		assert.Equal(t, VerdictRefuse, rep.Verdict())
		assert.ElementsMatch(t, []string{FindingSettingsUnreadable, FindingAuthProbeInconclusive}, codes(rep, SeverityRefuse))
		assert.Equal(t, 1, u.total(), "only the agent card was read")
		assert.Equal(t, 1, u.hits[uarCardSuffix])
		assert.Empty(t, u.authSeen)
	})

	t.Run("assert records both as info", func(t *testing.T) {
		u := newFakeUAR(t, "profile")
		in := u.input("1.4.0")
		in.Check = CheckAssert
		rep := nopath(u, in)
		assert.Equal(t, VerdictOK, rep.Verdict(), rep.Summary())
		assert.ElementsMatch(t, []string{FindingSettingsUnreadable, FindingAuthProbeInconclusive}, codes(rep, SeverityInfo))
		assert.Equal(t, 1, u.total())
	})

	t.Run("the version check still runs", func(t *testing.T) {
		u := newFakeUAR(t, "profile")
		u.set(func(u *fakeUAR) { u.version = "1.3.0" })
		in := u.input("1.4.0")
		in.Check = CheckAssert
		assert.Contains(t, codes(nopath(u, in), SeverityRefuse), FindingVersionTooOld)

		in = u.input("") // no per-principal minimum, and none pinned
		in.Check = CheckAssert
		assert.Contains(t, codes(nopath(u, in), SeverityRefuse), FindingNoMinVersion)
	})
}

// The shipped example config refuses while the contract pins no minimum
// version: it deliberately sets no uar.min_version (ADR-0007 section 2).
func TestUARPrincipalExampleRefusesWhileNoMinVersionIsPinned(t *testing.T) {
	pre, err := contract.LoadUARPreflight()
	require.NoError(t, err)
	if pre.MinVersion != nil {
		t.Skip("the contract pins a minimum UAR version; the example is no longer expected to refuse for min_version_not_set")
	}
	raw, err := fs.ReadFile(contract.FS(), "config/examples/uar-principal.json")
	require.NoError(t, err)
	cfg, err := ParseConfig(raw)
	require.NoError(t, err)
	p, ok := findConfigPrincipal(cfg, "uar")
	require.True(t, ok)
	require.NotNil(t, p.UAR)
	require.Nil(t, p.UAR.MinVersion, "the example must not set a minimum version")

	u := newFakeUAR(t, "profile")
	in := UARCheckInput{UAR: *p.UAR, Check: p.Check, EgressAck: p.EgressAck, KuboListen: cfg.HTTP.Listen}
	in.UAR.CardURL = u.cardURL() // only the address is replaced
	rep := NewUARChecker(nil, nil).Check(t.Context(), in)
	assert.Equal(t, VerdictRefuse, rep.Verdict())
	got := codes(rep, SeverityRefuse)
	assert.Contains(t, got, FindingNoMinVersion)
	assert.Contains(t, got, FindingAuthProbeInconclusive)
	assert.Contains(t, got, FindingSettingsUnreadable)
}

func findConfigPrincipal(cfg *Config, name string) (PrincipalConfig, bool) {
	for _, p := range cfg.Principals {
		if p.Name == name {
			return p, true
		}
	}
	return PrincipalConfig{}, false
}

// The admin credential is an Authorization: Bearer header, sent only to the
// settings path under the card URL's base, only over https or to loopback.
func TestCheckUARAdminCredentialIsBearerUnderTheCardBase(t *testing.T) {
	t.Setenv(fakeUARAdminEnv, fakeUARAdminToken)
	min := "1.4.0"
	in := func(card string) UARCheckInput {
		return UARCheckInput{UAR: UARConfig{
			CardURL: card, AdminCredential: &SecretRef{Env: fakeUARAdminEnv}, MinVersion: &min,
		}}
	}

	t.Run("https under a path prefix", func(t *testing.T) {
		rt := &rewriteTransport{body: map[string]string{"/prefix" + uarCardSuffix: `{"version":"1.4.0"}`}}
		NewUARChecker(&http.Client{Transport: rt}, nil, WithSettingsPath(testUARSettingsPath)).
			Check(t.Context(), in("https://uar.example.test/prefix"+uarCardSuffix))
		var withCredential []string
		for i, a := range rt.auth {
			if a != "" {
				withCredential = append(withCredential, rt.urls[i])
				assert.Equal(t, "Bearer "+fakeUARAdminToken, a)
			}
		}
		assert.Equal(t, []string{"https://uar.example.test/prefix" + testUARSettingsPath}, withCredential)
	})

	t.Run("loopback over http", func(t *testing.T) {
		u := newFakeUAR(t, "profile")
		rep := check(t, u.input("1.4.0"))
		assert.Equal(t, VerdictOK, rep.Verdict(), rep.Summary())
		assert.Contains(t, u.authSeen, "Bearer "+fakeUARAdminToken)
	})
}
