package server

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrMalformedReviewedEffect means explicitly supplied review metadata was not
// a contract ReviewedEffect. It must never be treated as an omitted echo.
var ErrMalformedReviewedEffect = errors.New("reviewed_effect must contain exactly one valid effect_digest")

// ParseReviewedEffect validates the optional echo at a resolver boundary. An
// absent field is nil; explicit null and unknown properties fail closed.
func ParseReviewedEffect(raw json.RawMessage) (*ReviewedEffect, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || len(fields) != 1 {
		return nil, ErrMalformedReviewedEffect
	}
	var reviewed ReviewedEffect
	if err := json.Unmarshal(fields["effect_digest"], &reviewed.EffectDigest); err != nil || !validEffectDigest(reviewed.EffectDigest) {
		return nil, ErrMalformedReviewedEffect
	}
	return &reviewed, nil
}

func validEffectDigest(digest string) bool {
	if len(digest) != len("sha256:")+64 || !strings.HasPrefix(digest, "sha256:") {
		return false
	}
	for _, c := range digest[len("sha256:"):] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// reviewedFromMeta reads only the contract namespace. Other SDK metadata is
// left alone; unknown fields within the response namespace are refused.
func reviewedFromMeta(meta mcp.Meta) (*ReviewedEffect, *ToolError) {
	v, ok := meta["ipfs-mcp"]
	if !ok {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, invalidArgument("invalid confirmation review metadata")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, invalidArgument("invalid confirmation review metadata")
	}
	for field := range fields {
		if field != "reviewed_effect" {
			return nil, invalidArgument("invalid confirmation review metadata")
		}
	}
	reviewed, err := ParseReviewedEffect(fields["reviewed_effect"])
	if err != nil {
		return nil, invalidArgument(ErrMalformedReviewedEffect.Error())
	}
	return reviewed, nil
}

// checkReview runs only after the adapter's legacy ownership/binding checks.
// Both explicit echoes must be valid and agree; neither has precedence.
func (g *gate) checkReview(c *call, p Pending, answerMeta mcp.Meta) *ToolError {
	if c.reviewError != nil {
		return c.reviewError
	}
	answer, err := reviewedFromMeta(answerMeta)
	if err != nil {
		return err
	}
	if c.reviewed != nil && answer != nil && c.reviewed.EffectDigest != answer.EffectDigest {
		return digestMismatch(c.tool.Name, p.ID)
	}
	if c.reviewed == nil {
		c.reviewed = answer
	}
	if err := g.broker.checkReviewed(p.ID, c.reviewed); err != nil {
		if errors.Is(err, ErrConfirmationDigestMismatch) {
			return digestMismatch(c.tool.Name, p.ID)
		}
		return invalidArgument("invalid confirmation review metadata")
	}
	return nil
}
