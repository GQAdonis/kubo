package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// confirmation.resolvers, confirmation.cli and confirmation.ttl pass the schema
// but no code reads them. A config that sets one is refused rather than given an
// off switch that does nothing (R11-Q02).
func TestUnreadConfirmationKeysAreRejected(t *testing.T) {
	for key, value := range map[string]string{
		"resolvers": `["linux-polkit"]`,
		"cli":       `true`,
		"ttl":       `"30s"`,
	} {
		t.Run(key, func(t *testing.T) {
			_, err := ParseConfig([]byte(`{"confirmation": {"` + key + `": ` + value + `}}`))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "confirmation."+key)
			assert.Contains(t, err.Error(), "confirmation.modes", "the error points at the working key")
		})
	}
}

// Disabled state: without the keys, and with the key that works, the config
// loads and behaves as before.
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
