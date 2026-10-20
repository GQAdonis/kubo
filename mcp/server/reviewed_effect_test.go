package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func reviewMeta(digest string) mcp.Meta {
	return mcp.Meta{"ipfs-mcp": map[string]any{"reviewed_effect": map[string]any{"effect_digest": digest}}}
}

func TestReviewedEffectRejectsExplicitMalformedEcho(t *testing.T) {
	for _, raw := range []string{
		`null`, `{}`, `[]`, `"absent"`, `{"effect_digest":null}`,
		`{"effect_digest":"sha256:abcd"}`, `{"effect_digest":"SHA256:` + strings.Repeat("a", 64) + `"}`,
		`{"effect_digest":"sha256:` + strings.Repeat("A", 64) + `"}`,
		`{"effect_digest":"sha256:` + strings.Repeat("a", 64) + `","approve":true}`,
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := ParseReviewedEffect(json.RawMessage(raw))
			require.ErrorIs(t, err, ErrMalformedReviewedEffect)
		})
	}
	reviewed, err := ParseReviewedEffect(nil)
	require.NoError(t, err)
	assert.Nil(t, reviewed)
	meta := mcp.Meta{"other-application": map[string]any{"anything": true}}
	got, te := reviewedFromMeta(meta)
	assert.Nil(t, te)
	assert.Nil(t, got)
	assert.Contains(t, meta, "other-application", "unrelated SDK metadata is untouched")
}

func TestMCPRepeatReviewedEchoChecksBeforeClaim(t *testing.T) {
	for _, token := range []bool{false, true} {
		t.Run(map[bool]string{false: "tokenless", true: "token"}[token], func(t *testing.T) {
			e := newGateEnv(t)
			cs := e.h.connect(t, nil, nil)
			conf := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired).Confirmation
			p, resolve := e.resolver.last(t)
			require.NoError(t, resolve(true))
			args := map[string]any{"path": "/ipfs/" + e.target}
			if token {
				args["confirm_token"] = conf.ConfirmToken
			}
			res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "pin_rm", Arguments: args, Meta: reviewMeta("sha256:" + strings.Repeat("0", 64))})
			require.NoError(t, err)
			te := confirmationOf(t, res, CodeConfirmDigestMismatch)
			assert.False(t, te.Retryable)
			snap, _ := e.s.gate.broker.get(p.ID)
			assert.Equal(t, StateApproved, snap.State)
			assert.Equal(t, ResolverLinuxPolkit, snap.ResolvedBy)
			e.assertNothingRan(t)
			res, err = cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "pin_rm", Arguments: args, Meta: reviewMeta(conf.EffectDigest)})
			require.NoError(t, err)
			require.False(t, res.IsError)
			assert.Equal(t, 1, e.d.count("pin/rm"))
		})
	}
}

func TestMCPReviewCannotReplaceLegacyTokenBindings(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, nil, nil)
	conf := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired).Confirmation
	_, resolve := e.resolver.last(t)
	require.NoError(t, resolve(true))
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "pin_rm", Arguments: map[string]any{"path": "/ipfs/" + testCID(t, 2), "confirm_token": conf.ConfirmToken},
		Meta: mcp.Meta{"ipfs-mcp": map[string]any{"reviewed_effect": nil}},
	})
	require.NoError(t, err)
	confirmationOf(t, res, CodeConfirmInvalid)
	e.assertNothingRan(t)
}

func TestElicitationReviewedEchoIsOptionalAndBound(t *testing.T) {
	for name, makeMeta := range map[string]func(string) mcp.Meta{
		"matching":  reviewMeta,
		"mismatch":  func(string) mcp.Meta { return reviewMeta("sha256:" + strings.Repeat("0", 64)) },
		"malformed": func(string) mcp.Meta { return mcp.Meta{"ipfs-mcp": map[string]any{"reviewed_effect": nil}} },
	} {
		t.Run(name, func(t *testing.T) {
			e := newGateEnv(t)
			cs := e.h.connect(t, elicitOptions(func(req *mcp.ElicitRequest) *mcp.ElicitResult {
				meta := req.Params.Meta["ipfs-mcp"].(map[string]any)
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}, Meta: makeMeta(meta["effect_digest"].(string))}
			}), nil)
			res := e.rm(t, cs, nil)
			switch name {
			case "matching":
				require.False(t, res.IsError)
				assert.Equal(t, 1, e.d.count("pin/rm"))
			case "mismatch":
				confirmationOf(t, res, CodeConfirmDigestMismatch)
				e.assertNothingRan(t)
			case "malformed":
				confirmationOf(t, res, CodeInvalidArgument)
				e.assertNothingRan(t)
			}
		})
	}
}

func TestElicitationDualReviewEchoesMustAgree(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, elicitOptions(func(*mcp.ElicitRequest) *mcp.ElicitResult { return &mcp.ElicitResult{Action: "decline"} }), nil)
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmDeclined)
	cid, state := e.openForged(t, e.sessionIDOf(t), MechanismElicitation)
	_, _, args, err := argsDigest(asJSON(t, map[string]any{"path": "/ipfs/" + e.target}))
	require.NoError(t, err)
	digest, err := effectDigest("pin_rm", args)
	require.NoError(t, err)
	e.s.gate.broker.mu.Lock()
	e.s.gate.broker.byID[cid].EffectDigest = digest
	e.s.gate.broker.mu.Unlock()
	answers := forgedAccept()
	answers[elicitKey].(*mcp.ElicitResult).Meta = reviewMeta("sha256:" + strings.Repeat("0", 64))
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "pin_rm", Arguments: map[string]any{"path": "/ipfs/" + e.target}, RequestState: state,
		InputResponses: answers, Meta: reviewMeta(digest),
	})
	require.NoError(t, err)
	confirmationOf(t, res, CodeConfirmDigestMismatch)
	snap, _ := e.s.gate.broker.get(cid)
	assert.Equal(t, StatePending, snap.State)
	e.assertNothingRan(t)
}

func TestResolverAttributionIsActualAndSurvivesClaim(t *testing.T) {
	clock := newFakeClock()
	b := newBroker(clock.Now, confirmTTL)
	p := openPending(b, "pin_rm")
	snap, _ := b.get(p.ID)
	assert.Empty(t, snap.ResolvedBy)
	require.NoError(t, b.Resolve(p.ID, ResolverLinuxPolkit, true))
	require.NoError(t, b.claim(p.ID, ""))
	snap, _ = b.get(p.ID)
	assert.Equal(t, ResolverLinuxPolkit, snap.ResolvedBy)
	assert.Equal(t, snap.Resolver, snap.ResolvedBy)

	p = openPending(b, "pin_add")
	b.Cancel(p.ID)
	snap, _ = b.get(p.ID)
	assert.Empty(t, snap.ResolvedBy, "system withdrawal has no human resolver")
	p = openPending(b, "key_rm")
	clock.Advance(confirmTTL + time.Second)
	snap, _ = b.get(p.ID)
	assert.Equal(t, StateExpired, snap.State)
	assert.Empty(t, snap.ResolvedBy, "expiry does not invent a human resolver")
}
