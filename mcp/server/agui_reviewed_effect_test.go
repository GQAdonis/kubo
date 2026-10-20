//go:build !noagent

package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAGUIReviewedEffectPreservesIdentityAndSingleUse(t *testing.T) {
	e := newSubjectEnv(t, pinAddScript(t, "tc-1", 5), sayStep("Pinned."))
	tokA, tokB := e.token(bindSubA), e.token(bindSubB)
	in := e.startWrite(tokA, "review-thread")
	meta := in["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)
	entry := resumeEntry(in["id"].(string), ResumeResolved, map[string]any{"approved": true})
	entry["reviewed_effect"] = map[string]any{"effect_digest": "sha256:" + strings.Repeat("0", 64)}
	body := aguiBody("review-thread", "r2", nil, entry)
	other := e.post(tokB, body, nil)
	assert.Equal(t, http.StatusNotFound, other.Status)
	assert.Equal(t, CodeNotFound, other.errCode(t), "another subject learns no digest verdict")
	r := e.post(tokA, body, nil)
	assert.Equal(t, http.StatusConflict, r.Status)
	assert.Equal(t, CodeConfirmDigestMismatch, r.errCode(t))
	assert.Zero(t, e.d.count("pin/add"))
	pending := e.s.PendingConfirmations()
	require.Len(t, pending, 1)
	assert.Empty(t, pending[0].ResolvedBy)

	entry["reviewed_effect"] = map[string]any{"effect_digest": meta["effect_digest"]}
	r = e.post(tokA, body, nil)
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	assert.Equal(t, 1, e.d.count("pin/add"))
	snap, ok := e.s.gate.broker.get(meta["confirmation_id"].(string))
	require.True(t, ok)
	assert.Equal(t, ResolverAGUIInterrupt, snap.ResolvedBy)
	assert.Equal(t, StateExecuted, snap.State)
	assert.Equal(t, http.StatusUnprocessableEntity, e.post(tokA, body, nil).Status)
	assert.Equal(t, 1, e.d.count("pin/add"))
}

func TestAGUIHumanDenialHasResolverSystemWithdrawalDoesNot(t *testing.T) {
	for _, status := range []string{ResumeResolved, ResumeCancelled} {
		t.Run(status, func(t *testing.T) {
			e := newAGUIEnv(t, aguiEnvOpts{}, pinAddScript(t, "tc-1", 5), sayStep("Denied."))
			in := e.startWrite(aguiTokChat, "deny-review")
			meta := in["metadata"].(map[string]any)["ipfs-mcp"].(map[string]any)
			var payload any
			if status == ResumeResolved {
				payload = map[string]any{"approved": false}
			}
			entry := resumeEntry(in["id"].(string), status, payload)
			entry["reviewed_effect"] = map[string]any{"effect_digest": meta["effect_digest"]}
			r := e.post(aguiTokChat, aguiBody("deny-review", "r2", nil, entry), nil)
			require.Equal(t, http.StatusOK, r.Status, r.Body)
			snap, ok := e.s.gate.broker.get(meta["confirmation_id"].(string))
			require.True(t, ok)
			assert.Equal(t, StateDenied, snap.State)
			assert.Equal(t, ResolverAGUIInterrupt, snap.ResolvedBy, "an actual chat gesture records its mechanism")
			assert.Zero(t, e.d.count("pin/add"))
		})
	}
}

func TestUARSyntheticApprovalNeverConsumesNodeReviewedEffect(t *testing.T) {
	e := newUARProxyEnv(t, replay(t, "run-tool-approval-interrupt.sse", "run-resumed-executed.sse"), uarProxyOpts{})
	in := finishedInterrupt(t, e.say("review-uar", "r1", "pin").Events)
	entry := resumeEntry(in["id"].(string), ResumeResolved, map[string]any{"approved": true})
	entry["reviewed_effect"] = map[string]any{"effect_digest": "sha256:" + strings.Repeat("0", 64)}
	body := aguiBody("review-uar", "r2", nil, entry)
	assert.Equal(t, http.StatusNotFound, e.post(aguiTokChat2, body).Status)
	r := e.post(aguiTokChat, body)
	assert.Equal(t, http.StatusConflict, r.Status)
	assert.Equal(t, CodeConfirmDigestMismatch, r.errCode(t))
	assert.Equal(t, 1, e.fake.runCount(), "the mismatched answer never reaches UAR")
	assert.Empty(t, e.s.PendingConfirmations(), "synthetic UAR approval never creates node consent")

	// The same backend interruption remains open for a legacy answer; no
	// reviewed digest or node success is fabricated from its text.
	delete(entry, "reviewed_effect")
	require.Equal(t, http.StatusOK, e.post(aguiTokChat, body).Status)
	assert.Equal(t, 2, e.fake.runCount())
}
