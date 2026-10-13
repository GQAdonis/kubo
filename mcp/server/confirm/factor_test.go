package confirm

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ranCmd struct {
	name  string
	args  []string
	env   []string
	stdin string
}

// scripted returns a runner that records commands and answers with codes in
// order (the last one repeats).
func scripted(codes ...int) (runner, *[]ranCmd) {
	var ran []ranCmd
	return func(_ context.Context, name string, args, env []string, stdin string) (int, error) {
		ran = append(ran, ranCmd{name, args, env, stdin})
		i := min(len(ran)-1, len(codes)-1)
		return codes[i], nil
	}, &ran
}

func failing(err error) runner {
	return func(context.Context, string, []string, []string, string) (int, error) { return -1, err }
}

func TestMacFactorProbeAndPrompt(t *testing.T) {
	run, ran := scripted(laAuthenticated, laAuthenticated)
	f := newMacFactor(run)
	f.swift = "/bin/sh" // any file that exists
	require.NoError(t, f.check(t.Context()))
	require.NoError(t, f.check(t.Context()))
	require.Len(t, *ran, 1, "the probe runs once")
	assert.Contains(t, (*ran)[0].env, envLAProbe+"=1")
	assert.Equal(t, laScript, (*ran)[0].stdin, "the embedded script is the only code run")
	assert.Equal(t, []string{"-"}, (*ran)[0].args)

	ok, err := f.authenticate(t.Context(), "Approve IPFS MCP operation: Remove a pin")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Contains(t, (*ran)[1].env, envLAReason+"=Approve IPFS MCP operation: Remove a pin")
	assert.NotContains(t, (*ran)[1].env, envLAProbe+"=1")
}

func TestMacFactorOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		run     runner
		wantOK  bool
		wantErr bool
	}{
		{"not authenticated", mustRun(laNotAuthentic), false, false},
		{"unavailable", mustRun(laUnavailable), false, true},
		{"crash", mustRun(139), false, true},
		{"cannot start", failing(errors.New("exec failed")), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newMacFactor(tc.run)
			ok, err := f.authenticate(t.Context(), "x")
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantErr, err != nil)
			if err != nil {
				assert.ErrorIs(t, err, ErrUnavailable)
			}
		})
	}
}

func mustRun(code int) runner {
	r, _ := scripted(code)
	return r
}

func TestMacFactorUnavailableWhenProbeFailsOrSwiftIsMissing(t *testing.T) {
	f := newMacFactor(mustRun(laUnavailable))
	f.swift = "/bin/sh"
	require.ErrorIs(t, f.check(t.Context()), ErrUnavailable)

	missing := newMacFactor(mustRun(0))
	missing.swift = "/nonexistent/swift"
	err := missing.check(t.Context())
	require.ErrorIs(t, err, ErrUnavailable)
	assert.Contains(t, err.Error(), "xcode-select")
}

func TestPolkitFactorRequiresTheInstalledAction(t *testing.T) {
	trusted := func(string) error { return nil }
	run, ran := scripted(1) // pkaction: action unknown
	f := newPolkitFactor(run, trusted)
	err := f.check(t.Context())
	require.ErrorIs(t, err, ErrUnavailable)
	assert.Contains(t, err.Error(), PolkitActionID)
	assert.Contains(t, err.Error(), "INSTALL.md")
	assert.Equal(t, []string{"--action-id", PolkitActionID}, (*ran)[0].args)

	noPolkit := newPolkitFactor(run, func(string) error { return exec.ErrNotFound })
	require.ErrorIs(t, noPolkit.check(t.Context()), ErrUnavailable)

	run, _ = scripted(0)
	require.NoError(t, newPolkitFactor(run, trusted).check(t.Context()))
}

func TestPolkitFactorOutcomes(t *testing.T) {
	trusted := func(string) error { return nil }
	for _, tc := range []struct {
		code    int
		wantOK  bool
		wantErr bool
	}{
		{pkAuthorized, true, false},
		{1, false, false},
		{2, false, false},
		{pkNoInteractor, false, true},
	} {
		run, ran := scripted(tc.code)
		f := newPolkitFactor(run, trusted)
		ok, err := f.authenticate(t.Context(), "Approve IPFS MCP operation: Remove a pin")
		assert.Equal(t, tc.wantOK, ok, tc.code)
		assert.Equal(t, tc.wantErr, err != nil, tc.code)
		args := (*ran)[0].args
		assert.True(t, slices.Contains(args, "--allow-user-interaction"))
		assert.True(t, slices.Contains(args, PolkitActionID))
		assert.True(t, strings.Contains(strings.Join(args, " "), "summary Approve IPFS MCP operation"))
	}
	f := newPolkitFactor(failing(errors.New("exec failed")), trusted)
	ok, err := f.authenticate(t.Context(), "x")
	assert.False(t, ok)
	assert.ErrorIs(t, err, ErrUnavailable)
}
