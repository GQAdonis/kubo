package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// AuthVerifier authenticates the bearer token of an HTTP request (ADR-0003
// section 5). It is the only seam between the credential scheme and the rest of
// the server: a static token today, Ory Hydra access tokens later, with no
// change to routes or tool handlers.
type AuthVerifier interface {
	// Verify checks token. It returns the caller's identity and the time the
	// verdict stops being valid. A token that does not authenticate must make
	// the error wrap auth.ErrInvalidToken; any other error is a server fault.
	Verify(ctx context.Context, token string, req *http.Request) (Identity, time.Time, error)
	// ProtectedResourceMetadata returns the RFC 9728 document that tells
	// clients which authorization server issues tokens for this resource, or
	// nil when the verifier has none (the static verifier has none). When it is
	// not nil the server serves it at ResourceMetadataPath and advertises it in
	// 401 responses.
	ProtectedResourceMetadata() *oauthex.ProtectedResourceMetadata
}

// ResourceMetadataPath is where RFC 9728 metadata is served when the verifier
// supplies it.
const ResourceMetadataPath = "/.well-known/oauth-protected-resource"

// TokenVerifier adapts an AuthVerifier to the go-sdk's auth.TokenVerifier. The
// identity travels in TokenInfo.Extra and its ID in TokenInfo.UserID, which the
// SDK uses to refuse a session presented by another user.
func TokenVerifier(v AuthVerifier) auth.TokenVerifier {
	return func(ctx context.Context, token string, req *http.Request) (*auth.TokenInfo, error) {
		id, exp, err := v.Verify(ctx, token, req)
		if err != nil {
			return nil, err
		}
		return &auth.TokenInfo{
			Scopes:     id.Scopes,
			Expiration: exp,
			UserID:     id.ID,
			Extra:      map[string]any{identityExtraKey: id},
		}, nil
	}
}

// anonymousVerifier gives every request the read-only anonymous identity. It
// is used only on loopback listeners that have no verifier configured.
type anonymousVerifier struct{}

func (anonymousVerifier) Verify(context.Context, string, *http.Request) (Identity, time.Time, error) {
	return anonymousIdentity(), time.Now().Add(verdictWindow), nil
}

func (anonymousVerifier) ProtectedResourceMetadata() *oauthex.ProtectedResourceMetadata {
	return nil
}

const (
	// verdictWindow is how long a static verification stays valid. The static
	// credential has no expiry of its own, so every verification expires soon
	// instead of turning on AllowMissingExpiration for all verifiers.
	verdictWindow = time.Minute
	// minTokenBytes rejects credentials too short to resist guessing.
	minTokenBytes = 16
)

// SecretRef names where a credential is read from. Literal values are not
// accepted anywhere (contract config schema, SecretRef).
type SecretRef struct {
	// Env is the name of an environment variable.
	Env string `json:"env,omitempty"`
	// File is an absolute path to a file whose trimmed content is the secret.
	File string `json:"file,omitempty"`
}

// Read returns the secret. It fails when the reference names nothing, the
// variable is unset or empty, or the file is relative, unreadable or empty.
func (r SecretRef) Read() (string, error) {
	switch {
	case r.Env != "" && r.File != "":
		return "", errors.New("credential names both env and file")
	case r.Env != "":
		v, ok := os.LookupEnv(r.Env)
		if !ok || strings.TrimSpace(v) == "" {
			return "", fmt.Errorf("environment variable %s is not set or empty", r.Env)
		}
		return strings.TrimSpace(v), nil
	case r.File != "":
		if !filepath.IsAbs(r.File) {
			return "", fmt.Errorf("credential file %q is not an absolute path", r.File)
		}
		data, err := os.ReadFile(r.File)
		if err != nil {
			return "", fmt.Errorf("read credential file: %w", err)
		}
		v := strings.TrimSpace(string(data))
		if v == "" {
			return "", fmt.Errorf("credential file %s is empty", r.File)
		}
		return v, nil
	}
	return "", errors.New("credential names neither env nor file")
}

// String describes the reference without its value, for messages.
func (r SecretRef) String() string {
	if r.Env != "" {
		return "env " + r.Env
	}
	return "file " + r.File
}

// StaticCredential binds one bearer token to one identity.
type StaticCredential struct {
	Identity Identity
	Token    string
}

// StaticVerifier is an AuthVerifier for configured bearer tokens. Tokens are
// kept as SHA-256 digests and compared in constant time, against every
// credential, so response time does not reveal which one matched.
type StaticVerifier struct {
	entries []staticEntry
}

type staticEntry struct {
	digest   [sha256.Size]byte
	identity Identity
}

// NewStaticVerifier builds a verifier. It rejects a token with no principal, a
// token that is too short, and duplicate tokens or principals.
func NewStaticVerifier(creds []StaticCredential) (*StaticVerifier, error) {
	v := &StaticVerifier{entries: make([]staticEntry, 0, len(creds))}
	ids := make(map[string]struct{}, len(creds))
	digests := make(map[[sha256.Size]byte]string, len(creds))
	for _, c := range creds {
		if c.Identity.ID == "" || c.Identity.Kind == "" {
			return nil, errors.New("a bearer token has no principal: every credential needs a principal name and kind")
		}
		if len(c.Token) < minTokenBytes {
			return nil, fmt.Errorf("principal %s: the token is shorter than %d bytes", c.Identity.ID, minTokenBytes)
		}
		if _, dup := ids[c.Identity.ID]; dup {
			return nil, fmt.Errorf("principal %s is configured twice", c.Identity.ID)
		}
		ids[c.Identity.ID] = struct{}{}
		digest := sha256.Sum256([]byte(c.Token))
		if other, dup := digests[digest]; dup {
			return nil, fmt.Errorf("principals %s and %s share one token", other, c.Identity.ID)
		}
		digests[digest] = c.Identity.ID
		v.entries = append(v.entries, staticEntry{digest: digest, identity: c.Identity})
	}
	return v, nil
}

// Len is the number of configured credentials.
func (v *StaticVerifier) Len() int { return len(v.entries) }

// Verify implements AuthVerifier.
func (v *StaticVerifier) Verify(_ context.Context, token string, _ *http.Request) (Identity, time.Time, error) {
	presented := sha256.Sum256([]byte(token))
	matched := -1
	for i := range v.entries {
		if subtle.ConstantTimeCompare(presented[:], v.entries[i].digest[:]) == 1 {
			matched = i
		}
	}
	if matched < 0 {
		return Identity{}, time.Time{}, fmt.Errorf("%w: unknown bearer token", auth.ErrInvalidToken)
	}
	return v.entries[matched].identity, time.Now().Add(verdictWindow), nil
}

// ProtectedResourceMetadata implements AuthVerifier. The static verifier has no
// authorization server to advertise.
func (v *StaticVerifier) ProtectedResourceMetadata() *oauthex.ProtectedResourceMetadata {
	return nil
}

// Identities returns the identities the verifier authenticates, without their
// tokens.
func (v *StaticVerifier) Identities() []Identity {
	out := make([]Identity, 0, len(v.entries))
	for _, e := range v.entries {
		out = append(out, e.identity)
	}
	return out
}
