package server

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// confirmation.resolvers, confirmation.cli and confirmation.ttl (change
// native-confirmation-prompt, task 1.6). The gate semantics are tested in
// mcp/server/confirm (narrowing, cli mode) and cmd/ipfs/kubo (precedence); this
// file covers parsing, validation and the confirmation lifetime.

func TestConfirmationKeysParse(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{"confirmation": {"resolvers": ["linux-polkit", "desktop"], "cli": true, "ttl": "30s"}}`))
	require.NoError(t, err)
	require.NotNil(t, cfg.Confirmation.Resolvers)
	assert.Equal(t, []string{"linux-polkit", "desktop"}, *cfg.Confirmation.Resolvers)
	require.NotNil(t, cfg.Confirmation.CLI)
	assert.True(t, *cfg.Confirmation.CLI)
	ttl, err := cfg.Confirmation.TTLDuration()
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, ttl)

	cfg, err = ParseConfig([]byte(`{"confirmation": {"resolvers": []}}`))
	require.NoError(t, err)
	require.NotNil(t, cfg.Confirmation.Resolvers, "an empty list is kept: it allows no resolver")
	assert.Empty(t, *cfg.Confirmation.Resolvers)
}

func TestConfirmationKeyDefaults(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{}`))
	require.NoError(t, err)
	assert.Nil(t, cfg.Confirmation.Resolvers, "absent: every resolver the modes enable")
	assert.Nil(t, cfg.Confirmation.CLI, "absent: cli is off")
	ttl, err := cfg.Confirmation.TTLDuration()
	require.NoError(t, err)
	assert.Equal(t, DefaultConfirmTTL, ttl)
	assert.Equal(t, 120*time.Second, DefaultConfirmTTL)
}

func TestConfirmationTTLBounds(t *testing.T) {
	for _, tc := range []struct {
		ttl  string
		want time.Duration
		bad  bool
	}{
		{ttl: "1s", want: time.Second},
		{ttl: "5s", want: 5 * time.Second},
		{ttl: "1.5m", want: 90 * time.Second},
		{ttl: "10m", want: MaxConfirmTTL},
		{ttl: "600s", want: MaxConfirmTTL},
		{ttl: "0s", bad: true},
		{ttl: "999ms", bad: true},
		{ttl: "1ms", bad: true},
		{ttl: "601s", bad: true},
		{ttl: "10m1s", bad: true},
		{ttl: "1h", bad: true},
		{ttl: "120", bad: true}, // no unit: the schema's Duration pattern refuses it
		{ttl: "-5s", bad: true},
	} {
		t.Run(tc.ttl, func(t *testing.T) {
			cfg, err := ParseConfig([]byte(`{"confirmation": {"ttl": "` + tc.ttl + `"}}`))
			if tc.bad {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "ttl")
				return
			}
			require.NoError(t, err)
			got, err := cfg.Confirmation.TTLDuration()
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestConfirmationTTLErrorNamesKeyAndBounds(t *testing.T) {
	_, err := ParseConfig([]byte(`{"confirmation": {"ttl": "1h"}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `confirmation.ttl "1h" is out of range: it must be at least 1s and at most 10m0s`)

	_, err = ParseConfig([]byte(`{"confirmation": {"ttl": "0s"}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `confirmation.ttl "0s" is out of range`)
}

// TTLDuration reports an invalid value instead of falling back to the default.
func TestConfirmationTTLDurationReportsInvalidValues(t *testing.T) {
	for _, bad := range []string{"0s", "1h", "nonsense"} {
		_, err := ConfirmationConfig{TTL: bad}.TTLDuration()
		assert.Error(t, err, bad)
	}
}

func TestConfirmationResolversRejectUnknownNames(t *testing.T) {
	for _, bad := range []string{`["cli"]`, `["elicitation"]`, `["desktop", "desktop"]`, `"desktop"`} {
		_, err := ParseConfig([]byte(`{"confirmation": {"resolvers": ` + bad + `}}`))
		assert.Error(t, err, bad)
	}
}

func TestConfirmationCLIContradiction(t *testing.T) {
	_, err := ParseConfig([]byte(`{"confirmation": {"cli": false, "modes": ["native", "cli"]}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "confirmation.cli")
	assert.Contains(t, err.Error(), "confirmation.modes")
	assert.Equal(t, `confirmation.cli is false but confirmation.modes contains "cli"; `+
		`remove "cli" from confirmation.modes or set confirmation.cli to true`, err.Error())

	// Not a contradiction: false with modes lacking cli, true with cli listed,
	// and cli false left unset.
	for _, ok := range []string{
		`{"confirmation": {"cli": false, "modes": ["native"]}}`,
		`{"confirmation": {"cli": false}}`,
		`{"confirmation": {"cli": true, "modes": ["cli"]}}`,
		`{"confirmation": {"modes": ["cli"]}}`,
	} {
		_, err := ParseConfig([]byte(ok))
		assert.NoError(t, err, ok)
	}
}

// A literal secret stays out of the schema error even next to the new keys.
func TestConfirmationKeysKeepSecretRedaction(t *testing.T) {
	_, err := ParseConfig([]byte(`{"confirmation": {"ttl": "30s"}, "principals": [{"name": "x", "kind": "bogus", "credential": {"env": "SUPERSECRETVALUE", "token": "SUPERSECRETVALUE"}}]}`))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SUPERSECRETVALUE")
}

// Disabled state: without the keys, and with the key that already worked, the
// config loads and behaves as before.
func TestConfirmationModesStillWorkWithTheOtherKeysAbsent(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{}`))
	require.NoError(t, err)
	assert.Nil(t, cfg.Confirmation.Modes, "unset: the default applies")

	cfg, err = ParseConfig([]byte(`{"confirmation": {"modes": []}}`))
	require.NoError(t, err)
	require.NotNil(t, cfg.Confirmation.Modes)
	assert.Empty(t, *cfg.Confirmation.Modes, "an empty list turns the out-of-band resolvers off")

	cfg, err = ParseConfig([]byte(`{"confirmation": {"modes": ["native", "cli"]}}`))
	require.NoError(t, err)
	assert.Equal(t, []string{"native", "cli"}, *cfg.Confirmation.Modes)
}

// The configured lifetime replaces 120 s: a pending confirmation expires at it
// and expiry is denial (virtual time).
func TestBrokerConfiguredTTLExpiryIsDenial(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{"confirmation": {"ttl": "5s"}}`))
	require.NoError(t, err)
	ttl, err := cfg.Confirmation.TTLDuration()
	require.NoError(t, err)

	synctest.Test(t, func(t *testing.T) {
		b := newBroker(time.Now, ttl)
		p := openPending(b, "pin_rm")
		assert.Equal(t, ttl, p.ExpiresAt.Sub(p.CreatedAt))

		time.Sleep(ttl - time.Second)
		got, _ := b.get(p.ID)
		assert.Equal(t, StatePending, got.State, "still pending just before the short deadline")

		time.Sleep(time.Second)
		got, _ = b.get(p.ID)
		assert.Equal(t, StateExpired, got.State)
		assert.ErrorIs(t, b.Resolve(p.ID, ResolverLinuxPolkit, true), ErrConfirmationExpired, "a late approval is rejected")
		assert.Error(t, b.claim(p.ID, ""), "an expired confirmation never executes")
	})
}

// Options.ConfirmTTL reaches the gate: the confirmation, its token and the
// outcome after the deadline all follow it.
func TestServerConfirmTTLOptionShortensConfirmation(t *testing.T) {
	const short = 20 * time.Second
	require.Less(t, short, confirmTTL)

	d := newFakeDaemon(t)
	target := testCID(t, 1)
	pins := installPins(d, map[string]fakePin{target: {Type: "recursive"}})
	clock := newFakeClock()
	res := newFakeResolver(ResolverLinuxPolkit)
	s := mustNew(t, d, Options{Tools: []string{"pin", "destructive"}, Resolvers: []Resolver{res}, ConfirmTTL: short, now: clock.Now})
	e := &gateEnv{s: s, h: newHarness(s), d: d, pins: pins, clock: clock, resolver: res, target: target}
	cs := e.h.connect(t, nil, nil)

	first := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
	assert.Equal(t, clock.Now().Add(short).Unix(), first.Confirmation.ExpiresAt.Unix())

	clock.Advance(short + time.Second) // far below 120 s
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmExpired)
	confirmationOf(t, e.rm(t, cs, map[string]any{"confirm_token": first.Confirmation.ConfirmToken}), CodeConfirmExpired)
	assert.Zero(t, e.d.count("pin/rm"))
	assert.True(t, pins.has(target))
}

func TestServerConfirmTTLOptionBounds(t *testing.T) {
	d := newFakeDaemon(t)
	for _, ttl := range []time.Duration{-time.Second, time.Millisecond, MinConfirmTTL - 1, MaxConfirmTTL + time.Second} {
		_, err := New(t.Context(), d, Options{ConfirmTTL: ttl})
		require.Error(t, err, ttl.String())
		assert.Contains(t, err.Error(), "out of range")
	}
	s := mustNew(t, d, Options{ConfirmTTL: MinConfirmTTL})
	assert.Equal(t, MinConfirmTTL, s.gate.broker.ttl)
	s = mustNew(t, d, Options{ConfirmTTL: MaxConfirmTTL})
	assert.Equal(t, MaxConfirmTTL, s.gate.broker.ttl)
	s = mustNew(t, d, Options{})
	assert.Equal(t, DefaultConfirmTTL, s.gate.broker.ttl, "zero keeps the default")
}

// With the shortest ttl an expired record is kept for DefaultConfirmTTL, so a
// retry sees "expired" and not "unknown".
func TestBrokerKeepsExpiredRecordsForTheDefaultGrace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newBroker(time.Now, MinConfirmTTL)
		p := openPending(b, "pin_rm")
		time.Sleep(2 * MinConfirmTTL)
		_ = openPending(b, "pin_add") // a create sweeps
		got, ok := b.get(p.ID)
		require.True(t, ok, "kept within the grace")
		assert.Equal(t, StateExpired, got.State)

		time.Sleep(DefaultConfirmTTL + MinConfirmTTL)
		_ = openPending(b, "pin_ls")
		_, ok = b.get(p.ID)
		assert.False(t, ok, "dropped after the grace")
	})
}
