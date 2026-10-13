package confirm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ipfs/kubo/mcp/server"
)

// PolkitActionID is the action the operator installs from
// polkit/org.ipfs.kubo.mcp.confirm.policy.
const PolkitActionID = "org.ipfs.kubo.mcp.confirm"

// The polkit tools are run from these absolute paths only. A lookup on $PATH
// would let a same-user process that controls the environment (a ~/.local/bin
// pkcheck that exits 0) attest a human authentication.
const (
	pkactionPath = "/usr/bin/pkaction"
	pkcheckPath  = "/usr/bin/pkcheck"
)

// pkcheck exit codes (pkcheck(1)).
const (
	pkAuthorized   = 0
	pkNoInteractor = 3
)

// polkitFactor asks polkit to authenticate the user (auth_self, no caching)
// for an operator-installed action. Without the action, or without an
// authentication agent in the session, it is unavailable.
type polkitFactor struct {
	run   runner
	pid   int
	pkact string
	pkchk string
	// trust reports whether a helper binary may be run: root-owned and not
	// writable by group or others. Tests replace it.
	trust func(path string) error
}

func newPolkitFactor(run runner, trust func(string) error) *polkitFactor {
	return &polkitFactor{run: run, pid: os.Getpid(), pkact: pkactionPath, pkchk: pkcheckPath, trust: trust}
}

// trustedBinary is the default trust check: the path and its directory resolve
// to root-owned entries that group and others cannot write, so no unprivileged
// process can swap what runs.
func trustedBinary(path string) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for _, p := range []string{resolved, filepath.Dir(resolved)} {
		fi, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		uid, known := fileOwner(fi)
		switch {
		case !known:
			return fmt.Errorf("%s: cannot determine its owner", p)
		case uid != 0:
			return fmt.Errorf("%s is not owned by root", p)
		case fi.Mode().Perm()&0o022 != 0:
			return fmt.Errorf("%s is writable by group or others (mode %v)", p, fi.Mode().Perm())
		}
	}
	return nil
}

func (p *polkitFactor) name() string { return server.ResolverLinuxPolkit }

func (p *polkitFactor) check(ctx context.Context) error {
	for _, bin := range []string{p.pkact, p.pkchk} {
		if err := p.trust(bin); err != nil {
			return fmt.Errorf("%w: %s cannot be trusted (install polkit, and keep it root-owned): %v", ErrUnavailable, bin, err)
		}
	}
	code, err := p.run(ctx, p.pkact, []string{"--action-id", PolkitActionID}, nil, "")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if code != 0 {
		return fmt.Errorf("%w: the polkit action %s is not installed; install mcp/server/confirm/polkit/%s.policy as root (see polkit/INSTALL.md)", ErrUnavailable, PolkitActionID, PolkitActionID)
	}
	return nil
}

func (p *polkitFactor) authenticate(ctx context.Context, reason string) (bool, error) {
	// The binary is checked again at the moment of use, not only at probe time.
	if err := p.trust(p.pkchk); err != nil {
		return false, fmt.Errorf("%w: %s cannot be trusted: %v", ErrUnavailable, p.pkchk, err)
	}
	args := []string{
		"--action-id", PolkitActionID,
		"--process", strconv.Itoa(p.pid),
		"--allow-user-interaction",
		"--detail", "summary", reason,
	}
	code, err := p.run(ctx, p.pkchk, args, nil, "")
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	switch code {
	case pkAuthorized:
		return true, nil
	case pkNoInteractor:
		return false, fmt.Errorf("%w: no polkit authentication agent in this session", ErrUnavailable)
	default:
		// Not authorized, or the person dismissed the dialog.
		return false, nil
	}
}
