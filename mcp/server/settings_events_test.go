//go:build !noagent

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sseStream is a read handle on the settings change stream.
type sseStream struct {
	reader *bufio.Reader
}

// openEvents subscribes to the settings change stream; a request-scoped
// timeout turns a stuck stream into a test failure instead of a hang, and
// cleanup disconnects before the server shuts down.
func (e *settingsEnv) openEvents(t *testing.T, token string) (*sseStream, *http.Response) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.base+RouteSettingsEvents, nil)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() {
		cancel()
		_ = resp.Body.Close()
	})
	return &sseStream{reader: bufio.NewReader(resp.Body)}, resp
}

// nextEvent reads lines until one SSE event's data payload is decoded.
func (s *sseStream) nextEvent(t *testing.T) map[string]any {
	t.Helper()
	for {
		line, err := s.reader.ReadString('\n')
		require.NoError(t, err)
		if data, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "data: "); ok {
			var doc map[string]any
			require.NoError(t, json.Unmarshal([]byte(data), &doc))
			return doc
		}
	}
}

// putSettings applies one write and returns the new revision (the response's
// ETag, unquoted).
func (e *settingsEnv) putSettings(t *testing.T, etag, body string) string {
	t.Helper()
	resp := e.do(t, http.MethodPut, tokSettingsAdmin, body, map[string]string{"If-Match": etag})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = bodyString(t, resp)
	return strings.Trim(resp.Header.Get("ETag"), `"`)
}

// TestSettingsChangeStreamCarriesOnlyRevision is the 1.5 acceptance check: the
// initial event and the event of a real write both match the contract's
// settings-event schema and carry only the new revision, which equals the ETag
// a subsequent GET returns.
func TestSettingsChangeStreamCarriesOnlyRevision(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	stream, resp := e.openEvents(t, tokSettingsAdmin)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	etag, _ := getSettings(t, e)
	initial := stream.nextEvent(t)
	validateSettingsSchema(t, "settings/settings-event.schema.json", initial)
	assert.Equal(t, "settings_changed", initial["type"])
	assert.Equal(t, strings.Trim(etag, `"`), initial["revision"],
		"the first event carries the current revision, so a change between the last read and the subscribe is not missed")

	newRevision := e.putSettings(t, etag, `{"backend":{"model":"m-new"},"system_prompt":"Be terse."}`)
	ev := stream.nextEvent(t)
	validateSettingsSchema(t, "settings/settings-event.schema.json", ev)
	assert.Equal(t, "settings_changed", ev["type"])
	assert.Equal(t, newRevision, ev["revision"])

	raw, _ := json.Marshal(ev)
	for _, leaked := range []string{settingsTestKey, "m-new", "Be terse", "59997", "base_url"} {
		assert.NotContains(t, string(raw), leaked, "an event carries no settings values or key material (%q)", leaked)
	}

	// The event's revision is the ETag of a fresh read.
	etagAfter, _ := getSettings(t, e)
	assert.Equal(t, strings.Trim(etagAfter, `"`), ev["revision"])
}

// TestSettingsChangeStreamAuth: the stream is inside the admin-scoped settings
// subtree and answers GET only.
func TestSettingsChangeStreamAuth(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	assert.Equal(t, http.StatusUnauthorized, e.doPath(t, http.MethodGet, RouteSettingsEvents, "", "").StatusCode)
	assert.Equal(t, http.StatusForbidden, e.doPath(t, http.MethodGet, RouteSettingsEvents, tokReader, "").StatusCode)
	assert.Equal(t, http.StatusForbidden, e.doPath(t, http.MethodGet, RouteSettingsEvents, tokChat, "").StatusCode)
	assert.Equal(t, http.StatusForbidden, e.doPath(t, http.MethodGet, RouteSettingsEvents, tokPeer, "").StatusCode)
	assert.Equal(t, http.StatusMethodNotAllowed, e.doPath(t, http.MethodPost, RouteSettingsEvents, tokSettingsAdmin, "{}").StatusCode)
}

// TestSettingsChangeStreamWithoutBackend: a write that installs no backend (a
// prompt-only write with none configured) never runs ApplyAgentSettings, so
// the hook does not fire; the stream is fed by the apply path directly and
// still emits exactly the new revision.
func TestSettingsChangeStreamWithoutBackend(t *testing.T) {
	e := newSettingsEnv(t, `{}`)
	stream, _ := e.openEvents(t, tokSettingsAdmin)
	initial := stream.nextEvent(t)
	etag, doc := getSettings(t, e)
	require.Nil(t, doc["backend"])
	assert.Equal(t, strings.Trim(etag, `"`), initial["revision"])

	newRevision := e.putSettings(t, etag, `{"system_prompt":"Only facts."}`)
	assert.Nil(t, e.s.Agent(), "a prompt alone does not install the loop")
	ev := stream.nextEvent(t)
	validateSettingsSchema(t, "settings/settings-event.schema.json", ev)
	assert.Equal(t, newRevision, ev["revision"])
}

// TestSettingsChangeStreamCoalesces: a subscriber that falls behind is handed
// the newest revision, not a backlog. The guarantee lives in the broadcaster's
// one-event buffer (publish replaces a pending older revision), pinned here at
// that level; over the wire a fast subscriber may also see the intermediate
// revision, so the HTTP part asserts the stream converges to the newest.
func TestSettingsChangeStreamCoalesces(t *testing.T) {
	b := newSettingsEvents()
	ch := b.subscribe()
	b.publish("rev-a")
	b.publish("rev-b")
	assert.Equal(t, "rev-b", <-ch, "the pending older revision is replaced, not queued")
	select {
	case extra := <-ch:
		t.Fatalf("a second revision was queued behind the newest: %q", extra)
	default:
	}

	e := newSettingsEnv(t, settingsBackendJSON)
	stream, _ := e.openEvents(t, tokSettingsAdmin)
	_ = stream.nextEvent(t) // the initial event

	etag, _ := getSettings(t, e)
	etag = `"` + e.putSettings(t, etag, `{"backend":{"model":"m-a"}}`) + `"`
	etag = `"` + e.putSettings(t, etag, `{"backend":{"model":"m-b"}}`) + `"`

	// The stream converges to the newest revision; whether the intermediate
	// one was delivered first is a scheduling detail, not part of the contract.
	want := strings.Trim(etag, `"`)
	for i := 0; i < 4; i++ {
		ev := stream.nextEvent(t)
		validateSettingsSchema(t, "settings/settings-event.schema.json", ev)
		if ev["revision"] == want {
			return
		}
	}
	t.Fatalf("the newest revision %s never arrived on the change stream", want)
}

// TestSettingsChangeStreamFansOut: every subscriber sees every applied write.
func TestSettingsChangeStreamFansOut(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	first, _ := e.openEvents(t, tokSettingsAdmin)
	second, _ := e.openEvents(t, tokSettingsAdmin)
	_ = first.nextEvent(t)
	_ = second.nextEvent(t)

	etag, _ := getSettings(t, e)
	newRevision := e.putSettings(t, etag, `{"backend":{"model":"m-fan"}}`)
	for name, stream := range map[string]*sseStream{"first": first, "second": second} {
		ev := stream.nextEvent(t)
		validateSettingsSchema(t, "settings/settings-event.schema.json", ev)
		assert.Equal(t, newRevision, ev["revision"], name)
	}
}
