//go:build !noagent

package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func surfaceBody(thread, run, interrupt, kind, instance string) map[string]any {
	body := aguiBody(thread, run, nil, resumeEntry(interrupt, ResumeResolved, map[string]any{"approved": true}))
	body["forwardedProps"] = map[string]any{"ipfs": map[string]any{"surface": map[string]any{"kind": kind, "instance": instance}}}
	return body
}

func TestAGUISurfaceInvalidBranchRejectsBeforeBackend(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc-1", 5), sayStep("Pinned."))
	for _, hint := range []any{
		nil, map[string]any{}, map[string]any{"kind": "chat"},
		map[string]any{"kind": "native_prompt", "instance": "pane"},
		map[string]any{"kind": "desktop", "instance": "pane"},
		map[string]any{"kind": "chat", "instance": strings.Repeat("a", 129)},
		map[string]any{"kind": "chat", "instance": "https://user:secret@host"},
		map[string]any{"kind": "chat", "instance": "pane", "resolved_by": ResolverMacOSLocalAuth},
	} {
		body := aguiBody("invalid-surface", "r1", []any{userMsg("m1", "pin it")})
		body["forwardedProps"] = map[string]any{"ipfs": map[string]any{"surface": hint}}
		r := e.post(aguiTokChat, body, nil)
		assert.Equal(t, http.StatusUnprocessableEntity, r.Status)
		assert.Equal(t, CodeInvalidArgument, r.errCode(t))
		assert.NotContains(t, r.Body, "secret")
	}
	assert.Zero(t, e.model.hits())
	assert.Zero(t, e.d.count("pin/add"))
	assert.Empty(t, e.s.PendingConfirmations())
}

func TestAGUISurfaceCannotCrossIdentityOrOverwriteWinningPane(t *testing.T) {
	e := newSubjectEnv(t, pinAddScript(t, "tc-1", 5), sayStep("Pinned."))
	tokA, tokB := e.token(bindSubA), e.token(bindSubB)
	in := e.startWrite(tokA, "surface-owned")
	meta := in["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)
	validateConfirmationDef(t, "EffectDigest", meta["effect_digest"])
	validateConfirmationDef(t, "Destination", meta["destination"])
	_, _, operation, err := argsDigest(asJSON(t, map[string]any{"path": "/ipfs/" + testCID(t, 5), "recursive": true}))
	require.NoError(t, err)
	wantDigest, err := effectDigest("pin_add", operation)
	require.NoError(t, err)
	assert.Equal(t, wantDigest, meta["effect_digest"], "new optional metadata binds actual model arguments")
	assert.Equal(t, "Configured Kubo node", meta["destination"].(map[string]any)["name"])
	interrupt := in["id"].(string)
	body := surfaceBody("surface-owned", "r2", interrupt, "chat", "pane_b")
	r := e.post(tokB, body, nil)
	assert.Equal(t, http.StatusNotFound, r.Status)
	p, _ := e.s.gate.broker.get(meta["confirmation_id"].(string))
	assert.Nil(t, p.ResolvedSurface)
	assert.Equal(t, StatePending, p.State)
	r = e.post(tokA, body, nil)
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	p, _ = e.s.gate.broker.get(meta["confirmation_id"].(string))
	assert.Equal(t, ResolverAGUIInterrupt, p.ResolvedBy)
	assert.Equal(t, &ResolvingSurface{Kind: "chat", Instance: "pane_b"}, p.ResolvedSurface)
	require.Equal(t, ExecutionSucceeded, p.Execution.State)
	require.NotNil(t, p.Execution.Receipt)

	// Refresh/reconnect using a delayed old interruption never rewrites the
	// completed operation or attributes it to whichever pane polled last.
	late := surfaceBody("surface-owned", "r3", interrupt, "inbox", "late_pane")
	assert.Equal(t, http.StatusUnprocessableEntity, e.post(tokA, late, nil).Status)
	again, _, ok := e.s.gate.broker.observed(p.ID)
	require.True(t, ok)
	assert.Equal(t, p.Execution.Receipt.ID, again.Execution.Receipt.ID)
	assert.Equal(t, "pane_b", again.ResolvedSurface.Instance)
	assert.Equal(t, 1, e.d.count("pin/add"))
}

func TestAGUIInboxSurfaceDoesNotSupplyNativePresence(t *testing.T) {
	e := destructiveEnv(t, sayStep("Waiting for the native prompt."))
	in := e.startWrite(aguiTokChat, "native-surface")
	body := surfaceBody("native-surface", "r2", in["id"].(string), "inbox", "pane_inbox")
	r := e.post(aguiTokChat, body, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status)
	assert.Zero(t, e.d.count("pin/rm"))
	p := e.s.PendingConfirmations()
	require.Len(t, p, 1)
	assert.Nil(t, p[0].ResolvedSurface)
	assert.Empty(t, p[0].ResolvedBy)
}

func TestUARSurfaceAttributionStaysInBackendApprovalDomain(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-tool-approval-interrupt.sse", "run-resumed-executed.sse"), uarProxyOpts{})
	in := finishedInterrupt(t, e.say("uar-surface", "r1", "pin").Events)
	meta := in["metadata"].(map[string]any)
	assert.Equal(t, SourceKindBackendApproval, meta["uar"].(map[string]any)["source_kind"])
	assert.Equal(t, false, meta["uar"].(map[string]any)["node_confirmation"])
	body := surfaceBody("uar-surface", "r2", in["id"].(string), "inbox", "backend_pane")
	require.Equal(t, http.StatusOK, e.post(aguiTokChat, body).Status)
	id := meta["ipfs-mcp"].(map[string]any)["confirmation_id"].(string)
	p, _, ok := e.s.gate.broker.observed(id)
	require.True(t, ok)
	assert.Equal(t, SourceKindBackendApproval, p.SourceKind)
	assert.Equal(t, &ResolvingSurface{Kind: "inbox", Instance: "backend_pane"}, p.ResolvedSurface)
	assert.Nil(t, p.Execution)
	assert.Empty(t, e.s.PendingConfirmations())
	// Surface attribution is local display data, not a forged backend grant.
	resume := resumeEntries(t, e.fake.run(1))
	require.Len(t, resume, 1)
	assert.NotContains(t, resume[0], "resolved_surface")
	assert.NotContains(t, resume[0], "resolved_by")
}
