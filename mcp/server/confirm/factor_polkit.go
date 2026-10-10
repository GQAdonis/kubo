package confirm

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/ipfs/kubo/mcp/server"
)

// PolkitActionID is the action the operator installs from
// polkit/org.ipfs.kubo.mcp.confirm.policy.
const PolkitActionID = "org.ipfs.kubo.mcp.confirm"

// pkcheck exit codes (pkcheck(1)).
const (
	pkAuthorized   = 0
	pkNoInteractor = 3
)

// polkitFactor asks polkit to authenticate the user (auth_self, no caching)
// for an operator-installed action. Without the action, or without an
// authentication agent in the session, it is unavailable.
type polkitFactor struct {
	run    runner
	pid    int
	pkact  string
	pkchk  string
	lookup func(string) (string, error)
}

func newPolkitFactor(run runner, lookup func(string) (string, error)) *polkitFactor {
	return &polkitFactor{run: run, pid: os.Getpid(), pkact: "pkaction", pkchk: "pkcheck", lookup: lookup}
}

func (p *polkitFactor) name() string { return server.ResolverLinuxPolkit }

func (p *polkitFactor) check(ctx context.Context) error {
	for _, bin := range []string{p.pkact, p.pkchk} {
		if _, err := p.lookup(bin); err != nil {
			return fmt.Errorf("%w: %s is not installed (install polkit)", ErrUnavailable, bin)
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
