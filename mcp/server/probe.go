package server

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/ipfs/kubo/mcp/contract"
)

// Unavailability reasons reported by ipfs_node_status. They are the values of
// the reason enum in the ipfs_node_status output schema.
const (
	ReasonMissingRPC   = "missing_rpc_command"
	ReasonDaemonTooOld = "daemon_too_old"
	// ReasonDestructiveDisabled: the tool's group is selected but the
	// destructive selector was not named (ADR-0003, destructive is opt-in).
	ReasonDestructiveDisabled = "destructive_disabled"
	ReasonGroupDisabled       = "group_disabled"
	ReasonReadOnly            = "read_only"
	// ReasonNotConfigured: the feature the tool needs has no configuration.
	ReasonNotConfigured  = "not_configured"
	commandsNotListedMsg = "daemon did not list its RPC commands; per-tool degradation is off"
)

// daemonVersion is the /api/v0/version response.
type daemonVersion struct {
	Version string
	Commit  string
	Repo    string
	System  string
	Golang  string
}

// commandNode is one node of the /api/v0/commands tree.
type commandNode struct {
	Name        string
	Subcommands []commandNode
}

// DaemonInfo is what the server learned from the daemon at startup.
type DaemonInfo struct {
	Version daemonVersion
	// Commands is the set of RPC command paths ("pin/ls"); nil when the
	// daemon would not list them.
	Commands map[string]struct{}
}

// parseVersion reads a Kubo version such as "0.44.0-dev" or "v0.29.0" and
// drops the pre-release and build parts, so a dev build of a release counts as
// that release.
func parseVersion(s string) (semver.Version, error) {
	v, err := semver.ParseTolerant(strings.TrimSpace(s))
	if err != nil {
		return semver.Version{}, fmt.Errorf("parse version %q: %w", s, err)
	}
	v.Pre, v.Build = nil, nil
	return v, nil
}

// versionAtLeast reports whether have >= min on major.minor.patch.
func versionAtLeast(have, min string) (bool, error) {
	h, err := parseVersion(have)
	if err != nil {
		return false, err
	}
	m, err := parseVersion(min)
	if err != nil {
		return false, err
	}
	return h.GTE(m), nil
}

// probe contacts the daemon, enforces the contract's global minimum version
// and reads the command list. A daemon that cannot be reached, or is too old,
// is an error that names the endpoint. A daemon that will not list its
// commands is not: the server then keeps every tool and relies on call-time
// errors.
func probe(ctx context.Context, d Daemon, cat *contract.Catalog, warn func(string)) (*DaemonInfo, error) {
	var info DaemonInfo
	if err := d.Call(ctx, "version", nil, nil, &info.Version); err != nil {
		return nil, fmt.Errorf("cannot use the ipfs daemon API at %s: %w", d.Endpoint(), err)
	}
	ok, err := versionAtLeast(info.Version.Version, cat.MinKuboVersion)
	if err != nil {
		return nil, fmt.Errorf("ipfs daemon API at %s: %w", d.Endpoint(), err)
	}
	if !ok {
		return nil, fmt.Errorf("ipfs daemon API at %s: %w", d.Endpoint(), &VersionError{Found: info.Version.Version, Minimum: cat.MinKuboVersion})
	}

	var root commandNode
	if err := d.Call(ctx, "commands", nil, nil, &root); err != nil {
		warn(commandsNotListedMsg + ": " + err.Error())
		return &info, nil
	}
	info.Commands = flattenCommands(root)
	return &info, nil
}

// Backoff of the startup wait for an unreachable daemon: the first retry comes
// after daemonWaitInitial and each later one after twice the previous delay, up
// to daemonWaitMax.
const (
	daemonWaitInitial = time.Second
	daemonWaitMax     = 5 * time.Second
)

// daemonWait is the startup wait for a daemon that is not reachable yet. Total
// zero turns it off. now and sleep are injected by tests; nil means the real
// clock.
type daemonWait struct {
	total time.Duration
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error
	// warn receives the line for the first failed attempt, so a waiting
	// process is visible at the default log level; nil falls back to info.
	warn func(msg string, args ...any)
	// info receives one line per later failed attempt.
	info func(msg string, args ...any)
}

// sleepContext waits for d or until ctx ends.
func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// probeWithWait runs probe and, while the daemon is unreachable (connection
// refused, no api file yet), retries it with a bounded backoff until w.total
// has passed. Any other failure, such as an RPC refusal (API.Authorizations)
// or an unsupported version, is returned at once. When the wait runs out it
// returns the last probe error, the same error a server without a wait gets.
func probeWithWait(ctx context.Context, d Daemon, cat *contract.Catalog, warn func(string), w daemonWait) (*DaemonInfo, error) {
	now, sleep := w.now, w.sleep
	if now == nil {
		now = time.Now
	}
	if sleep == nil {
		sleep = sleepContext
	}
	deadline := now().Add(w.total)
	delay := daemonWaitInitial
	for attempt := 1; ; attempt++ {
		info, err := probe(ctx, d, cat, warn)
		if err == nil || w.total <= 0 || !errors.Is(err, ErrDaemonUnreachable) {
			return info, err
		}
		remaining := deadline.Sub(now())
		if remaining <= 0 {
			return nil, err
		}
		delay = min(delay, remaining)
		logLine := w.info
		if attempt == 1 && w.warn != nil {
			logLine = w.warn
		}
		if logLine != nil {
			logLine("ipfs daemon API is not reachable yet; retrying",
				"endpoint", d.Endpoint(), "attempt", attempt, "retry_in", delay.String(), "gives_up_in", remaining.String())
		}
		if serr := sleep(ctx, delay); serr != nil {
			return nil, fmt.Errorf("%w (stopped waiting: %w)", err, serr)
		}
		delay = min(delay*2, daemonWaitMax)
	}
}

// flattenCommands turns the command tree into slash-separated paths without
// the root name, matching the contract's rpc entries.
func flattenCommands(root commandNode) map[string]struct{} {
	out := make(map[string]struct{})
	var walk func(prefix string, n commandNode)
	walk = func(prefix string, n commandNode) {
		for _, sub := range n.Subcommands {
			p := sub.Name
			if prefix != "" {
				p = prefix + "/" + sub.Name
			}
			out[p] = struct{}{}
			walk(p, sub)
		}
	}
	walk("", root)
	return out
}

// Unavailable is a catalog tool this server does not offer, with the reason.
type Unavailable struct {
	Name       string   `json:"name"`
	Reason     string   `json:"reason"`
	MissingRPC []string `json:"missing_rpc,omitempty"`
}

// checkTool decides whether the daemon can serve t. It returns nil when it
// can.
func (i *DaemonInfo) checkTool(t contract.Tool) *Unavailable {
	if t.MinKuboVersion != "" {
		if ok, err := versionAtLeast(i.Version.Version, t.MinKuboVersion); err == nil && !ok {
			return &Unavailable{Name: t.Name, Reason: ReasonDaemonTooOld}
		}
	}
	if i.Commands == nil {
		return nil
	}
	var missing []string
	for _, c := range t.RPC {
		if _, ok := i.Commands[c]; !ok {
			missing = append(missing, c)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return &Unavailable{Name: t.Name, Reason: ReasonMissingRPC, MissingRPC: missing}
}
