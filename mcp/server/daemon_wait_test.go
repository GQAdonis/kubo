package server

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flakyDaemon fails the version call with err for the first failures calls and
// then answers like the embedded fake daemon.
type flakyDaemon struct {
	*fakeDaemon
	mu       sync.Mutex
	failures int
	err      error
	attempts int
}

func (f *flakyDaemon) Call(ctx context.Context, command string, args []string, opts map[string]any, out any) error {
	if command == "version" {
		f.mu.Lock()
		f.attempts++
		fail := f.attempts <= f.failures
		f.mu.Unlock()
		if fail {
			return f.err
		}
	}
	return f.fakeDaemon.Call(ctx, command, args, opts, out)
}

// waitClock is a clock whose sleep only moves time forward, so a wait of
// minutes costs no real time.
type waitClock struct {
	t      time.Time
	sleeps []time.Duration
}

func (c *waitClock) now() time.Time { return c.t }

func (c *waitClock) sleep(_ context.Context, d time.Duration) error {
	c.sleeps = append(c.sleeps, d)
	c.t = c.t.Add(d)
	return nil
}

func (c *waitClock) slept() time.Duration {
	var total time.Duration
	for _, d := range c.sleeps {
		total += d
	}
	return total
}

func newFlaky(t *testing.T, failures int, err error) *flakyDaemon {
	t.Helper()
	return &flakyDaemon{fakeDaemon: newFakeDaemon(t), failures: failures, err: err}
}

func waitOptions(c *waitClock, wait time.Duration) Options {
	return Options{DaemonWait: wait, now: c.now, daemonSleep: c.sleep}
}

func unreachable() error { return &unreachableErr{msg: "connection refused"} }

// unreachableErr mimics RPCDaemon.mapError: it wraps ErrDaemonUnreachable.
type unreachableErr struct{ msg string }

func (e *unreachableErr) Error() string {
	return ErrDaemonUnreachable.Error() + " at " + testAPI + ": " + e.msg
}
func (e *unreachableErr) Unwrap() error { return ErrDaemonUnreachable }

func TestStartupWaitsForADaemonThatComesUp(t *testing.T) {
	clock := &waitClock{t: time.Unix(1_700_000_000, 0)}
	d := newFlaky(t, 3, unreachable())

	s, err := New(t.Context(), d, waitOptions(clock, DefaultDaemonWait))
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, 4, d.attempts, "three refused attempts, then the fourth succeeds")
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}, clock.sleeps, "bounded exponential backoff")
}

func TestStartupFailsAfterTheWaitBoundWithTheSameError(t *testing.T) {
	clock := &waitClock{t: time.Unix(1_700_000_000, 0)}
	d := newFlaky(t, 1<<30, unreachable())
	var lines []string
	var mu sync.Mutex
	opts := waitOptions(clock, 10*time.Second)
	opts.Logger = slog.New(slog.NewTextHandler(writerFunc(func(p []byte) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, string(p))
	}), &slog.HandlerOptions{Level: slog.LevelInfo}))

	_, err := New(t.Context(), d, opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDaemonUnreachable)
	assert.Contains(t, err.Error(), "cannot use the ipfs daemon API at "+testAPI)
	assert.Contains(t, err.Error(), "connection refused")
	assert.Equal(t, 10*time.Second, clock.slept(), "the wait is bounded by DaemonWait, the last delay is shortened to fit")
	assert.Equal(t, len(clock.sleeps)+1, d.attempts)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, lines, len(clock.sleeps), "one Info line per failed attempt that is retried")
	for _, l := range lines {
		assert.Contains(t, l, "level=INFO")
		assert.Contains(t, l, "not reachable yet")
	}
}

type writerFunc func([]byte)

func (w writerFunc) Write(p []byte) (int, error) { w(p); return len(p), nil }

func TestStartupWaitZeroFailsAtOnce(t *testing.T) {
	clock := &waitClock{t: time.Unix(1_700_000_000, 0)}
	d := newFlaky(t, 1<<30, unreachable())

	_, err := New(t.Context(), d, waitOptions(clock, 0))
	require.ErrorIs(t, err, ErrDaemonUnreachable)
	assert.Equal(t, 1, d.attempts)
	assert.Empty(t, clock.sleeps)
}

func TestStartupDoesNotRetryAnAuthRefusal(t *testing.T) {
	clock := &waitClock{t: time.Unix(1_700_000_000, 0)}
	forbidden := &RPCError{Command: "version", Message: "rpc authorization: forbidden", Code: cmds.ErrForbidden}
	d := newFlaky(t, 1<<30, forbidden)

	_, err := New(t.Context(), d, waitOptions(clock, DefaultDaemonWait))
	var rpcErr *RPCError
	require.ErrorAs(t, err, &rpcErr)
	assert.Equal(t, cmds.ErrForbidden, rpcErr.Code)
	assert.Equal(t, 1, d.attempts, "a 403 from API.Authorizations fails at once")
	assert.Empty(t, clock.sleeps)
}

func TestStartupDoesNotRetryAVersionMismatch(t *testing.T) {
	clock := &waitClock{t: time.Unix(1_700_000_000, 0)}
	d := newFlaky(t, 0, nil)
	d.version = "0.1.0"

	_, err := New(t.Context(), d, waitOptions(clock, DefaultDaemonWait))
	var verErr *VersionError
	require.ErrorAs(t, err, &verErr)
	assert.Equal(t, 1, d.attempts)
	assert.Empty(t, clock.sleeps)
}

func TestStartupWaitStopsWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	d := newFlaky(t, 1<<30, unreachable())
	opts := Options{DaemonWait: time.Minute, daemonSleep: func(context.Context, time.Duration) error {
		cancel()
		return ctx.Err()
	}}

	_, err := New(ctx, d, opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDaemonUnreachable, "the daemon error stays the primary cause")
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, d.attempts)
}

func TestSleepContextHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, sleepContext(ctx, time.Hour), context.Canceled)
	require.NoError(t, sleepContext(t.Context(), time.Nanosecond))
}

func TestDaemonWaitConfig(t *testing.T) {
	t.Run("absent is the 60s default", func(t *testing.T) {
		cfg, err := ParseConfig([]byte(`{}`))
		require.NoError(t, err)
		d, err := cfg.Daemon.WaitDuration()
		require.NoError(t, err)
		assert.Equal(t, 60*time.Second, d)
		assert.Equal(t, DefaultDaemonWait, d)
	})
	t.Run("0s is the off switch", func(t *testing.T) {
		cfg, err := ParseConfig([]byte(`{"daemon":{"wait":"0s"}}`))
		require.NoError(t, err)
		d, err := cfg.Daemon.WaitDuration()
		require.NoError(t, err)
		assert.Zero(t, d)
	})
	t.Run("a value is read", func(t *testing.T) {
		cfg, err := ParseConfig([]byte(`{"daemon":{"wait":"2m"}}`))
		require.NoError(t, err)
		d, err := cfg.Daemon.WaitDuration()
		require.NoError(t, err)
		assert.Equal(t, 2*time.Minute, d)
	})
	for name, body := range map[string]string{
		"not a duration":  `{"daemon":{"wait":"soon"}}`,
		"negative":        `{"daemon":{"wait":"-5s"}}`,
		"over the bound":  `{"daemon":{"wait":"11m"}}`,
		"unknown key":     `{"daemon":{"timeout":"5s"}}`,
		"number not text": `{"daemon":{"wait":60}}`,
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			_, err := ParseConfig([]byte(body))
			require.Error(t, err)
			assert.False(t, strings.Contains(err.Error(), "panic"))
		})
	}
}
