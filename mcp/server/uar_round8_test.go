//go:build !noagent

package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Round 8 N1: a failed resume keeps the earlier answers ----

func TestUARFailedResumeOfASecondInterruptKeepsTheFirstAnswerAndTheShownInterrupt(t *testing.T) {
	both := uarReleaseFixture(t, "run-both-interrupts.sse")
	executed := uarReleaseFixture(t, "run-resumed-executed.sse")
	e := newUARProxyEnv(t, scriptOf(ok(both), status(http.StatusBadGateway), ok(executed)), uarProxyOpts{})
	first := finishedInterrupt(t, e.say("t1", "r1", "pin").Events)
	require.Equal(t, "int-ta-1", first["id"])
	second := finishedInterrupt(t, e.answer("t1", "r2", "int-ta-1", true).Events)
	require.Equal(t, "int-el-1", second["id"])

	r := e.answer("t1", "r3", "int-el-1", true)
	require.Equal(t, http.StatusBadGateway, r.Status, r.Body)
	require.Equal(t, 2, e.fake.runCount())

	// The island still shows the second interrupt: it answers that one again, and
	// the run continues with both answers, the first one as it was given.
	r = e.answer("t1", "r4", "int-el-1", true)
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	require.Equal(t, 3, e.fake.runCount())
	entries := resumeEntries(t, e.fake.run(2))
	require.Len(t, entries, 2)
	assert.Equal(t, map[string]any{"approved": true}, entries[0]["payload"], "the first answer was kept")
	assert.Equal(t, "accept", entries[1]["payload"].(map[string]any)["action"])
}

func TestUARFailedResumeAfterADenialWithdrawsTheDenialsItFilledIn(t *testing.T) {
	both := uarReleaseFixture(t, "run-both-interrupts.sse")
	denied := uarReleaseFixture(t, "run-resumed-denied.sse")
	e := newUARProxyEnv(t, scriptOf(ok(both), status(http.StatusBadGateway), ok(denied)), uarProxyOpts{})
	finishedInterrupt(t, e.say("t1", "r1", "pin").Events)

	r := e.answer("t1", "r2", "int-ta-1", false) // denies int-el-1 with it
	require.Equal(t, http.StatusBadGateway, r.Status, r.Body)

	// The shown interrupt is open again, and so is the one the denial filled in.
	th, release, err := e.s.UARBackend().acquireThread("chat", "chat", "t1", false)
	require.NoError(t, err)
	defer release()
	require.NotNil(t, th.shownPending())
	assert.Equal(t, "int-ta-1", th.shownPending().ID)
	for _, p := range th.pending {
		assert.Nil(t, p.Answer, p.ID)
	}
}

// ---- Round 8 N4: every SSE line counts against the byte budget ----

func TestUARRelayBudgetCountsCommentLines(t *testing.T) {
	comment := ": " + strings.Repeat("x", 16<<10) + "\n"
	var body strings.Builder
	body.WriteString(`data: {"type":"RUN_STARTED","threadId":"t","runId":"r"}` + "\n\n")
	for body.Len() <= maxUARRequestBytes+(1<<20) {
		body.WriteString(comment)
	}
	e := newUARProxyEnv(t, scriptOf(ok([]byte(body.String()))), uarProxyOpts{})
	r := e.say("t1", "r1", "hello")
	require.Equal(t, http.StatusOK, r.Status, "the stream had begun")
	last := r.Events[len(r.Events)-1]
	assert.Equal(t, "RUN_ERROR", last["type"])
	assert.Contains(t, last["message"], "more data than one request allows", "the budget ended it, not the end of the stream")
}

// ---- Round 8 N5: a re-read api file must stay on loopback when the RPC is authenticated ----

func TestDiscoveringDaemonRefusesAReReadNonLoopbackAddressWithAPIAuth(t *testing.T) {
	dir := t.TempDir()
	d := NewDiscoveringDaemon(DiscoverOptions{IPFSPath: dir, Authorization: "Bearer secret-rpc-token"})
	first := rpcServer(t, "0.44.0-dev", "Bearer secret-rpc-token")
	writeAPIFile(t, dir, first.URL)
	require.NoError(t, d.Call(t.Context(), "version", nil, nil, nil))

	first.Close()
	require.ErrorIs(t, d.Call(t.Context(), "version", nil, nil, nil), ErrDaemonUnreachable, "the old port is gone")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "api"), []byte("/ip4/192.0.2.1/tcp/5001"), 0o600))
	for range 2 {
		err := d.Call(context.Background(), "version", nil, nil, nil)
		require.ErrorIs(t, err, ErrDaemonUnreachable, "the call keeps failing as unreachable")
		assert.Contains(t, err.Error(), "not a loopback address")
		assert.NotContains(t, err.Error(), "secret-rpc-token")
	}

	// Back on loopback it recovers.
	second := rpcServer(t, "0.45.0", "Bearer secret-rpc-token")
	writeAPIFile(t, dir, second.URL)
	require.NoError(t, d.Call(t.Context(), "version", nil, nil, nil))
}

func TestDiscoveringDaemonWithoutAPIAuthFollowsAnyReReadAddress(t *testing.T) {
	dir := t.TempDir()
	d := NewDiscoveringDaemon(DiscoverOptions{IPFSPath: dir})
	first := rpcServer(t, "0.44.0-dev", "")
	writeAPIFile(t, dir, first.URL)
	require.NoError(t, d.Call(t.Context(), "version", nil, nil, nil))
	first.Close()
	_ = d.Call(t.Context(), "version", nil, nil, nil)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "api"), []byte("/ip4/192.0.2.1/tcp/5001"), 0o600))
	cur, err := d.current()
	require.NoError(t, err)
	assert.Contains(t, cur.Endpoint(), "192.0.2.1")
}

// ---- Round 8 N2 and N3: settings ----

// N2: a uar backend with a grant that moves to a host egress_ack already covers
// still needs the typed confirmation.
func TestUARGrantMovingToACoveredHostNeedsTheTypedConfirmation(t *testing.T) {
	e, _, etag := keyMoveEnv(t, kindMoveConfig())
	_, doc := e.putRaw(t, etag, `{"backend":{"kind":"uar"},"egress_confirmation":{"host":"`+kindMoveHost+`"}}`)
	require.NotNil(t, e.s.UARBackend(), "%v", doc)
	etag, _ = getSettings(t, e)

	status, doc := e.putRaw(t, etag, `{"backend":{"base_url":"http://127.0.0.1:59997"}}`)
	require.Equal(t, http.StatusConflict, status, doc)
	errObj := doc["error"].(map[string]any)
	assert.Equal(t, "egress_confirmation_required", errObj["code"])
	assert.Contains(t, errObj["message"], "kubo MCP grant")
	assert.Contains(t, currentBaseURL(t, e), kindMoveHost, "nothing applied")

	status, doc = e.putRaw(t, etag, `{"backend":{"base_url":"http://127.0.0.1:59997"},"egress_confirmation":{"host":"127.0.0.1:59997"}}`)
	require.Equal(t, http.StatusOK, status, doc)
	assert.Contains(t, currentBaseURL(t, e), "59997")
}
