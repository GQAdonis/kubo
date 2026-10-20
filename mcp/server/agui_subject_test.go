//go:build !noagent

package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// subjectEnv is an /agui listener whose session verifier maps two subjects to
// one ui principal (routes include agui).
type subjectEnv struct {
	*aguiEnv
	iss *testIssuer
}

func newSubjectEnv(t *testing.T, steps ...func(modelRequest) []string) *subjectEnv {
	t.Helper()
	iss := newTestIssuer(t)
	e := newAGUIEnv(t, aguiEnvOpts{verifier: func(t *testing.T) AuthVerifier {
		cfg := iss.settings()
		cfg.RequireOrigin = false
		v, err := NewJWTVerifier(JWTOptions{OIDC: cfg, Principals: []SessionPrincipal{{
			Identity: aguiIdentity("shared-ui", KindUI, []string{"pin"}, []string{"pin", "diag"}, []string{"local"}),
			Subjects: []string{bindSubA, bindSubB},
		}}})
		require.NoError(t, err)
		return NewChainVerifier(testVerifier(t), v)
	}}, steps...)
	return &subjectEnv{aguiEnv: e, iss: iss}
}

func (e *subjectEnv) token(sub string) string {
	return e.iss.sign(testKidRSA, claimsAt(time.Now(), sub))
}

// B cannot continue A's thread, and A still can.
func TestAGUITwoSubjectsOnePrincipalCannotShareAThread(t *testing.T) {
	e := newSubjectEnv(t, sayStep("mine"), sayStep("again"))
	tokA, tokB := e.token(bindSubA), e.token(bindSubB)
	require.Equal(t, http.StatusOK, e.post(tokA, aguiBody("thread-a", "r1", []any{userMsg("m1", "hello")}), nil).Status)

	r := e.post(tokB, aguiBody("thread-a", "r2", []any{userMsg("m2", "peek")}), nil)
	assert.Equal(t, http.StatusNotFound, r.Status)
	assert.Equal(t, CodeNotFound, r.errCode(t))
	missing := e.post(tokB, aguiBody("thread-none", "r3", nil, resumeEntry("int-x", "cancelled", nil)), nil)
	assert.Equal(t, r.Body, missing.Body, "A's thread looks like no thread to B")

	assert.Equal(t, http.StatusOK, e.post(tokA, aguiBody("thread-a", "r4", []any{userMsg("m3", "more")}), nil).Status, "A continues")
}

// B cannot resume, approve, deny or cancel A's interrupt; A's confirmation
// stays pending and A can still approve.
func TestAGUITwoSubjectsOnePrincipalCannotAnswerEachOthersInterrupt(t *testing.T) {
	e := newSubjectEnv(t, pinAddScript(t, "tc-1", 5), sayStep("Pinned."))
	tokA, tokB := e.token(bindSubA), e.token(bindSubB)
	in := e.startWrite(tokA, "thread-a")
	id := in["id"].(string)

	for name, c := range map[string]struct {
		status  string
		payload any
	}{
		"approve": {"resolved", map[string]any{"approved": true}},
		"deny":    {"resolved", map[string]any{"approved": false}},
		"cancel":  {"cancelled", nil},
		"wait":    {"resolved", map[string]any{"wait": true}},
	} {
		r := e.resumeReq(tokB, "thread-a", id, c.status, c.payload)
		assert.Equal(t, http.StatusNotFound, r.Status, name)
		assert.Equal(t, CodeNotFound, r.errCode(t), name)
	}
	pending := e.s.PendingConfirmations()
	require.Len(t, pending, 1)
	assert.Equal(t, StatePending, pending[0].State, "A's confirmation is untouched")
	assert.Zero(t, e.d.count("pin/add"))

	r := e.resumeReq(tokA, "thread-a", id, "resolved", map[string]any{"approved": true})
	require.Equal(t, http.StatusOK, r.Status)
	assert.Equal(t, 1, e.d.count("pin/add"))
}

// B filling the shared per-principal quota evicts only B's own sessions.
func TestAGUITwoSubjectsOnePrincipalNeverEvictEachOther(t *testing.T) {
	steps := make([]func(modelRequest) []string, 0, 2*maxSessionsPerPrincipal+4)
	for range 2*maxSessionsPerPrincipal + 4 {
		steps = append(steps, sayStep("ok"))
	}
	e := newSubjectEnv(t, steps...)
	tokA, tokB := e.token(bindSubA), e.token(bindSubB)
	require.Equal(t, http.StatusOK, e.post(tokA, aguiBody("thread-a", "r0", []any{userMsg("m0", "hi")}), nil).Status)

	for i := range maxSessionsPerPrincipal + 4 {
		thread := "b-" + string(rune('a'+i))
		r := e.post(tokB, aguiBody(thread, "rb", []any{userMsg("mb", "hi")}), nil)
		// The quota is shared: once the principal is full B may be refused,
		// but it is never given A's slot.
		require.Contains(t, []int{http.StatusOK, http.StatusTooManyRequests, http.StatusServiceUnavailable}, r.Status, r.Body)
	}
	assert.Equal(t, http.StatusOK, e.post(tokA, aguiBody("thread-a", "r9", []any{userMsg("m9", "still here")}), nil).Status,
		"A's idle thread survived B's chats")
	e.s.Agent().mu.Lock()
	defer e.s.Agent().mu.Unlock()
	require.Contains(t, e.s.Agent().sessions, "thread-a")
	assert.LessOrEqual(t, len(e.s.Agent().sessions), maxSessionsPerPrincipal)
}

// The UAR thread table has no HTTP seam without a UAR agent; the same rule is
// checked on the backend's own acquire and eviction.
func TestUARThreadsAreOwnedAndEvictedPerSubject(t *testing.T) {
	b := &uarBackend{threads: map[string]*uarThread{}, now: time.Now, gen: "g"}
	a := Principal{ID: "shared-ui", Kind: KindUI, sub: bindSubA}
	bb := Principal{ID: "shared-ui", Kind: KindUI, sub: bindSubB}

	th, rel, err := b.acquireThread(a.ID, a.BindingKey(), "t1", true)
	require.NoError(t, err)
	rel()
	require.NotNil(t, th)
	assert.Equal(t, a.BindingKey(), th.owner)

	// B naming A's thread id gets a thread of its own, never A's.
	other, rel, err := b.acquireThread(bb.ID, bb.BindingKey(), "t1", false)
	require.ErrorIs(t, err, ErrSessionNotFound)
	assert.Nil(t, other)
	_ = rel

	// B fills the shared quota: its own idle threads are evicted, A's never.
	for i := range maxSessionsPerPrincipal + 3 {
		_, rel, err := b.acquireThread(bb.ID, bb.BindingKey(), "b-"+string(rune('a'+i)), true)
		if err != nil {
			require.ErrorIs(t, err, ErrTooManySessions)
			continue
		}
		rel()
	}
	_, ok := b.threads[uarThreadKey(a.BindingKey(), "t1")]
	assert.True(t, ok, "A's thread survived")
}
