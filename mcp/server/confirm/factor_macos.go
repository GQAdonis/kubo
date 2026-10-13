package confirm

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ipfs/kubo/mcp/server"
)

// laScript is the LocalAuthentication helper. It is embedded in the binary and
// interpreted by the system's /usr/bin/swift reading it from stdin, so there
// is no cgo, no bundled executable and no helper file on disk that a same-user
// process could swap.
//
//go:embed la_prompt.swift
var laScript string

const (
	swiftPath = "/usr/bin/swift"

	// Exit codes of la_prompt.swift.
	laAuthenticated = 0
	laUnavailable   = 2
	laNotAuthentic  = 3

	// probeTimeout bounds the availability probe. It does not follow the request
	// that triggered it, because its result is kept for the life of the process.
	probeTimeout = 30 * time.Second

	envLAReason = "IPFS_MCP_LA_REASON"
	envLAProbe  = "IPFS_MCP_LA_PROBE"
)

// runner runs a command and returns its exit code. A non-nil error means the
// command could not be run at all. It is replaced in tests.
type runner func(ctx context.Context, name string, args, env []string, stdin string) (int, error)

func execRunner(ctx context.Context, name string, args, env []string, stdin string) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = safeEnv(os.Environ(), env)
	cmd.Stdin = strings.NewReader(stdin)
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	if err != nil {
		return -1, err
	}
	return 0, nil
}

// macFactor asks macOS LocalAuthentication for the device-owner policy: Touch
// ID, an Apple Watch or the login password. A background process that was
// granted Accessibility can click the dialog's buttons but cannot supply the
// presence factor itself (see MECHANISMS.md).
type macFactor struct {
	run          runner
	swift        string
	probeTimeout time.Duration
	probe        sync.Once
	probed       error
}

func newMacFactor(run runner) *macFactor {
	return &macFactor{run: run, swift: swiftPath, probeTimeout: probeTimeout}
}

func (m *macFactor) name() string { return server.ResolverMacOSLocalAuth }

// check runs the swift probe once: the policy must be evaluable on this
// session (a logged-in GUI session with a password or biometrics).
func (m *macFactor) check(ctx context.Context) error {
	m.probe.Do(func() {
		if _, err := os.Stat(m.swift); err != nil {
			m.probed = fmt.Errorf("%w: %s not found (install the Xcode command line tools: xcode-select --install)", ErrUnavailable, m.swift)
			return
		}
		pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.probeTimeout)
		defer cancel()
		code, err := m.run(pctx, m.swift, []string{"-"}, []string{envLAProbe + "=1"}, laScript)
		switch {
		case pctx.Err() != nil:
			m.probed = fmt.Errorf("%w: the availability probe did not finish within %s", ErrUnavailable, m.probeTimeout)
		case err != nil:
			m.probed = fmt.Errorf("%w: %v", ErrUnavailable, err)
		case code != laAuthenticated:
			m.probed = fmt.Errorf("%w: LocalAuthentication cannot evaluate the device-owner policy in this session (exit %d)", ErrUnavailable, code)
		}
	})
	return m.probed
}

func (m *macFactor) authenticate(ctx context.Context, reason string) (bool, error) {
	code, err := m.run(ctx, m.swift, []string{"-"}, []string{envLAReason + "=" + reason}, laScript)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	switch code {
	case laAuthenticated:
		return true, nil
	case laNotAuthentic:
		return false, nil
	default:
		return false, fmt.Errorf("%w: helper exited %d", ErrUnavailable, code)
	}
}
