package confirm

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ipfs/kubo/mcp/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignedChannelReviewedEchoMustMatchBeforeCallback(t *testing.T) {
	e := newChanEnv(t, ModeDesktop)
	p := pending("Review this pin")
	p.ID = "cf_reviewed_effect_fixture"
	p.EffectDigest = "sha256:" + strings.Repeat("a", 64)
	var observed *server.ReviewedEffect
	called := 0
	resolver := &channelResolver{resolverName: server.ResolverDesktop, ch: e.m.ch}
	require.NoError(t, resolver.RequestReviewed(t.Context(), p, func(approve bool, reviewed *server.ReviewedEffect) error {
		if reviewed != nil && reviewed.EffectDigest != p.EffectDigest {
			return server.ErrConfirmationDigestMismatch
		}
		called++
		observed = reviewed
		return nil
	}))
	for _, malformed := range []any{nil, map[string]any{}, map[string]any{"effect_digest": "short"}} {
		status, _ := e.do(t, http.MethodPost, resolvePath, "v2:"+e.key, "", map[string]any{"id": p.ID, "approve": true, "reviewed_effect": malformed})
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Zero(t, called)
	}
	status, body := e.do(t, http.MethodPost, resolvePath, "v2:"+e.key, "", map[string]any{
		"id": p.ID, "approve": true, "reviewed_effect": map[string]any{"effect_digest": "sha256:" + strings.Repeat("b", 64)},
	})
	assert.Equal(t, http.StatusConflict, status)
	assert.Equal(t, server.CodeConfirmDigestMismatch, body["code"])
	assert.Equal(t, false, body["retryable"])
	assert.Zero(t, called)
	status, body = e.do(t, http.MethodPost, resolvePath, "v2:"+e.key, "", map[string]any{
		"id": p.ID, "approve": true, "reviewed_effect": map[string]any{"effect_digest": p.EffectDigest},
	})
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, 1, called)
	assert.Equal(t, p.EffectDigest, observed.EffectDigest)
	assert.Equal(t, server.ResolverDesktop, body["resolved_by"])
}
