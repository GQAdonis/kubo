//go:build !noagent

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A fixed verifier lifetime makes stream timing independent of middleware's
// cache window, while retaining the real authentication/protection chain.
type decisionLifetimeVerifier struct {
	AuthVerifier
	end        time.Time
	sessionEnd time.Time
}

// Test subjects may share one configured principal. Static configuration
// itself deliberately forbids duplicate principal rows; the verifier seam
// supplies the same subject binding that the JWT verifier supplies in use.
type decisionIdentityVerifier struct {
	AuthVerifier
	identities map[string]Identity
}

func (v decisionIdentityVerifier) Verify(ctx context.Context, token string, r *http.Request) (Identity, time.Time, error) {
	id, end, err := v.AuthVerifier.Verify(ctx, token, r)
	if mapped, ok := v.identities[token]; err == nil && ok {
		id = mapped
	}
	return id, end, err
}

func (v decisionLifetimeVerifier) Verify(ctx context.Context, token string, r *http.Request) (Identity, time.Time, error) {
	id, _, err := v.AuthVerifier.Verify(ctx, token, r)
	if !v.sessionEnd.IsZero() {
		id.Session = &Session{Expiry: v.sessionEnd}
	}
	return id, v.end, err
}

// This writer implements actual deadline behavior, including interruption of
// an already blocked Write. A recorder alone cannot exercise that contract.
type decisionDeadlineWriter struct {
	header   http.Header
	mu       sync.Mutex
	status   int
	deadline time.Time
	wake     chan struct{}
	frames   chan string
	entered  chan struct{}
	release  chan struct{}
	once     sync.Once
	block    bool
}

func newDecisionDeadlineWriter(block bool) *decisionDeadlineWriter {
	return &decisionDeadlineWriter{header: make(http.Header), wake: make(chan struct{}), frames: make(chan string, 128), entered: make(chan struct{}), release: make(chan struct{}), block: block}
}
func (w *decisionDeadlineWriter) Header() http.Header { return w.header }
func (w *decisionDeadlineWriter) WriteHeader(status int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.status = status
}
func (w *decisionDeadlineWriter) SetWriteDeadline(deadline time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.deadline = deadline
	close(w.wake)
	w.wake = make(chan struct{})
	return nil
}
func (w *decisionDeadlineWriter) FlushError() error { return nil }
func (w *decisionDeadlineWriter) Write(data []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	if w.block {
		for {
			w.mu.Lock()
			deadline, wake := w.deadline, w.wake
			w.mu.Unlock()
			var timer *time.Timer
			var tick <-chan time.Time
			if !deadline.IsZero() {
				timer = time.NewTimer(max(0, time.Until(deadline)))
				tick = timer.C
			}
			select {
			case <-w.release:
				if timer != nil {
					timer.Stop()
				}
				goto ready
			case <-tick:
				return 0, os.ErrDeadlineExceeded
			case <-wake:
				if timer != nil {
					timer.Stop()
				}
			}
		}
	}
ready:
	w.mu.Lock()
	expired := !w.deadline.IsZero() && !time.Now().Before(w.deadline)
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.mu.Unlock()
	if expired {
		return 0, os.ErrDeadlineExceeded
	}
	select {
	case w.frames <- string(data):
	default:
		return 0, fmt.Errorf("fixture sink exhausted")
	}
	return len(data), nil
}

func startDecisionFixtureStream(t *testing.T, e *decisionReadEnv, w *decisionDeadlineWriter, token string) (context.CancelFunc, <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+RouteDecisions+"/events", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Last-Event-ID", "untrusted-replay-id")
	done := make(chan struct{})
	go func() { defer close(done); e.h.Handler().ServeHTTP(w, req) }()
	return cancel, done
}
func decisionFixtureFrame(t *testing.T, w *decisionDeadlineWriter, event, def string) map[string]any {
	t.Helper()
	var frame string
	select {
	case frame = <-w.frames:
	default:
		t.Fatal("missing SSE frame")
	}
	require.LessOrEqual(t, len(frame), maxDecisionEventBytes)
	require.True(t, strings.HasPrefix(frame, "event: "+event+"\ndata: "), frame)
	require.True(t, strings.HasSuffix(frame, "\n\n"))
	assert.NotContains(t, frame, "\nid:")
	data := strings.TrimSuffix(strings.TrimPrefix(frame, "event: "+event+"\ndata: "), "\n\n")
	validateDecisionRead(t, def, []byte(data))
	var value map[string]any
	require.NoError(t, json.Unmarshal([]byte(data), &value))
	return value
}
func decisionFixtureStreams(b *Broker) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.decisionSubs)
}

func TestDecisionEventsInitialMinimalHintsReconnectAndOwnership(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
		w := newDecisionDeadlineWriter(false)
		cancel, done := startDecisionFixtureStream(t, e, w, tokChat)
		synctest.Wait()
		initial := decisionFixtureFrame(t, w, decisionSnapshotEvent, "SnapshotRequired")
		assert.Equal(t, "initial", initial["reason"])
		// Another identity cannot change this queue or its own revision.
		historyPending(e.s.gate.broker, Principal{ID: "other", Kind: KindHumanMCP})
		synctest.Wait()
		assert.Empty(t, w.frames)
		p := historyPending(e.s.gate.broker, Principal{ID: "chat", Kind: KindUI})
		synctest.Wait()
		change := decisionFixtureFrame(t, w, decisionChangedEvent, "DecisionChanged")
		assert.ElementsMatch(t, []string{"id", "state", "revision"}, keysOf(change))
		assert.Equal(t, p.ID, change["id"])
		assert.Equal(t, e.s.gate.broker.snapshotFor(p.Principal.BindingKey()).Revision, change["revision"])
		require.NoError(t, e.s.gate.broker.Resolve(p.ID, ResolverAGUIInterrupt, false))
		synctest.Wait()
		assert.Equal(t, StateDenied, decisionFixtureFrame(t, w, decisionChangedEvent, "DecisionChanged")["state"])
		cancel()
		<-done
		assert.Equal(t, "text/event-stream", w.header.Get("Content-Type"))
		assert.Equal(t, "no-store", w.header.Get("Cache-Control"))
		assert.Zero(t, decisionFixtureStreams(e.s.gate.broker))
		again := newDecisionDeadlineWriter(false)
		stop, closed := startDecisionFixtureStream(t, e, again, tokChat)
		synctest.Wait()
		assert.Equal(t, "initial", decisionFixtureFrame(t, again, decisionSnapshotEvent, "SnapshotRequired")["reason"])
		assert.Empty(t, again.frames, "Last-Event-ID is never a replay cursor")
		stop()
		<-closed
	})
}

func TestDecisionEventsAuthDeadlineSessionDeadlineAndMaximumLifetime(t *testing.T) {
	for _, fixture := range []struct {
		name                   string
		verdict, session, want time.Duration
	}{
		{"verdict", 12 * time.Second, 0, 12 * time.Second},
		{"session", time.Hour, 7 * time.Second, 7 * time.Second},
		{"maximum", time.Hour, 0, 15 * time.Minute},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				verifier := decisionLifetimeVerifier{AuthVerifier: testVerifier(t), end: start.Add(fixture.verdict)}
				if fixture.session != 0 {
					verifier.sessionEnd = start.Add(fixture.session)
				}
				e := newDecisionReadEnv(t, DecisionsConfig{}, verifier)
				w := newDecisionDeadlineWriter(false)
				cancel, done := startDecisionFixtureStream(t, e, w, tokChat)
				defer cancel()
				synctest.Wait()
				decisionFixtureFrame(t, w, decisionSnapshotEvent, "SnapshotRequired")
				time.Sleep(fixture.want)
				synctest.Wait()
				<-done
				assert.Equal(t, fixture.want, time.Since(start))
				assert.Zero(t, decisionFixtureStreams(e.s.gate.broker))
				assert.Len(t, w.frames, int((fixture.want-time.Nanosecond)/decisionStreamKeepalive))
				for len(w.frames) > 0 {
					assert.Equal(t, ": keepalive\n\n", <-w.frames)
				}
				count := len(w.frames)
				historyPending(e.s.gate.broker, Principal{ID: "chat", Kind: KindUI})
				synctest.Wait()
				assert.Len(t, w.frames, count, "no frame after authentication ends")
			})
		})
	}
}

func TestDecisionEventsOverflowKeepsInitialThenFreshSnapshot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
		w := newDecisionDeadlineWriter(true)
		cancel, done := startDecisionFixtureStream(t, e, w, tokChat)
		defer cancel()
		<-w.entered
		owner := Principal{ID: "chat", Kind: KindUI}
		for range 100 {
			historyPending(e.s.gate.broker, owner)
		}
		close(w.release)
		synctest.Wait()
		assert.Equal(t, "initial", decisionFixtureFrame(t, w, decisionSnapshotEvent, "SnapshotRequired")["reason"])
		overflow := decisionFixtureFrame(t, w, decisionSnapshotEvent, "SnapshotRequired")
		assert.Equal(t, "overflow", overflow["reason"])
		assert.Equal(t, e.s.gate.broker.snapshotFor(owner.BindingKey()).Revision, overflow["revision"])
		assert.Empty(t, w.frames)
		cancel()
		<-done
	})
}

func TestDecisionEventsSlowWriterAndUnsupportedDeadlinesFailClosed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
		w := newDecisionDeadlineWriter(true)
		cancel, done := startDecisionFixtureStream(t, e, w, tokChat)
		defer cancel()
		<-w.entered
		time.Sleep(decisionStreamWriteLimit)
		synctest.Wait()
		<-done
		assert.Empty(t, w.frames)
		assert.Zero(t, decisionFixtureStreams(e.s.gate.broker))
	})
	e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
	response := e.request(t, http.MethodGet, RouteDecisions+"/events", tokChat, "", nil)
	assert.Equal(t, http.StatusInternalServerError, response.Code, "ResponseRecorder has no write-deadline support")
	assert.Zero(t, decisionFixtureStreams(e.s.gate.broker))
}

func TestDecisionEventsProtectionAndOffSwitch(t *testing.T) {
	e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
	for _, fixture := range []struct {
		method, path, token, body string
		headers                   map[string]string
		status                    int
	}{
		{"GET", RouteDecisions + "/events", "", "", nil, 401},
		{"GET", RouteDecisions + "/events", "invalid-token", "", nil, 401},
		{"GET", RouteDecisions + "/events", tokChat, "", map[string]string{"Origin": "https://evil.example"}, 403},
		{"GET", RouteDecisions + "/events", tokChat, "", map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"GET", RouteDecisions + "/events?token=private", tokChat, "", nil, 400},
		{"GET", RouteDecisions + "/events?limit=1", tokChat, "", nil, 400},
		{"GET", RouteDecisions + "/events", tokChat, "{}", nil, 400},
		{"HEAD", RouteDecisions + "/events", tokChat, "", nil, 405},
		{"POST", RouteDecisions + "/events", tokChat, "", nil, 405},
	} {
		w := e.request(t, fixture.method, fixture.path, fixture.token, fixture.body, fixture.headers)
		assert.Equal(t, fixture.status, w.Code)
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
	preflight := e.request(t, "OPTIONS", RouteDecisions+"/events", "", "", map[string]string{"Origin": sessionOrigin, "Access-Control-Request-Method": "GET"})
	assert.Equal(t, 204, preflight.Code)
	assert.Empty(t, preflight.Header().Get("Access-Control-Allow-Credentials"))
	anonymous := newDecisionReadEnv(t, DecisionsConfig{}, nil)
	assert.Equal(t, 403, anonymous.request(t, "GET", RouteDecisions+"/events", "", "", nil).Code)
	off := false
	disabled := newDecisionReadEnv(t, DecisionsConfig{Enabled: &off}, testVerifier(t))
	assert.Equal(t, 404, disabled.request(t, "GET", RouteDecisions+"/events", tokChat, "", nil).Code)
	assert.Nil(t, disabled.s.decisionRuntimeExt)
	assert.Empty(t, disabled.h.serveHooks)
	assert.Empty(t, disabled.s.gate.broker.decisionSubs)
}

func TestDecisionEventsOwnerSubjectQuotasAndKnownInvalidation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		alice := Principal{ID: "shared", Kind: KindUI, sub: "alice-private"}
		bob := Principal{ID: alice.ID, Kind: alice.Kind, sub: "bob-private"}
		verifier := decisionIdentityVerifier{AuthVerifier: testVerifier(t), identities: map[string]Identity{
			tokChat: NewIdentity(alice, nil, nil, nil), tokOther: NewIdentity(bob, nil, nil, nil),
		}}
		e := newDecisionReadEnv(t, DecisionsConfig{}, verifier)
		var stops []context.CancelFunc
		var dones []<-chan struct{}
		for range maxDecisionStreamsPerOwner {
			w := newDecisionDeadlineWriter(false)
			stop, done := startDecisionFixtureStream(t, e, w, tokChat)
			stops = append(stops, stop)
			dones = append(dones, done)
			synctest.Wait()
			decisionFixtureFrame(t, w, decisionSnapshotEvent, "SnapshotRequired")
		}
		full := newDecisionDeadlineWriter(false)
		cancel, closed := startDecisionFixtureStream(t, e, full, tokChat)
		synctest.Wait()
		<-closed
		cancel()
		assert.Equal(t, 429, full.status)
		assert.Equal(t, "5", full.header.Get("Retry-After"))
		foreign := newDecisionDeadlineWriter(false)
		stopBob, doneBob := startDecisionFixtureStream(t, e, foreign, tokOther)
		synctest.Wait()
		decisionFixtureFrame(t, foreign, decisionSnapshotEvent, "SnapshotRequired")
		historyPending(e.s.gate.broker, alice)
		synctest.Wait()
		assert.Empty(t, foreign.frames, "subject is part of ownership")
		e.s.gate.broker.invalidateDecisionSubscriptions(alice.BindingKey(), "")
		synctest.Wait()
		for i, done := range dones {
			<-done
			stops[i]()
		}
		assert.Equal(t, 1, decisionFixtureStreams(e.s.gate.broker))
		historyPending(e.s.gate.broker, bob)
		synctest.Wait()
		decisionFixtureFrame(t, foreign, decisionChangedEvent, "DecisionChanged")
		e.s.gate.broker.invalidateDecisionSubscriptions("", alice.ID)
		synctest.Wait()
		<-doneBob
		stopBob()
		assert.Zero(t, decisionFixtureStreams(e.s.gate.broker))
	})
}

func TestDecisionEventsDisableDisconnectAndConcurrentShutdownInterruptWrites(t *testing.T) {
	for _, mode := range []string{"disable", "disconnect", "shutdown"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				e := newDecisionReadEnv(t, DecisionsConfig{}, testVerifier(t))
				runtime := e.s.decisionRuntime()
				stopRuntime := runtime.start(t.Context())
				defer stopRuntime()
				w := newDecisionDeadlineWriter(true)
				cancel, done := startDecisionFixtureStream(t, e, w, tokChat)
				defer cancel()
				<-w.entered
				switch mode {
				case "disable":
					e.s.gate.broker.setHistoryEnabled(false)
				case "disconnect":
					cancel()
				case "shutdown":
					var group sync.WaitGroup
					for range 8 {
						group.Go(runtime.shutdown)
					}
					group.Wait()
				}
				synctest.Wait()
				<-done
				assert.Empty(t, w.frames)
				assert.Zero(t, decisionFixtureStreams(e.s.gate.broker))
				if mode == "shutdown" {
					select {
					case <-runtime.done:
					default:
						t.Fatal("scheduler did not join")
					}
				}
			})
		})
	}
}

func TestDecisionEventsRealListenerUsesSupportedDeadlines(t *testing.T) {
	e := newHTTPEnv(t, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.base+RouteDecisions+"/events", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+tokChat)
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "text/event-stream", response.Header.Get("Content-Type"))
	// Read exactly through the first event boundary without waiting for EOF.
	var frame strings.Builder
	var one [1]byte
	for !strings.HasSuffix(frame.String(), "\n\n") {
		_, err := response.Body.Read(one[:])
		require.NoError(t, err)
		frame.WriteByte(one[0])
		require.LessOrEqual(t, frame.Len(), maxDecisionEventBytes)
	}
	assert.True(t, strings.HasPrefix(frame.String(), "event: "+decisionSnapshotEvent+"\n"))
	assert.NotContains(t, frame.String(), "\nid:")
}
