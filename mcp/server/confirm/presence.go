package confirm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"unicode"

	"github.com/ipfs/kubo/mcp/server"
)

// ErrUnavailable marks a presence factor that cannot be used on this host. It
// is the fail-closed outcome: the call is refused, never approved.
var ErrUnavailable = errors.New("native confirmation prompt is unavailable")

// factor is one presence mechanism. It must return true only after the
// operating system authenticated the person at the machine.
type factor interface {
	// name is the resolver name reported in the confirmation.
	name() string
	// check reports whether the mechanism can be used now. It wraps
	// ErrUnavailable when it cannot.
	check(ctx context.Context) error
	// authenticate shows the system prompt. (true, nil) is an authenticated
	// person; (false, nil) is a person who declined or failed; an error means
	// the prompt could not be shown and nothing was decided.
	authenticate(ctx context.Context, reason string) (bool, error)
}

// nativeResolver adapts a factor to server.Resolver. One system prompt is
// shown at a time.
type nativeResolver struct {
	f   factor
	log *slog.Logger
	one sync.Mutex
}

func newNativeResolver(f factor, log *slog.Logger) *nativeResolver {
	return &nativeResolver{f: f, log: log}
}

// Name implements server.Resolver.
func (r *nativeResolver) Name() string { return r.f.name() }

// Request implements server.Resolver. It checks that the mechanism is usable,
// then prompts in the background and resolves once.
func (r *nativeResolver) Request(ctx context.Context, p server.Pending, resolve server.ResolveFunc) error {
	if reason, off := disabledMechanism(r.f.name()); off {
		return fmt.Errorf("%w: disabled after a failed HG-1 check: %s", ErrUnavailable, reason)
	}
	if err := r.f.check(ctx); err != nil {
		return err
	}
	go r.prompt(ctx, p, resolve)
	return nil
}

func (r *nativeResolver) prompt(ctx context.Context, p server.Pending, resolve server.ResolveFunc) {
	r.one.Lock()
	defer r.one.Unlock()
	if ctx.Err() != nil {
		return // expired while queued: expiry is denial
	}
	ok, err := r.f.authenticate(ctx, reasonFor(p))
	switch {
	case err != nil && ctx.Err() != nil:
		// Expired during the prompt; the broker already denies it.
	case err != nil:
		r.log.Warn("native confirmation prompt failed; denying", "resolver", r.f.name(), "id", p.ID, "error", err)
		_ = resolve(false)
	default:
		if rerr := resolve(ok); rerr != nil {
			r.log.Warn("confirmation was not recorded", "resolver", r.f.name(), "id", p.ID, "error", rerr)
		}
	}
}

// maxReasonRunes keeps the dialog readable.
const maxReasonRunes = 240

// reasonFor is the text the system dialog shows. It names the exact
// operation; control characters are dropped so a tool argument cannot shape
// the dialog.
func reasonFor(p server.Pending) string {
	text := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, p.Summary.Text)
	text = strings.Join(strings.Fields(text), " ")
	if rs := []rune(text); len(rs) > maxReasonRunes {
		text = string(rs[:maxReasonRunes-3]) + "..."
	}
	return "Approve IPFS MCP operation: " + text
}
