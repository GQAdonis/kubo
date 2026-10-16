//go:build !noagent

package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// adminVerifierWithAck authenticates only the admin test principal, with the
// given configured egress_ack (D4: the settings write's host rule is checked
// against it).
func adminVerifierWithAck(t *testing.T, ack []string) *StaticVerifier {
	t.Helper()
	id := NewIdentity(Principal{ID: "settings-admin", Kind: KindAdmin}, nil, nil, nil)
	id.Configured = true
	id.EgressAck = ack
	v, err := NewStaticVerifier([]StaticCredential{{Identity: id, Token: tokSettingsAdmin}})
	require.NoError(t, err)
	return v
}

// requireSettingsError asserts the response is the contract's settings error
// shape with the expected status and code, and returns its details object.
func requireSettingsError(t *testing.T, resp *http.Response, status int, code string) map[string]any {
	t.Helper()
	require.Equal(t, status, resp.StatusCode)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(bodyString(t, resp)), &doc))
	validateSettingsSchema(t, "settings/settings-error.schema.json", doc)
	errObj, _ := doc["error"].(map[string]any)
	require.NotNil(t, errObj)
	assert.Equal(t, code, errObj["code"])
	details, _ := errObj["details"].(map[string]any)
	return details
}

// currentBaseURL reads the effective backend base_url of a settings env.
func currentBaseURL(t *testing.T, e *settingsEnv) string {
	t.Helper()
	_, doc := getSettings(t, e)
	backend, _ := doc["backend"].(map[string]any)
	require.NotNil(t, backend)
	base, _ := backend["base_url"].(string)
	return base
}

// overlayDoc reads the raw overlay file of a settings directory.
func overlayDoc(t *testing.T, dir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, settingsOverlayFileName))
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	return doc
}

// TestSettingsEgressHostChangeNeedsConfirmation is half of the 1.6 acceptance
// check: a write that moves the model backend to a host the calling admin
// principal's egress_ack does not cover is refused 409
// egress_confirmation_required without the typed host confirmation, and nothing
// changes — settings, running backend, persisted files.
func TestSettingsEgressHostChangeNeedsConfirmation(t *testing.T) {
	dir := t.TempDir()
	e := newSettingsEnvDir(t, settingsBackendJSON, dir)
	etag, before := getSettings(t, e)

	resp := e.do(t, http.MethodPut, tokSettingsAdmin,
		`{"backend":{"base_url":"http://127.0.0.1:59997/v1","model":"m-new"}}`,
		map[string]string{"If-Match": etag})
	details := requireSettingsError(t, resp, http.StatusConflict, "egress_confirmation_required")
	assert.Equal(t, "127.0.0.1:59997", details["host"], "the refusal names the unconfirmed host")

	etagAfter, after := getSettings(t, e)
	assert.Equal(t, etag, etagAfter)
	assert.Equal(t, before, after)
	assert.Equal(t, "127.0.0.1:59998", e.s.Agent().BackendHost(), "the running backend is unchanged")
	assert.Empty(t, readdirnames(t, dir), "a refused write leaves no file in the settings directory")
}

// TestSettingsEgressConfirmationNamesOnlyTheNewHost is the other half of the
// 1.6 acceptance check: a confirmation that names any host other than the
// newly chosen one — or that arrives when no new host is chosen — is refused,
// and the named host is never acknowledged as a side effect.
func TestSettingsEgressConfirmationNamesOnlyTheNewHost(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	etag, before := getSettings(t, e)

	cases := map[string]string{
		// A confirmation for a host the write does not choose.
		"other host": `{"backend":{"base_url":"http://127.0.0.1:59997/v1"},"egress_confirmation":{"host":"models.example.com"}}`,
		// A confirmation when the write changes no host at all.
		"no host change": `{"backend":{"model":"m-new"},"egress_confirmation":{"host":"127.0.0.1:59998"}}`,
		// A confirmation naming the right endpoint but not as a bare host.
		"host with scheme": `{"backend":{"base_url":"http://127.0.0.1:59997/v1"},"egress_confirmation":{"host":"http://127.0.0.1:59997/v1"}}`,
		// The right host on the wrong port is a different host.
		"wrong port": `{"backend":{"base_url":"http://127.0.0.1:59997/v1"},"egress_confirmation":{"host":"127.0.0.1:443"}}`,
	}
	for name, write := range cases {
		resp := e.do(t, http.MethodPut, tokSettingsAdmin, write, map[string]string{"If-Match": etag})
		requireSettingsError(t, resp, http.StatusConflict, "egress_confirmation_required")
		_, after := getSettings(t, e)
		assert.Equal(t, before, after, name)
	}
	assert.Equal(t, "127.0.0.1:59998", e.s.Agent().BackendHost())

	// The refused confirmations acknowledged nothing: a write to the host one
	// of them named still needs a confirmation.
	resp := e.do(t, http.MethodPut, tokSettingsAdmin,
		`{"backend":{"base_url":"https://models.example.com/v1"}}`,
		map[string]string{"If-Match": etag})
	details := requireSettingsError(t, resp, http.StatusConflict, "egress_confirmation_required")
	assert.Equal(t, "models.example.com", details["host"])
}

// TestSettingsEgressExtensionApplies: a host change with the matching typed
// confirmation applies and extends the calling principal's egress_ack with
// exactly that host — remembered for later writes in-process, persisted in the
// overlay, and still effective after a restart.
func TestSettingsEgressExtensionApplies(t *testing.T) {
	dir := t.TempDir()
	e := newSettingsEnvDir(t, settingsBackendJSON, dir)
	etag, _ := getSettings(t, e)

	// Move to a new host with its confirmation.
	etag = e.putEgress(t, etag, `{"backend":{"base_url":"http://127.0.0.1:59997/v1"},"egress_confirmation":{"host":"127.0.0.1:59997"}}`)
	assert.Equal(t, "http://127.0.0.1:59997/v1", currentBaseURL(t, e))

	// Move back to the original host, also unacknowledged, with its
	// confirmation.
	etag = e.putEgress(t, etag, `{"backend":{"base_url":"http://127.0.0.1:59998/v1"},"egress_confirmation":{"host":"127.0.0.1:59998"}}`)

	// Both hosts are now acknowledged: neither move needs a confirmation again.
	etag = e.putEgress(t, etag, `{"backend":{"base_url":"http://127.0.0.1:59997/v1"}}`)
	_ = e.putEgress(t, etag, `{"backend":{"base_url":"http://127.0.0.1:59998/v1"}}`)

	// The overlay persisted both extensions under the calling principal, and
	// nothing else.
	ov := overlayDoc(t, dir)
	ack, _ := ov["egress_ack"].(map[string]any)
	require.NotNil(t, ack, "the overlay records the egress_ack extension")
	require.Len(t, ack, 1, "extensions are attributed to the calling principal only")
	hosts, _ := ack["settings-admin"].([]any)
	require.ElementsMatch(t, []any{"127.0.0.1:59997", "127.0.0.1:59998"}, hosts)

	// A restart on the same directory keeps the acknowledgements.
	e2 := newSettingsEnvDir(t, settingsBackendJSON, dir)
	etag2, _ := getSettings(t, e2)
	etag2 = e2.putEgress(t, etag2, `{"backend":{"base_url":"http://127.0.0.1:59997/v1"}}`)
	assert.Equal(t, "http://127.0.0.1:59997/v1", currentBaseURL(t, e2))

	// A host that was never confirmed still requires confirmation.
	resp := e2.do(t, http.MethodPut, tokSettingsAdmin,
		`{"backend":{"base_url":"http://127.0.0.1:59995/v1"}}`,
		map[string]string{"If-Match": etag2})
	requireSettingsError(t, resp, http.StatusConflict, "egress_confirmation_required")
}

// TestSettingsEgressConfigAckCoversNewHost: a host already in the calling
// principal's configured egress_ack needs no confirmation — including the
// "local" sentinel for a loopback backend — and the write records no
// extension. The comparison is on the effective (flag-overridden) host.
func TestSettingsEgressConfigAckCoversNewHost(t *testing.T) {
	dir := t.TempDir()
	e := newSettingsEnvWith(t, settingsBackendJSON, dir, adminVerifierWithAck(t, []string{"local", "models.example.com"}))
	etag, _ := getSettings(t, e)

	// "local" covers the loopback host on any port.
	etag = e.putEgress(t, etag, `{"backend":{"base_url":"http://127.0.0.1:59997/v1"}}`)
	// A named host covers any port of that host.
	_ = e.putEgress(t, etag, `{"backend":{"base_url":"https://models.example.com/v1"}}`)
	assert.Equal(t, "https://models.example.com/v1", currentBaseURL(t, e))

	ov := overlayDoc(t, dir)
	assert.NotContains(t, ov, "egress_ack", "an already-acknowledged host records no extension")
}

// TestSettingsEgressConfirmationSuperfluousWhenAcked: a matching confirmation
// for an already-acknowledged host change is a no-op, not an error — the
// extension rule refuses other hosts, not a redundant acknowledgement of the
// chosen one.
func TestSettingsEgressConfirmationSuperfluousWhenAcked(t *testing.T) {
	dir := t.TempDir()
	e := newSettingsEnvWith(t, settingsBackendJSON, dir, adminVerifierWithAck(t, []string{"local"}))
	etag, _ := getSettings(t, e)
	e.putEgress(t, etag, `{"backend":{"base_url":"http://127.0.0.1:59997/v1"},"egress_confirmation":{"host":"127.0.0.1:59997"}}`)
	assert.Equal(t, "http://127.0.0.1:59997/v1", currentBaseURL(t, e))
	ov := overlayDoc(t, dir)
	assert.NotContains(t, ov, "egress_ack", "no extension was needed: the configured ack already covered the host")
}

// putEgress applies one write expected to succeed and returns the new ETag.
func (e *settingsEnv) putEgress(t *testing.T, etag, body string) string {
	t.Helper()
	resp := e.do(t, http.MethodPut, tokSettingsAdmin, body, map[string]string{"If-Match": etag})
	require.Equal(t, http.StatusOK, resp.StatusCode, bodyString(t, resp))
	return strings.Trim(resp.Header.Get("ETag"), `"`)
}

// TestSameHostPort pins the normalized comparison the D4 rule uses.
func TestSameHostPort(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"127.0.0.1:59997", "127.0.0.1:59997", true},
		{"LOCALHOST:59997", "localhost:59997", true},
		{"[::1]:443", "[::1]:443", true},
		{"::1", "::1", true},
		{"example.com.", "example.com", true},
		{"127.0.0.1", "127.0.0.1:59997", false},
		{"127.0.0.1:443", "127.0.0.1:59997", false},
		{"example.com", "other.example.com", false},
		{"http://127.0.0.1:59997/v1", "127.0.0.1:59997", false},
		{"", "127.0.0.1:59997", false},
	} {
		assert.Equal(t, tc.want, sameHostPort(tc.a, tc.b), "%q vs %q", tc.a, tc.b)
	}
}
