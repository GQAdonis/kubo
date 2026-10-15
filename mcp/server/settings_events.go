//go:build !noagent

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// RouteSettingsEvents is the settings change stream (mcp-agent-settings-api
// 1.5): one SSE event per applied write, carrying only the new revision
// (mcp/contract/settings/settings-event.schema.json). Subscribers re-read the
// settings route for the new state, so an event can never carry settings
// values or key material. It lives under the settings route's admin-scoped
// mount, so only an admin principal reaches it.
const RouteSettingsEvents = RouteSettings + "/events"

// settingsEventBody is the stream's only payload, the data field of each
// event.
type settingsEventBody struct {
	Type     string `json:"type"`
	Revision string `json:"revision"`
}

// settingsEvents fans a new revision out to the stream's subscribers.
type settingsEvents struct {
	mu   sync.Mutex
	subs map[chan string]struct{}
	max  int
}

func newSettingsEvents() *settingsEvents {
	return &settingsEvents{subs: map[chan string]struct{}{}, max: maxSettingsSubscribers}
}

// subscribe adds a subscriber; unsubscribe removes it. The buffer holds one
// event: a subscriber that falls behind is handed the newest revision next,
// which is all an event carries. It refuses (nil, false) once max subscribers
// are open, so a client cannot hold unbounded connections and goroutines.
func (b *settingsEvents) subscribe() (chan string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.subs) >= b.max {
		return nil, false
	}
	ch := make(chan string, 1)
	b.subs[ch] = struct{}{}
	return ch, true
}

func (b *settingsEvents) unsubscribe(ch chan string) {
	b.mu.Lock()
	delete(b.subs, ch)
	b.mu.Unlock()
}

// publish hands the revision to every subscriber without blocking: a pending
// older revision is replaced, because only the newest matters. It is safe to
// call with the settings state lock held; it never touches that lock.
func (b *settingsEvents) publish(revision string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- revision:
		default:
		}
	}
}

// events serves GET on the change stream. The first event carries the current
// revision, so a change between the client's last read and the subscribe is
// not missed; afterwards each applied write sends one event.
func (h *settingsHandler) events(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-cache")
	hd.Set("X-Accel-Buffering", "no")
	// Subscribe before reading the current revision, so a write landing between
	// the two is in the buffer and not lost.
	ch, ok := h.st.events.subscribe()
	if !ok {
		refuseRate(w, settingsStreamRetry)
		return
	}
	defer h.st.events.unsubscribe(ch)
	w.WriteHeader(http.StatusOK)

	send := func(revision string) bool {
		b, err := json.Marshal(settingsEventBody{Type: "settings_changed", Revision: revision})
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !send(h.st.currentRevision()) {
		return
	}
	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case revision := <-ch:
			if !send(revision) {
				return
			}
		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}
