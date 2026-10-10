package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// testUARSettingsPath is the path the fake serves settings on. The contract
// pins no path yet, so tests inject it with WithSettingsPath.
const testUARSettingsPath = "/api/uar/settings"

const (
	fakeUARAdminToken = "uar-admin-secret-0123456789"
	fakeUARAdminEnv   = "TEST_UAR_ADMIN_TOKEN"
)

// fakeUAR is an httptest stand-in for a UAR instance: an agent card, and the
// settings API behind an admin bearer token. It counts requests per path so a
// test can prove nothing contacted it.
type fakeUAR struct {
	srv *httptest.Server

	mu          sync.Mutex
	version     string
	settings    map[string]any
	requireAuth bool
	// rawBody, when not nil, is the settings response verbatim (for a
	// response that is not a JSON object).
	rawBody []byte
	// settingsStatus, when not zero, is answered instead of the settings.
	settingsStatus int
	hits           map[string]int
	authSeen       []string
}

// newFakeUAR serves the fixture testdata/uar/<fixture>.json. The version
// is a published release by default.
func newFakeUAR(t *testing.T, fixture string) *fakeUAR {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "uar", fixture+".json"))
	require.NoError(t, err)
	return newFakeUARBytes(t, raw)
}

// newFakeUARBytes serves raw as the settings response. A JSON object stays
// editable through u.settings; anything else is served verbatim.
func newFakeUARBytes(t *testing.T, raw []byte) *fakeUAR {
	t.Helper()
	u := &fakeUAR{version: "1.4.0", requireAuth: true, hits: map[string]int{}}
	if err := json.Unmarshal(raw, &u.settings); err != nil {
		u.rawBody = raw
	}
	t.Setenv(fakeUARAdminEnv, fakeUARAdminToken)
	u.srv = httptest.NewServer(http.HandlerFunc(u.serve))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *fakeUAR) serve(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.hits[r.URL.Path]++
	auth := r.Header.Get("Authorization")
	if r.URL.Path == testUARSettingsPath {
		u.authSeen = append(u.authSeen, auth)
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case uarCardSuffix:
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "fake uar", "version": u.version})
	case testUARSettingsPath:
		if u.settingsStatus != 0 {
			w.WriteHeader(u.settingsStatus)
			return
		}
		if auth != "Bearer "+fakeUARAdminToken && u.requireAuth {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if u.rawBody != nil {
			_, _ = w.Write(u.rawBody)
			return
		}
		_ = json.NewEncoder(w).Encode(u.settings)
	default:
		http.NotFound(w, r)
	}
}

func (u *fakeUAR) cardURL() string { return u.srv.URL + uarCardSuffix }

// set edits the version or settings under the lock.
func (u *fakeUAR) set(fn func(u *fakeUAR)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	fn(u)
}

// total is the number of requests the instance received.
func (u *fakeUAR) total() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	n := 0
	for _, c := range u.hits {
		n += c
	}
	return n
}

// field edits one nested settings object: path is the keys down to the object.
func (u *fakeUAR) field(key string) map[string]any {
	m, _ := u.settings[key].(map[string]any)
	return m
}

// input is a UARCheckInput for the instance with the admin credential set.
func (u *fakeUAR) input(minVersion string) UARCheckInput {
	return UARCheckInput{
		UAR: UARConfig{
			CardURL:         u.cardURL(),
			AdminCredential: &SecretRef{Env: fakeUARAdminEnv},
			MinVersion:      &minVersion,
		},
		KuboListen: "127.0.0.1:5050",
	}
}
